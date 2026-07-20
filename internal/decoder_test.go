package internal

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"slices"
	"testing"

	"github.com/MichaelFraser99/go-cbor/model"
)

func TestDecode(t *testing.T) {
	t1 := []byte("test")
	t2 := make([]byte, 8)
	binary.BigEndian.PutUint64(t2, math.Float64bits(12.8))
	tjson := `{"data":-18446744073709551615}`
	tmap := map[string]any{}
	err := json.Unmarshal([]byte(tjson), &tmap)
	if err != nil {
		t.Error(err)
	}

	t.Log(t1)
	t.Log(string(t1))
	t.Log(t2)
	t.Log(string(t2))
	t.Log(math.Float64frombits(binary.BigEndian.Uint64(t2)))

	testCases := []struct {
		input        string
		expected     model.DataItem
		expectedJson string
	}{
		{
			"0x00",
			model.DataItem{
				MajorType: 0,
				Sign:      1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":0,"majorType":0}`,
		},
		{
			"0x01",
			model.DataItem{
				MajorType: 0,
				Sign:      1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1},
			},
			`{"data":1,"majorType":0}`,
		},
		{
			"0x0a",
			model.DataItem{
				MajorType: 0,
				Sign:      1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa},
			},
			`{"data":10,"majorType":0}`,
		},
		{
			"0x17",
			model.DataItem{
				MajorType: 0,
				Sign:      1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x17},
			},
			`{"data":23,"majorType":0}`,
		},
		{
			"0x1818",
			model.DataItem{
				MajorType: 0,
				Sign:      1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x18},
			},
			`{"data":24,"majorType":0}`,
		},
		{
			"0x1819",
			model.DataItem{
				MajorType: 0,
				Sign:      1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x19},
			},
			`{"data":25,"majorType":0}`,
		},
		{
			"0x1864",
			model.DataItem{
				MajorType: 0,
				Sign:      1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x64},
			},
			`{"data":100,"majorType":0}`,
		},
		{
			"0x1903e8",
			model.DataItem{
				MajorType: 0,
				Sign:      1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x03, 0xe8},
			},
			`{"data":1000,"majorType":0}`,
		},
		{
			"0x1a000f4240",
			model.DataItem{
				MajorType: 0,
				Sign:      1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0f, 0x42, 0x40},
			},
			`{"data":1000000,"majorType":0}`,
		},
		{
			"0x1b000000e8d4a51000",
			model.DataItem{
				MajorType: 0,
				Sign:      1,
				Data:      []byte{0x0, 0x0, 0x0, 0xe8, 0xd4, 0xa5, 0x10, 0x0},
			},
			`{"data":1000000000000,"majorType":0}`,
		},
		{
			"0x1bffffffffffffffff",
			model.DataItem{
				MajorType: 0,
				Sign:      1,
				Data:      []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			},
			`{"data":18446744073709551615,"majorType":0}`,
		},
		{
			"0xc249010000000000000000",
			model.DataItem{
				MajorType: 6,
				Sign:      1,
				Data:      []byte{0x49, 0x01, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":18446744073709551616,"majorType":6}`,
		},
		{
			"0x3bffffffffffffffff",
			model.DataItem{
				MajorType: 1,
				Sign:      -1,
				Data:      []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			},
			`{"data":-18446744073709551616,"majorType":1}`,
		},
		{
			"0xc349010000000000000000",
			model.DataItem{
				MajorType: 6,
				Sign:      -1,
				Data:      []byte{0x49, 0x01, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":-18446744073709551617,"majorType":6}`,
		},
		{
			"0x20",
			model.DataItem{
				MajorType: 1,
				Sign:      -1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":-1,"majorType":1}`,
		},
		{
			"0x29",
			model.DataItem{
				MajorType: 1,
				Sign:      -1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9},
			},
			`{"data":-10,"majorType":1}`,
		},
		{
			"0x3863",
			model.DataItem{
				MajorType: 1,
				Sign:      -1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x63},
			},
			`{"data":-100,"majorType":1}`,
		},
		{
			"0x3903e7",
			model.DataItem{
				MajorType: 1,
				Sign:      -1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x03, 0xe7},
			},
			`{"data":-1000,"majorType":1}`,
		},
		{
			"0xf90000", model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":0.0,"majorType":7}`,
		},
		{
			"0xf98000",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":-0.0,"majorType":7}`,
		},
		{
			"0xf93c00",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x3f, 0xf0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":1.0,"majorType":7}`,
		},
		{
			"0xfb3ff199999999999a",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x3f, 0xf1, 0x99, 0x99, 0x99, 0x99, 0x99, 0x9a},
			},
			`{"data":1.1,"majorType":7}`,
		},
		{
			"0xf93e00",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x3f, 0xf8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":1.5,"majorType":7}`,
		},
		{
			"0xf97bff",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x40, 0xef, 0xfc, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":65504.0,"majorType":7}`,
		},
		{
			"0xfa47c35000",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x40, 0xf8, 0x6a, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":100000.0,"majorType":7}`,
		},
		{
			"0xfa7f7fffff",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x47, 0xef, 0xff, 0xff, 0xe0, 0x0, 0x0, 0x0},
			},
			`{"data":340282346638528859811704183484516925440.0,"majorType":7}`,
		},
		{
			"0xfb7e37e43c8800759c",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x7e, 0x37, 0xe4, 0x3c, 0x88, 0x0, 0x75, 0x9c},
			},
			`{"data":1000000000000000052504760255204420248704468581108159154915854115511802457988908195786371375080447864043704443832883878176942523235360430575644792184786706982848387200926575803737830233794788090059368953234970799945081119038967640880074652742780142494579258788820056842838115669472196386865459400540160.0,"majorType":7}`,
		},
		{
			"0xf90001",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x3e, 0x70, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":5.960464477539063e-8,"majorType":7}`,
		},
		{
			"0xf90400",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x3f, 0x10, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":0.00006103515625,"majorType":7}`,
		},
		{
			"0xf9c400",
			model.DataItem{
				MajorType: 7,
				Sign:      -1,
				Data:      []byte{0xc0, 0x10, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":-4.0,"majorType":7}`,
		},
		{
			"0xfbc010666666666666",
			model.DataItem{
				MajorType: 7,
				Sign:      -1,
				Data:      []byte{0xc0, 0x10, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66},
			},
			`{"data":-4.1,"majorType":7}`,
		},
		{
			"0xf97c00",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x7f, 0xf0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":"Infinity","majorType":7}`,
		},
		{
			"0xf97e00",
			model.DataItem{
				MajorType: 7,
				Sign:      -1,
				Data:      []byte{0x7f, 0xf8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x01},
			},
			`{"data":"NaN","majorType":7}`,
		},
		{
			"0xf9fc00",
			model.DataItem{
				MajorType: 7,
				Sign:      -1,
				Data:      []byte{0xff, 0xf0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":"-Infinity","majorType":7}`,
		},
		{
			"0xfa7f800000",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x7f, 0xf0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":"Infinity","majorType":7}`,
		},
		{
			"0xfa7fc00000",
			model.DataItem{
				MajorType: 7,
				Sign:      -1,
				Data:      []byte{0x7f, 0xf8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x01},
			},
			`{"data":"NaN","majorType":7}`,
		},
		{
			"0xfaff800000",
			model.DataItem{
				MajorType: 7,
				Sign:      -1,
				Data:      []byte{0xff, 0xf0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":"-Infinity","majorType":7}`,
		},
		{
			"0xfb7ff0000000000000",
			model.DataItem{
				MajorType: 7,
				Sign:      1,
				Data:      []byte{0x7f, 0xf0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":"Infinity","majorType":7}`,
		},
		{
			"0xfb7ff8000000000000",
			model.DataItem{
				MajorType: 7,
				Sign:      -1,
				Data:      []byte{0x7f, 0xf8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x01},
			},
			`{"data":"NaN","majorType":7}`,
		},
		{
			"0xfbfff0000000000000",
			model.DataItem{
				MajorType: 7,
				Sign:      -1,
				Data:      []byte{0xff, 0xf0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			`{"data":"-Infinity","majorType":7}`,
		},
		{
			"0xf4",
			model.DataItem{
				MajorType: 7,
				Sign:      0,
				Data:      []byte{0x66, 0x61, 0x6c, 0x73, 0x65},
			},
			`{"data":false,"majorType":7}`,
		},
		{
			"0xf5",
			model.DataItem{
				MajorType: 7,
				Sign:      0,
				Data:      []byte{0x74, 0x72, 0x75, 0x65},
			},
			`{"data":true,"majorType":7}`,
		},
		{
			"0xf6",
			model.DataItem{
				MajorType: 7,
				Sign:      0,
				Data:      []byte{0x3c, 0x6e, 0x69, 0x6c, 0x3e},
			},
			`{"data":null,"majorType":7}`,
		},
		{
			"0xf7",
			model.DataItem{
				MajorType: 7,
				Sign:      0,
				Data:      []byte{0x75, 0x6e, 0x64, 0x65, 0x66, 0x69, 0x6e, 0x65, 0x64},
			},
			`{"data":"undefined","majorType":7}`,
		},
		{
			"0xf0",
			model.DataItem{
				MajorType: 7,
				Sign:      0,
				Data:      []byte{16},
			},
			`{"data":16,"majorType":7}`,
		},
		{
			"0xf8ff",
			model.DataItem{
				MajorType: 7,
				Sign:      0,
				Data:      []byte{255},
			},
			`{"data":255,"majorType":7}`,
		},
		{
			"0xc074323031332d30332d32315432303a30343a30305a",
			model.DataItem{
				MajorType: 6,
				Sign:      0,
				Data:      append([]byte{0x74}, []byte("2013-03-21T20:04:00Z")...),
			},
			`{"data":"2013-03-21T20:04:00Z","majorType":6}`,
		},
		{
			"0xc11a514b67b0",
			model.DataItem{
				MajorType: 6,
				Sign:      0,
				Data:      []byte{0x1a, 0x51, 0x4b, 0x67, 0xb0},
			},
			`{"data":1363896240,"majorType":6}`,
		},
		{
			"0xc1fb41d452d9ec200000",
			model.DataItem{
				MajorType: 6,
				Sign:      0,
				Data:      []byte{0xfb, 0x41, 0xd4, 0x52, 0xd9, 0xec, 0x20, 0x00, 0x00},
			},
			`{"data":1363896240.5,"majorType":6}`,
		},
		{
			"0xd74401020304",
			model.DataItem{
				MajorType: 6,
				Sign:      0,
				Data:      []byte{0x44, 0x01, 0x02, 0x03, 0x04},
			},
			`{"data":"AQIDBA","majorType":6}`,
		},
		{
			"0xd818456449455446",
			model.DataItem{
				MajorType: 6,
				Sign:      0,
				Data:      []byte{0x45, 0x64, 0x49, 0x45, 0x54, 0x46},
			},
			`{"data":"ZElFVEY","majorType":6}`,
		},
		{
			"0xd82076687474703a2f2f7777772e6578616d706c652e636f6d",
			model.DataItem{
				MajorType: 6,
				Sign:      0,
				Data:      append([]byte{0x76}, []byte("http://www.example.com")...),
			},
			`{"data":"http://www.example.com","majorType":6}`,
		},
		{
			"0x4401020304",
			model.DataItem{
				MajorType: 2,
				Sign:      0,
				Data:      []byte{0x01, 0x02, 0x03, 0x04},
			},
			`{"data":"AQIDBA","majorType":2}`,
		},
		{
			"0x40",
			model.DataItem{
				MajorType: 2,
				Sign:      0,
				Data:      []byte{},
			},
			`{"data":"","majorType":2}`,
		},
		{
			"0x60",
			model.DataItem{
				MajorType: 3,
				Sign:      0,
				Data:      []byte{},
			},
			`{"data":"","majorType":3}`,
		},
		{
			"0x6161",
			model.DataItem{
				MajorType: 3,
				Sign:      0,
				Data:      []byte("a"),
			},
			`{"data":"a","majorType":3}`,
		},
		{
			"0x6449455446",
			model.DataItem{
				MajorType: 3,
				Sign:      0,
				Data:      []byte("IETF"),
			},
			`{"data":"IETF","majorType":3}`,
		},
		{
			"0x62225c",
			model.DataItem{
				MajorType: 3,
				Sign:      0,
				Data:      []byte("\"\\"),
			},
			`{"data":"\"\\","majorType":3}`,
		},
		{
			"0x62c3bc",
			model.DataItem{
				MajorType: 3,
				Sign:      0,
				Data:      []byte("ü"),
			},
			`{"data":"ü","majorType":3}`,
		},
		{
			"0x63e6b0b4",
			model.DataItem{
				MajorType: 3,
				Sign:      0,
				Data:      []byte("水"),
			},
			`{"data":"水","majorType":3}`,
		},
		{
			"0x64f0908591",
			model.DataItem{
				MajorType: 3,
				Sign:      0,
				Data:      []byte("𐅑"),
			},
			`{"data":"𐅑","majorType":3}`,
		},
		{
			"0x80",
			model.DataItem{
				MajorType: 4,
				Sign:      0,
			},
			`{"data":[],"majorType":4}`,
		},
		{
			"0x83010203",
			model.DataItem{
				MajorType: 4,
				Sign:      0,
			},
			`{"data":[1,2,3],"majorType":4}`,
		},
		{
			"0x8301820203820405",
			model.DataItem{
				MajorType: 4,
				Sign:      0,
			},
			`{"data":[1,[2,3],[4,5]],"majorType":4}`,
		},
		{
			"0x98190102030405060708090a0b0c0d0e0f101112131415161718181819",
			model.DataItem{
				MajorType: 4,
				Sign:      0,
			},
			`{"data":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25],"majorType":4}`,
		},
		{
			"0x826161a161626163",
			model.DataItem{
				MajorType: 4,
				Sign:      0,
			},
			`{"data":["a",[{"key":"b","value":"c"}]],"majorType":4}`,
		},
		{
			"0xa0",
			model.DataItem{
				MajorType: 5,
				Sign:      0,
			},
			`{"data":[],"majorType":5}`,
		},
		{
			"0xa201020304",
			model.DataItem{
				MajorType: 5,
				Sign:      0,
			},
			`{"data":[{"key":1,"value":2},{"key":3,"value":4}],"majorType":5}`,
		},
		{
			"0xa26161016162820203",
			model.DataItem{
				MajorType: 5,
				Sign:      0,
			},
			`{"data":[{"key":"a","value":1},{"key":"b","value":[2,3]}],"majorType":5}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := DecodeFromHexString(tc.input)
			if err != nil {
				t.Fatalf("DecodeFromHexString(%s) returned error: %v", tc.input, err)
			}
			if !slices.Equal(result.Data, tc.expected.Data) {
				t.Errorf("DecodeFromHexString(%s) returned data %v, want %v", tc.input, result.Data, tc.expected.Data)
			}
			if result.Sign != tc.expected.Sign {
				t.Errorf("DecodeFromHexString(%s) returned sign %d, want %d", tc.input, result.Sign, tc.expected.Sign)
			}
			if result.MajorType != tc.expected.MajorType {
				t.Errorf("DecodeFromHexString(%s) returned major type %d, want %d", tc.input, result.MajorType, tc.expected.MajorType)
			}

			jsonResult, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err.Error())
			}

			if string(jsonResult) != tc.expectedJson {
				t.Errorf("marshalling response from DecodeFromHexString(%s) returned json %s, want %s", tc.input, string(jsonResult), tc.expectedJson)
			}
			t.Log(string(jsonResult))
		})
	}

	// Malformed inputs must return an error, never panic.
	errorCases := []string{
		// major type 2 (byte string), additional info 24 => 1-byte length
		// 0x20 = 32, but only 1 content byte follows.
		"0x5820ff",
		// major type 3 (text string), additional info 25 => 2-byte length
		// 0x0010 = 16, but only 1 content byte follows.
		"0x790010ff",
		// major type 4 (array) claiming 3 elements but only 1 follows.
		"0x8301",
		// major type 4 (array), indefinite length (additional info 31) is
		// not yet supported and must error rather than mis-decode.
		"0x9f00ff",
	}
	for _, input := range errorCases {
		t.Run("error/"+input, func(t *testing.T) {
			if _, err := DecodeFromHexString(input); err == nil {
				t.Errorf("DecodeFromHexString(%s) = nil error, want error", input)
			}
		})
	}
}

func Test_floatDecoder(t *testing.T) {
	result, err := floatDecoder([]byte{0x3e, 0x00})
	if err != nil {
		t.Error(err)
	}
	if *result != 1.5 {
		t.Error("expected 1.5, got", result)
	}
	result, err = floatDecoder([]byte{0x49, 0x74, 0x24, 0x08})
	if err != nil {
		t.Error(err)
	}
	if *result != 1000000.5 {
		t.Error("expected 1000000.5, got", result)
	}
	result, err = floatDecoder([]byte{0x3f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	if err != nil {
		t.Error(err)
	}
	if *result != 1.0 {
		t.Error("expected 1.0, got", result)
	}

	// NaN with fraction bits only in the low bytes: exponent all ones, a
	// single trailing fraction bit. Must decode as NaN, not Infinity.
	result, err = floatDecoder([]byte{0x7f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01})
	if err != nil {
		t.Error(err)
	}
	if !math.IsNaN(*result) {
		t.Error("expected NaN for double 0x7ff0000000000001, got", *result)
	}
	result, err = floatDecoder([]byte{0x7f, 0x80, 0x00, 0x01})
	if err != nil {
		t.Error(err)
	}
	if !math.IsNaN(*result) {
		t.Error("expected NaN for single 0x7f800001, got", *result)
	}
}
