package cbor

import (
	"bytes"
	"errors"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type encodeState struct {
	buf    []byte
	sort   SortMode
	time   TimeMode
	float  FloatMode
	nan    NaNMode
	bigInt BigIntMode
}

func keyCompare(a, b []byte, mode SortMode) int {
	if mode == SortLengthFirst && len(a) != len(b) {
		return len(a) - len(b)
	}
	return bytes.Compare(a, b)
}

var marshalerType = reflect.TypeOf((*Marshaler)(nil)).Elem()

func (e *encodeState) writeHead(major byte, argument uint64) {
	switch {
	case argument < 24:
		e.buf = append(e.buf, major<<5|byte(argument))
	case argument < 1<<8:
		e.buf = append(e.buf, major<<5|24, byte(argument))
	case argument < 1<<16:
		e.buf = append(e.buf, major<<5|25, byte(argument>>8), byte(argument))
	case argument < 1<<32:
		e.buf = append(e.buf, major<<5|26, byte(argument>>24), byte(argument>>16), byte(argument>>8), byte(argument))
	default:
		e.buf = append(e.buf, major<<5|27,
			byte(argument>>56), byte(argument>>48), byte(argument>>40), byte(argument>>32),
			byte(argument>>24), byte(argument>>16), byte(argument>>8), byte(argument))
	}
}

func (e *encodeState) encode(v any) error {
	if v == nil {
		e.buf = append(e.buf, 0xf6)
		return nil
	}
	return e.encodeValue(reflect.ValueOf(v))
}

type encoderFunc func(e *encodeState, rv reflect.Value) error

var encoderCache sync.Map

func (e *encodeState) encodeValue(rv reflect.Value) error {
	if !rv.IsValid() {
		e.buf = append(e.buf, 0xf6)
		return nil
	}
	if m, ok := marshalerOf(rv); ok {
		b, err := m.MarshalCBOR()
		if err != nil {
			return err
		}
		e.buf = append(e.buf, b...)
		return nil
	}
	return typeEncoder(rv.Type())(e, rv)
}

func typeEncoder(t reflect.Type) encoderFunc {
	if f, ok := encoderCache.Load(t); ok {
		return f.(encoderFunc)
	}
	f := newTypeEncoder(t)
	encoderCache.Store(t, f)
	return f
}

func newTypeEncoder(t reflect.Type) encoderFunc {
	switch t {
	case dataItemType:
		return func(e *encodeState, rv reflect.Value) error {
			return e.encodeDataItem(new(rv.Interface().(DataItem)))
		}
	case tagType:
		return func(e *encodeState, rv reflect.Value) error {
			x := rv.Interface().(Tag)
			e.writeHead(6, x.Number)
			return e.encode(x.Content)
		}
	case rawTagType:
		return func(e *encodeState, rv reflect.Value) error {
			x := rv.Interface().(RawTag)
			e.writeHead(6, x.Number)
			if len(x.Content) == 0 {
				e.buf = append(e.buf, 0xf6)
				return nil
			}
			e.buf = append(e.buf, x.Content...)
			return nil
		}
	case simpleType:
		return func(e *encodeState, rv reflect.Value) error {
			return e.encodeSimple(byte(rv.Interface().(SimpleValue)))
		}
	case undefinedType:
		return func(e *encodeState, rv reflect.Value) error {
			e.buf = append(e.buf, 0xf7)
			return nil
		}
	case bigIntType:
		return func(e *encodeState, rv reflect.Value) error {
			e.encodeBigInt(new(rv.Interface().(big.Int)))
			return nil
		}
	case timeType:
		return func(e *encodeState, rv reflect.Value) error {
			return e.encodeTime(rv.Interface().(time.Time))
		}
	case mapType:
		return func(e *encodeState, rv reflect.Value) error {
			return e.encodeCBORMap(rv.Interface().(Map))
		}
	}
	switch t.Kind() {
	case reflect.Bool:
		return boolEncoder
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intEncoder
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintEncoder
	case reflect.Float32, reflect.Float64:
		return floatEncoder
	case reflect.String:
		return stringEncoder
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return byteSliceEncoder
		}
		return sliceEncoder
	case reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return byteArrayEncoder
		}
		return arrayEncoder
	case reflect.Map:
		return mapEncoder
	case reflect.Struct:
		return structEncoder
	case reflect.Pointer, reflect.Interface:
		return indirectEncoder
	default:
		return unsupportedEncoder
	}
}

func boolEncoder(e *encodeState, rv reflect.Value) error {
	if rv.Bool() {
		e.buf = append(e.buf, 0xf5)
	} else {
		e.buf = append(e.buf, 0xf4)
	}
	return nil
}

func intEncoder(e *encodeState, rv reflect.Value) error { e.encodeInt(rv.Int()); return nil }

func uintEncoder(e *encodeState, rv reflect.Value) error { e.writeHead(0, rv.Uint()); return nil }

func floatEncoder(e *encodeState, rv reflect.Value) error { e.encodeFloat(rv.Float()); return nil }

func stringEncoder(e *encodeState, rv reflect.Value) error {
	str := rv.String()
	e.writeHead(3, uint64(len(str)))
	e.buf = append(e.buf, str...)
	return nil
}

func byteSliceEncoder(e *encodeState, rv reflect.Value) error {
	if rv.IsNil() {
		e.buf = append(e.buf, 0xf6)
		return nil
	}
	b := rv.Bytes()
	e.writeHead(2, uint64(len(b)))
	e.buf = append(e.buf, b...)
	return nil
}

func byteArrayEncoder(e *encodeState, rv reflect.Value) error {
	n := rv.Len()
	e.writeHead(2, uint64(n))
	for i := 0; i < n; i++ {
		e.buf = append(e.buf, byte(rv.Index(i).Uint()))
	}
	return nil
}

func sliceEncoder(e *encodeState, rv reflect.Value) error {
	if rv.IsNil() {
		e.buf = append(e.buf, 0xf6)
		return nil
	}
	return e.encodeArray(rv)
}

func arrayEncoder(e *encodeState, rv reflect.Value) error { return e.encodeArray(rv) }

func mapEncoder(e *encodeState, rv reflect.Value) error {
	if rv.IsNil() {
		e.buf = append(e.buf, 0xf6)
		return nil
	}
	return e.encodeMap(rv)
}

func structEncoder(e *encodeState, rv reflect.Value) error { return e.encodeStruct(rv) }

func indirectEncoder(e *encodeState, rv reflect.Value) error {
	if rv.IsNil() {
		e.buf = append(e.buf, 0xf6)
		return nil
	}
	return e.encodeValue(rv.Elem())
}

func unsupportedEncoder(e *encodeState, rv reflect.Value) error {
	return &UnsupportedTypeError{Type: rv.Type()}
}

func marshalerOf(rv reflect.Value) (Marshaler, bool) {
	if rv.Type().Implements(marshalerType) {
		if rv.Kind() == reflect.Pointer && rv.IsNil() {
			return nil, false
		}
		return rv.Interface().(Marshaler), true
	}
	if rv.CanAddr() && rv.Addr().Type().Implements(marshalerType) {
		return rv.Addr().Interface().(Marshaler), true
	}
	return nil, false
}

func (e *encodeState) encodeInt(i int64) {
	if i >= 0 {
		e.writeHead(0, uint64(i))
	} else {
		e.writeHead(1, uint64(-1-i))
	}
}

func (e *encodeState) encodeSimple(v byte) error {
	switch {
	case v < 24:
		e.buf = append(e.buf, 0xe0|v)
	case v >= 32:
		e.buf = append(e.buf, 0xf8, v)
	default:
		return &UnsupportedTypeError{Type: reflect.TypeOf(SimpleValue(0))}
	}
	return nil
}

func (e *encodeState) encodeBigInt(b *big.Int) {
	if b.Sign() >= 0 {
		if e.bigInt == BigIntShortest && b.IsUint64() {
			e.writeHead(0, b.Uint64())
			return
		}
		e.writeHead(6, 2)
		raw := b.Bytes()
		e.writeHead(2, uint64(len(raw)))
		e.buf = append(e.buf, raw...)
		return
	}
	n := new(big.Int).Neg(b)
	n.Sub(n, big.NewInt(1))
	if e.bigInt == BigIntShortest && n.IsUint64() {
		e.writeHead(1, n.Uint64())
		return
	}
	e.writeHead(6, 3)
	raw := n.Bytes()
	e.writeHead(2, uint64(len(raw)))
	e.buf = append(e.buf, raw...)
}

func (e *encodeState) encodeArray(rv reflect.Value) error {
	n := rv.Len()
	e.writeHead(4, uint64(n))
	for i := 0; i < n; i++ {
		if err := e.encodeValue(rv.Index(i)); err != nil {
			return err
		}
	}
	return nil
}

func (e *encodeState) encodeTime(t time.Time) error {
	switch e.time {
	case TimeRFC3339:
		e.writeHead(6, 0)
		s := t.Format(time.RFC3339Nano)
		e.writeHead(3, uint64(len(s)))
		e.buf = append(e.buf, s...)
	case TimeNumericDate:
		e.encodeEpoch(t)
	default:
		e.writeHead(6, 1)
		e.encodeEpoch(t)
	}
	return nil
}

func (e *encodeState) encodeEpoch(t time.Time) {
	if t.Nanosecond() == 0 {
		e.encodeInt(t.Unix())
	} else {
		e.encodeFloat(float64(t.UnixNano()) / 1e9)
	}
}

func (e *encodeState) encodeCBORMap(m Map) error {
	type entry struct {
		start, end int
		val        any
	}
	entries := make([]entry, 0, len(m))
	ke := encodeState{sort: e.sort, time: e.time, float: e.float, nan: e.nan, bigInt: e.bigInt}
	for _, kv := range m {
		start := len(ke.buf)
		if err := ke.encode(kv.Key); err != nil {
			return err
		}
		entries = append(entries, entry{start, len(ke.buf), kv.Value})
	}
	if e.sort != SortNone {
		slices.SortFunc(entries, func(a, b entry) int {
			return keyCompare(ke.buf[a.start:a.end], ke.buf[b.start:b.end], e.sort)
		})
		for i := 1; i < len(entries); i++ {
			if bytes.Equal(ke.buf[entries[i-1].start:entries[i-1].end], ke.buf[entries[i].start:entries[i].end]) {
				return errors.New("cbor: duplicate key in map")
			}
		}
	}
	e.writeHead(5, uint64(len(entries)))
	for _, en := range entries {
		e.buf = append(e.buf, ke.buf[en.start:en.end]...)
		if err := e.encode(en.val); err != nil {
			return err
		}
	}
	return nil
}

func (e *encodeState) encodeMap(rv reflect.Value) error {
	type entry struct {
		start, end int
		val        reflect.Value
	}
	entries := make([]entry, 0, rv.Len())
	ke := encodeState{sort: e.sort, time: e.time, float: e.float, nan: e.nan, bigInt: e.bigInt}
	iter := rv.MapRange()
	for iter.Next() {
		start := len(ke.buf)
		if err := ke.encodeValue(iter.Key()); err != nil {
			return err
		}
		entries = append(entries, entry{start, len(ke.buf), iter.Value()})
	}
	if e.sort != SortNone {
		slices.SortFunc(entries, func(a, b entry) int {
			return keyCompare(ke.buf[a.start:a.end], ke.buf[b.start:b.end], e.sort)
		})
		for i := 1; i < len(entries); i++ {
			if bytes.Equal(ke.buf[entries[i-1].start:entries[i-1].end], ke.buf[entries[i].start:entries[i].end]) {
				return errors.New("cbor: duplicate key in map")
			}
		}
	}
	e.writeHead(5, uint64(len(entries)))
	for _, en := range entries {
		e.buf = append(e.buf, ke.buf[en.start:en.end]...)
		if err := e.encodeValue(en.val); err != nil {
			return err
		}
	}
	return nil
}

func (e *encodeState) encodeDataItem(d *DataItem) error {
	switch d.Major {
	case MajorUint:
		e.writeHead(0, d.Argument)
	case MajorNint:
		e.writeHead(1, d.Argument)
	case MajorBytes:
		e.writeHead(2, uint64(len(d.Bytes)))
		e.buf = append(e.buf, d.Bytes...)
	case MajorText:
		e.writeHead(3, uint64(len(d.Bytes)))
		e.buf = append(e.buf, d.Bytes...)
	case MajorArray:
		e.writeHead(4, uint64(len(d.Content)))
		for _, c := range d.Content {
			if err := e.encodeDataItem(c); err != nil {
				return err
			}
		}
	case MajorMap:
		if len(d.Content)%2 != 0 {
			return errors.New("cbor: map data item has an odd number of items")
		}
		type entry struct {
			key []byte
			val []byte
		}
		pairs := make([]entry, 0, len(d.Content)/2)
		seen := make(map[string]struct{}, len(d.Content)/2)
		for i := 0; i+1 < len(d.Content); i += 2 {
			ke := &encodeState{sort: e.sort, time: e.time, float: e.float, nan: e.nan, bigInt: e.bigInt}
			if err := ke.encodeDataItem(d.Content[i]); err != nil {
				return err
			}
			if _, dup := seen[string(ke.buf)]; dup {
				return errors.New("cbor: duplicate key in map data item")
			}
			seen[string(ke.buf)] = struct{}{}
			ve := &encodeState{sort: e.sort, time: e.time, float: e.float, nan: e.nan, bigInt: e.bigInt}
			if err := ve.encodeDataItem(d.Content[i+1]); err != nil {
				return err
			}
			pairs = append(pairs, entry{ke.buf, ve.buf})
		}
		if e.sort != SortNone {
			slices.SortFunc(pairs, func(a, b entry) int {
				return keyCompare(a.key, b.key, e.sort)
			})
		}
		e.writeHead(5, uint64(len(pairs)))
		for _, p := range pairs {
			e.buf = append(e.buf, p.key...)
			e.buf = append(e.buf, p.val...)
		}
	case MajorTag:
		e.writeHead(6, d.Argument)
		if len(d.Content) > 0 {
			return e.encodeDataItem(d.Content[0])
		}
	case MajorOther:
		if d.FloatWidth != 0 {
			e.encodeFloatWidth(d.Float, d.FloatWidth)
		} else {
			return e.encodeSimple(byte(d.Argument))
		}
	}
	return nil
}

func (e *encodeState) encodeFloat(f float64) {
	if math.IsNaN(f) && e.nan == NaN7e00 {
		e.buf = append(e.buf, 0xf9, 0x7e, 0x00)
		return
	}
	if e.float == FloatShortest {
		if hb, ok := tryFloat16(f); ok {
			e.buf = append(e.buf, 0xf9, byte(hb>>8), byte(hb))
			return
		}
		if f32 := float32(f); float64(f32) == f {
			e.appendSingle(f32)
			return
		}
	}
	e.appendDouble(f)
}

func (e *encodeState) encodeFloatWidth(f float64, width uint8) {
	if math.IsNaN(f) {

		switch width {
		case 2:
			e.buf = append(e.buf, 0xf9, 0x7e, 0x00)
		case 4:
			e.buf = append(e.buf, 0xfa, 0x7f, 0xc0, 0x00, 0x00)
		default:
			e.buf = append(e.buf, 0xfb, 0x7f, 0xf8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
		}
		return
	}
	switch width {
	case 2:
		hb := f32ToF16bits(math.Float32bits(float32(f)))
		e.buf = append(e.buf, 0xf9, byte(hb>>8), byte(hb))
	case 4:
		e.appendSingle(float32(f))
	default:
		e.appendDouble(f)
	}
}

func (e *encodeState) appendSingle(f float32) {
	b := math.Float32bits(f)
	e.buf = append(e.buf, 0xfa, byte(b>>24), byte(b>>16), byte(b>>8), byte(b))
}

func (e *encodeState) appendDouble(f float64) {
	b := math.Float64bits(f)
	e.buf = append(e.buf, 0xfb,
		byte(b>>56), byte(b>>48), byte(b>>40), byte(b>>32),
		byte(b>>24), byte(b>>16), byte(b>>8), byte(b))
}

func tryFloat16(f float64) (uint16, bool) {
	f32 := float32(f)
	if float64(f32) != f {
		return 0, false
	}
	hb := f32ToF16bits(math.Float32bits(f32))
	if float64(math.Float32frombits(halfBitsToSingle(hb))) == f {
		return hb, true
	}
	return 0, false
}

func f32ToF16bits(fb uint32) uint16 {
	sign := uint16((fb >> 16) & 0x8000)
	exponent := int32((fb >> 23) & 0xff)
	mantissa := fb & 0x7fffff
	if exponent == 0xff {
		if mantissa == 0 {
			return sign | 0x7c00
		}
		return sign | 0x7e00
	}
	if exponent == 0 && mantissa == 0 {
		return sign
	}
	e := exponent - 127 + 15
	if e >= 1 && e <= 30 {
		return sign | uint16(e)<<10 | uint16(mantissa>>13)
	}
	if e >= -9 && e <= 0 {
		m := mantissa | 0x800000
		shift := uint32(14 - e)
		if shift < 32 {
			return sign | uint16(m>>shift)
		}
	}
	return sign | 0x7fff
}

type structField struct {
	index     []int
	key       []byte
	omitEmpty bool
	omitZero  bool
}

type structPlan struct {
	toArray bool
	fields  []structField
	byKey   map[string]structField
}

var structPlanCache sync.Map

func (e *encodeState) encodeStruct(rv reflect.Value) error {
	plan := planFor(rv.Type())
	if plan.toArray {
		e.writeHead(4, uint64(len(plan.fields)))
		for _, f := range plan.fields {
			if err := e.encodeValue(rv.FieldByIndex(f.index)); err != nil {
				return err
			}
		}
		return nil
	}

	type activeField struct {
		key []byte
		val reflect.Value
	}
	active := make([]activeField, 0, len(plan.fields))
	for _, f := range plan.fields {
		fv, ok := fieldByIndexRead(rv, f.index)
		if !ok {
			continue
		}
		if (f.omitEmpty && isEmptyValue(fv)) || (f.omitZero && fv.IsZero()) {
			continue
		}
		active = append(active, activeField{f.key, fv})
	}
	if e.sort != SortNone {
		slices.SortFunc(active, func(a, b activeField) int {
			return keyCompare(a.key, b.key, e.sort)
		})
	}
	e.writeHead(5, uint64(len(active)))
	for _, f := range active {
		e.buf = append(e.buf, f.key...)
		if err := e.encodeValue(f.val); err != nil {
			return err
		}
	}
	return nil
}

type fieldCandidate struct {
	field  structField
	depth  int
	tagged bool
}

func planFor(t reflect.Type) *structPlan {
	if p, ok := structPlanCache.Load(t); ok {
		return p.(*structPlan)
	}
	p := &structPlan{}
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Name == "_" {
			_, opts := parseTag(t.Field(i).Tag.Get("cbor"))
			if hasOption(opts, "toarray") {
				p.toArray = true
			}
		}
	}
	if p.toArray {
		p.fields = topLevelFields(t)
	} else {
		var candidates []fieldCandidate
		collectFields(t, nil, 0, map[reflect.Type]bool{t: true}, &candidates)
		p.fields = resolveFields(candidates)
	}
	p.byKey = make(map[string]structField, len(p.fields))
	for _, f := range p.fields {
		p.byKey[string(f.key)] = f
	}
	structPlanCache.Store(t, p)
	return p
}

func makeField(index []int, name string, opts []string) structField {
	f := structField{
		index:     append([]int(nil), index...),
		omitEmpty: hasOption(opts, "omitempty"),
		omitZero:  hasOption(opts, "omitzero"),
	}
	if hasOption(opts, "asint") {
		if n, err := strconv.ParseInt(name, 10, 64); err == nil {
			ke := &encodeState{}
			ke.encodeInt(n)
			f.key = ke.buf
			return f
		}
	}
	f.key = encodeTextKey(name)
	return f
}

func topLevelFields(t reflect.Type) []structField {
	var fields []structField
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.Name == "_" || sf.PkgPath != "" {
			continue
		}
		name, opts := parseTag(sf.Tag.Get("cbor"))
		if name == "-" {
			continue
		}
		if name == "" {
			name = sf.Name
		}
		fields = append(fields, makeField(sf.Index, name, opts))
	}
	return fields
}

func collectFields(t reflect.Type, prefix []int, depth int, visited map[reflect.Type]bool, out *[]fieldCandidate) {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.Name == "_" {
			continue
		}
		name, opts := parseTag(sf.Tag.Get("cbor"))
		if name == "-" {
			continue
		}
		index := append(append([]int(nil), prefix...), i)
		if sf.Anonymous && name == "" {
			ft := sf.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				if visited[ft] {
					continue
				}
				nv := make(map[reflect.Type]bool, len(visited)+1)
				for k, v := range visited {
					nv[k] = v
				}
				nv[ft] = true
				collectFields(ft, index, depth+1, nv, out)
				continue
			}
		}
		if sf.PkgPath != "" {
			continue
		}
		fieldName := name
		if fieldName == "" {
			fieldName = sf.Name
		}
		*out = append(*out, fieldCandidate{
			field:  makeField(index, fieldName, opts),
			depth:  depth,
			tagged: name != "",
		})
	}
}

func resolveFields(candidates []fieldCandidate) []structField {
	byKey := make(map[string][]fieldCandidate)
	var order []string
	for _, c := range candidates {
		k := string(c.field.key)
		if _, seen := byKey[k]; !seen {
			order = append(order, k)
		}
		byKey[k] = append(byKey[k], c)
	}
	var fields []structField
	for _, k := range order {
		group := byKey[k]
		if len(group) == 1 {
			fields = append(fields, group[0].field)
			continue
		}
		if winner, ok := dominantField(group); ok {
			fields = append(fields, winner.field)
		}
	}
	return fields
}

func dominantField(group []fieldCandidate) (fieldCandidate, bool) {
	best := group[0]
	tie := false
	for _, c := range group[1:] {
		switch {
		case c.depth < best.depth:
			best, tie = c, false
		case c.depth > best.depth:
		case c.tagged && !best.tagged:
			best, tie = c, false
		case best.tagged && !c.tagged:
		default:
			tie = true
		}
	}
	if tie {
		return fieldCandidate{}, false
	}
	return best, true
}

func fieldByIndexRead(v reflect.Value, index []int) (reflect.Value, bool) {
	for k, i := range index {
		if k > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v, true
}

func fieldByIndexAlloc(v reflect.Value, index []int) reflect.Value {
	for k, i := range index {
		if k > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}

func encodeTextKey(s string) []byte {
	e := &encodeState{}
	e.writeHead(3, uint64(len(s)))
	e.buf = append(e.buf, s...)
	return e.buf
}

func parseTag(tag string) (name string, options []string) {
	parts := strings.Split(tag, ",")
	return parts[0], parts[1:]
}

func hasOption(options []string, want string) bool {
	for _, o := range options {
		if o == want {
			return true
		}
	}
	return false
}

func isEmptyValue(rv reflect.Value) bool {
	switch rv.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len() == 0
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return rv.IsNil()
	}
	return false
}
