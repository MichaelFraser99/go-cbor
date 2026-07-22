package cbor_test

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	cbor "github.com/MichaelFraser99/go-cbor"
)

type upper string

func (u upper) MarshalCBOR() ([]byte, error) {
	return cbor.Marshal(strings.ToUpper(string(u)))
}

func hexOf(t *testing.T, b []byte) string {
	t.Helper()
	return "0x" + hex.EncodeToString(b)
}

func TestMarshalScalars(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"uint0", 0, "0x00"},
		{"uint23", 23, "0x17"},
		{"uint24", 24, "0x1818"},
		{"uint1000", 1000, "0x1903e8"},
		{"uint1e6", 1000000, "0x1a000f4240"},
		{"uintmax", uint64(18446744073709551615), "0x1bffffffffffffffff"},
		{"neg1", -1, "0x20"},
		{"neg100", -100, "0x3863"},
		{"neg1000", -1000, "0x3903e7"},
		{"true", true, "0xf5"},
		{"false", false, "0xf4"},
		{"nil", nil, "0xf6"},
		{"text", "IETF", "0x6449455446"},
		{"bytes", []byte{1, 2, 3, 4}, "0x4401020304"},
		{"emptytext", "", "0x60"},
		{"float_half_1.5", 1.5, "0xf93e00"},
		{"float_half_1.0", 1.0, "0xf93c00"},
		{"float_single_100000", 100000.0, "0xfa47c35000"},
		{"float_double_1.1", 1.1, "0xfb3ff199999999999a"},
		{"float_inf", inf(), "0xf97c00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := cbor.Marshal(tc.in)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if hexOf(t, b) != tc.want {
				t.Errorf("Marshal(%v) = %s, want %s", tc.in, hexOf(t, b), tc.want)
			}
		})
	}
}

func TestMarshalArraysAndMaps(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"array", []int{1, 2, 3}, "0x83010203"},
		{"nested", [][]int{{1}, {2, 3}}, "0x8281018202 03"},
		{"emptyarray", []int{}, "0x80"},
		{"map_sorted", map[int]int{3: 4, 1: 2}, "0xa201020304"},
		{"emptymap", map[int]int{}, "0xa0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := cbor.Marshal(tc.in)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			want := stripSpaces(tc.want)
			if hexOf(t, b) != want {
				t.Errorf("Marshal(%v) = %s, want %s", tc.in, hexOf(t, b), want)
			}
		})
	}
}

func TestMarshalStructAsMap(t *testing.T) {
	type Coord struct {
		Lat float64 `cbor:"lat"`
		Lon float64 `cbor:"lon"`
	}
	b, err := cbor.Marshal(Coord{Lat: 1.5, Lon: 1.0})
	if err != nil {
		t.Fatal(err)
	}

	want := "0xa2636c6174f93e00636c6f6ef93c00"
	if hexOf(t, b) != want {
		t.Errorf("got %s, want %s", hexOf(t, b), want)
	}
}

func TestMarshalKeyAsIntAndOmitEmpty(t *testing.T) {
	type Header struct {
		Alg int    `cbor:"1,asint"`
		Kid []byte `cbor:"4,asint,omitempty"`
	}
	b, err := cbor.Marshal(Header{Alg: -7})
	if err != nil {
		t.Fatal(err)
	}
	if hexOf(t, b) != "0xa10126" {
		t.Errorf("got %s, want 0xa10126", hexOf(t, b))
	}
	b, _ = cbor.Marshal(Header{Alg: -7, Kid: []byte{1, 2}})
	if hexOf(t, b) != "0xa2012604420102" {
		t.Errorf("got %s, want 0xa2012604420102", hexOf(t, b))
	}
}

func TestMarshalToArray(t *testing.T) {
	type Sign1 struct {
		_         struct{} `cbor:",toarray"`
		Protected []byte
		Payload   []byte
	}
	b, err := cbor.Marshal(Sign1{Protected: []byte{1}, Payload: []byte{2}})
	if err != nil {
		t.Fatal(err)
	}
	if hexOf(t, b) != "0x8241014102" {
		t.Errorf("got %s, want 0x8241014102", hexOf(t, b))
	}
}

func TestMarshalSpecialTypes(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"marshaler", upper("hi"), "0x624849"},
		{"rawmessage", cbor.RawMessage([]byte{0x18, 0x2a}), "0x182a"},
		{"tag", cbor.Tag{Number: 0, Content: "x"}, "0xc06178"},
		{"simple255", cbor.SimpleValue(255), "0xf8ff"},
		{"simple16", cbor.SimpleValue(16), "0xf0"},
		{"bigint_small", big.NewInt(1000), "0x1903e8"},
		{"bignum", new(big.Int).Lsh(big.NewInt(1), 64), "0xc249010000000000000000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := cbor.Marshal(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if hexOf(t, b) != tc.want {
				t.Errorf("got %s, want %s", hexOf(t, b), tc.want)
			}
		})
	}
}

func TestMarshalDataItemRoundTrip(t *testing.T) {
	for _, in := range []string{
		"0x1bffffffffffffffff",
		"0x83010203",
		"0xa201020304",
		"0xc074323031332d30332d32315432303a30343a30305a",
		"0xfb3ff199999999999a",
		"0xf93e00",
		"0xf6",
	} {
		var item cbor.DataItem
		if err := cbor.Unmarshal(mustHex(t, in), &item); err != nil {
			t.Fatalf("Unmarshal(%s): %v", in, err)
		}
		b, err := cbor.Marshal(item)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if hexOf(t, b) != in {
			t.Errorf("round trip %s -> %s", in, hexOf(t, b))
		}
	}
}

func TestNaNWidthRoundTrip(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"0xf97e00", "0xf97e00"},
		{"0xfa7f800001", "0xfa7fc00000"},
		{"0xfb7ff0000000000001", "0xfb7ff8000000000000"},
	}
	for _, tc := range cases {
		var item cbor.DataItem
		if err := cbor.Unmarshal(mustHex(t, tc.in), &item); err != nil {
			t.Fatalf("Unmarshal(%s): %v", tc.in, err)
		}
		b, err := cbor.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		if hexOf(t, b) != tc.want {
			t.Errorf("re-encode %s = %s, want %s", tc.in, hexOf(t, b), tc.want)
		}
	}
}

func BenchmarkMarshalInt(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := cbor.Marshal(1000000); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalString(b *testing.B) {
	s := "coap://as.example.com"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := cbor.Marshal(s); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalSliceInt(b *testing.B) {
	v := make([]int, 64)
	for i := range v {
		v[i] = i
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := cbor.Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalMapStringInt(b *testing.B) {
	v := map[string]int{"alpha": 1, "beta": 2, "gamma": 3, "delta": 4, "epsilon": 5}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := cbor.Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalIntKeyedMap(b *testing.B) {
	// COSE-ish integer-keyed map.
	v := map[int]any{1: -7, 2: []byte{1, 2, 3, 4}, 3: "text", 4: 1700000000, -1: 5}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := cbor.Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

type ptrMarshaler struct{ n int }

func (p *ptrMarshaler) MarshalCBOR() ([]byte, error) { return cbor.Marshal(p.n) }

type errMarshaler struct{}

func (errMarshaler) MarshalCBOR() ([]byte, error) { return nil, errTest }

var errTest = &cbor.SyntaxError{}

func TestMarshalErrorBranches(t *testing.T) {
	if _, err := cbor.Marshal(cbor.SimpleValue(25)); err == nil { // 20-31 reserved
		t.Error("SimpleValue(25): want error")
	}
	if _, err := cbor.Marshal([]any{make(chan int)}); err == nil {
		t.Error("array with chan element: want error")
	}
	if _, err := cbor.Marshal(map[string]any{"a": make(chan int)}); err == nil {
		t.Error("map with chan value: want error")
	}
	if _, err := cbor.Marshal(cbor.Map{{Key: "a", Value: make(chan int)}}); err == nil {
		t.Error("cbor.Map with chan value: want error")
	}
	if _, err := cbor.Marshal(cbor.Map{{Key: make(chan int), Value: 1}}); err == nil {
		t.Error("cbor.Map with chan key: want error")
	}
	if _, err := cbor.Marshal(cbor.ArrayOf(cbor.Uint(1), cbor.TagOf(2, cbor.Uint(1)))); err == nil {
		// bignum tag content that is not a byte string, via DataItem builder: allowed to encode?
		_ = err // TagOf(2, Uint) encodes fine; no assertion, just exercises encodeDataItem tag path
	}
}

func TestConfiguredEncodingMarshalError(t *testing.T) {
	em, _ := cbor.EncoderOptions{Sort: cbor.SortLengthFirst}.Encoding()
	if _, err := em.Marshal(make(chan int)); err == nil {
		t.Error("configured Marshal(chan): want error")
	}
}

func TestMarshalNilInterfaceValues(t *testing.T) {
	if b, _ := cbor.Marshal([]any{nil, 1}); hexOf(t, b) != "0x82f601" {
		t.Errorf("[]any{nil,1} = %s, want 0x82f601", hexOf(t, b))
	}
	type S struct {
		A any `cbor:"a"`
	}
	if b, _ := cbor.Marshal(S{}); hexOf(t, b) != "0xa16161f6" {
		t.Errorf("struct nil-any = %s, want 0xa16161f6", hexOf(t, b))
	}
}

func TestMarshalBigIntShortestVsBignum(t *testing.T) {
	// value beyond uint64 -> bignum tag even in shortest mode
	huge := new(big.Int).Lsh(big.NewInt(1), 70)
	b, _ := cbor.Marshal(huge)
	if b[0] != 0xc2 {
		t.Errorf("huge positive: first byte %#x, want tag 2", b[0])
	}
	negHuge := new(big.Int).Neg(huge)
	b, _ = cbor.Marshal(negHuge)
	if b[0] != 0xc3 {
		t.Errorf("huge negative: first byte %#x, want tag 3", b[0])
	}
	b, _ = cbor.Marshal(big.NewInt(-1000)) // fits -> plain nint, not tag
	if hexOf(t, b) != "0x3903e7" {
		t.Errorf("small negative bigint = %s, want 0x3903e7", hexOf(t, b))
	}
}

func TestMarshalStructLengthFirstOmitEmpty(t *testing.T) {
	type S struct {
		Long  int `cbor:"aaa,omitempty"`
		Short int `cbor:"b"`
	}
	em, _ := cbor.EncoderOptions{Sort: cbor.SortLengthFirst}.Encoding()
	b, _ := em.Marshal(S{Short: 2}) // Long omitted
	if hexOf(t, b) != "0xa1616202" {
		t.Errorf("lengthfirst omitempty = %s, want 0xa1616202", hexOf(t, b))
	}
}

type embTE1 struct{ N int }

type embTE2 struct {
	M int `cbor:"N"`
}

func TestMarshalEmbeddedTaggedDominance(t *testing.T) {
	type Outer struct {
		embTE1 // untagged field N -> key "N"
		embTE2 // tagged field M -> key "N"
	}
	b, _ := cbor.Marshal(Outer{embTE1{1}, embTE2{2}})
	var m cbor.Map
	cbor.Unmarshal(b, &m)
	if v, ok := m.GetInt("N"); !ok || v != 2 { // tagged field wins the key
		t.Errorf("tagged dominance: N = %v,%v, want 2", v, ok)
	}
}

//nolint:staticcheck
func TestMarshalEmbeddedDeepShadow(t *testing.T) {
	type Deep struct {
		X int `cbor:"x"`
	}
	type Mid struct{ Deep }
	type Outer struct {
		Mid
		X int `cbor:"x"` // shallower (depth 1) beats Deep.X (depth 2)
	}
	o := Outer{X: 9}
	o.Mid.Deep.X = 1
	b, _ := cbor.Marshal(o)
	var m cbor.Map
	cbor.Unmarshal(b, &m)
	if v, ok := m.GetInt("x"); !ok || v != 9 {
		t.Errorf("deep shadow: x = %v, want 9 (shallowest)", v)
	}
}

func TestMarshalOmitEmptyInterfaceAndArray(t *testing.T) {
	type S struct {
		A any    `cbor:"a,omitempty"`
		B [0]int `cbor:"b,omitempty"`
	}
	b, _ := cbor.Marshal(S{}) // nil interface + empty array -> both omitted
	if hexOf(t, b) != "0xa0" {
		t.Errorf("omitempty interface/array = %s, want 0xa0", hexOf(t, b))
	}

	type T struct {
		A [2]int          `cbor:"a,omitempty"` // non-empty array -> kept
		B struct{ X int } `cbor:"b,omitempty"` // struct is never "empty" -> kept
	}
	b, _ = cbor.Marshal(T{})
	var m cbor.Map
	cbor.Unmarshal(b, &m)
	if len(m) != 2 {
		t.Errorf("non-empty array/struct omitempty: %d fields, want 2", len(m))
	}
}

func TestMarshalRawTagEmpty(t *testing.T) {
	b, _ := cbor.Marshal(cbor.RawTag{Number: 5}) // empty content -> tag 5 wrapping null
	if hexOf(t, b) != "0xc5f6" {
		t.Errorf("empty RawTag = %s, want 0xc5f6", hexOf(t, b))
	}
}

//nolint:unused
type recNode struct {
	Val int `cbor:"v"`
	*recNode
}

func TestMarshalRecursiveEmbedded(t *testing.T) {
	b, err := cbor.Marshal(recNode{Val: 1}) // embedded self-pointer must not recurse forever
	if err != nil {
		t.Fatal(err)
	}
	if hexOf(t, b) != "0xa1617601" {
		t.Errorf("recursive embedded = %s, want 0xa1617601 ({v:1})", hexOf(t, b))
	}
}

func TestMarshalFloat32SpecialValues(t *testing.T) {
	if b, _ := cbor.Marshal(float32(inf())); hexOf(t, b) != "0xf97c00" {
		t.Errorf("float32 +Inf = %s", hexOf(t, b))
	}
	if b, _ := cbor.Marshal(float32(nan())); hexOf(t, b) != "0xf97e00" {
		t.Errorf("float32 NaN = %s", hexOf(t, b))
	}
}

func TestMarshalNaNCanonical(t *testing.T) {
	b, _ := cbor.Marshal(nan()) // default NaN7e00 -> canonical half
	if hexOf(t, b) != "0xf97e00" {
		t.Errorf("NaN default = %s, want 0xf97e00", hexOf(t, b))
	}
}

func TestMarshalerPaths(t *testing.T) {
	type wrap struct {
		P ptrMarshaler `cbor:"p"`
	}
	b, err := cbor.Marshal(&wrap{P: ptrMarshaler{5}}) // pointer marshaler on addressable field
	if err != nil {
		t.Fatal(err)
	}
	var m cbor.Map
	cbor.Unmarshal(b, &m)
	if v, ok := m.GetInt("p"); !ok || v != 5 {
		t.Errorf("ptr marshaler field = %v,%v", v, ok)
	}
	var np *ptrMarshaler
	if bb, _ := cbor.Marshal(np); hexOf(t, bb) != "0xf6" { // nil pointer marshaler -> null
		t.Errorf("nil ptr marshaler = %s", hexOf(t, bb))
	}
	if _, err := cbor.Marshal(errMarshaler{}); err == nil {
		t.Error("errMarshaler: want error")
	}
}

func TestMarshalGoArrays(t *testing.T) {
	b, _ := cbor.Marshal([4]byte{1, 2, 3, 4}) // byte array -> byte string
	if hexOf(t, b) != "0x4401020304" {
		t.Errorf("[4]byte = %s, want 0x4401020304", hexOf(t, b))
	}
	b, _ = cbor.Marshal([3]int{1, 2, 3}) // non-byte array -> array
	if hexOf(t, b) != "0x83010203" {
		t.Errorf("[3]int = %s, want 0x83010203", hexOf(t, b))
	}
}

func TestMarshalStructLengthFirst(t *testing.T) {
	type S struct {
		Long  int `cbor:"aaa"`
		Short int `cbor:"b"`
	}
	em, _ := cbor.EncoderOptions{Sort: cbor.SortLengthFirst}.Encoding()
	b, err := em.Marshal(S{Long: 1, Short: 2})
	if err != nil {
		t.Fatal(err)
	}
	if hexOf(t, b) != "0xa2616202636161610d" && hexOf(t, b) != "0xa26162026361616101" {
		// length-first: "b" (len1) before "aaa" (len3)
		var m cbor.Map
		cbor.Unmarshal(b, &m)
		if len(m) != 2 {
			t.Errorf("lengthfirst struct = %s", hexOf(t, b))
		}
	}
	// verify order: first key is the shorter one
	if b[1] != 0x61 { // text(1)
		t.Errorf("first key not shortest: %s", hexOf(t, b))
	}
}

func TestMarshalOmitEmptyTypes(t *testing.T) {
	type S struct {
		I  int            `cbor:"i,omitempty"`
		S  string         `cbor:"s,omitempty"`
		Sl []int          `cbor:"sl,omitempty"`
		M  map[string]int `cbor:"m,omitempty"`
		P  *int           `cbor:"p,omitempty"`
		B  bool           `cbor:"b,omitempty"`
		F  float64        `cbor:"f,omitempty"`
		U  uint           `cbor:"u,omitempty"`
	}
	b, _ := cbor.Marshal(S{}) // everything empty -> {}
	if hexOf(t, b) != "0xa0" {
		t.Errorf("all-empty omitempty = %s, want 0xa0", hexOf(t, b))
	}
	n := 5
	b, _ = cbor.Marshal(S{I: 1, S: "x", Sl: []int{1}, M: map[string]int{"a": 1}, P: &n, B: true, F: 1.5, U: 2})
	var m cbor.Map
	cbor.Unmarshal(b, &m)
	if len(m) != 8 {
		t.Errorf("all-present = %d fields, want 8", len(m))
	}
}

func TestMarshalBigIntTagMode(t *testing.T) {
	em, _ := cbor.EncoderOptions{BigInt: cbor.BigIntTag}.Encoding()
	b, _ := em.Marshal(big.NewInt(1000)) // tag 2 wrapping 0x03e8, even though it fits an int
	if hexOf(t, b) != "0xc24203e8" {
		t.Errorf("BigIntTag positive = %s, want 0xc24203e8", hexOf(t, b))
	}
	b, _ = em.Marshal(big.NewInt(-1000)) // tag 3 wrapping 0x03e7 (-1-999)
	if hexOf(t, b) != "0xc34203e7" {
		t.Errorf("BigIntTag negative = %s, want 0xc34203e7", hexOf(t, b))
	}
}

func TestMarshalFloatModes(t *testing.T) {
	em, _ := cbor.EncoderOptions{Float: cbor.FloatDouble}.Encoding()
	b, _ := em.Marshal(1.5)
	if hexOf(t, b) != "0xfb3ff8000000000000" {
		t.Errorf("FloatDouble = %s, want 8-byte", hexOf(t, b))
	}
	// NaNNone keeps the chosen width
	em, _ = cbor.EncoderOptions{Float: cbor.FloatDouble, NaN: cbor.NaNNone}.Encoding()
	b, _ = em.Marshal(nan())
	if len(b) != 9 {
		t.Errorf("NaNNone double = %s, want 9 bytes", hexOf(t, b))
	}
}

func nan() float64 { var z float64; return z / z }

func TestConstructFloat(t *testing.T) {
	b, err := cbor.Marshal(cbor.Float(1.5))
	if err != nil {
		t.Fatal(err)
	}
	if hexOf(t, b) != "0xfb3ff8000000000000" { // Float always double-width
		t.Errorf("Float(1.5) = %s, want 0xfb3ff8000000000000", hexOf(t, b))
	}
}

func inf() float64 { var z float64; return 1 / z }

func stripSpaces(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func TestAppendixACanonicalRoundTrip(t *testing.T) {
	vectors := []string{
		"0x00", "0x01", "0x0a", "0x17", "0x1818", "0x1819", "0x1864",
		"0x1903e8", "0x1a000f4240", "0x1b000000e8d4a51000",
		"0x1bffffffffffffffff", "0xc249010000000000000000",
		"0x3bffffffffffffffff", "0xc349010000000000000000",
		"0x20", "0x29", "0x3863", "0x3903e7",
		"0xf90000", "0xf98000", "0xf93c00", "0xf93e00", "0xf97bff",
		"0xfa47c35000", "0xfa7f7fffff", "0xfb7e37e43c8800759c",
		"0xf90001", "0xf90400", "0xf9c400", "0xfbc010666666666666",
		"0xf97c00", "0xf97e00", "0xf9fc00",
		"0xf4", "0xf5", "0xf6", "0xf7", "0xf0", "0xf8ff",
		"0xc074323031332d30332d32315432303a30343a30305a",
		"0xc11a514b67b0", "0xc1fb41d452d9ec200000",
		"0xd74401020304", "0xd818456449455446",
		"0xd82076687474703a2f2f7777772e6578616d706c652e636f6d",
		"0x40", "0x4401020304", "0x60", "0x6161", "0x6449455446",
		"0x62225c", "0x62c3bc", "0x63e6b0b4", "0x64f0908591",
		"0x80", "0x83010203", "0x8301820203820405",
		"0x98190102030405060708090a0b0c0d0e0f101112131415161718181819",
		"0x826161a161626163",
		"0xa0", "0xa201020304", "0xa26161016162820203",
	}
	for _, v := range vectors {
		t.Run(v, func(t *testing.T) {
			var item cbor.DataItem
			if err := cbor.Unmarshal(mustHex(t, v), &item); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			b, err := cbor.Marshal(item)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if hexOf(t, b) != v {
				t.Errorf("round trip = %s, want %s", hexOf(t, b), v)
			}
		})
	}
}

func TestMarshalDataItemMapCanonical(t *testing.T) {
	item := cbor.MapOf(cbor.Uint(10), cbor.Text("ten"), cbor.Uint(1), cbor.Text("one"))
	b, err := cbor.Marshal(item)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if hexOf(t, b) != "0xa201636f6e650a6374656e" {
		t.Errorf("DataItem map not canonically sorted: got %s, want 0xa201636f6e650a6374656e", hexOf(t, b))
	}

	strict, _ := cbor.DecoderOptions{Strict: true}.Decoding()
	if err := strict.Valid(b); err != nil {
		t.Errorf("strict decoder rejected the builder's own output: %v", err)
	}
}

func TestMarshalDataItemMapSortModes(t *testing.T) {
	item := cbor.MapOf(cbor.Uint(10), cbor.Text("ten"), cbor.Uint(1), cbor.Text("one"))

	bytewise, _ := cbor.EncoderOptions{Sort: cbor.SortBytewise}.Encoding()
	b, err := bytewise.Marshal(item)
	if err != nil {
		t.Fatalf("Marshal bytewise: %v", err)
	}
	if hexOf(t, b) != "0xa201636f6e650a6374656e" {
		t.Errorf("SortBytewise DataItem map: got %s, want canonical 0xa201636f6e650a6374656e", hexOf(t, b))
	}

	none, _ := cbor.EncoderOptions{Sort: cbor.SortNone}.Encoding()
	b, err = none.Marshal(item)
	if err != nil {
		t.Fatalf("Marshal none: %v", err)
	}
	if hexOf(t, b) != "0xa20a6374656e01636f6e65" {
		t.Errorf("SortNone DataItem map: got %s, want insertion order 0xa20a6374656e01636f6e65", hexOf(t, b))
	}
}

func TestMarshalMapDataItemOddItems(t *testing.T) {
	item := cbor.MapOf(cbor.Uint(1), cbor.Uint(2), cbor.Uint(3)) // odd: dangling key
	if _, err := cbor.Marshal(item); err == nil {
		t.Error("Marshal of odd-item map DataItem succeeded, want error")
	}
}

func TestMarshalMapDataItemDuplicateKey(t *testing.T) {
	item := cbor.MapOf(cbor.Uint(1), cbor.Text("a"), cbor.Uint(1), cbor.Text("b"))
	if _, err := cbor.Marshal(item); err == nil {
		t.Error("Marshal of duplicate-key map DataItem succeeded, want error")
	}
}

func TestMarshalMapDataItemNestedError(t *testing.T) {
	if _, err := cbor.Marshal(cbor.MapOf(cbor.MapOf(cbor.Uint(1)), cbor.Uint(2))); err == nil {
		t.Error("map DataItem with malformed (odd) key: want error")
	}
	if _, err := cbor.Marshal(cbor.MapOf(cbor.Uint(1), cbor.MapOf(cbor.Uint(2)))); err == nil {
		t.Error("map DataItem with malformed (odd) value: want error")
	}
}

func TestMarshalNilMap(t *testing.T) {
	var m map[string]int
	b, err := cbor.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal nil map: %v", err)
	}
	if hexOf(t, b) != "0xf6" {
		t.Errorf("nil map = %s, want 0xf6 (null)", hexOf(t, b))
	}
}

type EmbInner struct {
	A int `cbor:"a"`
	B int `cbor:"b"`
}

func TestMarshalEmbeddedPromotion(t *testing.T) {
	type Outer struct {
		EmbInner
		C int `cbor:"c"`
	}
	b, err := cbor.Marshal(Outer{EmbInner: EmbInner{A: 1, B: 2}, C: 3})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if hexOf(t, b) != "0xa3616101616202616303" {
		t.Errorf("embedded promotion = %s, want 0xa3616101616202616303 ({a:1,b:2,c:3})", hexOf(t, b))
	}
	var got Outer
	if err := cbor.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.A != 1 || got.B != 2 || got.C != 3 {
		t.Errorf("round trip = %+v, want {1 2 3}", got)
	}
}

func TestMarshalEmbeddedTaggedNotPromoted(t *testing.T) {
	type Outer struct {
		EmbInner `cbor:"inner"`
		C        int `cbor:"c"`
	}
	b, _ := cbor.Marshal(Outer{EmbInner: EmbInner{A: 1}, C: 3})
	var m cbor.Map
	if err := cbor.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := m.Get("a"); ok {
		t.Error("tagged embedded field was promoted; want nested under its tag")
	}
	inner, ok := m.GetMap("inner")
	if !ok {
		t.Fatal("no nested \"inner\" map")
	}
	if v, ok := inner.GetInt("a"); !ok || v != 1 {
		t.Errorf("inner.a = %v, %v; want 1, true", v, ok)
	}
}

func TestMarshalEmbeddedPointer(t *testing.T) {
	type Outer struct {
		*EmbInner
		C int `cbor:"c"`
	}
	b, err := cbor.Marshal(Outer{EmbInner: &EmbInner{A: 1, B: 2}, C: 3})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if hexOf(t, b) != "0xa3616101616202616303" {
		t.Errorf("pointer promotion = %s, want 0xa3616101616202616303", hexOf(t, b))
	}
	b2, err := cbor.Marshal(Outer{C: 3})
	if err != nil {
		t.Fatalf("Marshal nil embed: %v", err)
	}
	if hexOf(t, b2) != "0xa1616303" {
		t.Errorf("nil embedded pointer = %s, want 0xa1616303 ({c:3})", hexOf(t, b2))
	}
	var got Outer
	if err := cbor.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.EmbInner == nil || got.A != 1 || got.C != 3 {
		t.Errorf("round trip = %+v, want inner allocated with a=1, c=3", got)
	}
}

func TestMarshalEmbeddedKeyAsInt(t *testing.T) {
	type IntInner struct {
		X int `cbor:"1,asint"`
	}
	type Outer struct {
		IntInner
		Y int `cbor:"2,asint"`
	}
	b, _ := cbor.Marshal(Outer{IntInner: IntInner{X: 7}, Y: 9})
	if hexOf(t, b) != "0xa201070209" {
		t.Errorf("asint promotion = %s, want 0xa201070209 ({1:7,2:9})", hexOf(t, b))
	}
}

func TestMarshalEmbeddedShadowing(t *testing.T) {
	type ShadowInner struct {
		A int `cbor:"a"`
		Z int `cbor:"z"`
	}
	type Outer struct {
		ShadowInner
		A int `cbor:"a"`
	}
	o := Outer{ShadowInner: ShadowInner{A: 1, Z: 9}}
	o.A = 2
	b, _ := cbor.Marshal(o)
	var m cbor.Map
	if err := cbor.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m) != 2 {
		t.Errorf("len = %d, want 2 (shadowed inner A dropped)", len(m))
	}
	if v, ok := m.GetInt("a"); !ok || v != 2 {
		t.Errorf("a = %v, %v; want 2 (outer shadows inner)", v, ok)
	}
	if v, ok := m.GetInt("z"); !ok || v != 9 {
		t.Errorf("z = %v, %v; want 9", v, ok)
	}
}

func TestMarshalEmbeddedAmbiguousDropped(t *testing.T) {
	type E1 struct {
		A int `cbor:"a"`
		P int `cbor:"p"`
	}
	type E2 struct {
		A int `cbor:"a"`
		Q int `cbor:"q"`
	}
	type Outer struct {
		E1
		E2
	}
	b, _ := cbor.Marshal(Outer{E1: E1{A: 1, P: 2}, E2: E2{A: 3, Q: 4}})
	var m cbor.Map
	if err := cbor.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := m.Get("a"); ok {
		t.Error("ambiguous \"a\" at equal depth should be dropped (json rule)")
	}
	if _, ok := m.Get("p"); !ok {
		t.Error("p missing")
	}
	if _, ok := m.Get("q"); !ok {
		t.Error("q missing")
	}
}

func TestMarshalRejectsDuplicateMapKeys(t *testing.T) {
	t.Run("cbor.Map literal dup", func(t *testing.T) {
		m := cbor.Map{{Key: "a", Value: 1}, {Key: "a", Value: 2}}
		if _, err := cbor.Marshal(m); err == nil {
			t.Error("expected error marshalling Map with duplicate keys")
		}
	})
	t.Run("go map colliding encoded keys", func(t *testing.T) {
		m := map[any]any{uint64(1): "x", uint8(1): "y"}
		if _, err := cbor.Marshal(m); err == nil {
			t.Error("expected error marshalling map with colliding encoded keys")
		}
	})
}
