# go-cbor

A zero-dependency CBOR ([RFC 8949](https://www.rfc-editor.org/rfc/rfc8949) / STD 94) codec for Go, with an API that mirrors `encoding/json`. **Deterministic (canonical) encoding by default** — ready for COSE, CWT, and ISO mdoc/VICAL signing without configuration. Round-trips the RFC 8949 Appendix A test vectors, is fuzzed (round-trip and streaming), and never panics on malformed input.

```go
import cbor "github.com/MichaelFraser99/go-cbor"
```

## Quick start

```go
b, _ := cbor.Marshal(map[string]int{"a": 1}) // canonical CBOR bytes
var v map[string]int
_ = cbor.Unmarshal(b, &v)
```

`Marshal` / `Unmarshal` / `Valid` behave like their `encoding/json` counterparts.

## Struct tags

```go
type Header struct {
    Alg int    `cbor:"1,asint"`           // integer map key (COSE label)
    Kid []byte `cbor:"4,asint,omitempty"`
}
type Sign1 struct {
    _         struct{} `cbor:",toarray"`      // encode struct as an array, not a map
    Protected []byte
    Payload   []byte
    Signature []byte
}
```

| Tag option | Effect |
|---|---|
| `cbor:"name"` | field key is the text string `name` |
| `cbor:"1,asint"` | field key is the integer `1` |
| `cbor:"-"` | field is skipped |
| `,omitempty` | omit when the field is its zero value |
| `,omitzero` | omit when the field `IsZero()` |
| `,toarray` on `_ struct{}` | encode the whole struct as a positional array |

> **Coming from `encoding/json`?** The API shape matches, but the semantics are CBOR's. Only `cbor:` tags are read — `json:` tags (including `json:"-"`) are ignored; untagged fields use the Go field name; field-name matching on decode is **case-sensitive** (unlike json). Embedded/anonymous struct fields follow `encoding/json`'s rules: an untagged embedded struct is flattened into the parent, a tagged one nests under its name, and conflicts resolve shallowest-wins. `json.RawMessage` has no special meaning — use `cbor.RawMessage`. Use `,omitzero` (not `,omitempty`) for `time.Time` and other structs — `omitempty` never omits a struct, so a zero `time.Time` is emitted as its year-1 sentinel. Decoding into `any` yields `cbor.Map` (ordered), not `map[string]any` — use `Map.ToStringMap()` or decode into a concrete type.

## Type mapping

**Marshal (Go → CBOR)**

| Go | CBOR |
|---|---|
| `bool`, `nil` | `true`/`false`, `null` |
| nil slice / nil map / nil pointer | `null` |
| `int*`, `uint*` | integer (shortest form) |
| `float32/64` | float (shortest width) |
| `string` / `[]byte` | text / byte string |
| slice, array | array |
| map, struct | map (or array with `toarray`) |
| `*big.Int` | integer or bignum tag |
| `time.Time` | tag 1 epoch (or tag 0 text via options) |
| `Marshaler` | whatever `MarshalCBOR` returns |
| `RawMessage`, `Tag`, `RawTag`, `SimpleValue`, `Undefined`, `DataItem`, `Map`, `EncodedCBOR` | as described |

**Unmarshal into `any`** yields native values: `int64` (or `uint64` / `*big.Int` at the 64-bit extremes; bignum tags 2/3 also yield `*big.Int`), `float64`, `string`, `[]byte`, `bool`, `nil`, `[]any`, `cbor.Map` (ordered key/value list), `cbor.Tag`, `cbor.SimpleValue`, `cbor.Undefined{}` (for the `undefined` value). Decode into a concrete `map[K]V` when you know the schema and want O(1) lookup; into `*cbor.Map` for the ordered map directly (`Map.ToStringMap()` converts to `map[string]any` when keys are strings); into `*cbor.DataItem` for the loss-free tree; into `*cbor.RawMessage` for exact bytes; into `*cbor.RawTag` to capture a tag's content verbatim (e.g. a COSE protected header a verifier must hash as received).

```go
var v any
_ = cbor.Unmarshal(data, &v)
m := v.(cbor.Map)             // for a CBOR map
val, ok := m.Get(int64(1))    // keys match by exact Go type: int64, not int
n, ok := m.GetInt(int64(1))   // typed getters: GetInt/GetUint/GetFloat/GetBool/
                              // GetString/GetBytes/GetSlice/GetMap/GetTag.
                              // GetInt/GetUint coerce across int64/uint64/*big.Int.
```

## Options

Immutable, reusable, goroutine-safe modes:

```go
em, _ := cbor.EncoderOptions{Sort: cbor.SortLengthFirst}.Encoding()
b, _ := em.Marshal(v)
```

| `EncoderOptions` | Values (default first) |
|---|---|
| `Sort` | `SortBytewise`, `SortLengthFirst`, `SortNone` |
| `Time` | `TimeUnix`, `TimeRFC3339`, `TimeNumericDate` (untagged epoch, for CWT/JWT) |
| `Float` | `FloatShortest`, `FloatDouble` |
| `NaN` | `NaN7e00`, `NaNNone` |
| `BigInt` | `BigIntShortest`, `BigIntTag` |

| `DecoderOptions` | Meaning |
|---|---|
| `MaxNestingDepth` | container-nesting cap (default 1024) |
| `DuplicateKeys` | `DupAllow`, `DupError` |
| `Strict` | reject non-shortest / non-minimal encodings and unsorted keys (but **not** indefinite — see below) |
| `RejectIndefinite` | reject indefinite-length items |
| `MaxArrayLen` / `MaxMapLen` | element caps (0 = unlimited) |
| `MaxStringLength` | per-string byte cap (0 = unlimited) |

Two presets return a ready-made `DecoderOptions`:

- **`UntrustedDecoderOptions()`** — hardened for attacker input (caps + dup-key + no indefinite).
- **`CanonicalDecoderOptions()`** — rejects anything not in RFC 8949 §4.2.1 canonical form (non-shortest, non-minimal bignums, unsorted keys, indefinite, duplicate keys) for the verify side of COSE/CWT/mdoc. Note `Strict` alone does *not* reject indefinite-length items, which is why this preset also sets `RejectIndefinite`.

## Untrusted input

```go
dm, _ := cbor.UntrustedDecoderOptions().Decoding() // depth + dup-key + element/string caps + no indefinite
err := dm.Unmarshal(networkBytes, &v)

// Streaming from a connection: wrap the reader so total input is bounded too.
sd, _ := cbor.UntrustedDecoderOptions().NewDecoder(io.LimitReader(conn, 1<<20))
```

The default decoder is bounds-checked and never panics, but by default accepts duplicate keys, non-canonical encodings, and indefinite-length items; `UntrustedDecoderOptions` tightens all of these and adds element, pair, and per-string (1 MiB) caps. It rejects indefinite-length items deliberately — their streaming form otherwise sidesteps the element and duplicate-key caps. It is not a substitute for bounding total request size; pair it with an `io.LimitReader` / `http.MaxBytesReader` — especially with the streaming `Decoder`, whose *default* (unconfigured) form will buffer a hostile declared string length without limit.

## Streaming

```go
enc := cbor.NewEncoder(w) // enc.Encode(x) per item
dec := cbor.NewDecoder(r) // dec.Decode(&x) until io.EOF
```

> `Decode` reads only as many bytes as each item needs, so it works on a connection that stays open between items — it returns as soon as one complete item has arrived rather than waiting for EOF. A truncated final item returns `io.ErrUnexpectedEOF`. `Encoder` reuses its buffer (no per-item allocation); `Decoder.InputOffset()` and `Decoder.Buffered()` support connection hand-off to another protocol.

## Inspecting

```go
s, _ := cbor.Diagnostic(data) // CBOR diagnostic notation, e.g. {1: [1, 2, 3]}
```

See the runnable [examples](example_test.go) for COSE-style headers, `toarray`, streaming, `RawMessage`, and more.
