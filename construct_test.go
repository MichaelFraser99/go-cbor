package cbor_test

import (
	"testing"

	cbor "github.com/MichaelFraser99/go-cbor"
)

func TestConstructors(t *testing.T) {
	cases := []struct {
		name string
		item *cbor.DataItem
		want string
	}{
		{"uint", cbor.Uint(1000), "0x1903e8"},
		{"nint", cbor.Nint(99), "0x3863"},
		{"int_neg", cbor.Int(-100), "0x3863"},
		{"int_pos", cbor.Int(1000), "0x1903e8"},
		{"text", cbor.Text("IETF"), "0x6449455446"},
		{"bytes", cbor.ByteString([]byte{1, 2, 3, 4}), "0x4401020304"},
		{"array", cbor.ArrayOf(cbor.Uint(1), cbor.Uint(2), cbor.Uint(3)), "0x83010203"},
		{"map", cbor.MapOf(cbor.Uint(1), cbor.Uint(2)), "0xa10102"},
		{"tag", cbor.TagOf(0, cbor.Text("x")), "0xc06178"},
		{"bool", cbor.Bool(true), "0xf5"},
		{"null", cbor.Null(), "0xf6"},
		{"simple", cbor.Simple(255), "0xf8ff"},
		{"nested", cbor.ArrayOf(cbor.Uint(1), cbor.Text("a"), cbor.TagOf(0, cbor.Text("x"))), "0x83016161c06178"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := cbor.Marshal(tc.item)
			if err != nil {
				t.Fatal(err)
			}
			if hexOf(t, b) != tc.want {
				t.Errorf("got %s, want %s", hexOf(t, b), tc.want)
			}
		})
	}
}
