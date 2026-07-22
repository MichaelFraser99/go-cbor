package cbor_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	cbor "github.com/MichaelFraser99/go-cbor"
)

func FuzzValidMatchesDecode(f *testing.F) {
	for _, s := range []string{"0x01", "0x83010203", "0xa201020304", "0xc249010000000000000000",
		"0x1c", "0xff", "0x9f00ff", "0xc201", "0xf97e00", "0xd81845a10126"} {
		b, _ := hex.DecodeString(strings.TrimPrefix(s, "0x"))
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		validErr := cbor.Valid(data)
		var item cbor.DataItem
		decErr := cbor.Unmarshal(data, &item)
		if (validErr == nil) != (decErr == nil) {
			t.Fatalf("Valid=%v but Unmarshal(DataItem)=%v for %x", validErr, decErr, data)
		}
	})
}

var coverageVectors = []string{
	"0x00", "0x01", "0x0a", "0x17", "0x1818", "0x1903e8", "0x1a000f4240", "0x1b000000e8d4a51000",
	"0x1bffffffffffffffff", "0xc249010000000000000000", "0x3bffffffffffffffff", "0xc349010000000000000000",
	"0x20", "0x29", "0x3863", "0x3903e7",
	"0xf90000", "0xf93c00", "0xf93e00", "0xfa47c35000", "0xfb3ff199999999999a",
	"0xf97c00", "0xf9fc00", "0xf97e00",
	"0xf4", "0xf5", "0xf6", "0xf7", "0xf0", "0xf8ff",
	"0xc074323031332d30332d32315432303a30343a30305a", "0xc11a514b67b0", "0xc1fb41d452d9ec200000",
	"0xd74401020304", "0xd818456449455446",
	"0x40", "0x4401020304", "0x60", "0x6161", "0x6449455446",
	"0x80", "0x83010203", "0x8301820203820405", "0x826161a161626163",
	"0xa0", "0xa201020304", "0xa26161016162820203",
	"0x9f010203ff", "0xbf616101616202ff", "0x5f42010243030405ff", "0x7f6261626163ff",
}

func TestDecodeAllVectorsEveryPath(t *testing.T) {
	var stream bytes.Buffer
	for _, v := range coverageVectors {
		data := mustHex(t, v)
		var a any
		if err := cbor.Unmarshal(data, &a); err != nil {
			t.Errorf("Unmarshal(%s) into any: %v", v, err)
		}
		var item cbor.DataItem
		if err := cbor.Unmarshal(data, &item); err != nil {
			t.Errorf("Unmarshal(%s) into DataItem: %v", v, err)
		}
		_ = item.Native()
		if err := cbor.Valid(data); err != nil {
			t.Errorf("Valid(%s): %v", v, err)
		}
		if _, err := cbor.Diagnostic(data); err != nil {
			t.Errorf("Diagnostic(%s): %v", v, err)
		}
		stream.Write(data)
	}
	dec := cbor.NewDecoder(&stream)
	for i := range coverageVectors {
		var a any
		if err := dec.Decode(&a); err != nil {
			t.Fatalf("stream Decode item %d (%s): %v", i, coverageVectors[i], err)
		}
	}
}

func TestDecodeTruncatedAndMismatched(t *testing.T) {
	// Exercises the error/mismatch branches across every decoder without panicking.
	decodeInto := func(data []byte) {
		var (
			a    any
			item cbor.DataItem
			i    int
			u    uint
			s    string
			f    float64
			sl   []int
			ba   [2]int
			by   []byte
			m    map[string]int
			st   struct {
				A int `cbor:"a"`
			}
			tg  cbor.Tag
			tm  time.Time
			bi  big.Int
			raw cbor.RawMessage
		)
		_ = cbor.Unmarshal(data, &a)
		_ = cbor.Unmarshal(data, &item)
		_ = cbor.Unmarshal(data, &i)
		_ = cbor.Unmarshal(data, &u)
		_ = cbor.Unmarshal(data, &s)
		_ = cbor.Unmarshal(data, &f)
		_ = cbor.Unmarshal(data, &sl)
		_ = cbor.Unmarshal(data, &ba)
		_ = cbor.Unmarshal(data, &by)
		_ = cbor.Unmarshal(data, &m)
		_ = cbor.Unmarshal(data, &st)
		_ = cbor.Unmarshal(data, &tg)
		_ = cbor.Unmarshal(data, &tm)
		_ = cbor.Unmarshal(data, &bi)
		_ = cbor.Unmarshal(data, &raw)
		_ = cbor.Valid(data)
	}
	for _, v := range coverageVectors {
		data := mustHex(t, v)
		decodeInto(data)
		for cut := 1; cut < len(data); cut++ {
			decodeInto(data[:cut]) // every truncation length
		}
	}
}

func BenchmarkUnmarshalInt(b *testing.B) {
	data := []byte{0x1a, 0x00, 0x0f, 0x42, 0x40} // 1000000
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v int
		if err := cbor.Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalSliceInt(b *testing.B) {
	src := make([]int, 64)
	for i := range src {
		src[i] = i
	}
	data, _ := cbor.Marshal(src)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v []int
		if err := cbor.Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalMapStringInt(b *testing.B) {
	data, _ := cbor.Marshal(map[string]int{"alpha": 1, "beta": 2, "gamma": 3, "delta": 4, "epsilon": 5})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v map[string]int
		if err := cbor.Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalIntoAny(b *testing.B) {
	data, _ := cbor.Marshal(map[string]any{
		"id": 42, "items": []any{"alpha", "beta", "gamma"}, "ratio": 1.5, "data": []byte{1, 2, 3, 4},
	})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v any
		if err := cbor.Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValid(b *testing.B) {
	data, _ := cbor.Marshal(map[string]any{
		"id": 42, "items": []any{"alpha", "beta", "gamma"}, "ratio": 1.5, "data": []byte{1, 2, 3, 4},
	})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := cbor.Valid(data); err != nil {
			b.Fatal(err)
		}
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

func TestUnmarshalUint(t *testing.T) {
	var item cbor.DataItem
	if err := cbor.Unmarshal(mustHex(t, "0x1903e8"), &item); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if item.Major != cbor.MajorUint {
		t.Errorf("Major = %d, want MajorUint", item.Major)
	}
	if item.Argument != 1000 {
		t.Errorf("Argument = %d, want 1000", item.Argument)
	}
}

func TestUnmarshalNint(t *testing.T) {
	var item cbor.DataItem
	if err := cbor.Unmarshal(mustHex(t, "0x3863"), &item); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if item.Major != cbor.MajorNint {
		t.Errorf("Major = %d, want MajorNint", item.Major)
	}

	if item.Argument != 99 {
		t.Errorf("Argument = %d, want 99", item.Argument)
	}
}

func TestValidRejectsTrailingBytes(t *testing.T) {
	if err := cbor.Valid(mustHex(t, "0x0000")); err == nil {
		t.Error("Valid accepted trailing bytes, want error")
	}
}

func TestUnmarshalIntoAny(t *testing.T) {
	cases := []struct {
		input string
		want  any
	}{
		{"0x1903e8", int64(1000)},
		{"0x1bffffffffffffffff", uint64(18446744073709551615)},
		{"0x20", int64(-1)},
		{"0x3863", int64(-100)},
		{"0x6449455446", "IETF"},
		{"0x4401020304", []byte{1, 2, 3, 4}},
		{"0xf4", false},
		{"0xf5", true},
		{"0xf6", nil},
		{"0xfb3ff199999999999a", 1.1},
		{"0x83010203", []any{int64(1), int64(2), int64(3)}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			var v any
			if err := cbor.Unmarshal(mustHex(t, tc.input), &v); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(v, tc.want) {
				t.Errorf("got %#v (%T), want %#v (%T)", v, v, tc.want, tc.want)
			}
		})
	}
}

func TestUnmarshalMapIntoAnyYieldsMap(t *testing.T) {
	var v any
	if err := cbor.Unmarshal(mustHex(t, "0xa201020304"), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	m, ok := v.(cbor.Map)
	if !ok {
		t.Fatalf("got %T, want cbor.Map", v)
	}
	if len(m) != 2 {
		t.Fatalf("len = %d, want 2", len(m))
	}
	if got, _ := m.Get(int64(1)); got != int64(2) {
		t.Errorf("m.Get(1) = %v, want 2", got)
	}
	if got, _ := m.Get(int64(3)); got != int64(4) {
		t.Errorf("m.Get(3) = %v, want 4", got)
	}
	b, err := cbor.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if hexOf(t, b) != "0xa201020304" {
		t.Errorf("re-encode cbor.Map = %s, want 0xa201020304", hexOf(t, b))
	}
}

func TestUnmarshalRawMessage(t *testing.T) {
	data := mustHex(t, "0x83010203")
	var raw cbor.RawMessage
	if err := cbor.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !bytes.Equal(raw, data) {
		t.Errorf("raw = %x, want %x", []byte(raw), data)
	}
}

func TestMalformedInputErrors(t *testing.T) {
	for _, in := range []string{
		"0x5820ff",
		"0x790010ff",
		"0x8301",
		"0x9f0102",
		"0x1c",
		"0x00ff",
		"0x9affffffff00",
		"0xbaffffffff00",
	} {
		if err := cbor.Valid(mustHex(t, in)); err == nil {
			t.Errorf("Valid(%s) = nil, want error", in)
		}
	}
}

func TestFloatDecoding(t *testing.T) {
	cases := []struct {
		input string
		want  float64
		width uint8
	}{
		{"0xf93e00", 1.5, 2},
		{"0xf90001", 5.960464477539063e-08, 2},
		{"0xfa47c35000", 100000, 4},
		{"0xfb3ff199999999999a", 1.1, 8},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			var item cbor.DataItem
			if err := cbor.Unmarshal(mustHex(t, tc.input), &item); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if item.Float != tc.want {
				t.Errorf("Float = %v, want %v", item.Float, tc.want)
			}
			if item.FloatWidth != tc.width {
				t.Errorf("FloatWidth = %d, want %d", item.FloatWidth, tc.width)
			}
		})
	}
}

func TestLowFractionNaNDecodesAsNaN(t *testing.T) {
	for _, in := range []string{"0xfb7ff0000000000001", "0xfa7f800001"} {
		var item cbor.DataItem
		if err := cbor.Unmarshal(mustHex(t, in), &item); err != nil {
			t.Fatalf("Unmarshal(%s): %v", in, err)
		}
		if !math.IsNaN(item.Float) {
			t.Errorf("%s: Float = %v, want NaN", in, item.Float)
		}
	}
}

func TestBignumTagContentValidated(t *testing.T) {
	for _, in := range []string{"0xc201", "0xc3820102", "0xc36161"} {
		if err := cbor.Valid(mustHex(t, in)); err == nil {
			t.Errorf("Valid(%s) = nil, want error", in)
		}
	}
	if err := cbor.Valid(mustHex(t, "0xc249010000000000000000")); err != nil {
		t.Errorf("Valid(bignum) = %v, want nil", err)
	}
}

func TestMisplacedBreakAndOverflow(t *testing.T) {
	for _, in := range []string{"0xff", "0x81ff", "0x5b8000000000000000"} {
		if err := cbor.Valid(mustHex(t, in)); err == nil {
			t.Errorf("Valid(%s) = nil, want error", in)
		}
	}
}

func TestSyntaxErrorType(t *testing.T) {
	err := cbor.Valid(mustHex(t, "0x1c"))
	if _, ok := errors.AsType[*cbor.SyntaxError](err); !ok {
		t.Errorf("error %v (%T) is not *cbor.SyntaxError", err, err)
	}
}

func TestSyntaxErrorOffset(t *testing.T) {
	var item cbor.DataItem
	err := cbor.Unmarshal(mustHex(t, "0x8301021c"), &item)
	var se *cbor.SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v (%T), want *SyntaxError", err, err)
	}
	if se.Offset != 3 {
		t.Errorf("Offset = %d, want 3", se.Offset)
	}
}

func FuzzUnmarshal(f *testing.F) {
	seeds := []string{
		"0x00", "0x1bffffffffffffffff", "0x3bffffffffffffffff",
		"0x83010203", "0xa201020304", "0xc074616263",
		"0xfb3ff199999999999a", "0xf97e00", "0x9f01820203ff",
		"0x5f42010243030405ff", "0xd818456449455446",
	}
	for _, s := range seeds {
		if b, err := hex.DecodeString(strings.TrimPrefix(s, "0x")); err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var item cbor.DataItem
		if err := cbor.Unmarshal(data, &item); err == nil {
			if b, err := cbor.Marshal(item); err == nil {
				if err := cbor.Valid(b); err != nil {
					t.Fatalf("re-marshaled bytes are not valid: %v", err)
				}
			}
		}
		var v any
		_ = cbor.Unmarshal(data, &v)
	})
}

func FuzzRoundTrip(f *testing.F) {
	seeds := []string{
		"0x00", "0x83010203", "0xa201020304", "0xfb3ff199999999999a",
		"0x6449455446", "0x4401020304", "0xc074616263",
	}
	for _, s := range seeds {
		if b, err := hex.DecodeString(strings.TrimPrefix(s, "0x")); err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var item cbor.DataItem
		if err := cbor.Unmarshal(data, &item); err != nil {
			return
		}
		b1, err := cbor.Marshal(item)
		if err != nil {
			return
		}
		var item2 cbor.DataItem
		if err := cbor.Unmarshal(b1, &item2); err != nil {
			t.Fatalf("re-decode of marshaled item failed: %v", err)
		}
		b2, err := cbor.Marshal(item2)
		if err != nil {
			t.Fatalf("re-marshal failed: %v", err)
		}
		if string(b1) != string(b2) {
			t.Fatalf("round trip not stable: %x vs %x", b1, b2)
		}
	})
}

func TestUnmarshalAnyByteStringCopied(t *testing.T) {
	in := mustHex(t, "0x43010203") // byte string 01 02 03
	var v any
	if err := cbor.Unmarshal(in, &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	b := v.([]byte)
	in[2] = 0xff // mutate the input after decoding
	if b[1] == 0xff {
		t.Error("decoded []byte aliases the input buffer; want an independent copy")
	}
}

func TestUnmarshalDataItemByteStringCopied(t *testing.T) {
	in := mustHex(t, "0x43010203")
	var item cbor.DataItem
	if err := cbor.Unmarshal(in, &item); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	in[2] = 0xff
	if item.Bytes[1] == 0xff {
		t.Error("DataItem.Bytes aliases the input buffer; want an independent copy")
	}
}

func TestIndefiniteArrayRespectsElementCap(t *testing.T) {
	dm, _ := cbor.DecoderOptions{MaxArrayLen: 3}.Decoding()
	indefinite := mustHex(t, "0x9f0001020304ff")
	var v any
	if err := dm.Unmarshal(indefinite, &v); err == nil {
		t.Error("indefinite array of 5 elements accepted under MaxArrayLen=3, want error")
	}
	if err := dm.Valid(indefinite); err == nil {
		t.Error("Valid accepted over-cap indefinite array, want error")
	}
}

func TestIndefiniteMapRespectsPairCap(t *testing.T) {
	dm, _ := cbor.DecoderOptions{MaxMapLen: 1}.Decoding()
	indefinite := mustHex(t, "0xbf00000101ff")
	var v any
	if err := dm.Unmarshal(indefinite, &v); err == nil {
		t.Error("indefinite map of 2 pairs accepted under MaxMapLen=1, want error")
	}
}

func TestIndefiniteMapRespectsDupKey(t *testing.T) {
	dm, _ := cbor.DecoderOptions{DuplicateKeys: cbor.DupError}.Decoding()
	indefinite := mustHex(t, "0xbf01010102ff")
	var v any
	if err := dm.Unmarshal(indefinite, &v); err == nil {
		t.Error("indefinite map with duplicate key 1 accepted, want error")
	}
	if err := dm.Valid(indefinite); err == nil {
		t.Error("Valid accepted indefinite map with duplicate key, want error")
	}
}

func TestCanonicalRejectsNonMinimalBignum(t *testing.T) {
	dm, err := cbor.CanonicalDecoderOptions().Decoding()
	if err != nil {
		t.Fatal(err)
	}
	bad := map[string][]byte{
		"fits basic int":    {0xC2, 0x41, 0x00},
		"fits uint64":       {0xC2, 0x48, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		"empty":             {0xC2, 0x40},
		"leading zero long": {0xC2, 0x49, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}
	for name, data := range bad {
		t.Run(name, func(t *testing.T) {
			if err := dm.Valid(data); err == nil {
				t.Error("Valid accepted non-minimal bignum, want error")
			}
			var b *big.Int
			if err := dm.Unmarshal(data, &b); err == nil {
				t.Error("Unmarshal accepted non-minimal bignum, want error")
			}
		})
	}
	good := []byte{0xC2, 0x49, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if err := dm.Valid(good); err != nil {
		t.Errorf("Valid rejected minimal bignum 2^64: %v", err)
	}
	var b *big.Int
	if err := dm.Unmarshal(good, &b); err != nil {
		t.Errorf("Unmarshal rejected minimal bignum 2^64: %v", err)
	}
}
