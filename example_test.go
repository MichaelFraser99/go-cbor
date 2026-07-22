package cbor_test

import (
	"bytes"
	"fmt"
	"io"

	cbor "github.com/MichaelFraser99/go-cbor"
)

func ExampleMarshal() {
	b, _ := cbor.Marshal(map[string]int{"a": 1, "b": 2})
	fmt.Printf("%x\n", b)
	// Output: a2616101616202
}

func ExampleUnmarshal() {
	data := []byte{0xa2, 0x61, 0x61, 0x01, 0x61, 0x62, 0x02}
	var m map[string]int
	_ = cbor.Unmarshal(data, &m)
	fmt.Println(m["a"], m["b"])
	// Output: 1 2
}

func ExampleMarshal_structTags() {
	type Header struct {
		Alg int    `cbor:"1,asint"`
		Kid []byte `cbor:"4,asint,omitempty"`
	}
	b, _ := cbor.Marshal(Header{Alg: -7}) // {1: -7}, Kid omitted
	fmt.Printf("%x\n", b)
	// Output: a10126
}

func ExampleMarshal_toarray() {
	type Pair struct {
		_ struct{} `cbor:",toarray"`
		A int
		B int
	}
	b, _ := cbor.Marshal(Pair{A: 1, B: 2}) // encodes as [1, 2]
	fmt.Printf("%x\n", b)
	// Output: 820102
}

func ExampleEncoderOptions() {
	em, _ := cbor.EncoderOptions{Sort: cbor.SortLengthFirst}.Encoding()
	b, _ := em.Marshal(map[int]int{24: 1, -1: 2}) // shorter key (-1) first
	fmt.Printf("%x\n", b)
	// Output: a22002181801
}

func ExampleUntrustedDecoderOptions() {
	dm, _ := cbor.UntrustedDecoderOptions().Decoding()
	err := dm.Valid([]byte{0xa2, 0x01, 0x01, 0x01, 0x02}) // {1:1, 1:2} duplicate key
	fmt.Println(err != nil)
	// Output: true
}

func ExampleMap() {
	var v any
	_ = cbor.Unmarshal([]byte{0xa2, 0x01, 0x02, 0x03, 0x04}, &v) // {1:2, 3:4}
	m := v.(cbor.Map)
	val, _ := m.Get(int64(3))
	fmt.Println(val)
	// Output: 4
}

func ExampleRawMessage() {
	type Envelope struct {
		Type int             `cbor:"t"`
		Body cbor.RawMessage `cbor:"b"` // captured verbatim, decoded later
	}
	data, _ := cbor.Marshal(map[string]any{"t": 1, "b": []int{1, 2, 3}})

	var env Envelope
	_ = cbor.Unmarshal(data, &env)
	fmt.Printf("%x\n", []byte(env.Body))

	var body []int
	_ = cbor.Unmarshal(env.Body, &body)
	fmt.Println(body)
	// Output:
	// 83010203
	// [1 2 3]
}

func ExampleDiagnostic() {
	s, _ := cbor.Diagnostic([]byte{0xa1, 0x01, 0x83, 0x01, 0x02, 0x03})
	fmt.Println(s)
	// Output: {1: [1, 2, 3]}
}

func ExampleNewEncoder() {
	var buf bytes.Buffer
	enc := cbor.NewEncoder(&buf)
	_ = enc.Encode(1)
	_ = enc.Encode("two")

	dec := cbor.NewDecoder(&buf)
	for {
		var v any
		if err := dec.Decode(&v); err == io.EOF {
			break
		}
		fmt.Println(v)
	}
	// Output:
	// 1
	// two
}

func ExampleRawTag() {
	// A verifier captures a tag's content byte-for-byte, without a schema and
	// without the re-canonicalisation that decoding into cbor.Tag would apply.
	msg, _ := cbor.Marshal(cbor.Tag{Number: 18, Content: []int{1, 2}})

	var rt cbor.RawTag
	_ = cbor.Unmarshal(msg, &rt)
	fmt.Printf("tag %d, content %x\n", rt.Number, []byte(rt.Content))
	// Output: tag 18, content 820102
}

func ExampleEncodedCBOR() {
	inner, _ := cbor.Marshal(map[int]int{1: 2})

	type Signed struct {
		Payload cbor.EncodedCBOR `cbor:"p"` // tag-24-wrapped embedded CBOR
	}
	b, _ := cbor.Marshal(Signed{Payload: inner})
	fmt.Printf("%x\n", b)

	var got Signed
	_ = cbor.Unmarshal(b, &got)
	fmt.Printf("%x\n", []byte(got.Payload))
	// Output:
	// a16170d81843a10102
	// a10102
}

func ExampleArrayOf() {
	item := cbor.ArrayOf(cbor.Uint(1), cbor.Text("hi"), cbor.TagOf(0, cbor.Text("t")))
	b, _ := cbor.Marshal(item)
	fmt.Printf("%x\n", b)
	// Output: 8301626869c06174
}
