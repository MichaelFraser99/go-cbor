# AGENTS.md

Guidance for AI agents (and new contributors) working in this repo. Read this before changing code.

`github.com/MichaelFraser99/go-cbor` — a zero-dependency CBOR (RFC 8949 / STD 94) codec whose public API mirrors `encoding/json`. Primary use: COSE / CWT / ISO mdoc signing and decoding untrusted network input. See `doc.go` and `README.md` for the user-facing API.

## Commands

```sh
go test ./...                                   # full suite
go test -coverprofile=c.out ./... && go tool cover -func=c.out   # coverage (keep >= 95%)
go test -bench . -benchmem -run '^$' ./...      # benchmarks
go test -run xxx -fuzz FuzzRoundTrip -fuzztime 30s ./...         # one fuzz target
go vet ./... && gofmt -l .                       # must be clean (gofmt -l prints nothing)
golangci-lint run                                # lint + gosec security checks
govulncheck ./...                                # known-vulnerability scan
```

Fuzz targets: `FuzzRoundTrip`, `FuzzUnmarshal`, `FuzzValidMatchesDecode`, `FuzzDecoderStream`. Run the relevant one after any change to encode/decode/framing — they are the real regression net. CI (`.github/workflows/ci.yml`) runs test/vet/gofmt/lint/govulncheck plus a 30s fuzz smoke on every push and PR; a weekly job (`fuzz.yml`) fuzzes each target for 45 minutes against an accumulating cached corpus and, on a crash, opens a PR adding the reproducer under `testdata/fuzz/`.

One-time local setup (macOS): `brew install golangci-lint pre-commit`, `go install golang.org/x/vuln/cmd/govulncheck@latest`, then `pre-commit install`. The pre-commit hooks (`.pre-commit-config.yaml`) run gofmt, go vet, golangci-lint, and the test suite.

## Layout

One file per concern; each has a shadow `_test.go` of the same name (`encode.go` ↔ `encode_test.go`).

| File | Responsibility |
|---|---|
| `cbor.go` | Entry points (`Marshal`/`Unmarshal`/`Valid`), public types (`Map`, `Tag`, `RawMessage`, `RawTag`, `SimpleValue`, `EncodedCBOR`, `DataItem`) |
| `encode.go` | Encoder: reflection → CBOR bytes, canonical key sorting |
| `decode.go` | Tree decoder: bytes → `*DataItem`; `Valid` well-formedness |
| `decode_reflect.go` | Reflect decoder: bytes → concrete Go types; `skipItem` |
| `native.go` | `DataItem` → native `any` values |
| `construct.go` | Helpers to build `DataItem` trees |
| `stream.go` | `Encoder`/`Decoder` and the incremental stream **framer** |
| `options.go` | `EncoderOptions`/`DecoderOptions`, `Encoding`/`Decoding`, presets |
| `diagnostic.go` | RFC 8949 §8 diagnostic notation |
| `errors.go` | Error types |

## Conventions

- **Zero dependencies.** Standard library only. Never add a `require` to `go.mod`.
- **Comments: exported-symbol godoc only.** No inline or function-body comments; no doc comments on unexported symbols. Keep godoc succinct.
- **Shadow test files.** New tests go in the `_test.go` shadowing the file under test; don't create ad-hoc test files. Tests are `package cbor_test` (external, use the `cbor.` prefix).
- **TDD.** Write the failing test first, watch it fail, then implement.
- **Canonical by default.** `Marshal` and the plain `Decoder` are deterministic per RFC 8949 §4.2.1 with no configuration. Do not change default sort/int/float forms — signers depend on byte-for-byte stability.

## Invariants — do not break these

1. **The grammar lives in three places that must agree.** `decode.go` (tree), `decode_reflect.go` (`skipItem`/reflect), and `stream.go` (the framer) each parse CBOR independently. A change to depth caps, element/pair caps, indefinite handling, or well-formedness in one usually needs the same change in the others. `FuzzDecoderStream` cross-checks the framer against one-shot `Unmarshal` — run it. (Historically the top source of bugs here.)
2. **Never panic on untrusted input.** Every malformed/truncated/hostile input must return an error, not panic. Bounds-check before indexing/slicing.
3. **No input aliasing.** Any bytes handed back to the caller (strings, `[]byte`, keys) must be copied out of the decode buffer. The only documented exceptions are `RawMessage` and the `data` passed to a `Unmarshaler`.
4. **Bignum tag 2/3 content is a byte string on every path;** strict/canonical mode additionally requires minimal form (`bignumMinimal`).
5. **RFC 8949 Appendix A vectors must keep round-tripping** (`decode_test.go` / `encode_test.go` vector tables).

## Untrusted input

Default decoding is bounds-checked and panic-free but permissive (no size/dup/indefinite limits). For attacker-controlled bytes use `UntrustedDecoderOptions()`, and bound total input with an `io.LimitReader` / `http.MaxBytesReader` — the streaming `Decoder` frames items incrementally but still buffers a declared string length whole.

## Workflow

Work on `dev`; merge into `main` and keep `main` green (all tests + fuzz seed corpus pass). Commit only when the human explicitly asks.
