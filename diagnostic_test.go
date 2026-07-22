package cbor_test

import (
	"encoding/json"
	"testing"

	cbor "github.com/MichaelFraser99/go-cbor"
)

func TestDiagnosticJSON(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{

		{"0x00", `{"data":0,"majorType":0}`},
		{"0x01", `{"data":1,"majorType":0}`},
		{"0x0a", `{"data":10,"majorType":0}`},
		{"0x17", `{"data":23,"majorType":0}`},
		{"0x1818", `{"data":24,"majorType":0}`},
		{"0x1819", `{"data":25,"majorType":0}`},
		{"0x1864", `{"data":100,"majorType":0}`},
		{"0x1903e8", `{"data":1000,"majorType":0}`},
		{"0x1a000f4240", `{"data":1000000,"majorType":0}`},
		{"0x1b000000e8d4a51000", `{"data":1000000000000,"majorType":0}`},
		{"0x1bffffffffffffffff", `{"data":"18446744073709551615","majorType":0}`},

		{"0x20", `{"data":-1,"majorType":1}`},
		{"0x29", `{"data":-10,"majorType":1}`},
		{"0x3863", `{"data":-100,"majorType":1}`},
		{"0x3903e7", `{"data":-1000,"majorType":1}`},
		{"0x3bffffffffffffffff", `{"data":"-18446744073709551616","majorType":1}`},

		{"0xf90000", `{"data":0.0,"majorType":7}`},
		{"0xf98000", `{"data":-0.0,"majorType":7}`},
		{"0xf93c00", `{"data":1.0,"majorType":7}`},
		{"0xfb3ff199999999999a", `{"data":1.1,"majorType":7}`},
		{"0xf93e00", `{"data":1.5,"majorType":7}`},
		{"0xf97bff", `{"data":65504.0,"majorType":7}`},
		{"0xfa47c35000", `{"data":100000.0,"majorType":7}`},
		{"0xfa7f7fffff", `{"data":340282346638528859811704183484516925440.0,"majorType":7}`},
		{"0xf90001", `{"data":5.960464477539063e-8,"majorType":7}`},
		{"0xf90400", `{"data":0.00006103515625,"majorType":7}`},
		{"0xf9c400", `{"data":-4.0,"majorType":7}`},
		{"0xfbc010666666666666", `{"data":-4.1,"majorType":7}`},
		{"0xf97c00", `{"data":"Infinity","majorType":7}`},
		{"0xf97e00", `{"data":"NaN","majorType":7}`},
		{"0xf9fc00", `{"data":"-Infinity","majorType":7}`},
		{"0xfa7f800000", `{"data":"Infinity","majorType":7}`},
		{"0xfa7fc00000", `{"data":"NaN","majorType":7}`},
		{"0xfaff800000", `{"data":"-Infinity","majorType":7}`},
		{"0xfb7ff0000000000000", `{"data":"Infinity","majorType":7}`},
		{"0xfb7ff8000000000000", `{"data":"NaN","majorType":7}`},
		{"0xfbfff0000000000000", `{"data":"-Infinity","majorType":7}`},

		{"0xf4", `{"data":false,"majorType":7}`},
		{"0xf5", `{"data":true,"majorType":7}`},
		{"0xf6", `{"data":null,"majorType":7}`},
		{"0xf7", `{"data":"undefined","majorType":7}`},
		{"0xf0", `{"data":16,"majorType":7}`},
		{"0xf8ff", `{"data":255,"majorType":7}`},

		{"0x4401020304", `{"data":"AQIDBA","majorType":2}`},
		{"0x40", `{"data":"","majorType":2}`},
		{"0x60", `{"data":"","majorType":3}`},
		{"0x6161", `{"data":"a","majorType":3}`},
		{"0x6449455446", `{"data":"IETF","majorType":3}`},
		{"0x62225c", `{"data":"\"\\","majorType":3}`},
		{"0x62c3bc", `{"data":"ü","majorType":3}`},
		{"0x63e6b0b4", `{"data":"水","majorType":3}`},
		{"0x64f0908591", `{"data":"𐅑","majorType":3}`},

		{"0xc249010000000000000000", `{"data":"18446744073709551616","majorType":6}`},
		{"0xc349010000000000000000", `{"data":"-18446744073709551617","majorType":6}`},
		{"0xc074323031332d30332d32315432303a30343a30305a", `{"data":"2013-03-21T20:04:00Z","majorType":6}`},
		{"0xc11a514b67b0", `{"data":1363896240,"majorType":6}`},
		{"0xc1fb41d452d9ec200000", `{"data":1363896240.5,"majorType":6}`},
		{"0xd74401020304", `{"data":"AQIDBA","majorType":6}`},
		{"0xd818456449455446", `{"data":"ZElFVEY","majorType":6}`},
		{"0xd82076687474703a2f2f7777772e6578616d706c652e636f6d", `{"data":"http://www.example.com","majorType":6}`},

		{"0x80", `{"data":[],"majorType":4}`},
		{"0x83010203", `{"data":[1,2,3],"majorType":4}`},
		{"0x8301820203820405", `{"data":[1,[2,3],[4,5]],"majorType":4}`},
		{"0x98190102030405060708090a0b0c0d0e0f101112131415161718181819", `{"data":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25],"majorType":4}`},
		{"0x826161a161626163", `{"data":["a",[{"key":"b","value":"c"}]],"majorType":4}`},

		{"0xa0", `{"data":[],"majorType":5}`},
		{"0xa201020304", `{"data":[{"key":1,"value":2},{"key":3,"value":4}],"majorType":5}`},
		{"0xa26161016162820203", `{"data":[{"key":"a","value":1},{"key":"b","value":[2,3]}],"majorType":5}`},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			var item cbor.DataItem
			if err := cbor.Unmarshal(mustHex(t, tc.input), &item); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			got, err := json.Marshal(item)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("json = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestIndefiniteDiagnostic(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"0x9fff", `{"data":[],"majorType":4}`},
		{"0x9f01820203820405ff", `{"data":[1,[2,3],[4,5]],"majorType":4}`},
		{"0xbf61610161629f0203ffff", `{"data":[{"key":"a","value":1},{"key":"b","value":[2,3]}],"majorType":5}`},
		{"0x5f42010243030405ff", `{"data":"AQIDBAU","majorType":2}`},
		{"0x7f657374726561646d696e67ff", `{"data":"streaming","majorType":3}`},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			var item cbor.DataItem
			if err := cbor.Unmarshal(mustHex(t, tc.input), &item); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			got, _ := json.Marshal(item)
			if string(got) != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDiagnostic(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"0x83010203", "[1, 2, 3]"},
		{"0xa201020304", "{1: 2, 3: 4}"},
		{"0x4401020304", "h'01020304'"},
		{"0x6449455446", `"IETF"`},
		{"0xf5", "true"},
		{"0xf6", "null"},
		{"0xf7", "undefined"},
		{"0x20", "-1"},
		{"0xf93e00", "1.5"},
		{"0xf93c00", "1.0"},
		{"0xf97e00", "NaN"},
		{"0xc074323031332d30332d32315432303a30343a30305a", `0("2013-03-21T20:04:00Z")`},
		{"0xc249010000000000000000", "2(h'010000000000000000')"},
		{"0x826161a161626163", `["a", {"b": "c"}]`},
	}
	for _, tc := range cases {
		got, err := cbor.Diagnostic(mustHex(t, tc.in))
		if err != nil {
			t.Errorf("Diagnostic(%s): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Diagnostic(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDiagnosticEdges(t *testing.T) {
	if _, err := cbor.Diagnostic(mustHex(t, "0x0000")); err == nil {
		t.Error("Diagnostic with trailing bytes: want error")
	}
	if _, err := cbor.Diagnostic(mustHex(t, "0x1c")); err == nil {
		t.Error("Diagnostic of malformed: want error")
	}
	cases := []struct{ in, want string }{
		{"0xf8ff", "simple(255)"},
		{"0xf97c00", "Infinity"},
		{"0xf9fc00", "-Infinity"},
		{"0x9f010203ff", "[1, 2, 3]"},
		{"0xbf01020304ff", "{1: 2, 3: 4}"},
		{"0x00", "0"},
	}
	for _, tc := range cases {
		got, err := cbor.Diagnostic(mustHex(t, tc.in))
		if err != nil {
			t.Errorf("Diagnostic(%s): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Diagnostic(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
