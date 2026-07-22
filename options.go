package cbor

import "fmt"

// SortMode controls how map (and struct-as-map) keys are ordered when encoding.
type SortMode uint8

const (
	// SortBytewise orders keys by the bytewise lexicographic order of their
	// encodings (RFC 8949 §4.2.1 core deterministic). This is the default.
	SortBytewise SortMode = iota
	// SortLengthFirst orders keys by encoded length, then bytewise (RFC 7049 /
	// CTAP2 canonical).
	SortLengthFirst
	// SortNone preserves insertion/declaration order and is non-deterministic
	// for Go maps.
	SortNone
)

// TimeMode controls how time.Time values are encoded.
type TimeMode uint8

const (
	// TimeUnix encodes as tag 1: an integer epoch, or a float when sub-second.
	TimeUnix TimeMode = iota
	// TimeRFC3339 encodes as tag 0: an RFC 3339 text string.
	TimeRFC3339
	// TimeNumericDate encodes as a bare (untagged) numeric epoch — an integer, or a
	// float when sub-second. This is the NumericDate form used by CWT (RFC 8392) and
	// JWT (RFC 7519), where exp/nbf/iat are untagged numbers. The decoder accepts a
	// bare number into a time.Time field regardless of this setting.
	TimeNumericDate
)

// FloatMode controls float width selection.
type FloatMode uint8

const (
	// FloatShortest uses the smallest of 16/32/64 bits that preserves the value.
	FloatShortest FloatMode = iota
	// FloatDouble always encodes 64-bit doubles.
	FloatDouble
)

// NaNMode controls how NaN floats are encoded.
type NaNMode uint8

const (
	// NaN7e00 encodes every NaN as the canonical half-precision 0xf97e00.
	NaN7e00 NaNMode = iota
	// NaNNone encodes NaN at the width chosen by FloatMode without canonicalising.
	NaNNone
)

// BigIntMode controls how *big.Int values are encoded.
type BigIntMode uint8

const (
	// BigIntShortest uses a plain integer when the value fits, else a bignum tag.
	BigIntShortest BigIntMode = iota
	// BigIntTag always uses a bignum tag (2 or 3).
	BigIntTag
)

// EncoderOptions configures an Encoding. The zero value is the canonical default used
// by Marshal.
type EncoderOptions struct {
	Sort   SortMode
	Time   TimeMode
	Float  FloatMode
	NaN    NaNMode
	BigInt BigIntMode
}

// Encoding is an immutable, reusable, goroutine-safe encoder configuration.
type Encoding interface {
	Marshal(v any) ([]byte, error)
}

type encoding struct {
	sort   SortMode
	time   TimeMode
	float  FloatMode
	nan    NaNMode
	bigInt BigIntMode
}

// Encoding builds an immutable Encoding from the options, or an error if any
// option value is out of range.
func (o EncoderOptions) Encoding() (Encoding, error) {
	if o.Sort > SortNone {
		return nil, fmt.Errorf("cbor: invalid SortMode %d", o.Sort)
	}
	if o.Time > TimeNumericDate {
		return nil, fmt.Errorf("cbor: invalid TimeMode %d", o.Time)
	}
	if o.Float > FloatDouble {
		return nil, fmt.Errorf("cbor: invalid FloatMode %d", o.Float)
	}
	if o.NaN > NaNNone {
		return nil, fmt.Errorf("cbor: invalid NaNMode %d", o.NaN)
	}
	if o.BigInt > BigIntTag {
		return nil, fmt.Errorf("cbor: invalid BigIntMode %d", o.BigInt)
	}
	return encoding{sort: o.Sort, time: o.Time, float: o.Float, nan: o.NaN, bigInt: o.BigInt}, nil
}

func (m encoding) Marshal(v any) ([]byte, error) {
	return m.marshalInto(nil, v)
}

func (m encoding) marshalInto(buf []byte, v any) ([]byte, error) {
	e := &encodeState{buf: buf, sort: m.sort, time: m.time, float: m.float, nan: m.nan, bigInt: m.bigInt}
	if err := e.encode(v); err != nil {
		return buf, err
	}
	return e.buf, nil
}

// DupMode controls the duplicate-map-key policy on decode.
type DupMode uint8

const (
	// DupAllow keeps the last value for a duplicate key (default).
	DupAllow DupMode = iota
	// DupError rejects any map containing duplicate keys.
	DupError
)

const defaultMaxDepth = 1024

// DecoderOptions configures a Decoding. The zero value matches the default Unmarshal
// (a 1024 nesting-depth cap, otherwise permissive). See UntrustedDecoderOptions for a
// hardened preset.
type DecoderOptions struct {
	// MaxNestingDepth bounds container nesting; <= 0 uses the default (1024).
	MaxNestingDepth int
	// DuplicateKeys selects the duplicate-key policy.
	DuplicateKeys DupMode
	// Strict rejects non-shortest integer/float encodings, non-minimal bignums,
	// and unsorted map keys.
	Strict bool
	// RejectIndefinite rejects indefinite-length items.
	RejectIndefinite bool
	// MaxArrayLen bounds an array's element count; 0 means unlimited.
	MaxArrayLen int
	// MaxMapLen bounds a map's key/value pair count; 0 means unlimited.
	MaxMapLen int
	// MaxStringLength bounds the byte length of any single byte or text string
	// (and of a bignum's byte string); 0 means unlimited.
	MaxStringLength int
}

// UntrustedDecoderOptions returns a conservative preset for decoding untrusted input:
// a shallow nesting cap, duplicate-key rejection, element/pair caps, a per-string
// length cap (1 MiB), and rejection of indefinite-length items (whose streaming
// form otherwise sidesteps the element and duplicate-key caps). It is not a
// substitute for bounding the total input size. Adjust the returned value before
// calling Decoding if the limits don't fit your data — for example, raise
// MaxStringLength for larger embedded blobs.
func UntrustedDecoderOptions() DecoderOptions {
	return DecoderOptions{
		MaxNestingDepth:  16,
		DuplicateKeys:    DupError,
		RejectIndefinite: true,
		MaxArrayLen:      1 << 16,
		MaxMapLen:        1 << 16,
		MaxStringLength:  1 << 20,
	}
}

// CanonicalDecoderOptions returns a preset that rejects any input not in RFC 8949
// §4.2.1 deterministic (canonical) form: non-shortest integers/floats, non-minimal
// bignums, unsorted map keys, indefinite-length items, and duplicate map keys. Use
// it on the verify side
// of COSE/CWT/mdoc, where accepting a non-canonical re-encoding of signed content is
// a hazard. Note that Strict alone does NOT reject indefinite-length items, which is
// why this preset sets RejectIndefinite too. It adds no size caps; combine with
// UntrustedDecoderOptions (or set the Max* fields) to also bound allocation.
func CanonicalDecoderOptions() DecoderOptions {
	return DecoderOptions{
		Strict:           true,
		RejectIndefinite: true,
		DuplicateKeys:    DupError,
	}
}

// Decoding is an immutable, reusable, goroutine-safe decoder configuration.
type Decoding interface {
	Unmarshal(data []byte, v any) error
	Valid(data []byte) error
}

type decoding struct {
	maxDepth    int
	dupMapKey   DupMode
	strict      bool
	rejectIndef bool
	maxArray    int
	maxMap      int
	maxString   int
}

// Decoding builds an immutable Decoding from the options, or an error if any
// option value is out of range.
func (o DecoderOptions) Decoding() (Decoding, error) {
	if o.DuplicateKeys > DupError {
		return nil, fmt.Errorf("cbor: invalid DupMode %d", o.DuplicateKeys)
	}
	maxDepth := o.MaxNestingDepth
	if maxDepth <= 0 {
		maxDepth = defaultMaxDepth
	}
	return decoding{
		maxDepth:    maxDepth,
		dupMapKey:   o.DuplicateKeys,
		strict:      o.Strict,
		rejectIndef: o.RejectIndefinite,
		maxArray:    o.MaxArrayLen,
		maxMap:      o.MaxMapLen,
		maxString:   o.MaxStringLength,
	}, nil
}

func (m decoding) newState() *decodeState {
	return &decodeState{
		maxDepth:    m.maxDepth,
		dupMapKey:   m.dupMapKey,
		strict:      m.strict,
		rejectIndef: m.rejectIndef,
		maxArray:    m.maxArray,
		maxMap:      m.maxMap,
		maxString:   m.maxString,
	}
}

func (m decoding) Unmarshal(data []byte, v any) error {
	return unmarshalState(data, v, m.newState())
}

func (m decoding) Valid(data []byte) error {
	return validState(data, m.newState())
}
