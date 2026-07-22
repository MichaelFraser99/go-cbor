// Package cbor implements encoding and decoding of CBOR (Concise Binary Object
// Representation, RFC 8949 / STD 94) with an API that mirrors encoding/json.
//
// # Quick start
//
//	b, _ := cbor.Marshal(map[string]int{"a": 1}) // deterministic CBOR bytes
//	var v map[string]int
//	_ = cbor.Unmarshal(b, &v)
//
// Marshal produces deterministic (canonical) CBOR by default, per RFC 8949
// §4.2.1: shortest-form integers and floats, and map keys sorted by the bytewise
// lexicographic order of their encodings. COSE, CWT and ISO mdoc all require
// this, so nothing needs configuring for signing or hashing.
//
// # Struct tags
//
// Fields use cbor:"..." tags, like encoding/json, with CBOR-specific extras:
//
//	type Header struct {
//	    Alg int    `cbor:"1,asint"`           // integer map key (e.g. COSE labels)
//	    Kid []byte `cbor:"4,asint,omitempty"`
//	}
//
//	type Sign1 struct {
//	    _         struct{} `cbor:",toarray"`      // encode as an array, not a map
//	    Protected []byte
//	    Payload   []byte
//	    Signature []byte
//	}
//
// Recognised options: a name (or an integer with asint), "-" to skip a field,
// "omitempty", "omitzero", and ",toarray" on a blank _ field to encode the whole
// struct as a positional array.
//
// The API shape mirrors encoding/json, but the semantics are CBOR's, not JSON's.
// In particular: only cbor:"..." tags are read (json:"..." tags, including
// json:"-", are ignored); untagged fields use the Go field name; and field-name
// matching on decode is case-sensitive (unlike encoding/json). Embedded
// (anonymous) struct fields follow encoding/json's rules — an untagged embedded
// struct's fields are promoted to the parent, an embedded field with a tag is
// nested under that name, and name conflicts resolve by the same shallowest-wins,
// tagged-beats-untagged, ties-dropped rules. Prefer omitzero over omitempty for
// time.Time and other struct types: omitempty tests a zero value and never omits a
// struct, so a zero time.Time is emitted as its year-1 sentinel rather than dropped.
//
// # Type mapping
//
// Marshal: bool→true/false, nil→null, int/uint→major 0/1, float32/64→shortest
// float, string→text string, []byte→byte string, slice/array→array,
// map/struct→map (or array with toarray), *big.Int→integer or bignum tag,
// time.Time→tag 1 epoch (or tag 0 text, or a bare untagged epoch, via [TimeMode]).
// A nil slice, nil map, or nil pointer encodes to null. Types implementing Marshaler
// encode themselves.
//
// Unmarshal into a concrete Go type is the reverse. Unmarshal into an any yields
// native values: int64 (or uint64 / *big.Int at the 64-bit extremes; bignum tags
// 2 and 3 also yield *big.Int), float64, string, []byte, bool, nil, []any, [Map]
// (an ordered key/value list), [Tag], [SimpleValue] and [Undefined]. Unmarshal into
// a *[DataItem] yields the loss-free tree; into a *[RawMessage] captures the item's
// exact bytes undecoded; into a *[RawTag] captures a tag's content verbatim; into a
// *[Map] yields the ordered map directly. A bare (untagged) number also decodes into
// a time.Time (RFC 8392 / RFC 7519 NumericDate).
//
// # Options
//
// Encoding and decoding are configured through immutable, reusable, goroutine-
// safe modes:
//
//	em, _ := cbor.EncoderOptions{Sort: cbor.SortLengthFirst}.Encoding()
//	b, _ := em.Marshal(v)
//
//	dm, _ := cbor.UntrustedDecoderOptions().Decoding() // safe limits for untrusted input
//	err := dm.Unmarshal(data, &v)
//
// See [EncoderOptions] (sort order, float/NaN/bignum form, time format) and
// [DecoderOptions] (nesting depth, duplicate keys, element caps, strict canonical
// validation, indefinite-length rejection).
//
// # Streaming
//
// [NewEncoder] and [NewDecoder] write and read a sequence of items;
// Decoder.Decode returns io.EOF at the end of the stream.
//
// # Inspecting
//
// [Diagnostic] renders bytes in CBOR diagnostic notation (RFC 8949 §8), and
// DataItem.MarshalJSON gives a JSON view for debugging.
package cbor
