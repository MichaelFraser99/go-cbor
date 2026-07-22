package cbor_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	cbor "github.com/MichaelFraser99/go-cbor"
)

func TestStreamingRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	enc := cbor.NewEncoder(&buf)
	if err := enc.Encode(1000); err != nil {
		t.Fatal(err)
	}
	if err := enc.Encode("hi"); err != nil {
		t.Fatal(err)
	}
	if err := enc.Encode([]int{1, 2, 3}); err != nil {
		t.Fatal(err)
	}

	dec := cbor.NewDecoder(&buf)
	var a int
	if err := dec.Decode(&a); err != nil || a != 1000 {
		t.Fatalf("a = %d, err = %v", a, err)
	}
	var b string
	if err := dec.Decode(&b); err != nil || b != "hi" {
		t.Fatalf("b = %q, err = %v", b, err)
	}
	var c []int
	if err := dec.Decode(&c); err != nil || !reflect.DeepEqual(c, []int{1, 2, 3}) {
		t.Fatalf("c = %v, err = %v", c, err)
	}
	var d int
	if err := dec.Decode(&d); err != io.EOF {
		t.Fatalf("final Decode = %v, want io.EOF", err)
	}
}

func TestDecoderStreamsIncrementally(t *testing.T) {
	pr, pw := io.Pipe()
	dec := cbor.NewDecoder(pr)
	gate := make(chan struct{})
	errc := make(chan error, 1)

	go func() {
		enc := cbor.NewEncoder(pw)
		if err := enc.Encode(1); err != nil {
			errc <- err
			return
		}
		<-gate // block with the pipe still open until the reader has item 1
		if err := enc.Encode("two"); err != nil {
			errc <- err
			return
		}
		errc <- pw.Close()
	}()

	var a int
	if err := dec.Decode(&a); err != nil {
		t.Fatalf("Decode 1: %v", err)
	}
	if a != 1 {
		t.Fatalf("a = %d, want 1", a)
	}
	close(gate) // reader got item 1 while the pipe is still open -> proves incremental

	var s string
	if err := dec.Decode(&s); err != nil {
		t.Fatalf("Decode 2: %v", err)
	}
	if s != "two" {
		t.Fatalf("s = %q, want two", s)
	}

	var x int
	if err := dec.Decode(&x); err != io.EOF {
		t.Fatalf("final Decode = %v, want io.EOF", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("writer: %v", err)
	}
}

func FuzzDecoderStream(f *testing.F) {
	f.Add([]byte{0x9f, 0x01, 0x02, 0x03, 0xff})
	f.Add([]byte{0x83, 0x01, 0x02})
	f.Add([]byte{0x1c})
	f.Add([]byte{0xa1, 0x61, 0x61, 0x01})
	f.Fuzz(func(t *testing.T, data []byte) {
		dec := cbor.NewDecoder(bytes.NewReader(data))
		for i := 0; i < len(data)+2; i++ {
			var v any
			if err := dec.Decode(&v); err != nil {
				break
			}
		}
	})
}

func TestDecoderDeepNestNoCrash(t *testing.T) {
	if os.Getenv("CBOR_DEEP_CHILD") == "1" {
		n := 6_000_000
		buf := make([]byte, 0, n+1)
		for i := 0; i < n; i++ {
			buf = append(buf, 0x81) // array-of-1 header, nested
		}
		buf = append(buf, 0x00)
		dec := cbor.NewDecoder(bytes.NewReader(buf))
		var v any
		_ = dec.Decode(&v) // must return an error, must NOT crash the process
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestDecoderDeepNestNoCrash$", "-test.timeout=60s")
	cmd.Env = append(os.Environ(), "CBOR_DEEP_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("framing a deeply-nested stream crashed the process (want clean error): %v\n%s", err, out)
	}
}

func TestDecoderRejectsDeepNestWithCap(t *testing.T) {
	var buf bytes.Buffer
	for i := 0; i < 100; i++ {
		buf.WriteByte(0x81)
	}
	buf.WriteByte(0x00)
	dm, err := cbor.DecoderOptions{MaxNestingDepth: 16}.NewDecoder(&buf)
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	var v any
	if err := dm.Decode(&v); err == nil {
		t.Error("Decode of 100-deep nesting under MaxNestingDepth=16 succeeded, want error")
	}
}

func TestDecoderFramingRejectsIndefiniteUnderPreset(t *testing.T) {
	// Unterminated indefinite array: 0x9f then bytes, no break. Finite reader so the
	// test can't hang; the bug would buffer to EOF and return ErrUnexpectedEOF.
	data := append([]byte{0x9f}, make([]byte, 32)...)
	dm, err := cbor.UntrustedDecoderOptions().NewDecoder(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var v any
	err = dm.Decode(&v)
	if err == nil {
		t.Fatal("indefinite item accepted under UntrustedDecoderOptions, want error")
	}
	if err == io.ErrUnexpectedEOF {
		t.Errorf("framing ignored RejectIndefinite (buffered to EOF): got %v, want an indefinite-not-allowed error", err)
	}
}

func TestDecoderFramingStringCapError(t *testing.T) {
	data := append([]byte{0x78, 0x64}, make([]byte, 10)...) // text string, declared len 100, short body
	dm, err := cbor.DecoderOptions{MaxStringLength: 4}.NewDecoder(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var v any
	err = dm.Decode(&v)
	if err == nil {
		t.Fatal("over-cap string accepted, want error")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("streaming string-cap error = %q, want it to mention the limit", err.Error())
	}
}

func TestDecoderFramingArrayCapEarly(t *testing.T) {
	data := append([]byte{0x9a, 0x00, 0x0f, 0x42, 0x40}, make([]byte, 8)...) // array declared 1,000,000, tiny body
	dm, err := cbor.DecoderOptions{MaxArrayLen: 16}.NewDecoder(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var v any
	err = dm.Decode(&v)
	if err == nil {
		t.Fatal("over-cap array accepted, want error")
	}
	if err == io.ErrUnexpectedEOF {
		t.Errorf("array cap not applied during framing (buffered whole body): got %v", err)
	}
}

func TestDecoderFramingIndefiniteArrayCapEarly(t *testing.T) {
	data := append([]byte{0x9f}, make([]byte, 20)...) // indefinite array, 20 elements, no break, then EOF
	dm, err := cbor.DecoderOptions{MaxArrayLen: 16}.NewDecoder(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var v any
	err = dm.Decode(&v)
	if err == nil {
		t.Fatal("over-cap indefinite array accepted, want error")
	}
	if err == io.ErrUnexpectedEOF {
		t.Errorf("indefinite array cap not applied during framing (buffered to EOF): got %v", err)
	}
}

type dripReader struct {
	data []byte
	pos  int
}

func (r *dripReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}

func TestDecoderFramesLargeItemOneBytePerRead(t *testing.T) {
	src := make([]any, 400)
	for i := range src {
		src[i] = []any{int64(i), map[string]any{"k": int64(i)}}
	}
	encoded, err := cbor.Marshal(src)
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := cbor.Unmarshal(encoded, &want); err != nil {
		t.Fatal(err)
	}
	dec := cbor.NewDecoder(&dripReader{data: encoded})
	var got any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("streaming decode of large item failed: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("byte-drip stream decode mismatch")
	}
	if dec.InputOffset() != int64(len(encoded)) {
		t.Errorf("InputOffset = %d, want %d", dec.InputOffset(), len(encoded))
	}
}

func TestDecoderFramingEdgeCases(t *testing.T) {
	malformed := []string{
		"0x1f",       // uint with additional info 31
		"0x3f",       // nint with additional info 31
		"0xff",       // stray break at top level
		"0x7f4100ff", // indefinite text string with a byte-string chunk
		"0x5f5fffff", // indefinite byte string with an indefinite chunk
		"0xbf00ff",   // indefinite map, odd number of items
	}
	for _, h := range malformed {
		dec := cbor.NewDecoder(bytes.NewReader(mustHex(t, h)))
		var v any
		if err := dec.Decode(&v); err == nil {
			t.Errorf("%s: Decode succeeded, want error", h)
		}
	}
}

func TestDecoderFramingDepthCap(t *testing.T) {
	nested := []string{
		"0x818100",   // definite array within array
		"0x81a10000", // definite map within array
		"0x9f9fffff", // indefinite array within indefinite array
		"0x9fbfffff", // indefinite map within indefinite array
		"0x9fc000ff", // tag within indefinite array
	}
	for _, h := range nested {
		dm, err := cbor.DecoderOptions{MaxNestingDepth: 1}.NewDecoder(bytes.NewReader(mustHex(t, h)))
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := dm.Decode(&v); err == nil {
			t.Errorf("%s: Decode succeeded under depth cap 1, want error", h)
		}
	}
}

func TestDecoderFramingHugeMapCount(t *testing.T) {
	data := []byte{0xbb, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01, 0x02}
	dec := cbor.NewDecoder(bytes.NewReader(data))
	var v any
	if err := dec.Decode(&v); err == nil {
		t.Error("map with count >= 2^63 accepted, want error")
	}
}

func TestDecoderFramingErrorOffset(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(mustHex(t, "0x83010203")) // valid array [1,2,3] — 4 bytes
	buf.Write([]byte{0x1c})             // malformed byte at stream offset 4
	dec := cbor.NewDecoder(&buf)
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("first Decode: %v", err)
	}
	err := dec.Decode(&v)
	var se *cbor.SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("second Decode err = %T %v, want *SyntaxError", err, err)
	}
	if se.Offset != 4 {
		t.Errorf("SyntaxError.Offset = %d, want 4 (stream position of the malformed item)", se.Offset)
	}
}

func TestDecoderInputOffsetAndBuffered(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(mustHex(t, "0x01"))       // item 1: 1 (1 byte)
	buf.Write(mustHex(t, "0x820102"))   // item 2: [1,2] (3 bytes)
	buf.Write(mustHex(t, "0x63616263")) // item 3: "abc" (4 bytes)
	dec := cbor.NewDecoder(&buf)

	var a int
	if err := dec.Decode(&a); err != nil {
		t.Fatal(err)
	}
	if dec.InputOffset() != 1 {
		t.Errorf("InputOffset after item 1 = %d, want 1", dec.InputOffset())
	}
	var b []int
	if err := dec.Decode(&b); err != nil {
		t.Fatal(err)
	}
	if dec.InputOffset() != 4 {
		t.Errorf("InputOffset after item 2 = %d, want 4", dec.InputOffset())
	}
	rest, _ := io.ReadAll(dec.Buffered())
	if hexOf(t, rest) != "0x63616263" {
		t.Errorf("Buffered = %x, want the unconsumed item-3 bytes 63616263", rest)
	}
}

func TestEncoderZeroAlloc(t *testing.T) {
	enc := cbor.NewEncoder(io.Discard)
	var v any = 42
	_ = enc.Encode(v) // prime the buffer
	avg := testing.AllocsPerRun(1000, func() { _ = enc.Encode(v) })
	if avg != 0 {
		t.Errorf("streaming Encode allocs/op = %v, want 0 (encodeState + buffer reused)", avg)
	}
}

func BenchmarkEncoderStream(b *testing.B) {
	enc := cbor.NewEncoder(io.Discard)
	v := map[string]int{"alpha": 1, "beta": 2, "gamma": 3}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := enc.Encode(v); err != nil {
			b.Fatal(err)
		}
	}
}

type rewindReader struct {
	data []byte
	pos  int
}

func (r *rewindReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		r.pos = 0
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func BenchmarkDecoderStream(b *testing.B) {
	one, _ := cbor.Marshal(map[string]int{"alpha": 1, "beta": 2, "gamma": 3})
	dec := cbor.NewDecoder(&rewindReader{data: one})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v map[string]int
		if err := dec.Decode(&v); err != nil {
			b.Fatal(err)
		}
	}
}

func TestEncoderOptionsNewEncoder(t *testing.T) {
	var buf bytes.Buffer
	enc, err := cbor.EncoderOptions{Sort: cbor.SortLengthFirst, Time: cbor.TimeRFC3339}.NewEncoder(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.Encode(map[int]int{24: 1, -1: 2}); err != nil {
		t.Fatal(err)
	}
	dec := cbor.NewDecoder(&buf)
	var m cbor.Map
	if err := dec.Decode(&m); err != nil || len(m) != 2 {
		t.Errorf("round trip: len=%d err=%v", len(m), err)
	}
	if _, err := (cbor.EncoderOptions{Sort: 99}).NewEncoder(&buf); err == nil {
		t.Error("NewEncoder with invalid option: want error")
	}
}

func TestNewDecoderInvalidOption(t *testing.T) {
	if _, err := (cbor.DecoderOptions{DuplicateKeys: 99}).NewDecoder(bytes.NewReader(nil)); err == nil {
		t.Error("NewDecoder with invalid DupMode: want error")
	}
}

func TestDecoderStreamsAllTypes(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(mustHex(t, "0x5f42010243030405ff")) // indefinite byte string
	buf.Write(mustHex(t, "0xfb3ff199999999999a")) // float64
	buf.Write(mustHex(t, "0xc11a514b67b0"))       // tag 1
	buf.Write(mustHex(t, "0xf7"))                 // undefined
	buf.Write(mustHex(t, "0x1817"))               // uint (2-byte header)
	dec := cbor.NewDecoder(&buf)

	var b []byte
	if err := dec.Decode(&b); err != nil || len(b) != 5 {
		t.Fatalf("indefinite bytes: b=%x err=%v", b, err)
	}
	var f float64
	if err := dec.Decode(&f); err != nil || f != 1.1 {
		t.Fatalf("float: %v %v", f, err)
	}
	var tg cbor.Tag
	if err := dec.Decode(&tg); err != nil || tg.Number != 1 {
		t.Fatalf("tag: %+v %v", tg, err)
	}
	var u any
	if err := dec.Decode(&u); err != nil {
		t.Fatalf("undefined: %v", err)
	}
	if _, ok := u.(cbor.Undefined); !ok {
		t.Errorf("undefined = %T", u)
	}
	var n int
	if err := dec.Decode(&n); err != nil || n != 23 {
		t.Fatalf("uint: %d %v", n, err)
	}
}

func TestDecoderStreamHardeningAndPartial(t *testing.T) {
	for _, in := range []string{"0x9f00ff", "0x5f4101ff", "0x7f6161ff", "0xbf0001ff"} { // indefinite array/bytes/text/map
		sd, _ := cbor.UntrustedDecoderOptions().NewDecoder(bytes.NewReader(mustHex(t, in)))
		var v0 any
		if err := sd.Decode(&v0); err == nil {
			t.Errorf("untrusted stream indefinite %s: want error", in)
		}
	}
	var v any
	// tag with indefinite length (ai 31) is malformed, rejected at framing
	td := cbor.NewDecoder(bytes.NewReader([]byte{0xdf}))
	if err := td.Decode(&v); err == nil {
		t.Error("tag ai31 via stream: want error")
	}

	full := mustHex(t, "0x9f010203ff")
	pr, pw := io.Pipe()
	go func() {
		for _, b := range full {
			pw.Write([]byte{b})
		}
		pw.Close()
	}()
	dec := cbor.NewDecoder(pr)
	var arr []int
	if err := dec.Decode(&arr); err != nil || !reflect.DeepEqual(arr, []int{1, 2, 3}) {
		t.Errorf("byte-at-a-time indefinite: %v %v", arr, err)
	}

	// indefinite byte string with a non-byte-string chunk
	if err := cbor.Valid(mustHex(t, "0x5f00ff")); err == nil {
		t.Error("indefinite bytes with uint chunk: want error")
	}
	// indefinite text string streamed one byte at a time (framing scanIncomplete path)
	str := mustHex(t, "0x7f6261626163ff")
	sr, sw := io.Pipe()
	go func() {
		for _, b := range str {
			sw.Write([]byte{b})
		}
		sw.Close()
	}()
	sdec := cbor.NewDecoder(sr)
	var got string
	if err := sdec.Decode(&got); err != nil || got != "abc" {
		t.Errorf("byte-at-a-time indefinite text = %q, %v", got, err)
	}
	// indefinite map via stream with a cap
	cd, _ := cbor.DecoderOptions{MaxMapLen: 1}.NewDecoder(bytes.NewReader(mustHex(t, "0xbf01010202ff")))
	if err := cd.Decode(&v); err == nil {
		t.Error("indefinite map over MaxMapLen: want error")
	}

	// items with 1/2/4/8-byte headers streamed one byte at a time (scanIncomplete on header)
	var wide []byte
	for _, h := range []string{"0x1818", "0x1903e8", "0x1a000f4240", "0x1b000000e8d4a51000", "0x4401020304"} {
		wide = append(wide, mustHex(t, h)...)
	}
	wr, ww := io.Pipe()
	go func() {
		for _, bb := range wide {
			ww.Write([]byte{bb})
		}
		ww.Close()
	}()
	wdec := cbor.NewDecoder(wr)
	for i := 0; i < 5; i++ {
		var x any
		if err := wdec.Decode(&x); err != nil {
			t.Fatalf("wide-header stream item %d: %v", i, err)
		}
	}
}

func TestEncoderEncodeError(t *testing.T) {
	enc := cbor.NewEncoder(io.Discard)
	if err := enc.Encode(make(chan int)); err == nil {
		t.Error("Encode(chan): want error")
	}
	if err := enc.Encode(42); err != nil { // encoder still usable after an error
		t.Errorf("Encode after error: %v", err)
	}
}

func TestDecoderRejectsMalformed(t *testing.T) {
	dec := cbor.NewDecoder(bytes.NewReader([]byte{0x1c})) // reserved additional info
	var v any
	err := dec.Decode(&v)
	if err == nil || err == io.EOF {
		t.Fatalf("Decode of malformed byte = %v, want a syntax error", err)
	}
}

func TestDecoderTruncatedItem(t *testing.T) {
	dec := cbor.NewDecoder(bytes.NewReader([]byte{0x83, 0x01, 0x02})) // array(3), only 2 present
	var v []int
	if err := dec.Decode(&v); err != io.ErrUnexpectedEOF {
		t.Fatalf("Decode of truncated item = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestDecoderStreamsNestedAndIndefinite(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(mustHex(t, "0x9f010203ff"))       // indefinite array [1,2,3]
	buf.Write(mustHex(t, "0xa1616101"))         // {"a":1}
	buf.Write(mustHex(t, "0x8261788261796162")) // [ "x", ["y","b"] ]... nested

	dec := cbor.NewDecoder(&buf)
	var a []int
	if err := dec.Decode(&a); err != nil || !reflect.DeepEqual(a, []int{1, 2, 3}) {
		t.Fatalf("indefinite array: a=%v err=%v", a, err)
	}
	var m map[string]int
	if err := dec.Decode(&m); err != nil || m["a"] != 1 {
		t.Fatalf("map: m=%v err=%v", m, err)
	}
	var nested []any
	if err := dec.Decode(&nested); err != nil {
		t.Fatalf("nested: err=%v", err)
	}
}

func TestDecoderSplitItemReassembles(t *testing.T) {
	pr, pw := io.Pipe()
	dec := cbor.NewDecoder(pr)

	go func() {
		full, _ := cbor.Marshal(map[string]int{"aa": 1, "bb": 2})
		pw.Write(full[:3])
		pw.Write(full[3:])
		pw.Close()
	}()

	var m map[string]int
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["aa"] != 1 || m["bb"] != 2 {
		t.Errorf("m = %v, want aa=1 bb=2", m)
	}
}
