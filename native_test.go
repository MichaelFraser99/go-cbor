package cbor_test

import (
	"math/big"
	"testing"

	cbor "github.com/MichaelFraser99/go-cbor"
)

func TestNativeResolvesBignumTags(t *testing.T) {
	positive := cbor.TagOf(2, cbor.ByteString([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0}))
	got := positive.Native()
	want, _ := new(big.Int).SetString("18446744073709551616", 10)
	b, ok := got.(*big.Int)
	if !ok {
		t.Fatalf("tag 2 Native() = %T, want *big.Int", got)
	}
	if b.Cmp(want) != 0 {
		t.Errorf("tag 2 Native() = %v, want %v", b, want)
	}

	negative := cbor.TagOf(3, cbor.ByteString([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0}))
	got = negative.Native()
	want, _ = new(big.Int).SetString("-18446744073709551617", 10)
	b, ok = got.(*big.Int)
	if !ok {
		t.Fatalf("tag 3 Native() = %T, want *big.Int", got)
	}
	if b.Cmp(want) != 0 {
		t.Errorf("tag 3 Native() = %v, want %v", b, want)
	}
}

func TestUndefinedDistinctInAny(t *testing.T) {
	var v any
	if err := cbor.Unmarshal(mustHex(t, "0xf7"), &v); err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(cbor.Undefined); !ok {
		t.Errorf("undefined decoded as %T (%v), want cbor.Undefined", v, v)
	}
	b, err := cbor.Marshal(cbor.Undefined{})
	if err != nil {
		t.Fatal(err)
	}
	if hexOf(t, b) != "0xf7" {
		t.Errorf("Marshal(Undefined{}) = %s, want 0xf7", hexOf(t, b))
	}
}

func TestSimpleValueString(t *testing.T) {
	if got := cbor.SimpleValue(200).String(); got != "simple(200)" {
		t.Errorf("SimpleValue(200).String() = %q, want simple(200)", got)
	}
	// undefined (23) still decodes to SimpleValue when the target is SimpleValue.
	var s cbor.SimpleValue
	if err := cbor.Unmarshal(mustHex(t, "0xf0"), &s); err != nil {
		t.Fatal(err)
	}
	if s != 16 {
		t.Errorf("SimpleValue = %d, want 16", byte(s))
	}
}

func TestUnmarshalBignumTagIntoAny(t *testing.T) {
	var v any
	if err := cbor.Unmarshal(mustHex(t, "0xc249010000000000000000"), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	b, ok := v.(*big.Int)
	if !ok {
		t.Fatalf("tag 2 into any = %T, want *big.Int", v)
	}
	want, _ := new(big.Int).SetString("18446744073709551616", 10)
	if b.Cmp(want) != 0 {
		t.Errorf("tag 2 into any = %v, want %v", b, want)
	}
}
