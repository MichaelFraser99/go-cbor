package internal

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"slices"

	"github.com/MichaelFraser99/go-cbor/model"
)

const HalfPrecisionExponentBits = 5
const HalfPrecisionExponentBias = 15
const HalfPrecisionFractionBits = 10

const SinglePrecisionExponentBits = 8
const SinglePrecisionExponentBias = 127
const SinglePrecisionFractionBits = 23

const DoublePrecisionExponentBits = 11
const DoublePrecisionExponentBias = 1023
const DoublePrecisionFractionBits = 52

func DecodeFromHexString(dataItem string) (*model.DataItem, error) {
	dataItemBytes, err := hexToBytes(dataItem)
	if err != nil {
		return nil, err
	}
	item, consumed, err := decodeDataItem(dataItemBytes)
	if err != nil {
		return nil, err
	}
	if consumed != len(dataItemBytes) {
		return nil, fmt.Errorf("unexpected %d trailing bytes after data item", len(dataItemBytes)-consumed)
	}
	return item, nil
}

func decodeDataItem(dataItemBytes []byte) (*model.DataItem, int, error) {
	if len(dataItemBytes) == 0 {
		return nil, 0, fmt.Errorf("empty data item")
	}

	majorType, additionalInformation, err := identifyMajorType(dataItemBytes[0])
	if err != nil {
		return nil, 0, err
	}

	var dataBytes []byte
	var sign int8
	var consumed int
	switch majorType {
	case model.MajorTypeTag:
		return tagDecoder(dataItemBytes, additionalInformation)
	case model.MajorTypeArray:
		return arrayDecoder(dataItemBytes, additionalInformation)
	case model.MajorTypeMap:
		return mapDecoder(dataItemBytes, additionalInformation)
	case model.MajorTypeInteger:
		sign = int8(1)
		v, err := integerDecoder(dataItemBytes, additionalInformation)
		if err != nil {
			return nil, 0, err
		}
		dataBytes = make([]byte, 8)
		binary.BigEndian.PutUint64(dataBytes, *v)
		consumed = 1 + argumentSize(additionalInformation)
	case model.MajorTypeNegativeInteger:
		sign = int8(-1)
		v, err := integerDecoder(dataItemBytes, additionalInformation)
		if err != nil {
			return nil, 0, err
		}
		dataBytes = make([]byte, 8)
		binary.BigEndian.PutUint64(dataBytes, *v)
		consumed = 1 + argumentSize(additionalInformation)
	case model.MajorTypeByteString:
		dataBytes, consumed, err = byteStringDecoder(dataItemBytes, additionalInformation)
		if err != nil {
			return nil, 0, err
		}
	case model.MajorTypeTextString:
		dataBytes, consumed, err = textStringDecoder(dataItemBytes, additionalInformation)
		if err != nil {
			return nil, 0, err
		}
	case model.MajorTypeFloatOrSimple:
		if additionalInformation <= 24 {
			v, err := simpleDecoder(dataItemBytes, additionalInformation)
			if err != nil {
				return nil, 0, err
			}
			switch sv := v.(type) {
			case int:
				dataBytes = []byte{byte(sv)}
			default:
				dataBytes = []byte(fmt.Sprintf("%v", sv))
			}
			consumed = 1 + argumentSize(additionalInformation)
		} else {
			argBytes := argumentSize(additionalInformation)
			if argBytes < 0 || len(dataItemBytes) < 1+argBytes {
				return nil, 0, fmt.Errorf("float requires %d bytes but only %d are present", 1+argBytes, len(dataItemBytes))
			}
			v, err := floatDecoder(dataItemBytes[1 : 1+argBytes])
			if err != nil {
				return nil, 0, err
			}
			dataBytes = make([]byte, 8)
			if *v >= 0 {
				sign = int8(1)
			} else {
				sign = int8(-1)
			}
			binary.BigEndian.PutUint64(dataBytes, math.Float64bits(*v))
			consumed = 1 + argBytes
		}
	default:
		return nil, 0, fmt.Errorf("invalid major type %d", majorType)
	}
	return &model.DataItem{
		MajorType: int8(majorType),
		Sign:      sign,
		Data:      dataBytes,
	}, consumed, nil
}

func tagDecoder(in []byte, additionalInformation byte) (*model.DataItem, int, error) {
	tagNumber, contentOffset, err := readArgument(in, additionalInformation)
	if err != nil {
		return nil, 0, err
	}

	content, contentConsumed, err := decodeDataItem(in[contentOffset:])
	if err != nil {
		return nil, 0, err
	}

	item := &model.DataItem{
		MajorType: int8(model.MajorTypeTag),
		Data:      in[contentOffset : contentOffset+contentConsumed],
		Content:   []*model.DataItem{content},
	}
	switch tagNumber {
	case 2:
		item.Sign = int8(1)
	case 3:
		item.Sign = int8(-1)
	}
	return item, contentOffset + contentConsumed, nil
}

func arrayDecoder(in []byte, additionalInformation byte) (*model.DataItem, int, error) {
	count, offset, err := readArgument(in, additionalInformation)
	if err != nil {
		return nil, 0, err
	}

	var content []*model.DataItem
	for i := uint64(0); i < count; i++ {
		element, consumed, err := decodeDataItem(in[offset:])
		if err != nil {
			return nil, 0, err
		}
		content = append(content, element)
		offset += consumed
	}

	return &model.DataItem{
		MajorType: int8(model.MajorTypeArray),
		Content:   content,
	}, offset, nil
}

func mapDecoder(in []byte, additionalInformation byte) (*model.DataItem, int, error) {
	count, offset, err := readArgument(in, additionalInformation)
	if err != nil {
		return nil, 0, err
	}

	var content []*model.DataItem
	for i := uint64(0); i < count; i++ {
		key, keyConsumed, err := decodeDataItem(in[offset:])
		if err != nil {
			return nil, 0, err
		}
		offset += keyConsumed

		value, valueConsumed, err := decodeDataItem(in[offset:])
		if err != nil {
			return nil, 0, err
		}
		offset += valueConsumed

		content = append(content, key, value)
	}

	return &model.DataItem{
		MajorType: int8(model.MajorTypeMap),
		Content:   content,
	}, offset, nil
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

func readArgument(in []byte, additionalInformation byte) (value uint64, headerSize int, err error) {
	switch {
	case additionalInformation < 24:
		return uint64(additionalInformation), 1, nil
	case additionalInformation == 24:
		if len(in) < 2 {
			return 0, 0, fmt.Errorf("additional information of 24 requires a data item of at least 2 bytes")
		}
		return uint64(in[1]), 2, nil
	case additionalInformation == 25:
		if len(in) < 3 {
			return 0, 0, fmt.Errorf("additional information of 25 requires a data item of at least 3 bytes")
		}
		return combineNextNBytes(2, in[1:]), 3, nil
	case additionalInformation == 26:
		if len(in) < 5 {
			return 0, 0, fmt.Errorf("additional information of 26 requires a data item of at least 5 bytes")
		}
		return combineNextNBytes(4, in[1:]), 5, nil
	case additionalInformation == 27:
		if len(in) < 9 {
			return 0, 0, fmt.Errorf("additional information of 27 requires a data item of at least 9 bytes")
		}
		return combineNextNBytes(8, in[1:]), 9, nil
	case additionalInformation == 31:
		return 0, 0, fmt.Errorf("indefinite-length items not yet implemented")
	default:
		return 0, 0, fmt.Errorf("invalid additional information (%d)", additionalInformation)
	}
}

func identifyMajorType(headByte byte) (majorType model.MajorType, additionalInformation byte, err error) {
	switch headByte >> 5 {
	case 0:
		majorType = model.MajorTypeInteger
	case 1:
		majorType = model.MajorTypeNegativeInteger
	case 2:
		majorType = model.MajorTypeByteString
	case 3:
		majorType = model.MajorTypeTextString
	case 4:
		majorType = model.MajorTypeArray
	case 5:
		majorType = model.MajorTypeMap
	case 6:
		majorType = model.MajorTypeTag
	case 7:
		majorType = model.MajorTypeFloatOrSimple
	default:
		return 0, 0, fmt.Errorf("invalid major type") //not mathematically possible
	}

	additionalInformation = headByte & 0x1f

	switch {
	case additionalInformation <= 27 == true:
		return majorType, additionalInformation, nil
	case slices.Contains([]byte{28, 29, 30}, additionalInformation):
		return 0, 0, fmt.Errorf("not well-formed: invalid additional information")
	case additionalInformation == 31:
		if slices.Contains([]model.MajorType{0, 1, 6}, majorType) {
			return 0, 0, fmt.Errorf("not well-formed: invalid additional information for major type")
		}
		return majorType, additionalInformation, nil
	default:
		return 0, 0, fmt.Errorf("invalid additional information") //also not mathematically possible
	}
}

func hexToBytes(hexStr string) ([]byte, error) {
	if len(hexStr) >= 2 && hexStr[:2] == "0x" {
		hexStr = hexStr[2:]
	}

	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}

	return hex.DecodeString(hexStr)
}

func integerDecoder(in []byte, additionalInformation byte) (*uint64, error) {
	var result uint64
	switch {
	case additionalInformation < 24:
		result = uint64(additionalInformation)
	case additionalInformation == 24:
		if len(in) < 2 {
			return nil, fmt.Errorf("additional information of 24 requires a data item of at least 2 bytes")
		}
		result = uint64(in[1])
	case additionalInformation == 25:
		if len(in) < 3 {
			return nil, fmt.Errorf("additional information of 25 requires a data item of at least 3 bytes")
		}
		result = combineNextNBytes(2, in[1:])
	case additionalInformation == 26:
		if len(in) < 5 {
			return nil, fmt.Errorf("additional information of 26 requires a data item of at least 5 bytes")
		}
		result = combineNextNBytes(4, in[1:])
	case additionalInformation == 27:
		if len(in) < 9 {
			return nil, fmt.Errorf("additional information of 27 requires a data item of at least 9 bytes")
		}
		result = combineNextNBytes(8, in[1:])
	default:
		return nil, fmt.Errorf("invalid additional information for an integer major type")
	}
	return &result, nil
}

func byteStringDecoder(in []byte, additionalInformation byte) ([]byte, int, error) { //todo: indeterminate length byte strings
	length, offset, err := stringLengthFromAdditionalInformation(in, additionalInformation)
	if err != nil {
		return nil, 0, err
	}
	return in[offset : offset+length], offset + length, nil
}

func textStringDecoder(in []byte, additionalInformation byte) ([]byte, int, error) { //todo: indeterminate length byte strings
	length, offset, err := stringLengthFromAdditionalInformation(in, additionalInformation)
	if err != nil {
		return nil, 0, err
	}
	return in[offset : offset+length], offset + length, nil
}

func simpleDecoder(in []byte, additionalInformation byte) (any, error) {
	switch {
	case additionalInformation < 20:
		return int(additionalInformation), nil
	case additionalInformation == 20:
		return false, nil
	case additionalInformation == 21:
		return true, nil
	case additionalInformation == 22:
		return nil, nil
	case additionalInformation == 23:
		return "undefined", nil
	case additionalInformation == 24:
		if len(in) < 2 {
			return nil, fmt.Errorf("additional information of 24 requires a data item of at least 2 bytes")
		}
		if in[1] < 32 {
			return nil, fmt.Errorf("a two byte simple value cannot be less than 32")
		}
		return int(in[1]), nil
	default:
		return nil, fmt.Errorf("invalid additional information (%d) for a simple major type", additionalInformation)
	}
}

func floatDecoder(in []byte) (*float64, error) {
	switch len(in) * 8 {
	case HalfPrecisionExponentBits + HalfPrecisionFractionBits + 1:
		result, _, err := ieeDecoder(in[0]>>7, uint16((in[0]<<1)>>3), append([]byte{in[0] & 0x3}, in[1:]...), HalfPrecisionExponentBias, HalfPrecisionExponentBits, HalfPrecisionFractionBits)
		return result, err
	case SinglePrecisionExponentBits + SinglePrecisionFractionBits + 1:
		result, _, err := ieeDecoder(in[0]>>7, ((uint16(in[0])<<8|uint16(in[1]))<<1)>>8, append([]byte{0x00, in[1] & 0x7f}, in[2:]...), SinglePrecisionExponentBias, SinglePrecisionExponentBits, SinglePrecisionFractionBits)
		return result, err
	case DoublePrecisionExponentBits + DoublePrecisionFractionBits + 1:
		result, _, err := ieeDecoder(in[0]>>7, ((uint16(in[0])<<8|uint16(in[1]))<<1)>>5, append([]byte{0x00, in[1] & 0xf}, in[2:]...), DoublePrecisionExponentBias, DoublePrecisionExponentBits, DoublePrecisionFractionBits)
		return result, err
	default:
		return nil, fmt.Errorf("invalid number of bits for well-formed float")
	}
}

func ieeDecoder(signBit byte, exponentBytes uint16, mantissaBytes []byte, offset int, exponentBitCount, fractionBitCount int) (*float64, *byte, error) {
	sign := math.Pow(float64(-1), float64(signBit))
	exponent := func(exponent uint16) uint16 {
		if exponent == 0 {
			return 1
		}
		return exponent
	}(exponentBytes)
	hiddenBit := 1
	fractionIsZero := true
	for _, b := range mantissaBytes {
		if b != 0 {
			fractionIsZero = false
			break
		}
	}
	if exponentBytes == 0 {
		hiddenBit = 0
	}
	if float64(exponent) == math.Pow(float64(2), float64(exponentBitCount))-1 {
		if fractionIsZero {
			return new(math.Inf(int(sign))), new(byte(sign)), nil
		}
		return new(math.NaN()), new(byte(sign)), nil
	}
	mantissaValue := func(in []byte) float64 {
		startingIndex := fractionBitCount - (len(in) * 8)
		total := 0.0
		bitCounter := 0
		byteIndex := 0
		for i := startingIndex; i < fractionBitCount; i++ {
			if i >= 0 {
				shift := 7 - bitCounter
				bi := (in[byteIndex] >> shift) & 0x01
				total += float64(bi) * math.Pow(2, (-1)*float64(i+1))
			}
			bitCounter++
			if bitCounter == 8 {
				bitCounter = 0
				byteIndex++
			}
		}
		return total
	}(mantissaBytes)
	return new(sign * math.Pow(2, float64(int(exponent)-offset)) * (float64(hiddenBit) + mantissaValue)), new(byte(sign)), nil
}

func combineNextNBytes(n int, in []byte) uint64 {
	var result uint64
	for i := 0; i < n; i++ {
		result = result<<8 | uint64(in[i])
	}
	return result
}

func stringLengthFromAdditionalInformation(in []byte, additionalInformation byte) (length int, offset int, err error) {
	switch {
	case additionalInformation < 24:
		length = int(additionalInformation)
		offset = 1
	case additionalInformation == 24:
		if len(in) < 2 {
			return 0, 0, fmt.Errorf("additional information of 24 requires a data item of at least 2 bytes")
		}
		length = int(in[1])
		offset = 2
	case additionalInformation == 25:
		if len(in) < 3 {
			return 0, 0, fmt.Errorf("additional information of 25 requires a data item of at least 3 bytes")
		}
		length = int(combineNextNBytes(2, in[1:]))
		offset = 3
	case additionalInformation == 26:
		if len(in) < 5 {
			return 0, 0, fmt.Errorf("additional information of 26 requires a data item of at least 5 bytes")
		}
		length = int(combineNextNBytes(4, in[1:]))
		offset = 5
	case additionalInformation == 27:
		if len(in) < 9 {
			return 0, 0, fmt.Errorf("additional information of 27 requires a data item of at least 9 bytes")
		}
		length = int(combineNextNBytes(8, in[1:]))
		offset = 9
	case additionalInformation == 31:
		//todo
		return 0, 0, fmt.Errorf("indefinite-length strings not yet implemented")
	default:
		return 0, 0, fmt.Errorf("invalid additional information (%d) for a string major type", additionalInformation)
	}
	if length < 0 || len(in) < offset+length {
		return 0, 0, fmt.Errorf("string of length %d requires %d bytes but only %d are present", length, offset+length, len(in))
	}
	return length, offset, nil
}
