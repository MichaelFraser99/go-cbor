package cbor_test

import (
	"math"
	"math/big"
	"testing"
	"time"

	cbor "github.com/MichaelFraser99/go-cbor"
)

func TestFloatBigIntModes(t *testing.T) {
	double, _ := cbor.EncoderOptions{Float: cbor.FloatDouble}.Encoding()
	b, _ := double.Marshal(1.5)
	if hexOf(t, b) != "0xfb3ff8000000000000" {
		t.Errorf("FloatDouble(1.5) = %s, want 0xfb3ff8000000000000", hexOf(t, b))
	}

	nanNone, _ := cbor.EncoderOptions{NaN: cbor.NaNNone}.Encoding()
	b, _ = nanNone.Marshal(math.NaN())
	if len(b) != 9 || b[0] != 0xfb {
		t.Errorf("NaNNone(NaN) = %s, want a 9-byte double", hexOf(t, b))
	}

	bigTag, _ := cbor.EncoderOptions{BigInt: cbor.BigIntTag}.Encoding()
	b, _ = bigTag.Marshal(big.NewInt(1))
	if hexOf(t, b) != "0xc24101" {
		t.Errorf("BigIntTag(1) = %s, want 0xc24101", hexOf(t, b))
	}
}

func TestRejectIndefinite(t *testing.T) {
	dm, _ := cbor.DecoderOptions{RejectIndefinite: true}.Decoding()
	if err := dm.Valid(mustHex(t, "0x9f00ff")); err == nil {
		t.Error("RejectIndefinite accepted indefinite array, want error")
	}
	if err := cbor.Valid(mustHex(t, "0x9f00ff")); err != nil {
		t.Errorf("default rejected indefinite array: %v", err)
	}
}

func TestElementCaps(t *testing.T) {
	arr, _ := cbor.DecoderOptions{MaxArrayLen: 2}.Decoding()
	if err := arr.Valid(mustHex(t, "0x83010203")); err == nil {
		t.Error("MaxArrayLen not enforced on 3-element array")
	}
	if err := arr.Valid(mustHex(t, "0x820102")); err != nil {
		t.Errorf("2-element array rejected under cap 2: %v", err)
	}
	mp, _ := cbor.DecoderOptions{MaxMapLen: 1}.Decoding()
	if err := mp.Valid(mustHex(t, "0xa201020304")); err == nil {
		t.Error("MaxMapLen not enforced on 2-pair map")
	}

	u, _ := cbor.UntrustedDecoderOptions().Decoding()
	if err := u.Valid(mustHex(t, "0x83010203")); err != nil {
		t.Errorf("untrusted preset rejected a small array: %v", err)
	}
	if err := u.Valid(mustHex(t, "0x9f00ff")); err == nil {
		t.Error("untrusted preset accepted indefinite array, want error")
	}
}

func TestSortModes(t *testing.T) {
	m := map[int]int{24: 1, -1: 2}

	bytewise, err := cbor.EncoderOptions{Sort: cbor.SortBytewise}.Encoding()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := bytewise.Marshal(m)
	if hexOf(t, b) != "0xa21818012002" {
		t.Errorf("bytewise = %s, want 0xa21818012002", hexOf(t, b))
	}

	lengthFirst, _ := cbor.EncoderOptions{Sort: cbor.SortLengthFirst}.Encoding()
	b, _ = lengthFirst.Marshal(m)
	if hexOf(t, b) != "0xa22002181801" {
		t.Errorf("lengthFirst = %s, want 0xa22002181801", hexOf(t, b))
	}
}

func TestTimeModes(t *testing.T) {
	tm := time.Unix(1363896240, 0).UTC()

	b, _ := cbor.Marshal(tm)
	if b[0] != 0xc1 {
		t.Errorf("default time first byte = %#x, want 0xc1 (tag 1)", b[0])
	}

	em, _ := cbor.EncoderOptions{Time: cbor.TimeRFC3339}.Encoding()
	b, _ = em.Marshal(tm)
	if b[0] != 0xc0 {
		t.Errorf("rfc3339 time first byte = %#x, want 0xc0 (tag 0)", b[0])
	}
	var back time.Time
	if err := cbor.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Equal(tm) {
		t.Errorf("time round trip: got %v, want %v", back, tm)
	}
}

func TestTimeSubSecondPreserved(t *testing.T) {
	tm := time.Unix(1363896240, 500000000).UTC()
	b, _ := cbor.Marshal(tm)
	if b[0] != 0xc1 {
		t.Fatalf("first byte = %#x, want tag 1", b[0])
	}
	var back time.Time
	if err := cbor.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if d := back.Sub(tm); d < -time.Millisecond || d > time.Millisecond {
		t.Errorf("sub-second lost: got %v, want ~%v (delta %v)", back, tm, d)
	}
}

func TestDecModeMaxDepth(t *testing.T) {

	deep := mustHex(t, "0x818181818100")
	shallow, _ := cbor.DecoderOptions{MaxNestingDepth: 4}.Decoding()
	var item cbor.DataItem
	if err := shallow.Unmarshal(deep, &item); err == nil {
		t.Error("depth-4 mode accepted depth-5 input, want error")
	}

	if err := cbor.Unmarshal(deep, &item); err != nil {
		t.Errorf("default Unmarshal rejected depth 5: %v", err)
	}
}

func TestDecModeDuplicateKeys(t *testing.T) {
	dup := mustHex(t, "0xa201010102")
	strict, _ := cbor.DecoderOptions{DuplicateKeys: cbor.DupError}.Decoding()

	var item cbor.DataItem
	if err := strict.Unmarshal(dup, &item); err == nil {
		t.Error("DupError mode accepted duplicate keys into DataItem, want error")
	}
	var m map[int]int
	if err := strict.Unmarshal(dup, &m); err == nil {
		t.Error("DupError mode accepted duplicate keys into map, want error")
	}

	if err := cbor.Unmarshal(dup, &m); err != nil {
		t.Errorf("default Unmarshal rejected duplicate keys: %v", err)
	}
}

func TestStrictMode(t *testing.T) {
	strict, _ := cbor.DecoderOptions{Strict: true}.Decoding()
	for _, in := range []string{
		"0x1817",
		"0xa202010102",
		"0xfb3ff0000000000000",
	} {
		var v any
		if err := strict.Unmarshal(mustHex(t, in), &v); err == nil {
			t.Errorf("strict accepted %s, want error", in)
		}
		if err := cbor.Unmarshal(mustHex(t, in), &v); err != nil {
			t.Errorf("default rejected %s: %v", in, err)
		}
	}
	if err := strict.Valid(mustHex(t, "0x17")); err != nil {
		t.Errorf("strict rejected canonical input: %v", err)
	}
	if err := strict.Valid(mustHex(t, "0xa201020304")); err != nil {
		t.Errorf("strict rejected sorted map: %v", err)
	}
}

func TestOmitZero(t *testing.T) {
	type T struct {
		A int `cbor:"a,omitzero"`
		B int `cbor:"b"`
	}
	b, _ := cbor.Marshal(T{B: 5})
	if hexOf(t, b) != "0xa1616205" {
		t.Errorf("omitzero: got %s, want 0xa1616205", hexOf(t, b))
	}
}

func TestMaxStringLength(t *testing.T) {
	dm, _ := cbor.DecoderOptions{MaxStringLength: 4}.Decoding()

	var s string
	if err := dm.Unmarshal(mustHex(t, "0x6568656c6c6f"), &s); err == nil { // "hello" (5)
		t.Error("5-byte text string accepted under MaxStringLength=4, want error")
	}
	if err := dm.Unmarshal(mustHex(t, "0x6461626364"), &s); err != nil { // "abcd" (4)
		t.Errorf("4-byte text string rejected under MaxStringLength=4: %v", err)
	}

	var b []byte
	if err := dm.Unmarshal(mustHex(t, "0x450102030405"), &b); err == nil { // 5-byte bstr
		t.Error("5-byte byte string accepted under MaxStringLength=4, want error")
	}
}

func TestUntrustedDecOptionsHasStringCap(t *testing.T) {
	if cbor.UntrustedDecoderOptions().MaxStringLength <= 0 {
		t.Error("UntrustedDecoderOptions has no MaxStringLength cap")
	}
}

func TestOptionValidationErrors(t *testing.T) {
	for name, o := range map[string]cbor.EncoderOptions{
		"sort": {Sort: 99}, "time": {Time: 99}, "float": {Float: 99}, "nan": {NaN: 99}, "bigint": {BigInt: 99},
	} {
		if _, err := o.Encoding(); err == nil {
			t.Errorf("EncoderOptions %s=99: want error", name)
		}
	}
	if _, err := (cbor.DecoderOptions{DuplicateKeys: 99}).Decoding(); err == nil {
		t.Error("DecoderOptions bad DupMode: want error")
	}
}

func TestCanonicalDecoderOptions(t *testing.T) {
	dm, err := cbor.CanonicalDecoderOptions().Decoding()
	if err != nil {
		t.Fatalf("Decoding: %v", err)
	}
	// Each input is non-canonical in a different way and must be rejected.
	for _, tc := range []struct{ name, in string }{
		{"non-shortest int", "0x1817"},     // 24 encoded in 2 bytes
		{"unsorted keys", "0xa202010101"},  // {2:1, 1:1}
		{"indefinite array", "0x9f00ff"},   // indefinite length
		{"duplicate keys", "0xa201010102"}, // {1:1, 1:2}
	} {
		if err := dm.Valid(mustHex(t, tc.in)); err == nil {
			t.Errorf("%s (%s) accepted, want rejected", tc.name, tc.in)
		}
	}
	if err := dm.Valid(mustHex(t, "0xa201020304")); err != nil { // canonical {1:2,3:4}
		t.Errorf("canonical map rejected: %v", err)
	}
}

func TestUntrustedDecOptionsRejectsIndefinite(t *testing.T) {
	dm, err := cbor.UntrustedDecoderOptions().Decoding()
	if err != nil {
		t.Fatalf("Decoding: %v", err)
	}
	for _, in := range []string{
		"0x9f00ff",       // indefinite array
		"0xbf0001ff",     // indefinite map
		"0x5f4101ff",     // indefinite byte string
		"0x7f6161ff",     // indefinite text string
		"0xbf01010102ff", // indefinite map with duplicate key
	} {
		var v any
		if err := dm.Unmarshal(mustHex(t, in), &v); err == nil {
			t.Errorf("UntrustedDecoderOptions accepted indefinite input %s, want error", in)
		}
	}
}
