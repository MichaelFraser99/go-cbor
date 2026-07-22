package cbor

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
)

func syntaxErrorf(format string, args ...any) error {
	return &SyntaxError{msg: fmt.Sprintf(format, args...)}
}

type decodeState struct {
	data        []byte
	maxDepth    int
	dupMapKey   DupMode
	strict      bool
	rejectIndef bool
	maxArray    int
	maxMap      int
	maxString   int
	depth       int
	keyScratch  []byte
}

func newDecodeState() *decodeState { return &decodeState{maxDepth: defaultMaxDepth} }

func (s *decodeState) canonicalKeyBytes(key *DataItem) []byte {
	s.keyScratch = s.keyScratch[:0]
	e := encodeState{buf: s.keyScratch}
	_ = e.encodeDataItem(key)
	s.keyScratch = e.buf
	return e.buf
}

func bignumMinimal(b []byte) error {
	if len(b) <= 8 {
		return syntaxErrorf("cbor: bignum not in preferred serialization (value fits a basic integer)")
	}
	if b[0] == 0x00 {
		return syntaxErrorf("cbor: bignum has a leading zero byte")
	}
	return nil
}

func (s *decodeState) shortest(ai byte, value uint64) error {
	if !s.strict {
		return nil
	}
	var want byte
	switch {
	case value < 24:
		want = byte(value)
	case value < 1<<8:
		want = 24
	case value < 1<<16:
		want = 25
	case value < 1<<32:
		want = 26
	default:
		want = 27
	}
	if ai != want {
		return syntaxErrorf("cbor: argument not in shortest form")
	}
	return nil
}

func canonicalFloatWidth(f float64) uint8 {
	if math.IsNaN(f) {
		return 2
	}
	if _, ok := tryFloat16(f); ok {
		return 2
	}
	if f32 := float32(f); float64(f32) == f {
		return 4
	}
	return 8
}

func (s *decodeState) shortestFloat(value float64, width uint8) error {
	if !s.strict {
		return nil
	}
	if width != canonicalFloatWidth(value) {
		return syntaxErrorf("cbor: float not in shortest form")
	}
	return nil
}

func (s *decodeState) stamp(in []byte, err error) error {
	if err == nil {
		return nil
	}
	offset := int64(len(s.data) - len(in))
	switch e := err.(type) {
	case *SyntaxError:
		if e.Offset == 0 {
			e.Offset = offset
		}
	case *UnmarshalTypeError:
		if e.Offset == 0 {
			e.Offset = offset
		}
	}
	return err
}

func (s *decodeState) descend() error {
	s.depth++
	if s.depth > s.maxDepth {
		return syntaxErrorf("cbor: nesting depth exceeds limit of %d", s.maxDepth)
	}
	return nil
}

func (s *decodeState) ascend() { s.depth-- }

func decodeDataItem(in []byte) (*DataItem, int, error) {
	return newDecodeState().decodeItem(in)
}

func (s *decodeState) decodeItem(in []byte) (item *DataItem, n int, err error) {
	defer func() { err = s.stamp(in, err) }()
	if len(in) == 0 {
		return nil, 0, syntaxErrorf("cbor: empty data item")
	}

	major, additionalInformation, err := identifyMajorType(in[0])
	if err != nil {
		return nil, 0, err
	}

	switch major {
	case MajorUint, MajorNint:
		value, consumed, err := s.readArgument(in, additionalInformation)
		if err != nil {
			return nil, 0, err
		}
		return &DataItem{Major: major, Argument: value}, consumed, nil
	case MajorBytes, MajorText:
		if additionalInformation == 31 {
			return s.decodeIndefiniteString(in, major)
		}
		content, consumed, err := s.decodeString(in, additionalInformation)
		if err != nil {
			return nil, 0, err
		}
		return &DataItem{Major: major, Bytes: append([]byte(nil), content...)}, consumed, nil
	case MajorArray:
		if additionalInformation == 31 {
			return s.decodeIndefiniteArray(in)
		}
		return s.decodeArray(in, additionalInformation)
	case MajorMap:
		if additionalInformation == 31 {
			return s.decodeIndefiniteMap(in)
		}
		return s.decodeMap(in, additionalInformation)
	case MajorTag:
		return s.decodeTag(in, additionalInformation)
	case MajorOther:
		return s.decodeOther(in, additionalInformation)
	default:
		return nil, 0, syntaxErrorf("cbor: unreachable major type %d", major)
	}
}

func (s *decodeState) decodeString(in []byte, additionalInformation byte) ([]byte, int, error) {
	length, offset, err := s.stringLength(in, additionalInformation)
	if err != nil {
		return nil, 0, err
	}
	return in[offset : offset+length], offset + length, nil
}

func (s *decodeState) decodeArray(in []byte, additionalInformation byte) (*DataItem, int, error) {
	count, offset, err := s.readArgument(in, additionalInformation)
	if err != nil {
		return nil, 0, err
	}
	if count > uint64(len(in)) {
		return nil, 0, syntaxErrorf("cbor: array length %d exceeds input", count)
	}
	if s.maxArray > 0 && count > uint64(s.maxArray) {
		return nil, 0, syntaxErrorf("cbor: array of %d elements exceeds limit %d", count, s.maxArray)
	}
	if err := s.descend(); err != nil {
		return nil, 0, err
	}
	defer s.ascend()

	var content []*DataItem
	for i := uint64(0); i < count; i++ {
		element, consumed, err := s.decodeItem(in[offset:])
		if err != nil {
			return nil, 0, err
		}
		content = append(content, element)
		offset += consumed
	}

	return &DataItem{Major: MajorArray, Content: content}, offset, nil
}

func (s *decodeState) decodeMap(in []byte, additionalInformation byte) (*DataItem, int, error) {
	count, offset, err := s.readArgument(in, additionalInformation)
	if err != nil {
		return nil, 0, err
	}
	if count > uint64(len(in)) {
		return nil, 0, syntaxErrorf("cbor: map length %d exceeds input", count)
	}
	if s.maxMap > 0 && count > uint64(s.maxMap) {
		return nil, 0, syntaxErrorf("cbor: map of %d pairs exceeds limit %d", count, s.maxMap)
	}
	if err := s.descend(); err != nil {
		return nil, 0, err
	}
	defer s.ascend()

	var seen map[string]struct{}
	if s.dupMapKey == DupError {
		seen = make(map[string]struct{})
	}

	var content []*DataItem
	var prevKey []byte
	for i := uint64(0); i < count; i++ {
		keyStart := offset
		key, keyConsumed, err := s.decodeItem(in[offset:])
		if err != nil {
			return nil, 0, err
		}
		rawKey := in[keyStart : keyStart+keyConsumed]
		if s.strict {
			if prevKey != nil && bytes.Compare(rawKey, prevKey) <= 0 {
				return nil, 0, syntaxErrorf("cbor: map keys not in canonical order")
			}
			prevKey = rawKey
		}
		if seen != nil {
			ck := s.canonicalKeyBytes(key)
			if _, dup := seen[string(ck)]; dup {
				return nil, 0, syntaxErrorf("cbor: duplicate map key")
			}
			seen[string(ck)] = struct{}{}
		}
		offset += keyConsumed

		value, valueConsumed, err := s.decodeItem(in[offset:])
		if err != nil {
			return nil, 0, err
		}
		offset += valueConsumed

		content = append(content, key, value)
	}

	return &DataItem{Major: MajorMap, Content: content}, offset, nil
}

const breakByte = 0xff

func (s *decodeState) decodeIndefiniteArray(in []byte) (*DataItem, int, error) {
	if s.rejectIndef {
		return nil, 0, syntaxErrorf("cbor: indefinite-length items not allowed")
	}
	if err := s.descend(); err != nil {
		return nil, 0, err
	}
	defer s.ascend()
	offset := 1
	var content []*DataItem
	for {
		if offset >= len(in) {
			return nil, 0, syntaxErrorf("cbor: unterminated indefinite-length array")
		}
		if in[offset] == breakByte {
			offset++
			break
		}
		if s.maxArray > 0 && len(content) >= s.maxArray {
			return nil, 0, syntaxErrorf("cbor: array exceeds limit %d", s.maxArray)
		}
		element, consumed, err := s.decodeItem(in[offset:])
		if err != nil {
			return nil, 0, err
		}
		content = append(content, element)
		offset += consumed
	}
	return &DataItem{Major: MajorArray, Content: content}, offset, nil
}

func (s *decodeState) decodeIndefiniteMap(in []byte) (*DataItem, int, error) {
	if s.rejectIndef {
		return nil, 0, syntaxErrorf("cbor: indefinite-length items not allowed")
	}
	if err := s.descend(); err != nil {
		return nil, 0, err
	}
	defer s.ascend()
	offset := 1
	var seen map[string]struct{}
	if s.dupMapKey == DupError {
		seen = make(map[string]struct{})
	}
	var content []*DataItem
	for {
		if offset >= len(in) {
			return nil, 0, syntaxErrorf("cbor: unterminated indefinite-length map")
		}
		if in[offset] == breakByte {
			offset++
			break
		}
		if s.maxMap > 0 && len(content)/2 >= s.maxMap {
			return nil, 0, syntaxErrorf("cbor: map exceeds limit %d", s.maxMap)
		}
		key, keyConsumed, err := s.decodeItem(in[offset:])
		if err != nil {
			return nil, 0, err
		}
		if seen != nil {
			ck := s.canonicalKeyBytes(key)
			if _, dup := seen[string(ck)]; dup {
				return nil, 0, syntaxErrorf("cbor: duplicate map key")
			}
			seen[string(ck)] = struct{}{}
		}
		offset += keyConsumed
		if offset >= len(in) || in[offset] == breakByte {
			return nil, 0, syntaxErrorf("cbor: indefinite-length map with odd number of items")
		}
		value, valueConsumed, err := s.decodeItem(in[offset:])
		if err != nil {
			return nil, 0, err
		}
		offset += valueConsumed
		content = append(content, key, value)
	}
	return &DataItem{Major: MajorMap, Content: content}, offset, nil
}

func (s *decodeState) decodeIndefiniteString(in []byte, major MajorType) (*DataItem, int, error) {
	if s.rejectIndef {
		return nil, 0, syntaxErrorf("cbor: indefinite-length items not allowed")
	}
	offset := 1
	var buf []byte
	for {
		if offset >= len(in) {
			return nil, 0, syntaxErrorf("cbor: unterminated indefinite-length string")
		}
		if in[offset] == breakByte {
			offset++
			break
		}
		chunkMajor, chunkInfo, err := identifyMajorType(in[offset])
		if err != nil {
			return nil, 0, err
		}
		if chunkMajor != major || chunkInfo == 31 {
			return nil, 0, syntaxErrorf("cbor: invalid chunk in indefinite-length string")
		}
		content, consumed, err := s.decodeString(in[offset:], chunkInfo)
		if err != nil {
			return nil, 0, err
		}
		buf = append(buf, content...)
		offset += consumed
	}
	return &DataItem{Major: major, Bytes: buf}, offset, nil
}

func (s *decodeState) decodeTag(in []byte, additionalInformation byte) (*DataItem, int, error) {
	tagNumber, offset, err := s.readArgument(in, additionalInformation)
	if err != nil {
		return nil, 0, err
	}
	if err := s.descend(); err != nil {
		return nil, 0, err
	}
	defer s.ascend()

	content, consumed, err := s.decodeItem(in[offset:])
	if err != nil {
		return nil, 0, err
	}

	if tagNumber == 2 || tagNumber == 3 {
		if content.Major != MajorBytes {
			return nil, 0, syntaxErrorf("cbor: bignum tag %d content must be a byte string, got major type %d", tagNumber, content.Major)
		}
		if s.strict {
			if err := bignumMinimal(content.Bytes); err != nil {
				return nil, 0, err
			}
		}
	}

	return &DataItem{
		Major:    MajorTag,
		Argument: tagNumber,
		Content:  []*DataItem{content},
	}, offset + consumed, nil
}

func (s *decodeState) decodeOther(in []byte, additionalInformation byte) (*DataItem, int, error) {
	if additionalInformation == 31 {
		return nil, 0, syntaxErrorf("cbor: unexpected break stop code")
	}
	if additionalInformation <= 24 {
		value, consumed, err := simpleValue(in, additionalInformation)
		if err != nil {
			return nil, 0, err
		}
		return &DataItem{Major: MajorOther, Argument: uint64(value)}, consumed, nil
	}

	width := argumentSize(additionalInformation)
	if width < 0 || len(in) < 1+width {
		return nil, 0, syntaxErrorf("cbor: float requires %d bytes but only %d are present", 1+width, len(in))
	}
	value, encodedWidth, err := decodeFloat(in[1 : 1+width])
	if err != nil {
		return nil, 0, err
	}
	if err := s.shortestFloat(value, encodedWidth); err != nil {
		return nil, 0, err
	}
	return &DataItem{Major: MajorOther, Float: value, FloatWidth: encodedWidth}, 1 + width, nil
}

func simpleValue(in []byte, additionalInformation byte) (byte, int, error) {
	if additionalInformation < 24 {
		return additionalInformation, 1, nil
	}
	if len(in) < 2 {
		return 0, 0, syntaxErrorf("cbor: simple value with additional information 24 requires 2 bytes")
	}
	if in[1] < 32 {
		return 0, 0, syntaxErrorf("cbor: a two-byte simple value cannot be less than 32")
	}
	return in[1], 2, nil
}

func decodeFloat(argument []byte) (float64, uint8, error) {
	switch len(argument) {
	case 2:
		return float64(math.Float32frombits(halfBitsToSingle(binary.BigEndian.Uint16(argument)))), 2, nil
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(argument))), 4, nil
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(argument)), 8, nil
	default:
		return 0, 0, syntaxErrorf("cbor: invalid float width %d", len(argument))
	}
}

func halfBitsToSingle(h uint16) uint32 {
	sign := uint32(h&0x8000) << 16
	exponent := uint32(h>>10) & 0x1f
	mantissa := uint32(h & 0x03ff)
	switch exponent {
	case 0:
		if mantissa == 0 {
			return sign
		}
		exponent = 1
		for mantissa&0x0400 == 0 {
			mantissa <<= 1
			exponent--
		}
		mantissa &= 0x03ff
		return sign | ((exponent + (127 - 15)) << 23) | (mantissa << 13)
	case 0x1f:
		return sign | 0x7f800000 | (mantissa << 13)
	default:
		return sign | ((exponent + (127 - 15)) << 23) | (mantissa << 13)
	}
}

func identifyMajorType(headByte byte) (major MajorType, additionalInformation byte, err error) {
	major = MajorType(headByte >> 5)
	additionalInformation = headByte & 0x1f

	switch {
	case additionalInformation <= 27:
		return major, additionalInformation, nil
	case slices.Contains([]byte{28, 29, 30}, additionalInformation):
		return 0, 0, syntaxErrorf("cbor: not well-formed: reserved additional information %d", additionalInformation)
	case additionalInformation == 31:
		if slices.Contains([]MajorType{MajorUint, MajorNint, MajorTag}, major) {
			return 0, 0, syntaxErrorf("cbor: not well-formed: indefinite length invalid for major type %d", major)
		}
		return major, additionalInformation, nil
	default:
		return 0, 0, syntaxErrorf("cbor: invalid additional information %d", additionalInformation)
	}
}

func argumentSize(additionalInformation byte) int {
	switch {
	case additionalInformation < 24:
		return 0
	case additionalInformation == 24:
		return 1
	case additionalInformation == 25:
		return 2
	case additionalInformation == 26:
		return 4
	case additionalInformation == 27:
		return 8
	default:
		return -1
	}
}

func (s *decodeState) readArgument(in []byte, additionalInformation byte) (uint64, int, error) {
	value, headerSize, err := readArgument(in, additionalInformation)
	if err != nil {
		return 0, 0, err
	}
	if err := s.shortest(additionalInformation, value); err != nil {
		return 0, 0, err
	}
	return value, headerSize, nil
}

func readArgument(in []byte, additionalInformation byte) (value uint64, headerSize int, err error) {
	switch {
	case additionalInformation < 24:
		return uint64(additionalInformation), 1, nil
	case additionalInformation == 24:
		if len(in) < 2 {
			return 0, 0, syntaxErrorf("cbor: additional information 24 requires at least 2 bytes")
		}
		return uint64(in[1]), 2, nil
	case additionalInformation == 25:
		if len(in) < 3 {
			return 0, 0, syntaxErrorf("cbor: additional information 25 requires at least 3 bytes")
		}
		return combineNextNBytes(2, in[1:]), 3, nil
	case additionalInformation == 26:
		if len(in) < 5 {
			return 0, 0, syntaxErrorf("cbor: additional information 26 requires at least 5 bytes")
		}
		return combineNextNBytes(4, in[1:]), 5, nil
	case additionalInformation == 27:
		if len(in) < 9 {
			return 0, 0, syntaxErrorf("cbor: additional information 27 requires at least 9 bytes")
		}
		return combineNextNBytes(8, in[1:]), 9, nil
	case additionalInformation == 31:
		return 0, 0, syntaxErrorf("cbor: indefinite length has no single argument")
	default:
		return 0, 0, syntaxErrorf("cbor: invalid additional information %d", additionalInformation)
	}
}

func (s *decodeState) stringLength(in []byte, additionalInformation byte) (length int, offset int, err error) {
	value, headerSize, err := s.readArgument(in, additionalInformation)
	if err != nil {
		return 0, 0, err
	}
	if s.maxString > 0 && value > uint64(s.maxString) {
		return 0, 0, syntaxErrorf("cbor: string of length %d exceeds limit %d", value, s.maxString)
	}
	available := uint64(len(in) - headerSize)
	if value > available {
		return 0, 0, syntaxErrorf("cbor: string of length %d exceeds the %d available bytes", value, available)
	}
	return int(value), headerSize, nil
}

func combineNextNBytes(n int, in []byte) uint64 {
	var result uint64
	for i := 0; i < n; i++ {
		result = result<<8 | uint64(in[i])
	}
	return result
}
