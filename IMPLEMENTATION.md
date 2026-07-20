# go-cbor — Implementation Guide

This document explains, in plain English, how this library decodes CBOR and how
the code is put together. It is written for someone reading the codebase for the
first time.

## What is CBOR?

CBOR (Concise Binary Object Representation) is a compact binary data format —
think of it as a binary cousin of JSON. It is defined by **RFC 8949**, which is
Internet Standard STD 94 and obsoletes the older RFC 7049. Everything here is
built to that current specification.

CBOR is used as the wire format for things like IETF COSE (signed/encrypted
messages) and ISO mobile-driving-licence structures such as VICAL. This library
aims to be a faithful, well-tested foundation those higher-level formats can be
built on top of.

## The shape of a CBOR data item

Every CBOR value ("data item") starts with a single **initial byte**, split into
two parts:

```
  bits:  7 6 5   4 3 2 1 0
        [ major ][  additional  ]
        [ type  ][ information  ]
```

- The top **3 bits** are the *major type* (0–7) — what kind of value this is.
- The bottom **5 bits** are the *additional information* (0–31) — usually the
  value itself, or a hint about how many more bytes carry the value.

The additional information tells you how to read the **argument** that follows:

| Additional info | Meaning                                   | Argument bytes |
| --------------- | ----------------------------------------- | -------------- |
| 0–23            | the argument *is* this number             | 0              |
| 24              | argument is the next 1 byte               | 1              |
| 25              | argument is the next 2 bytes (big-endian) | 2              |
| 26              | argument is the next 4 bytes              | 4              |
| 27              | argument is the next 8 bytes              | 8              |
| 28–30           | reserved / not well-formed                | —              |
| 31              | indefinite length (a streaming form)      | —              |

Depending on the major type, the argument means different things: it might be the
number itself, the length of a string, a tag number, or the number of elements in
a container.

## The eight major types

| Major type | Name             | What the argument means                     |
| ---------- | ---------------- | ------------------------------------------- |
| 0          | Integer          | the (non-negative) value                    |
| 1          | Negative integer | encodes `-1 - argument`                     |
| 2          | Byte string      | length in bytes, followed by that many bytes|
| 3          | Text string      | length in bytes (UTF-8), then the bytes     |
| 4          | Array            | number of elements, then that many items    |
| 5          | Map              | number of **pairs**, then key/value items   |
| 6          | Tag              | a tag number, then one tagged data item     |
| 7          | Float / simple   | floats, plus `true`/`false`/`null`/etc.     |

Note the two easy-to-miss details, both from RFC 8949:

- Negative integers store `n` but *mean* `-1 - n`. So the byte for "-1" carries a
  `0`, and "-100" carries `99`.
- A map's argument is the number of key/value **pairs**, so a map of argument `2`
  is followed by **four** data items (key, value, key, value).

## How a decoded item is represented: `DataItem`

Decoding produces a tree of `DataItem` values (see `model/model.go`):

```go
type DataItem struct {
    MajorType int8
    Sign      int8
    Data      []byte
    Content   []*DataItem
}
```

- **`MajorType`** — which of the eight types above this item is.
- **`Sign`** — a small helper flag. It carries the sign for floats, and marks
  positive/negative bignums (tags 2 and 3). It is `0` for most items.
- **`Data`** — the decoded payload for leaf values: the raw bytes of a string,
  the 8-byte big-endian form of an integer or float, and so on. For a tag it holds
  the exact raw encoding of the item the tag wraps (useful for byte-exact
  round-tripping — important for COSE signatures).
- **`Content`** — the child items, used by every container-like type, always in
  wire order:
  - a **tag** has one child (the tagged item),
  - an **array** has one child per element,
  - a **map** has two children per pair, flattened as `key, value, key, value…`.

  Leaf values (integers, strings, floats, simple values) leave `Content` empty.

## How decoding works

The entry point is `DecodeFromHexString` in `internal/decoder.go`, which turns a
hex string into bytes and hands off to the core function:

```go
func decodeDataItem(in []byte) (*model.DataItem, int, error)
```

It returns three things: the decoded item, **how many bytes it consumed**, and an
error. The consumed-byte count is the key to decoding nested and sequential data.

The function reads the initial byte, works out the major type and additional
information, and dispatches to the matching decoder:

- **Integers / negatives** read the argument and store it as 8 big-endian bytes.
- **Byte / text strings** read a length, then slice out exactly that many bytes.
- **Floats / simple values** either decode a half/single/double float or a simple
  value (`true`, `false`, `null`, `undefined`, or a small unsigned number).
- **Tags** read the tag number, then recursively decode the single item that
  follows and attach it as the tag's child.
- **Arrays** read the element count, then decode that many items one after
  another, advancing by each item's consumed length.
- **Maps** do the same but decode twice the count, alternating keys and values.

Because each decode call reports how many bytes it used, a container can walk its
children by starting the next child where the previous one ended. This is why the
scalar decoders check for *at least* the bytes they need rather than *exactly*
that many — in a container, there is usually more data following.

`DecodeFromHexString` stays strict at the top level: after decoding one item it
checks that **no trailing bytes** are left over, so a whole input must be exactly
one well-formed data item.

Malformed input (a length or count that runs past the end of the data) produces
an error — the decoders bounds-check before slicing, so bad input never panics.

## The JSON view is a diagnostic, not the codec

`DataItem` implements `MarshalJSON`, producing human-readable JSON like:

```json
{"data": [1, [2, 3], [4, 5]], "majorType": 4}
```

This is deliberately a **debugging / inspection** view — a readable window onto a
decoded item. It is *not* the CBOR encoder and is not meant for round-tripping.
The value under `"data"` is rendered by walking the tree:

- containers render their children's values (arrays as a list, maps as an ordered
  list of `{"key": …, "value": …}` pairs so that non-string keys and wire order
  are preserved),
- tags render as the value of the item they wrap (with bignums shown as numbers),
- everything else renders as its natural value.

## What is not implemented yet

- **Indefinite-length items** (additional information 31 — the streaming form of
  strings, arrays, and maps). These currently return a clear error. COSE and
  VICAL use definite-length encoding, so this does not block those use cases.
- **Encoding** (Go values → CBOR bytes). The long-term goal is an
  `encoding/json`-style API: package-level `Marshal`/`Unmarshal`, `cbor:"…"`
  struct tags (including integer keys for COSE labels), and `MarshalCBOR` /
  `UnmarshalCBOR` interfaces for custom types. That is a separate layer to be
  built on top of this decoder.

## A note on RFC 8949 errata

RFC 8949 has one verified erratum (ID 8589): when detecting **duplicate** map
keys, two NaN keys must also share the same sign bit to count as equal. This only
matters for enforcing map-key uniqueness, which is a *validity* check distinct
from well-formed decoding. This decoder faithfully decodes maps as given
(including duplicate keys) and does not enforce uniqueness, so the erratum does
not affect it today. It is worth revisiting if strict/canonical validation is
added later.
