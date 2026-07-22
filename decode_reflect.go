package cbor

import (
	"bytes"
	"math"
	"math/big"
	"reflect"
	"time"
)

var (
	rawMessageType  = reflect.TypeOf(RawMessage(nil))
	mapType         = reflect.TypeOf(Map(nil))
	dataItemType    = reflect.TypeOf(DataItem{})
	tagType         = reflect.TypeOf(Tag{})
	rawTagType      = reflect.TypeOf(RawTag{})
	undefinedType   = reflect.TypeOf(Undefined{})
	simpleType      = reflect.TypeOf(SimpleValue(0))
	bigIntType      = reflect.TypeOf(big.Int{})
	timeType        = reflect.TypeOf(time.Time{})
	unmarshalerType = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
)

func (s *decodeState) itemSpan(data []byte) (int, error) {
	return s.skipItem(data)
}

func (s *decodeState) skipItem(in []byte) (n int, err error) {
	defer func() { err = s.stamp(in, err) }()
	if len(in) == 0 {
		return 0, syntaxErrorf("cbor: empty data item")
	}
	major, ai, err := identifyMajorType(in[0])
	if err != nil {
		return 0, err
	}
	switch major {
	case MajorUint, MajorNint:
		_, n, err := s.readArgument(in, ai)
		return n, err
	case MajorBytes, MajorText:
		if ai == 31 {
			_, n, err := s.decodeIndefiniteString(in, major)
			return n, err
		}
		length, offset, err := s.stringLength(in, ai)
		if err != nil {
			return 0, err
		}
		return offset + length, nil
	case MajorArray:
		if ai == 31 {
			_, n, err := s.decodeIndefiniteArray(in)
			return n, err
		}
		if err := s.descend(); err != nil {
			return 0, err
		}
		defer s.ascend()
		count, offset, err := s.readArgument(in, ai)
		if err != nil {
			return 0, err
		}
		for i := uint64(0); i < count; i++ {
			n, err := s.skipItem(in[offset:])
			if err != nil {
				return 0, err
			}
			offset += n
		}
		return offset, nil
	case MajorMap:
		if ai == 31 {
			_, n, err := s.decodeIndefiniteMap(in)
			return n, err
		}
		if err := s.descend(); err != nil {
			return 0, err
		}
		defer s.ascend()
		count, offset, err := s.readArgument(in, ai)
		if err != nil {
			return 0, err
		}
		for i := uint64(0); i < count; i++ {
			kn, err := s.skipItem(in[offset:])
			if err != nil {
				return 0, err
			}
			offset += kn
			vn, err := s.skipItem(in[offset:])
			if err != nil {
				return 0, err
			}
			offset += vn
		}
		return offset, nil
	case MajorTag:
		if err := s.descend(); err != nil {
			return 0, err
		}
		defer s.ascend()
		tagNumber, offset, err := s.readArgument(in, ai)
		if err != nil {
			return 0, err
		}
		if tagNumber == 2 || tagNumber == 3 {
			if offset >= len(in) {
				return 0, syntaxErrorf("cbor: empty data item")
			}
			cMajor, cai, err := identifyMajorType(in[offset])
			if err != nil {
				return 0, err
			}
			if cMajor != MajorBytes {
				return 0, syntaxErrorf("cbor: bignum tag %d content must be a byte string, got major type %d", tagNumber, cMajor)
			}
			if s.strict {
				plen, hlen, err := s.stringLength(in[offset:], cai)
				if err != nil {
					return 0, err
				}
				if err := bignumMinimal(in[offset+hlen : offset+hlen+plen]); err != nil {
					return 0, err
				}
			}
		}
		n, err := s.skipItem(in[offset:])
		if err != nil {
			return 0, err
		}
		return offset + n, nil
	case MajorOther:
		if ai == 31 {
			return 0, syntaxErrorf("cbor: unexpected break stop code")
		}
		if ai <= 24 {
			_, n, err := simpleValue(in, ai)
			return n, err
		}
		width := argumentSize(ai)
		if width < 0 || len(in) < 1+width {
			return 0, syntaxErrorf("cbor: float requires %d bytes but only %d are present", 1+width, len(in))
		}
		return 1 + width, nil
	default:
		return 0, syntaxErrorf("cbor: unreachable major type %d", major)
	}
}

func (s *decodeState) decodeScalar(in []byte, rv reflect.Value) (n int, err error) {
	if len(in) == 0 {
		return 0, syntaxErrorf("cbor: empty data item")
	}
	major, ai, err := identifyMajorType(in[0])
	if err != nil {
		return 0, err
	}
	switch major {
	case MajorUint:
		v, n, err := s.readArgument(in, ai)
		if err != nil {
			return 0, err
		}
		return n, setInteger(rv, v, false)
	case MajorNint:
		v, n, err := s.readArgument(in, ai)
		if err != nil {
			return 0, err
		}
		return n, setInteger(rv, v, true)
	case MajorBytes:
		if ai == 31 {
			item, n, err := s.decodeItem(in)
			if err != nil {
				return 0, err
			}
			return n, assignScalar(item, rv)
		}
		b, n, err := s.decodeString(in, ai)
		if err != nil {
			return 0, err
		}
		return n, setBytes(rv, b)
	case MajorText:
		if ai == 31 {
			item, n, err := s.decodeItem(in)
			if err != nil {
				return 0, err
			}
			return n, assignScalar(item, rv)
		}
		b, n, err := s.decodeString(in, ai)
		if err != nil {
			return 0, err
		}
		return n, setString(rv, string(b))
	case MajorOther:
		if ai == 31 {
			return 0, syntaxErrorf("cbor: unexpected break stop code")
		}
		if ai <= 24 {
			v, n, err := simpleValue(in, ai)
			if err != nil {
				return 0, err
			}
			return n, assignSimple(rv, v)
		}
		width := argumentSize(ai)
		if width < 0 || len(in) < 1+width {
			return 0, syntaxErrorf("cbor: float requires %d bytes but only %d are present", 1+width, len(in))
		}
		f, w, err := decodeFloat(in[1 : 1+width])
		if err != nil {
			return 0, err
		}
		if err := s.shortestFloat(f, w); err != nil {
			return 0, err
		}
		return 1 + width, setFloat(rv, f)
	default:
		return 0, &UnmarshalTypeError{CBORType: majorName(major), GoType: rv.Type()}
	}
}

func assignSimple(rv reflect.Value, v byte) error {
	switch v {
	case 20, 21:
		if rv.Kind() != reflect.Bool {
			return &UnmarshalTypeError{CBORType: "bool", GoType: rv.Type()}
		}
		rv.SetBool(v == 21)
		return nil
	case 22, 23:
		if canNil(rv) {
			rv.Set(reflect.Zero(rv.Type()))
		}
		return nil
	default:
		if rv.Type() == simpleType {
			rv.SetUint(uint64(v))
			return nil
		}
		return &UnmarshalTypeError{CBORType: "simple value", GoType: rv.Type()}
	}
}

func majorName(m MajorType) string {
	switch m {
	case MajorUint:
		return "unsigned integer"
	case MajorNint:
		return "negative integer"
	case MajorBytes:
		return "byte string"
	case MajorText:
		return "text string"
	case MajorArray:
		return "array"
	case MajorMap:
		return "map"
	case MajorTag:
		return "tag"
	default:
		return "simple/float"
	}
}

func (s *decodeState) reflect(data []byte, rv reflect.Value) (int, error) {
	n, err := s.reflectValue(data, rv)
	if err != nil {
		err = s.stamp(data, err)
	}
	return n, err
}

func (s *decodeState) reflectValue(data []byte, rv reflect.Value) (int, error) {
	if len(data) == 0 {
		return 0, syntaxErrorf("cbor: empty data item")
	}

	if rv.CanAddr() {
		if pv := rv.Addr(); pv.Type().Implements(unmarshalerType) {
			n, err := s.itemSpan(data)
			if err != nil {
				return 0, err
			}
			return n, pv.Interface().(Unmarshaler).UnmarshalCBOR(data[:n])
		}
	}

	switch rv.Type() {
	case rawMessageType:
		n, err := s.itemSpan(data)
		if err != nil {
			return 0, err
		}
		rv.SetBytes(append([]byte(nil), data[:n]...))
		return n, nil
	case dataItemType:
		item, n, err := s.decodeItem(data)
		if err != nil {
			return 0, err
		}
		rv.Set(reflect.ValueOf(*item))
		return n, nil
	case mapType:
		item, n, err := s.decodeItem(data)
		if err != nil {
			return 0, err
		}
		if item.Major != MajorMap {
			return 0, &UnmarshalTypeError{CBORType: majorName(item.Major), GoType: mapType}
		}
		rv.Set(reflect.ValueOf(item.Native()))
		return n, nil
	case rawTagType:
		major, ai, err := identifyMajorType(data[0])
		if err != nil {
			return 0, err
		}
		if major != MajorTag {
			return 0, &UnmarshalTypeError{CBORType: majorName(major), GoType: rawTagType}
		}
		num, off, err := s.readArgument(data, ai)
		if err != nil {
			return 0, err
		}
		span, err := s.itemSpan(data[off:])
		if err != nil {
			return 0, err
		}
		rv.Set(reflect.ValueOf(RawTag{Number: num, Content: append(RawMessage(nil), data[off:off+span]...)}))
		return off + span, nil
	}

	if rv.Kind() == reflect.Interface && rv.NumMethod() == 0 {
		item, n, err := s.decodeItem(data)
		if err != nil {
			return 0, err
		}
		v := item.Native()
		if v == nil {
			rv.Set(reflect.Zero(rv.Type()))
		} else {
			rv.Set(reflect.ValueOf(v))
		}
		return n, nil
	}

	if rv.Kind() == reflect.Pointer {
		if data[0] == 0xf6 {
			rv.Set(reflect.Zero(rv.Type()))
			return 1, nil
		}
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		return s.reflect(data, rv.Elem())
	}

	major, additionalInformation, err := identifyMajorType(data[0])
	if err != nil {
		return 0, err
	}

	switch major {
	case MajorArray:
		return s.decodeArrayReflect(data, additionalInformation, rv)
	case MajorMap:
		return s.decodeMapReflect(data, additionalInformation, rv)
	case MajorTag:
		return s.decodeTagReflect(data, additionalInformation, rv)
	default:
		return s.decodeScalar(data, rv)
	}
}

func assignScalar(item *DataItem, rv reflect.Value) error {
	if item.Major == MajorBytes {
		return setBytes(rv, item.Bytes)
	}
	return setString(rv, string(item.Bytes))
}

func setInteger(rv reflect.Value, argument uint64, negative bool) error {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if argument > uint64(math.MaxInt64) {
			return &UnmarshalTypeError{CBORType: "integer", GoType: rv.Type()}
		}
		value := int64(argument)
		if negative {
			value = -1 - value
		}
		if rv.OverflowInt(value) {
			return &UnmarshalTypeError{CBORType: "integer", GoType: rv.Type()}
		}
		rv.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if negative {
			return &UnmarshalTypeError{CBORType: "negative integer", GoType: rv.Type()}
		}
		if rv.OverflowUint(argument) {
			return &UnmarshalTypeError{CBORType: "integer", GoType: rv.Type()}
		}
		rv.SetUint(argument)
	case reflect.Float32, reflect.Float64:
		if negative {
			rv.SetFloat(-1 - float64(argument))
		} else {
			rv.SetFloat(float64(argument))
		}
	default:
		if rv.Type() == bigIntType {
			b := new(big.Int).SetUint64(argument)
			if negative {
				b.Add(b, big.NewInt(1))
				b.Neg(b)
			}
			rv.Set(reflect.ValueOf(*b))
			return nil
		}
		if rv.Type() == timeType {
			if argument >= 1<<63 {
				return &UnmarshalTypeError{CBORType: "integer", GoType: rv.Type()}
			}
			sec := int64(argument)
			if negative {
				sec = -1 - sec
			}
			rv.Set(reflect.ValueOf(time.Unix(sec, 0).UTC()))
			return nil
		}
		return &UnmarshalTypeError{CBORType: "integer", GoType: rv.Type()}
	}
	return nil
}

func setFloat(rv reflect.Value, f float64) error {
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		rv.SetFloat(f)
		return nil
	default:
		if rv.Type() == timeType {
			sec, frac := math.Modf(f)
			rv.Set(reflect.ValueOf(time.Unix(int64(sec), int64(frac*1e9)).UTC()))
			return nil
		}
		return &UnmarshalTypeError{CBORType: "float", GoType: rv.Type()}
	}
}

func setBytes(rv reflect.Value, b []byte) error {
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
		rv.SetBytes(append([]byte(nil), b...))
		return nil
	}
	if rv.Kind() == reflect.Array && rv.Type().Elem().Kind() == reflect.Uint8 {
		reflect.Copy(rv, reflect.ValueOf(b))
		return nil
	}
	return &UnmarshalTypeError{CBORType: "byte string", GoType: rv.Type()}
}

func setString(rv reflect.Value, str string) error {
	if rv.Kind() == reflect.String {
		rv.SetString(str)
		return nil
	}
	return &UnmarshalTypeError{CBORType: "text string", GoType: rv.Type()}
}

func canNil(rv reflect.Value) bool {
	switch rv.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		return true
	}
	return false
}

func (s *decodeState) decodeIndefiniteReflect(data []byte, rv reflect.Value) (int, error) {
	item, n, err := s.decodeItem(data)
	if err != nil {
		return 0, err
	}
	definite, err := Marshal(*item)
	if err != nil {
		return 0, err
	}
	if _, err := s.reflect(definite, rv); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *decodeState) decodeArrayReflect(data []byte, additionalInformation byte, rv reflect.Value) (int, error) {
	if additionalInformation == 31 {
		return s.decodeIndefiniteReflect(data, rv)
	}
	if err := s.descend(); err != nil {
		return 0, err
	}
	defer s.ascend()

	count, offset, err := s.readArgument(data, additionalInformation)
	if err != nil {
		return 0, err
	}
	if count > uint64(len(data)) {
		return 0, syntaxErrorf("cbor: array length exceeds input")
	}
	if s.maxArray > 0 && count > uint64(s.maxArray) {
		return 0, syntaxErrorf("cbor: array of %d elements exceeds limit %d", count, s.maxArray)
	}

	switch rv.Kind() {
	case reflect.Slice:
		slice := reflect.MakeSlice(rv.Type(), int(count), int(count))
		for i := 0; i < int(count); i++ {
			n, err := s.reflect(data[offset:], slice.Index(i))
			if err != nil {
				return 0, err
			}
			offset += n
		}
		rv.Set(slice)
	case reflect.Array:
		for i := 0; i < int(count); i++ {
			if i < rv.Len() {
				n, err := s.reflect(data[offset:], rv.Index(i))
				if err != nil {
					return 0, err
				}
				offset += n
			} else {
				n, err := s.itemSpan(data[offset:])
				if err != nil {
					return 0, err
				}
				offset += n
			}
		}
	case reflect.Struct:
		plan := planFor(rv.Type())
		if !plan.toArray {
			return 0, &UnmarshalTypeError{CBORType: "array", GoType: rv.Type()}
		}
		for i := 0; i < int(count); i++ {
			if i < len(plan.fields) {
				n, err := s.reflect(data[offset:], fieldByIndexAlloc(rv, plan.fields[i].index))
				if err != nil {
					return 0, err
				}
				offset += n
			} else {
				n, err := s.itemSpan(data[offset:])
				if err != nil {
					return 0, err
				}
				offset += n
			}
		}
	default:
		return 0, &UnmarshalTypeError{CBORType: "array", GoType: rv.Type()}
	}
	return offset, nil
}

func (s *decodeState) decodeMapReflect(data []byte, additionalInformation byte, rv reflect.Value) (int, error) {
	if additionalInformation == 31 {
		return s.decodeIndefiniteReflect(data, rv)
	}
	if err := s.descend(); err != nil {
		return 0, err
	}
	defer s.ascend()

	count, offset, err := s.readArgument(data, additionalInformation)
	if err != nil {
		return 0, err
	}
	if count > uint64(len(data)) {
		return 0, syntaxErrorf("cbor: map length exceeds input")
	}
	if s.maxMap > 0 && count > uint64(s.maxMap) {
		return 0, syntaxErrorf("cbor: map of %d pairs exceeds limit %d", count, s.maxMap)
	}

	var seen map[string]struct{}
	if s.dupMapKey == DupError {
		seen = make(map[string]struct{})
	}
	var prevKey []byte

	switch rv.Kind() {
	case reflect.Struct:
		plan := planFor(rv.Type())
		for i := 0; i < int(count); i++ {
			kn, err := s.skipItem(data[offset:])
			if err != nil {
				return 0, err
			}
			if s.strict {
				rawKey := data[offset : offset+kn]
				if prevKey != nil && bytes.Compare(rawKey, prevKey) <= 0 {
					return 0, syntaxErrorf("cbor: map keys not in canonical order")
				}
				prevKey = rawKey
			}
			lookup := string(data[offset : offset+kn])
			offset += kn
			f, ok := plan.byKey[lookup]
			if !ok {
				keyItem, _, err := s.decodeItem(data[offset-kn:])
				if err != nil {
					return 0, err
				}
				canonical, err := Marshal(*keyItem)
				if err != nil {
					return 0, err
				}
				lookup = string(canonical)
				f, ok = plan.byKey[lookup]
			}
			if seen != nil {
				if _, dup := seen[lookup]; dup {
					return 0, syntaxErrorf("cbor: duplicate map key")
				}
				seen[lookup] = struct{}{}
			}
			if ok {
				n, err := s.reflect(data[offset:], fieldByIndexAlloc(rv, f.index))
				if err != nil {
					return 0, err
				}
				offset += n
			} else {
				n, err := s.itemSpan(data[offset:])
				if err != nil {
					return 0, err
				}
				offset += n
			}
		}
	case reflect.Map:
		if rv.IsNil() {
			rv.Set(reflect.MakeMap(rv.Type()))
		}
		keyType := rv.Type().Key()
		elemType := rv.Type().Elem()
		for i := 0; i < int(count); i++ {
			key := reflect.New(keyType).Elem()
			kn, err := s.reflect(data[offset:], key)
			if err != nil {
				return 0, err
			}
			if s.strict {
				rawKey := data[offset : offset+kn]
				if prevKey != nil && bytes.Compare(rawKey, prevKey) <= 0 {
					return 0, syntaxErrorf("cbor: map keys not in canonical order")
				}
				prevKey = rawKey
			}
			offset += kn
			if !key.Comparable() {
				return 0, &UnmarshalTypeError{CBORType: "map key", GoType: keyType}
			}
			if seen != nil {
				kb, err := Marshal(key.Interface())
				if err != nil {
					return 0, err
				}
				if _, dup := seen[string(kb)]; dup {
					return 0, syntaxErrorf("cbor: duplicate map key")
				}
				seen[string(kb)] = struct{}{}
			}
			val := reflect.New(elemType).Elem()
			vn, err := s.reflect(data[offset:], val)
			if err != nil {
				return 0, err
			}
			offset += vn
			rv.SetMapIndex(key, val)
		}
	default:
		return 0, &UnmarshalTypeError{CBORType: "map", GoType: rv.Type()}
	}
	return offset, nil
}

func (s *decodeState) decodeTagReflect(data []byte, additionalInformation byte, rv reflect.Value) (int, error) {
	if rv.Type() == tagType {
		item, n, err := s.decodeItem(data)
		if err != nil {
			return 0, err
		}
		rv.Set(reflect.ValueOf(Tag{Number: item.Argument, Content: item.Content[0].Native()}))
		return n, nil
	}
	if rv.Type() == bigIntType {
		item, n, err := s.decodeItem(data)
		if err != nil {
			return 0, err
		}
		rv.Set(reflect.ValueOf(*bigIntFromTag(item)))
		return n, nil
	}

	if err := s.descend(); err != nil {
		return 0, err
	}
	defer s.ascend()

	tagNumber, offset, err := s.readArgument(data, additionalInformation)
	if err != nil {
		return 0, err
	}

	if rv.Type() == timeType {
		return s.decodeTimeReflect(data, offset, tagNumber, rv)
	}

	n, err := s.reflect(data[offset:], rv)
	if err != nil {
		return 0, err
	}
	return offset + n, nil
}

func bigIntFromTag(item *DataItem) *big.Int {
	magnitude := new(big.Int).SetBytes(item.Content[0].Bytes)
	if item.Argument == 3 {
		magnitude.Add(magnitude, big.NewInt(1))
		magnitude.Neg(magnitude)
	}
	return magnitude
}

func (s *decodeState) decodeTimeReflect(data []byte, offset int, tagNumber uint64, rv reflect.Value) (int, error) {
	inner, n, err := s.decodeItem(data[offset:])
	if err != nil {
		return 0, err
	}
	switch {
	case tagNumber == 1 && inner.Major == MajorUint:
		if inner.Argument >= 1<<63 {
			return 0, &UnmarshalTypeError{CBORType: "tag", GoType: timeType}
		}
		rv.Set(reflect.ValueOf(time.Unix(int64(inner.Argument), 0).UTC()))
	case tagNumber == 1 && inner.Major == MajorNint:
		if inner.Argument >= 1<<63 {
			return 0, &UnmarshalTypeError{CBORType: "tag", GoType: timeType}
		}
		rv.Set(reflect.ValueOf(time.Unix(-1-int64(inner.Argument), 0).UTC()))
	case tagNumber == 1 && inner.Major == MajorOther && inner.FloatWidth != 0:
		sec, frac := math.Modf(inner.Float)
		rv.Set(reflect.ValueOf(time.Unix(int64(sec), int64(frac*1e9)).UTC()))
	case tagNumber == 0 && inner.Major == MajorText:
		parsed, perr := time.Parse(time.RFC3339, string(inner.Bytes))
		if perr != nil {
			return 0, perr
		}
		rv.Set(reflect.ValueOf(parsed))
	default:
		return 0, &UnmarshalTypeError{CBORType: "tag", GoType: timeType}
	}
	return offset + n, nil
}
