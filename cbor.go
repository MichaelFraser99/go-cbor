package cbor

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
)

// MajorType is a CBOR major type (0–7), the top three bits of an item's initial
// byte.
type MajorType uint8

const (
	MajorUint  MajorType = 0 // unsigned integer
	MajorNint  MajorType = 1 // negative integer (value is -1 - Argument)
	MajorBytes MajorType = 2 // byte string
	MajorText  MajorType = 3 // UTF-8 text string
	MajorArray MajorType = 4 // array
	MajorMap   MajorType = 5 // map
	MajorTag   MajorType = 6 // tagged value
	MajorOther MajorType = 7 // float or simple value
)

// DataItem is the loss-free, fully-typed representation of one decoded CBOR
// item. Unmarshal populates it when the target is a *DataItem, and Marshal
// re-encodes it. Its fields are interpreted by Major (see the MajorType
// constants). Build one directly or with the constructor helpers (Uint, ArrayOf,
// TagOf, …), and use Native to convert it to ordinary Go values.
type DataItem struct {
	// Major is the item's major type.
	Major MajorType

	// Argument holds the head argument: the value for MajorUint/MajorNint, the
	// tag number for MajorTag, or the simple value for MajorOther when
	// FloatWidth is 0.
	Argument uint64

	// Float and FloatWidth apply when Major is MajorOther and FloatWidth is
	// non-zero; FloatWidth is the encoded width in bytes (2, 4, or 8).
	Float      float64
	FloatWidth uint8

	// Bytes holds a byte string (MajorBytes) or a text string's UTF-8 bytes
	// (MajorText).
	Bytes []byte

	// Content holds child items: a tag's single item, an array's elements, or a
	// map's flattened key, value, key, value… sequence.
	Content []*DataItem
}

// Marshaler is implemented by types that encode themselves to CBOR. Define
// MarshalCBOR on a value receiver if instances are ever marshaled by value
// (including as a field, slice element, or map value): a pointer-receiver method
// is not in the method set of a non-addressable value, so it is silently skipped
// and the value is encoded with the default rules instead. MarshalCBOR's bytes are
// written verbatim and are not validated or re-canonicalised, so returning malformed
// or non-canonical CBOR breaks the well-formedness or determinism of the surrounding
// document.
type Marshaler interface {
	MarshalCBOR() ([]byte, error)
}

// Unmarshaler is implemented by types that decode themselves from CBOR. The
// input is exactly one well-formed CBOR item and is only valid for the duration
// of the call; an implementation that retains it must copy it first (data may
// alias the caller's buffer).
type Unmarshaler interface {
	UnmarshalCBOR(data []byte) error
}

// RawMessage is a raw encoded CBOR item. It implements Marshaler and Unmarshaler,
// so a struct field of this type is captured verbatim on decode and re-emitted
// unchanged on encode — useful for deferred/two-stage decoding and for preserving
// exact bytes (e.g. a COSE protected header). An empty RawMessage marshals to
// null.
type RawMessage []byte

// MarshalCBOR returns m as the raw CBOR encoding, or null for an empty message.
func (m RawMessage) MarshalCBOR() ([]byte, error) {
	if len(m) == 0 {
		return []byte{0xf6}, nil
	}
	return m, nil
}

// UnmarshalCBOR stores a copy of the raw CBOR bytes in m.
func (m *RawMessage) UnmarshalCBOR(data []byte) error {
	*m = append((*m)[:0], data...)
	return nil
}

// EncodedCBOR is an already-encoded CBOR data item carried as "embedded CBOR": a
// byte string wrapped in tag 24 (RFC 8949 §3.4.5.1). Marshal emits tag 24 around
// the bytes; Unmarshal expects tag 24 wrapping a byte string and stores a copy of
// its content. This is the common shape in COSE and ISO mdoc/VICAL (for example
// IssuerSignedItemBytes and MobileSecurityObjectBytes), where a structure is frozen
// as opaque bytes so it can be signed or hashed independently of re-encoding. An
// empty EncodedCBOR marshals to null.
type EncodedCBOR []byte

// MarshalCBOR wraps the embedded item in tag 24, or emits null when empty.
func (e EncodedCBOR) MarshalCBOR() ([]byte, error) {
	if len(e) == 0 {
		return []byte{0xf6}, nil
	}
	out := &encodeState{}
	out.writeHead(6, 24)
	out.writeHead(2, uint64(len(e)))
	out.buf = append(out.buf, e...)
	return out.buf, nil
}

// Decode unmarshals the embedded item into v. The embedded bytes are captured
// verbatim and are NOT validated by UnmarshalCBOR, so Decode is where malformed
// embedded CBOR surfaces. For untrusted input, pass the EncodedCBOR to a configured
// Decoding.Unmarshal instead, so the decode limits apply to the embedded item too.
func (e EncodedCBOR) Decode(v any) error {
	return Unmarshal(e, v)
}

// UnmarshalCBOR stores a copy of the tag-24 byte string's content, or nil for null.
// It does not check that the content is well-formed CBOR — the bytes are captured
// verbatim so they can be signed or hashed as received; use Decode or Valid to check.
func (e *EncodedCBOR) UnmarshalCBOR(data []byte) error {
	if len(data) == 1 && data[0] == 0xf6 {
		*e = nil
		return nil
	}
	item, _, err := decodeDataItem(data)
	if err != nil {
		return err
	}
	if item.Major != MajorTag || item.Argument != 24 || len(item.Content) != 1 || item.Content[0].Major != MajorBytes {
		return &UnmarshalTypeError{CBORType: majorName(item.Major), GoType: reflect.TypeOf(EncodedCBOR(nil))}
	}
	*e = append((*e)[:0], item.Content[0].Bytes...)
	return nil
}

// Tag is a CBOR tag number together with its content. Marshal(Tag{N, c}) emits
// tag N wrapping the encoding of c; decoding a tag into an any yields a Tag,
// except tags 2 and 3 (bignums), which decode to a *big.Int.
type Tag struct {
	Number  uint64
	Content any
}

// RawTag is a tag number together with its content as an undecoded RawMessage. It
// lets a verifier capture a tag's content byte-for-byte without a schema (e.g. a
// COSE protected header), avoiding the re-canonicalisation that decoding into Tag
// would apply. Marshal(RawTag{N, raw}) emits tag N wrapping raw's bytes.
type RawTag struct {
	Number  uint64
	Content RawMessage
}

// SimpleValue is a CBOR simple value (major type 7, values 0–19 and 32–255).
// The named simples true, false and null map to Go bool and nil instead.
type SimpleValue byte

// String renders the simple value as simple(N).
func (s SimpleValue) String() string {
	return fmt.Sprintf("simple(%d)", byte(s))
}

// Undefined is the decoded form of the CBOR "undefined" simple value (0xf7).
// Decoding 0xf7 into an any yields Undefined{}, distinct from other simple values
// so an inspector can tell them apart. Marshal(Undefined{}) emits 0xf7.
type Undefined struct{}

// MapEntry is one key/value pair of a Map.
type MapEntry struct {
	Key   any
	Value any
}

// Map is the ordered representation of a CBOR map produced when decoding into an
// any. It preserves wire order and supports keys of any type (including byte
// strings, arrays and maps, which a Go map cannot hold). Marshal(Map) re-encodes
// it as a CBOR map, reporting an error if two entries encode to the same key bytes;
// Unmarshal accepts a *Map as a decode target. Decode into a Go map[K]V instead
// when you know the schema and want O(1) lookup.
type Map []MapEntry

// Get returns the value for the first entry whose key deep-equals key. Keys must
// match by exact Go type: integer keys decode as int64, so use Get(int64(k)), not
// Get(k) with an untyped constant that defaults to int.
func (m Map) Get(key any) (any, bool) {
	for _, e := range m {
		if reflect.DeepEqual(e.Key, key) {
			return e.Value, true
		}
	}
	return nil, false
}

// GetInt returns the value for key as an int64, coercing across the integer
// representations: it succeeds for a value that decoded as int64, as a uint64
// within the int64 range, or as a *big.Int that fits in an int64. ok is false if
// the key is absent, the value is not an integer, or it does not fit in an int64.
func (m Map) GetInt(key any) (int64, bool) {
	v, ok := m.Get(key)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return n, true
	case uint64:
		if n <= math.MaxInt64 {
			return int64(n), true
		}
	case *big.Int:
		if n.IsInt64() {
			return n.Int64(), true
		}
	}
	return 0, false
}

// GetUint returns the value for key as a uint64, coercing across the integer
// representations: it succeeds for a value that decoded as uint64, as a
// non-negative int64, or as a *big.Int that fits in a uint64. ok is false if the
// key is absent, the value is not an integer, or it is negative or too large.
func (m Map) GetUint(key any) (uint64, bool) {
	v, ok := m.Get(key)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case uint64:
		return n, true
	case int64:
		if n >= 0 {
			return uint64(n), true
		}
	case *big.Int:
		if n.IsUint64() {
			return n.Uint64(), true
		}
	}
	return 0, false
}

// GetFloat returns the value for key as a float64. ok is false if the key is
// absent or the value is not a float.
func (m Map) GetFloat(key any) (float64, bool) {
	v, ok := m.Get(key)
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

// GetBool returns the value for key as a bool. ok is false if the key is absent
// or the value is not a bool.
func (m Map) GetBool(key any) (bool, bool) {
	v, ok := m.Get(key)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// GetString returns the value for key as a string. ok is false if the key is
// absent or the value is not a text string.
func (m Map) GetString(key any) (string, bool) {
	v, ok := m.Get(key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetBytes returns the value for key as a byte string. ok is false if the key is
// absent or the value is not a byte string.
func (m Map) GetBytes(key any) ([]byte, bool) {
	v, ok := m.Get(key)
	if !ok {
		return nil, false
	}
	b, ok := v.([]byte)
	return b, ok
}

// GetSlice returns the value for key as a []any. ok is false if the key is absent
// or the value is not an array.
func (m Map) GetSlice(key any) ([]any, bool) {
	v, ok := m.Get(key)
	if !ok {
		return nil, false
	}
	s, ok := v.([]any)
	return s, ok
}

// GetTag returns the value for key as a Tag. ok is false if the key is absent or
// the value is not a tag. Note that bignum tags 2 and 3 decode to *big.Int, not Tag.
func (m Map) GetTag(key any) (Tag, bool) {
	v, ok := m.Get(key)
	if !ok {
		return Tag{}, false
	}
	t, ok := v.(Tag)
	return t, ok
}

// ToStringMap converts the Map to a map[string]any, returning an error if any key
// is not a text string. Nested maps remain as Map values (call ToStringMap on them
// as needed). Use this at a call site when you know the keys are strings and want a
// plain Go map; note it discards key order and rejects the non-string keys a Map can
// otherwise hold.
func (m Map) ToStringMap() (map[string]any, error) {
	out := make(map[string]any, len(m))
	for _, e := range m {
		k, ok := e.Key.(string)
		if !ok {
			return nil, fmt.Errorf("cbor: map key %#v is not a text string", e.Key)
		}
		out[k] = e.Value
	}
	return out, nil
}

// GetMap returns the value for key as a nested Map. ok is false if the key is
// absent or the value is not a map.
func (m Map) GetMap(key any) (Map, bool) {
	v, ok := m.Get(key)
	if !ok {
		return nil, false
	}
	nested, ok := v.(Map)
	return nested, ok
}

// Marshal returns the deterministic (canonical, RFC 8949 §4.2.1) CBOR encoding of
// v. See the package overview for the Go-to-CBOR type mapping and struct tags,
// and EncoderOptions for non-default encoding.
func Marshal(v any) ([]byte, error) {
	e := &encodeState{}
	if err := e.encode(v); err != nil {
		return nil, err
	}
	return e.buf, nil
}

// Unmarshal parses one CBOR item from data and stores it in the value pointed to
// by v, which must be a non-nil pointer. v may be a pointer to a concrete Go type,
// to an any (yielding native values, with maps as a Map), to a DataItem (the
// loss-free tree), or to a RawMessage (the item's exact bytes). It is an error for
// data to contain trailing bytes after the item; use a Decoder for a sequence.
//
// When a tagged item is decoded into a concrete (non-Tag) type, the tag is unwrapped
// and its number is not checked. Decode into a Tag or an any if the tag's identity
// is significant — for example the COSE message type carried by tag 18 vs 17.
func Unmarshal(data []byte, v any) error {
	return unmarshalState(data, v, newDecodeState())
}

func unmarshalState(data []byte, v any, s *decodeState) error {
	s.data = data
	rv := reflect.ValueOf(v)
	if v == nil || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &InvalidUnmarshalError{Type: reflect.TypeOf(v)}
	}
	n, err := s.reflect(data, rv.Elem())
	if err != nil {
		return err
	}
	if n != len(data) {
		return &SyntaxError{Offset: int64(n), msg: fmt.Sprintf("cbor: %d trailing bytes after top-level item", len(data)-n)}
	}
	return nil
}

// Valid reports whether data is exactly one well-formed CBOR item with no
// trailing bytes, returning a *SyntaxError otherwise.
func Valid(data []byte) error {
	return validState(data, newDecodeState())
}

func validState(data []byte, s *decodeState) error {
	s.data = data
	var (
		n   int
		err error
	)
	if s.strict || s.dupMapKey == DupError || s.maxArray > 0 || s.maxMap > 0 {
		_, n, err = s.decodeItem(data)
	} else {
		n, err = s.skipItem(data)
	}
	if err != nil {
		return err
	}
	if n != len(data) {
		return &SyntaxError{Offset: int64(n), msg: fmt.Sprintf("cbor: %d trailing bytes after top-level item", len(data)-n)}
	}
	return nil
}
