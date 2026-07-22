package cbor_test

import (
	"bytes"
	"crypto/ed25519"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	cbor "github.com/MichaelFraser99/go-cbor"
)

type coseHeader struct {
	Alg int    `cbor:"1,asint"`
	Kid []byte `cbor:"4,asint,omitempty"`
}

type sign1 struct {
	_           struct{} `cbor:",toarray"`
	Protected   []byte
	Unprotected map[int]any
	Payload     []byte
	Signature   []byte
}

func TestCOSESign1RoundTrip(t *testing.T) {
	protected, err := cbor.Marshal(coseHeader{Alg: -7})
	if err != nil {
		t.Fatal(err)
	}

	msg := sign1{
		Protected:   protected,
		Unprotected: map[int]any{4: []byte{9, 9}},
		Payload:     []byte("payload"),
		Signature:   []byte{0xaa, 0xbb, 0xcc},
	}

	wire, err := cbor.Marshal(cbor.Tag{Number: 18, Content: msg})
	if err != nil {
		t.Fatal(err)
	}
	if wire[0] != 0xd2 {
		t.Fatalf("first byte = %#x, want 0xd2 (tag 18)", wire[0])
	}

	var got sign1
	if err := cbor.Unmarshal(wire, &got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Protected, protected) {
		t.Errorf("Protected = %x, want %x (byte-exact)", got.Protected, protected)
	}
	if !bytes.Equal(got.Payload, []byte("payload")) {
		t.Errorf("Payload = %q, want payload", got.Payload)
	}
	if !bytes.Equal(got.Signature, []byte{0xaa, 0xbb, 0xcc}) {
		t.Errorf("Signature = %x", got.Signature)
	}

	var hdr coseHeader
	if err := cbor.Unmarshal(got.Protected, &hdr); err != nil {
		t.Fatal(err)
	}
	if hdr.Alg != -7 {
		t.Errorf("Alg = %d, want -7", hdr.Alg)
	}
}

type ed25519Header struct {
	Alg int `cbor:"1,asint"`
}

func TestCOSESign1Ed25519KnownAnswer(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)

	protected, err := cbor.Marshal(ed25519Header{Alg: -8})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("This is the content.")

	sigStructure, err := cbor.Marshal([]any{"Signature1", protected, []byte{}, payload})
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(priv, sigStructure)

	msg := sign1{
		Protected:   protected,
		Unprotected: map[int]any{},
		Payload:     payload,
		Signature:   signature,
	}
	wire, err := cbor.Marshal(cbor.Tag{Number: 18, Content: msg})
	if err != nil {
		t.Fatal(err)
	}

	wire2, _ := cbor.Marshal(cbor.Tag{Number: 18, Content: msg})
	if !bytes.Equal(wire, wire2) {
		t.Fatal("encoding is not deterministic")
	}
	if !bytes.Equal(signature, ed25519.Sign(priv, sigStructure)) {
		t.Fatal("Ed25519 signature is not deterministic")
	}

	var got sign1
	if err := cbor.Unmarshal(wire, &got); err != nil {
		t.Fatal(err)
	}
	verifyStructure, err := cbor.Marshal([]any{"Signature1", got.Protected, []byte{}, got.Payload})
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(pub, verifyStructure, got.Signature) {
		t.Fatal("signature did not verify after round trip")
	}
}

type vical struct {
	Version          string            `cbor:"version"`
	VicalProvider    string            `cbor:"vicalProvider"`
	Date             time.Time         `cbor:"date"`
	VicalIssueID     uint64            `cbor:"vicalIssueID,omitempty"`
	CertificateInfos []certificateInfo `cbor:"certificateInfos"`
}

type certificateInfo struct {
	Certificate  []byte   `cbor:"certificate"`
	SerialNumber *big.Int `cbor:"serialNumber"`
	DocType      []string `cbor:"docType"`
}

func TestVICALRoundTrip(t *testing.T) {
	issued := time.Unix(1700000000, 0).UTC()
	in := vical{
		Version:       "1.0",
		VicalProvider: "Example TA",
		Date:          issued,
		VicalIssueID:  42,
		CertificateInfos: []certificateInfo{{
			Certificate:  []byte{0x30, 0x82, 0x01},
			SerialNumber: big.NewInt(1234567),
			DocType:      []string{"org.iso.18013.5.1.mDL"},
		}},
	}

	data, err := cbor.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}

	var out vical
	if err := cbor.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}

	if out.Version != in.Version || out.VicalProvider != in.VicalProvider {
		t.Errorf("scalars mismatch: %+v", out)
	}
	if !out.Date.Equal(in.Date) {
		t.Errorf("Date = %v, want %v", out.Date, in.Date)
	}
	if out.VicalIssueID != 42 {
		t.Errorf("VicalIssueID = %d, want 42", out.VicalIssueID)
	}
	if len(out.CertificateInfos) != 1 {
		t.Fatalf("CertificateInfos len = %d, want 1", len(out.CertificateInfos))
	}
	ci := out.CertificateInfos[0]
	if ci.SerialNumber.Cmp(big.NewInt(1234567)) != 0 {
		t.Errorf("SerialNumber = %s, want 1234567", ci.SerialNumber)
	}
	if !reflect.DeepEqual(ci.DocType, []string{"org.iso.18013.5.1.mDL"}) {
		t.Errorf("DocType = %v", ci.DocType)
	}
}

type benchInner struct {
	Name  string `cbor:"name"`
	Count int    `cbor:"count"`
}

type benchOuter struct {
	ID    uint64       `cbor:"id"`
	Items []benchInner `cbor:"items"`
	Ratio float64      `cbor:"ratio"`
	Data  []byte       `cbor:"data"`
}

func benchValue() benchOuter {
	return benchOuter{
		ID:    42,
		Items: []benchInner{{"alpha", 1}, {"beta", 2}, {"gamma", 3}},
		Ratio: 1.5,
		Data:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
}

func BenchmarkMarshalStruct(b *testing.B) {
	v := benchValue()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := cbor.Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalStruct(b *testing.B) {
	data, _ := cbor.Marshal(benchValue())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out benchOuter
		if err := cbor.Unmarshal(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalDataItem(b *testing.B) {
	data, _ := cbor.Marshal(benchValue())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var item cbor.DataItem
		if err := cbor.Unmarshal(data, &item); err != nil {
			b.Fatal(err)
		}
	}
}

func TestEncodedCBORWrap(t *testing.T) {
	inner, _ := cbor.Marshal(map[int]int{1: 2})
	b, err := cbor.Marshal(cbor.EncodedCBOR(inner))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if hexOf(t, b) != "0xd81843a10102" {
		t.Errorf("EncodedCBOR wrap = %s, want 0xd81843a10102 (tag24(bstr(a10102)))", hexOf(t, b))
	}
}

func TestEncodedCBORRoundTripInStruct(t *testing.T) {
	inner, _ := cbor.Marshal(map[string]int{"a": 1})
	type Outer struct {
		E cbor.EncodedCBOR `cbor:"e"`
	}
	b, err := cbor.Marshal(Outer{E: inner})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Outer
	if err := cbor.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !bytes.Equal(got.E, inner) {
		t.Errorf("round trip = %x, want %x", []byte(got.E), inner)
	}

	var m map[string]int
	if err := cbor.Unmarshal(got.E, &m); err != nil {
		t.Fatalf("Unmarshal inner: %v", err)
	}
	if m["a"] != 1 {
		t.Errorf("inner decoded = %v, want a=1", m)
	}
}

func TestEncodedCBORUnmarshalMalformed(t *testing.T) {
	var e cbor.EncodedCBOR
	if err := (&e).UnmarshalCBOR([]byte{0xd8}); err == nil { // truncated tag header
		t.Error("UnmarshalCBOR(malformed): want error")
	}
}

func TestUnmarshalTrailingBytes(t *testing.T) {
	var i int
	if err := cbor.Unmarshal(mustHex(t, "0x0000"), &i); err == nil {
		t.Error("Unmarshal with trailing bytes: want error")
	}
}

func TestEncodedCBORRejectsNonTag24(t *testing.T) {
	var e cbor.EncodedCBOR
	if err := cbor.Unmarshal(mustHex(t, "0xa10102"), &e); err == nil {
		t.Error("EncodedCBOR accepted a bare map (not tag 24), want error")
	}
	err := cbor.Unmarshal(mustHex(t, "0x43010203"), &e) // a bare byte string
	if err == nil {
		t.Fatal("EncodedCBOR accepted a bare byte string, want error")
	}
	if !strings.Contains(err.Error(), "byte string") {
		t.Errorf("error names wrong CBOR type: %q, want it to mention \"byte string\"", err.Error())
	}
}

func TestEncodedCBORDecodeHelper(t *testing.T) {
	inner, _ := cbor.Marshal(map[string]int{"a": 1})
	ec := cbor.EncodedCBOR(inner)
	var m map[string]int
	if err := ec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["a"] != 1 {
		t.Errorf("decoded inner = %v, want a=1", m)
	}
}

func TestRawTagRoundTrip(t *testing.T) {
	inner, _ := cbor.Marshal([]int{1, 2})
	b, err := cbor.Marshal(cbor.RawTag{Number: 18, Content: inner})
	if err != nil {
		t.Fatal(err)
	}
	if hexOf(t, b) != "0xd2820102" { // tag 18 wrapping [1,2]
		t.Errorf("RawTag encode = %s, want 0xd2820102", hexOf(t, b))
	}
	var got cbor.RawTag
	if err := cbor.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Number != 18 {
		t.Errorf("Number = %d, want 18", got.Number)
	}
	if hexOf(t, []byte(got.Content)) != "0x820102" {
		t.Errorf("Content = %x, want 820102", []byte(got.Content))
	}
}

func TestRawTagCapturesNonCanonicalVerbatim(t *testing.T) {
	// tag 18 wrapping a non-canonically-ordered map {2:0, 1:0}
	var rt cbor.RawTag
	if err := cbor.Unmarshal(mustHex(t, "0xd2a202000100"), &rt); err != nil {
		t.Fatal(err)
	}
	if hexOf(t, []byte(rt.Content)) != "0xa202000100" {
		t.Errorf("Content = %x, want a202000100 (verbatim, not re-sorted)", []byte(rt.Content))
	}
}

func TestMapTypedAccessors(t *testing.T) {
	var m cbor.Map
	if err := cbor.Unmarshal(mustHex(t, "0xa3016568656c6c6f0243010203182a1904d2"), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if s, ok := m.GetString(int64(1)); !ok || s != "hello" {
		t.Errorf("GetString(1) = %q, %v; want hello, true", s, ok)
	}
	if b, ok := m.GetBytes(int64(2)); !ok || !bytes.Equal(b, []byte{1, 2, 3}) {
		t.Errorf("GetBytes(2) = %x, %v; want 010203, true", b, ok)
	}
	if n, ok := m.GetInt(int64(42)); !ok || n != 1234 {
		t.Errorf("GetInt(42) = %d, %v; want 1234, true", n, ok)
	}
	if _, ok := m.GetInt(int64(99)); ok {
		t.Error("GetInt(99) ok = true, want false for missing key")
	}
	if _, ok := m.GetString(int64(2)); ok {
		t.Error("GetString(2) ok = true, want false for wrong-typed value")
	}
}

func TestMapAccessorEdgeCases(t *testing.T) {
	m := cbor.Map{
		{Key: int64(1), Value: uint64(18446744073709551615)},
		{Key: int64(2), Value: int64(-5)},
		{Key: int64(3), Value: new(big.Int).Lsh(big.NewInt(1), 100)},
	}
	if _, ok := m.GetInt(int64(1)); ok {
		t.Error("GetInt on uint64-max: want !ok (overflow)")
	}
	if v, ok := m.GetUint(int64(1)); !ok || v != 18446744073709551615 {
		t.Errorf("GetUint(1) = %d,%v", v, ok)
	}
	if _, ok := m.GetUint(int64(2)); ok {
		t.Error("GetUint on negative: want !ok")
	}
	if _, ok := m.GetInt(int64(3)); ok {
		t.Error("GetInt on huge bigint: want !ok")
	}
	if _, ok := m.GetUint(int64(3)); ok {
		t.Error("GetUint on huge bigint: want !ok")
	}
	if _, ok := m.GetFloat(int64(9)); ok {
		t.Error("GetFloat absent")
	}
	if _, ok := m.GetBool(int64(9)); ok {
		t.Error("GetBool absent")
	}
	if _, ok := m.GetBytes(int64(9)); ok {
		t.Error("GetBytes absent")
	}
	if _, ok := m.GetSlice(int64(9)); ok {
		t.Error("GetSlice absent")
	}
	if _, ok := m.GetTag(int64(9)); ok {
		t.Error("GetTag absent")
	}
	if _, ok := m.GetString(int64(9)); ok {
		t.Error("GetString absent")
	}
	if _, ok := m.GetMap(int64(9)); ok {
		t.Error("GetMap absent")
	}

	// coercion success paths across representations
	fits := cbor.Map{
		{Key: "i", Value: int64(5)},
		{Key: "u", Value: uint64(5)},
		{Key: "bi", Value: big.NewInt(5)},
		{Key: "bu", Value: big.NewInt(7)},
	}
	if v, ok := fits.GetUint("i"); !ok || v != 5 { // int64 -> uint64
		t.Errorf("GetUint(int64) = %d,%v", v, ok)
	}
	if v, ok := fits.GetInt("u"); !ok || v != 5 { // uint64 -> int64
		t.Errorf("GetInt(uint64) = %d,%v", v, ok)
	}
	if v, ok := fits.GetInt("bi"); !ok || v != 5 { // *big.Int -> int64
		t.Errorf("GetInt(bigint) = %d,%v", v, ok)
	}
	if v, ok := fits.GetUint("bu"); !ok || v != 7 { // *big.Int -> uint64
		t.Errorf("GetUint(bigint) = %d,%v", v, ok)
	}
}

func TestRawMessageAndEncodedCBOREdges(t *testing.T) {
	b, _ := cbor.Marshal(cbor.RawMessage(nil)) // empty -> null
	if hexOf(t, b) != "0xf6" {
		t.Errorf("empty RawMessage = %s, want 0xf6", hexOf(t, b))
	}
	b, _ = cbor.Marshal(cbor.EncodedCBOR(nil)) // empty -> null
	if hexOf(t, b) != "0xf6" {
		t.Errorf("empty EncodedCBOR = %s, want 0xf6", hexOf(t, b))
	}
	var e cbor.EncodedCBOR
	if err := cbor.Unmarshal(mustHex(t, "0xf6"), &e); err != nil || e != nil {
		t.Errorf("null into EncodedCBOR = %x, %v", []byte(e), err)
	}
}

func TestMarshalTypeErrorMajorNames(t *testing.T) {
	// Trigger UnmarshalTypeError with different CBORType names (exercises majorName).
	cases := []string{"0x83010203", "0xa10102", "0x40", "0xf5"} // array, map, bytes, bool -> into int
	for _, in := range cases {
		var i int
		if err := cbor.Unmarshal(mustHex(t, in), &i); err == nil {
			t.Errorf("Unmarshal(%s) into int: want error", in)
		}
	}
}

func TestMarshalNils(t *testing.T) {
	var bs []byte
	if b, _ := cbor.Marshal(bs); hexOf(t, b) != "0xf6" {
		t.Errorf("nil []byte = %s, want null", hexOf(t, b))
	}
	var sl []int
	if b, _ := cbor.Marshal(sl); hexOf(t, b) != "0xf6" {
		t.Errorf("nil slice = %s, want null", hexOf(t, b))
	}
	var p *int
	if b, _ := cbor.Marshal(p); hexOf(t, b) != "0xf6" {
		t.Errorf("nil pointer = %s, want null", hexOf(t, b))
	}
}

func TestMapCoercingAccessors(t *testing.T) {
	// {1:5, 2:1.5, 3:true, 4:[1,2], 5:0("x"), 6:18446744073709551615}
	var m cbor.Map
	if err := cbor.Unmarshal(mustHex(t, "0xa6010502f93e0003f50482010205c06178061bffffffffffffffff"), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if n, ok := m.GetInt(int64(1)); !ok || n != 5 {
		t.Errorf("GetInt(1) = %d,%v; want 5,true", n, ok)
	}
	if n, ok := m.GetUint(int64(1)); !ok || n != 5 { // int64 value coerces to uint64
		t.Errorf("GetUint(1) = %d,%v; want 5,true", n, ok)
	}
	if f, ok := m.GetFloat(int64(2)); !ok || f != 1.5 {
		t.Errorf("GetFloat(2) = %v,%v; want 1.5,true", f, ok)
	}
	if v, ok := m.GetBool(int64(3)); !ok || !v {
		t.Errorf("GetBool(3) = %v,%v; want true,true", v, ok)
	}
	if s, ok := m.GetSlice(int64(4)); !ok || len(s) != 2 {
		t.Errorf("GetSlice(4) = %v,%v; want len 2,true", s, ok)
	}
	if tag, ok := m.GetTag(int64(5)); !ok || tag.Number != 0 {
		t.Errorf("GetTag(5) = %v,%v; want number 0,true", tag, ok)
	}
	if n, ok := m.GetUint(int64(6)); !ok || n != 18446744073709551615 { // decoded as uint64
		t.Errorf("GetUint(6) = %d,%v; want max,true", n, ok)
	}
	if _, ok := m.GetInt(int64(6)); ok { // doesn't fit int64
		t.Error("GetInt(6) ok = true; want false (uint64 max exceeds int64)")
	}
}

func TestMapToStringMap(t *testing.T) {
	var m cbor.Map
	if err := cbor.Unmarshal(mustHex(t, "0xa2616101616202"), &m); err != nil { // {"a":1,"b":2}
		t.Fatal(err)
	}
	sm, err := m.ToStringMap()
	if err != nil {
		t.Fatalf("ToStringMap: %v", err)
	}
	if sm["a"] != int64(1) || sm["b"] != int64(2) {
		t.Errorf("ToStringMap = %v, want a=1 b=2", sm)
	}

	var m2 cbor.Map
	if err := cbor.Unmarshal(mustHex(t, "0xa10102"), &m2); err != nil { // {1:2}
		t.Fatal(err)
	}
	if _, err := m2.ToStringMap(); err == nil {
		t.Error("ToStringMap with a non-string key: want error")
	}
}

func TestMapGetMap(t *testing.T) {
	var m cbor.Map
	if err := cbor.Unmarshal(mustHex(t, "0xa16169a0"), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	inner, ok := m.GetMap("i")
	if !ok {
		t.Fatal("GetMap(\"i\") ok = false, want true")
	}
	if inner == nil || len(inner) != 0 {
		t.Errorf("GetMap = %v, want empty Map", inner)
	}
}
