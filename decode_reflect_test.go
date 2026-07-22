package cbor_test

import (
	"bytes"
	"errors"
	"math/big"
	"reflect"
	"testing"
	"time"

	cbor "github.com/MichaelFraser99/go-cbor"
)

func TestUnmarshalScalars(t *testing.T) {
	var i int
	mustUnmarshal(t, "0x1903e8", &i)
	if i != 1000 {
		t.Errorf("int = %d, want 1000", i)
	}
	var neg int
	mustUnmarshal(t, "0x3863", &neg)
	if neg != -100 {
		t.Errorf("int = %d, want -100", neg)
	}
	var u uint16
	mustUnmarshal(t, "0x1903e8", &u)
	if u != 1000 {
		t.Errorf("uint16 = %d, want 1000", u)
	}
	var f float64
	mustUnmarshal(t, "0xfb3ff199999999999a", &f)
	if f != 1.1 {
		t.Errorf("float = %v, want 1.1", f)
	}
	var s string
	mustUnmarshal(t, "0x6449455446", &s)
	if s != "IETF" {
		t.Errorf("string = %q, want IETF", s)
	}
	var b []byte
	mustUnmarshal(t, "0x4401020304", &b)
	if !bytes.Equal(b, []byte{1, 2, 3, 4}) {
		t.Errorf("bytes = %x, want 01020304", b)
	}
	var tf bool
	mustUnmarshal(t, "0xf5", &tf)
	if !tf {
		t.Errorf("bool = %v, want true", tf)
	}
}

func TestUnmarshalSliceAndMap(t *testing.T) {
	var xs []int
	mustUnmarshal(t, "0x83010203", &xs)
	if !reflect.DeepEqual(xs, []int{1, 2, 3}) {
		t.Errorf("slice = %v, want [1 2 3]", xs)
	}
	var m map[int]int
	mustUnmarshal(t, "0xa201020304", &m)
	if !reflect.DeepEqual(m, map[int]int{1: 2, 3: 4}) {
		t.Errorf("map = %v, want map[1:2 3:4]", m)
	}
}

func TestUnmarshalStruct(t *testing.T) {
	type Header struct {
		Alg int    `cbor:"1,asint"`
		Kid []byte `cbor:"4,asint,omitempty"`
	}
	var h Header
	mustUnmarshal(t, "0xa2012604420102", &h)
	if h.Alg != -7 {
		t.Errorf("Alg = %d, want -7", h.Alg)
	}
	if !bytes.Equal(h.Kid, []byte{1, 2}) {
		t.Errorf("Kid = %x, want 0102", h.Kid)
	}
}

func TestUnmarshalToArrayStruct(t *testing.T) {
	type Pair struct {
		_ struct{} `cbor:",toarray"`
		A []byte
		B []byte
	}
	var p Pair
	mustUnmarshal(t, "0x8241014102", &p)
	if !bytes.Equal(p.A, []byte{1}) || !bytes.Equal(p.B, []byte{2}) {
		t.Errorf("got A=%x B=%x, want 01 / 02", p.A, p.B)
	}
}

func TestUnmarshalBigInt(t *testing.T) {
	var b big.Int
	mustUnmarshal(t, "0xc249010000000000000000", &b)
	if b.String() != "18446744073709551616" {
		t.Errorf("bigint = %s, want 2^64", b.String())
	}
}

func TestUnmarshalNullIntoPointer(t *testing.T) {
	p := new(int)
	mustUnmarshal(t, "0xf6", &p)
	if p != nil {
		t.Errorf("pointer = %v, want nil", p)
	}
}

type celsius float64

func (c *celsius) UnmarshalCBOR(data []byte) error {
	var f float64
	if err := cbor.Unmarshal(data, &f); err != nil {
		return err
	}
	*c = celsius(f)
	return nil
}

func TestUnmarshalerInterface(t *testing.T) {
	var c celsius
	mustUnmarshal(t, "0xfb3ff199999999999a", &c)
	if c != celsius(1.1) {
		t.Errorf("celsius = %v, want 1.1", c)
	}
}

func TestUnmarshalerNestedAndDoublePointer(t *testing.T) {
	var m map[string]celsius
	mustUnmarshal(t, "0xa16161fb3ff199999999999a", &m) // {"a": 1.1} -> map value Unmarshaler
	if m["a"] != celsius(1.1) {
		t.Errorf("map celsius = %v", m["a"])
	}
	var sl []celsius
	mustUnmarshal(t, "0x81fb3ff199999999999a", &sl) // slice element Unmarshaler
	if len(sl) != 1 || sl[0] != celsius(1.1) {
		t.Errorf("slice celsius = %v", sl)
	}
	var pp **int
	mustUnmarshal(t, "0x182a", &pp) // double pointer allocates through both levels
	if pp == nil || *pp == nil || **pp != 42 {
		t.Errorf("double pointer = %v", pp)
	}
}

func TestUnmarshalInvalidTarget(t *testing.T) {
	var i int
	if err := cbor.Unmarshal([]byte{0x00}, i); err == nil {
		t.Error("Unmarshal into non-pointer: want error")
	}
	if err := cbor.Unmarshal([]byte{0x00}, nil); err == nil {
		t.Error("Unmarshal(nil): want error")
	}
}

func TestRoundTripStructs(t *testing.T) {
	type Inner struct {
		Name  string `cbor:"name"`
		Count int    `cbor:"count"`
	}
	type Outer struct {
		ID    uint64  `cbor:"id"`
		Items []Inner `cbor:"items"`
		Ratio float64 `cbor:"ratio"`
	}
	in := Outer{
		ID:    42,
		Items: []Inner{{"a", 1}, {"b", 2}},
		Ratio: 1.5,
	}
	data, err := cbor.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Outer
	if err := cbor.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\n in = %+v\nout = %+v", in, out)
	}
}

func mustUnmarshal(t *testing.T, hexStr string, v any) {
	t.Helper()
	if err := cbor.Unmarshal(mustHex(t, hexStr), v); err != nil {
		t.Fatalf("Unmarshal(%s): %v", hexStr, err)
	}
}

func TestIndefiniteIntoGoTypes(t *testing.T) {
	var xs []int
	if err := cbor.Unmarshal(mustHex(t, "0x9f010203ff"), &xs); err != nil {
		t.Fatal(err)
	}
	if len(xs) != 3 || xs[0] != 1 || xs[2] != 3 {
		t.Errorf("slice = %v, want [1 2 3]", xs)
	}

	var b []byte
	if err := cbor.Unmarshal(mustHex(t, "0x5f42010243030405ff"), &b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte{1, 2, 3, 4, 5}) {
		t.Errorf("bytes = %x, want 0102030405", b)
	}
}

func TestUnmarshalIntegerOverflow(t *testing.T) {
	var i8 int8
	if err := cbor.Unmarshal(mustHex(t, "0x1903e8"), &i8); err == nil {
		t.Errorf("1000 into int8: err = nil (value %d), want overflow error", i8)
	}
	var u8 uint8
	if err := cbor.Unmarshal(mustHex(t, "0x1903e8"), &u8); err == nil {
		t.Errorf("1000 into uint8: err = nil (value %d), want overflow error", u8)
	}
	var i8neg int8
	if err := cbor.Unmarshal(mustHex(t, "0x3903e7"), &i8neg); err == nil {
		t.Errorf("-1000 into int8: err = nil (value %d), want overflow error", i8neg)
	}
	var u16 uint16
	if err := cbor.Unmarshal(mustHex(t, "0x190190"), &u16); err != nil {
		t.Errorf("400 into uint16: err = %v, want nil", err)
	} else if u16 != 400 {
		t.Errorf("400 into uint16 = %d", u16)
	}
}

func TestTimeNegativeEpochRoundTrip(t *testing.T) {
	orig := time.Date(1969, 1, 1, 0, 0, 0, 0, time.UTC)
	b, err := cbor.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got time.Time
	if err := cbor.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal of self-encoded pre-1970 time (%x) failed: %v", b, err)
	}
	if !got.Equal(orig) {
		t.Errorf("round trip = %v, want %v", got, orig)
	}
}

func TestUnmarshalNullIntoNilables(t *testing.T) {
	n := 5
	p := &n
	mustUnmarshal(t, "0xf6", &p)
	if p != nil {
		t.Error("null into *int: want nil")
	}
	s := []int{1}
	mustUnmarshal(t, "0xf6", &s)
	if s != nil {
		t.Error("null into slice: want nil")
	}
	m := map[string]int{"a": 1}
	mustUnmarshal(t, "0xf6", &m)
	if m != nil {
		t.Error("null into map: want nil")
	}
	var v any = 5
	mustUnmarshal(t, "0xf6", &v)
	if v != nil {
		t.Errorf("null into any: want nil, got %v", v)
	}
}

func TestUnmarshalSimpleValues(t *testing.T) {
	var b bool
	mustUnmarshal(t, "0xf4", &b)
	if b {
		t.Error("0xf4 -> false")
	}
	if err := cbor.Unmarshal(mustHex(t, "0xf5"), new(int)); err == nil {
		t.Error("bool into int: want error")
	}
	var s cbor.SimpleValue
	mustUnmarshal(t, "0xf8ff", &s)
	if s != 255 {
		t.Errorf("simple(255) = %d", byte(s))
	}
	if err := cbor.Unmarshal(mustHex(t, "0xf8ff"), new(int)); err == nil {
		t.Error("simple into int: want error")
	}
}

func TestUnmarshalIntegerConversions(t *testing.T) {
	var f float64
	mustUnmarshal(t, "0x01", &f) // int into float
	if f != 1 {
		t.Errorf("int->float = %v", f)
	}
	mustUnmarshal(t, "0x20", &f) // -1 into float
	if f != -1 {
		t.Errorf("nint->float = %v", f)
	}
	var u uint
	if err := cbor.Unmarshal(mustHex(t, "0x20"), &u); err == nil {
		t.Error("negative into uint: want error")
	}
	var i64 int64
	if err := cbor.Unmarshal(mustHex(t, "0x1bffffffffffffffff"), &i64); err == nil {
		t.Error("uint64-max into int64: want error")
	}
	var bi big.Int
	mustUnmarshal(t, "0x1903e8", &bi) // plain uint into *big.Int
	if bi.String() != "1000" {
		t.Errorf("uint->bigint = %s", bi.String())
	}
	mustUnmarshal(t, "0x3903e7", &bi) // nint into *big.Int
	if bi.String() != "-1000" {
		t.Errorf("nint->bigint = %s", bi.String())
	}
}

func TestUnmarshalBignumTagIntoBigInt(t *testing.T) {
	var b big.Int
	mustUnmarshal(t, "0xc249010000000000000000", &b) // tag 2
	if b.String() != "18446744073709551616" {
		t.Errorf("tag2->bigint = %s", b.String())
	}
	mustUnmarshal(t, "0xc349010000000000000000", &b) // tag 3
	if b.String() != "-18446744073709551617" {
		t.Errorf("tag3->bigint = %s", b.String())
	}
}

func TestUnmarshalIntoGoArray(t *testing.T) {
	var a [3]int
	mustUnmarshal(t, "0x83010203", &a)
	if a != [3]int{1, 2, 3} {
		t.Errorf("array = %v", a)
	}
	var short [2]int
	mustUnmarshal(t, "0x83010203", &short) // 3 elems into [2] -> extras skipped
	if short != [2]int{1, 2} {
		t.Errorf("truncated array = %v", short)
	}
	var ba [4]byte
	mustUnmarshal(t, "0x4401020304", &ba)
	if ba != [4]byte{1, 2, 3, 4} {
		t.Errorf("byte array = %v", ba)
	}
	if err := cbor.Unmarshal(mustHex(t, "0x4401020304"), new(int)); err == nil {
		t.Error("byte string into int: want error")
	}
}

func TestUnmarshalToArrayExtraElements(t *testing.T) {
	type Pair struct {
		_ struct{} `cbor:",toarray"`
		A int
		B int
	}
	var p Pair
	mustUnmarshal(t, "0x83010203", &p) // 3 elems into 2-field toarray -> extra skipped
	if p.A != 1 || p.B != 2 {
		t.Errorf("toarray extra = %+v", p)
	}
}

func TestUnmarshalMapUnknownKeysSkipped(t *testing.T) {
	type S struct {
		A int `cbor:"a"`
	}
	var s S
	mustUnmarshal(t, "0xa2616101616202", &s) // {"a":1,"b":2} -> b unknown, skipped
	if s.A != 1 {
		t.Errorf("A = %d, want 1", s.A)
	}
	if err := cbor.Unmarshal(mustHex(t, "0xa10102"), new(int)); err == nil {
		t.Error("map into int: want error")
	}
}

func TestUnmarshalTagVariants(t *testing.T) {
	var tag cbor.Tag
	mustUnmarshal(t, "0xc074323031332d30332d32315432303a30343a30305a", &tag) // tag 0 text
	if tag.Number != 0 {
		t.Errorf("tag number = %d", tag.Number)
	}
	// tag wrapping a value, decoded into a concrete type (tag unwrapped)
	var n int
	mustUnmarshal(t, "0xc0182a", &n) // tag 0 wrapping 42 -> unwrapped into int
	if n != 42 {
		t.Errorf("unwrapped tag = %d", n)
	}
}

func TestUnmarshalTimeVariants(t *testing.T) {
	var tm time.Time
	mustUnmarshal(t, "0xc074323031332d30332d32315432303a30343a30305a", &tm) // tag 0 RFC3339
	if tm.IsZero() {
		t.Error("tag0 time not parsed")
	}
	mustUnmarshal(t, "0xc11a514b67b0", &tm) // tag 1 uint epoch
	if tm.Unix() != 1363896240 {
		t.Errorf("tag1 uint = %d", tm.Unix())
	}
	mustUnmarshal(t, "0xc13a01e1337f", &tm) // tag 1 negative epoch
	if tm.Unix() != -31536000 {
		t.Errorf("tag1 nint = %d", tm.Unix())
	}
	if err := cbor.Unmarshal(mustHex(t, "0xc16161"), &tm); err == nil {
		t.Error("tag1 text into time: want error")
	}
	if err := cbor.Unmarshal(mustHex(t, "0xc13bffffffffffffffff"), &tm); err == nil { // epoch below int64 min
		t.Error("tag1 out-of-range negative epoch: want error")
	}
}

func TestStrictNonShortestIntegers(t *testing.T) {
	strict, _ := cbor.DecoderOptions{Strict: true}.Decoding()
	for _, in := range []string{"0x1817", "0x190017", "0x1a00000017", "0x1b0000000000000017", "0x3817"} {
		if err := strict.Valid(mustHex(t, in)); err == nil {
			t.Errorf("strict accepted non-shortest int %s", in)
		}
	}
	for _, in := range []string{"0x17", "0x1818", "0x190100", "0x1a00010000", "0x1b0000000100000000"} {
		if err := strict.Valid(mustHex(t, in)); err != nil {
			t.Errorf("strict rejected shortest int %s: %v", in, err)
		}
	}
}

func TestStrictFloatWidths(t *testing.T) {
	strict, _ := cbor.DecoderOptions{Strict: true}.Decoding()
	for _, in := range []string{
		"0xf93c00",             // canonical half 1.0
		"0xfa47c35000",         // canonical single 100000.0
		"0xfb3ff199999999999a", // canonical double 1.1
		"0xf97e00",             // NaN (canonical half)
		"0xf97c00",             // +Inf
	} {
		if err := strict.Valid(mustHex(t, in)); err != nil {
			t.Errorf("strict rejected canonical float %s: %v", in, err)
		}
	}
	if err := strict.Valid(mustHex(t, "0xfa3f800000")); err == nil { // 1.0 as single, non-shortest
		t.Error("strict accepted non-shortest single float")
	}
}

func TestDecodeMalformedIntoConcrete(t *testing.T) {
	type ToArr struct {
		_ struct{} `cbor:",toarray"`
		A int
		B int
	}
	type S struct {
		A int `cbor:"a"`
	}
	cases := []struct {
		hex string
		v   any
	}{
		{"0x8201", new(ToArr)},            // toarray declares 2 elems, only 1 present
		{"0x82011c", new(ToArr)},          // second toarray element is reserved AI
		{"0x821c02", new([2]int)},         // reserved AI element in Go array
		{"0x83010203", new([2]int)},       // more elements than the Go array (skip path)
		{"0xa161611c", new(S)},            // {"a": <reserved value>}
		{"0xa11c01", new(S)},              // {<reserved key>: 1}
		{"0xa161611c", &map[string]int{}}, // bad value into map
		{"0xa11c01", &map[string]int{}},   // bad key into map
	}
	for _, tc := range cases {
		if err := cbor.Unmarshal(mustHex(t, tc.hex), tc.v); err == nil {
			// the "more elements than Go array" case is valid (extras skipped); others error
			if tc.hex != "0x83010203" {
				t.Errorf("Unmarshal(%s): want error", tc.hex)
			}
		}
	}
}

func TestDecodeIntoWrongSpecialType(t *testing.T) {
	var m cbor.Map
	if err := cbor.Unmarshal(mustHex(t, "0x01"), &m); err == nil {
		t.Error("uint into cbor.Map: want error")
	}
	var rt cbor.RawTag
	if err := cbor.Unmarshal(mustHex(t, "0x01"), &rt); err == nil {
		t.Error("uint into RawTag: want error")
	}
}

func TestDecodeMapIntoStructStrictAndDup(t *testing.T) {
	type S struct {
		A int `cbor:"a"`
		B int `cbor:"b"`
	}
	var s S
	dup, _ := cbor.DecoderOptions{DuplicateKeys: cbor.DupError}.Decoding()
	if err := dup.Unmarshal(mustHex(t, "0xa2616101616102"), &s); err == nil { // {"a":1,"a":2}
		t.Error("duplicate key into struct: want error")
	}
	strict, _ := cbor.DecoderOptions{Strict: true}.Decoding()
	if err := strict.Unmarshal(mustHex(t, "0xa2616201616102"), &s); err == nil { // {"b":1,"a":2} unsorted
		t.Error("unsorted keys into struct: want error")
	}
}

func TestDecodeMapKVStrictAndDup(t *testing.T) {
	var m map[int]int
	dup, _ := cbor.DecoderOptions{DuplicateKeys: cbor.DupError}.Decoding()
	if err := dup.Unmarshal(mustHex(t, "0xa201010102"), &m); err == nil { // {1:1,1:2}
		t.Error("duplicate key into map: want error")
	}
	strict, _ := cbor.DecoderOptions{Strict: true}.Decoding()
	if err := strict.Unmarshal(mustHex(t, "0xa202010101"), &m); err == nil { // {2:1,1:1} unsorted
		t.Error("unsorted keys into map: want error")
	}
}

func TestUnmarshalNullIntoNonNilable(t *testing.T) {
	i := 7
	mustUnmarshal(t, "0xf6", &i) // null into int: leaves it unchanged
	if i != 7 {
		t.Errorf("null into int changed value to %d", i)
	}
}

func TestUnmarshalIndefiniteTextIntoString(t *testing.T) {
	var s string
	mustUnmarshal(t, "0x7f6261626163ff", &s) // indefinite text "ab"+"c"
	if s != "abc" {
		t.Errorf("indefinite text = %q, want abc", s)
	}
}

func TestUnmarshalTwoByteSimple(t *testing.T) {
	var sv cbor.SimpleValue
	mustUnmarshal(t, "0xf820", &sv) // simple 32 (valid two-byte form)
	if sv != 32 {
		t.Errorf("simple(32) = %d", byte(sv))
	}
	if err := cbor.Unmarshal(mustHex(t, "0xf817"), &sv); err == nil { // two-byte simple < 32 is invalid
		t.Error("two-byte simple < 32: want error")
	}
}

func TestUnmarshalReservedAndWideArgs(t *testing.T) {
	for _, in := range []string{"0x1c", "0x1d", "0x1e", "0x3c", "0x5c"} { // reserved additional info 28-30
		var v any
		if err := cbor.Unmarshal(mustHex(t, in), &v); err == nil {
			t.Errorf("reserved AI %s: want error", in)
		}
	}
	var u uint64
	mustUnmarshal(t, "0x1b0000000000000001", &u) // 8-byte argument
	if u != 1 {
		t.Errorf("8-byte arg = %d", u)
	}
	// indefinite length (ai 31) is invalid for uint, nint, and tag
	for _, in := range []string{"0x1f", "0x3f", "0xdf"} {
		var v any
		if err := cbor.Unmarshal(mustHex(t, in), &v); err == nil {
			t.Errorf("ai 31 for %s: want error", in)
		}
	}
}

func TestStrictFloatRejection(t *testing.T) {
	strict, _ := cbor.DecoderOptions{Strict: true}.Decoding()
	// 1.0 encoded as double (non-shortest; canonical is float16)
	if err := strict.Valid(mustHex(t, "0xfb3ff0000000000000")); err == nil {
		t.Error("strict accepted non-shortest float, want error")
	}
	if err := strict.Valid(mustHex(t, "0xf93c00")); err != nil { // canonical 1.0 half
		t.Errorf("strict rejected shortest float: %v", err)
	}
}

func TestTimeNumericDate(t *testing.T) {
	tm := time.Unix(1444064944, 0).UTC()
	em, err := cbor.EncoderOptions{Time: cbor.TimeNumericDate}.Encoding()
	if err != nil {
		t.Fatal(err)
	}
	b, err := em.Marshal(tm)
	if err != nil {
		t.Fatal(err)
	}
	if hexOf(t, b) != "0x1a5612aeb0" { // bare integer, no tag
		t.Errorf("TimeNumericDate = %s, want bare int 0x1a5612aeb0", hexOf(t, b))
	}

	var back time.Time
	if err := cbor.Unmarshal(b, &back); err != nil {
		t.Fatalf("Unmarshal bare numeric into time.Time: %v", err)
	}
	if !back.Equal(tm) {
		t.Errorf("round trip = %v, want %v", back, tm)
	}
}

func TestUnmarshalBareNumericIntoTime(t *testing.T) {
	// A bare integer, negative integer, and float should each decode into time.Time
	// (RFC 8392 / RFC 7519 NumericDate is untagged).
	cases := []struct {
		hex  string
		want time.Time
	}{
		{"0x1a5612aeb0", time.Unix(1444064944, 0).UTC()},
		{"0x3a0012d686", time.Unix(-1234567, 0).UTC()},
		{"0xf93e00", time.Unix(1, 500000000).UTC()}, // float16 1.5
	}
	for _, tc := range cases {
		var got time.Time
		if err := cbor.Unmarshal(mustHex(t, tc.hex), &got); err != nil {
			t.Errorf("Unmarshal(%s) into time.Time: %v", tc.hex, err)
			continue
		}
		if !got.Equal(tc.want) {
			t.Errorf("Unmarshal(%s) = %v, want %v", tc.hex, got, tc.want)
		}
	}
}

func TestCWTBareNumericDateDecodes(t *testing.T) {
	// RFC 8392-style claims: integer keys, bare-numeric time claims.
	type Claims struct {
		Iss string    `cbor:"1,asint"`
		Exp time.Time `cbor:"4,asint"`
	}
	// {1: "x", 4: 1444064944}
	var c Claims
	if err := cbor.Unmarshal(mustHex(t, "0xa2016178041a5612aeb0"), &c); err != nil {
		t.Fatalf("Unmarshal RFC-style claims: %v", err)
	}
	if c.Iss != "x" || !c.Exp.Equal(time.Unix(1444064944, 0).UTC()) {
		t.Errorf("claims = %+v", c)
	}
}

func TestUnmarshalIntoCborMap(t *testing.T) {
	var m cbor.Map
	if err := cbor.Unmarshal(mustHex(t, "0xa201020304"), &m); err != nil {
		t.Fatalf("Unmarshal into cbor.Map: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("len(m) = %d, want 2", len(m))
	}
	if v, ok := m.Get(int64(1)); !ok || v != int64(2) {
		t.Errorf("m.Get(1) = %v, %v; want 2, true", v, ok)
	}
	if v, ok := m.Get(int64(3)); !ok || v != int64(4) {
		t.Errorf("m.Get(3) = %v, %v; want 4, true", v, ok)
	}
}

func TestUnmarshalUnhashableMapKeyNoPanic(t *testing.T) {
	cases := map[string][]byte{
		"array key":      {0xA1, 0x82, 0x01, 0x02, 0x03},
		"map key":        {0xA1, 0xA0, 0x03},
		"bytestring key": {0xA1, 0x41, 0x00, 0x00},
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			var m map[any]any
			err := cbor.Unmarshal(data, &m)
			if err == nil {
				t.Fatalf("expected error, got nil (m=%v)", m)
			}
			var ute *cbor.UnmarshalTypeError
			if !errors.As(err, &ute) {
				t.Errorf("expected *UnmarshalTypeError, got %T: %v", err, err)
			}
		})
	}
}

func TestUnmarshalTimeTagUintOverflow(t *testing.T) {
	data := []byte{0xC1, 0x1B, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	var tm time.Time
	if err := cbor.Unmarshal(data, &tm); err == nil {
		t.Errorf("expected error for overflowing tag-1 epoch, got %v", tm)
	}
}
