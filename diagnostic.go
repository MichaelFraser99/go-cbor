package cbor

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Diagnostic renders one CBOR item as CBOR diagnostic notation (RFC 8949 §8):
// e.g. [1, 2, 3], {1: 2}, h'01ff', "text", 0("2013-…"), true, NaN. Unlike the
// JSON view it distinguishes byte strings from text strings and shows tag numbers.
func Diagnostic(data []byte) (string, error) {
	item, n, err := decodeDataItem(data)
	if err != nil {
		return "", err
	}
	if n != len(data) {
		return "", &SyntaxError{Offset: int64(n), msg: fmt.Sprintf("cbor: %d trailing bytes after top-level item", len(data)-n)}
	}
	return item.diagnostic(), nil
}

func (d DataItem) diagnostic() string {
	switch d.Major {
	case MajorUint:
		return strconv.FormatUint(d.Argument, 10)
	case MajorNint:
		return nintString(d.Argument)
	case MajorBytes:
		return "h'" + hex.EncodeToString(d.Bytes) + "'"
	case MajorText:
		return strconv.Quote(string(d.Bytes))
	case MajorArray:
		parts := make([]string, len(d.Content))
		for i, e := range d.Content {
			parts[i] = e.diagnostic()
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case MajorMap:
		parts := make([]string, 0, len(d.Content)/2)
		for i := 0; i+1 < len(d.Content); i += 2 {
			parts = append(parts, d.Content[i].diagnostic()+": "+d.Content[i+1].diagnostic())
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case MajorTag:
		return strconv.FormatUint(d.Argument, 10) + "(" + d.Content[0].diagnostic() + ")"
	case MajorOther:
		if d.FloatWidth != 0 {
			switch {
			case math.IsInf(d.Float, 1):
				return "Infinity"
			case math.IsInf(d.Float, -1):
				return "-Infinity"
			case math.IsNaN(d.Float):
				return "NaN"
			}
			s := strconv.FormatFloat(d.Float, 'g', -1, 64)
			if !strings.ContainsAny(s, ".eE") {
				s += ".0"
			}
			return s
		}
		switch d.Argument {
		case 20:
			return "false"
		case 21:
			return "true"
		case 22:
			return "null"
		case 23:
			return "undefined"
		default:
			return "simple(" + strconv.FormatUint(d.Argument, 10) + ")"
		}
	default:
		return ""
	}
}

// MarshalJSON renders the item as a JSON debugging view of the form
// {"majorType":N,"data":…}; it is not the CBOR codec. For CBOR diagnostic
// notation, use Diagnostic. Conventions: byte strings are base64url (no padding);
// integers outside ±(2^53-1) and bignums are emitted as strings to avoid JSON
// precision loss; NaN, Infinity, -Infinity and undefined are emitted as the strings
// "NaN"/"Infinity"/"-Infinity"/"undefined" (JSON has no literal for them), so use
// the majorType field to disambiguate them from genuine text strings.
func (d DataItem) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"majorType": d.Major,
		"data":      d.dataValue(),
	})
}

const maxSafeJSONInt = 1<<53 - 1

func (d DataItem) dataValue() any {
	switch d.Major {
	case MajorUint:
		if d.Argument > maxSafeJSONInt {
			return strconv.FormatUint(d.Argument, 10)
		}
		return d.Argument
	case MajorNint:
		if d.Argument >= 1<<53 {
			return nintString(d.Argument)
		}
		return json.Number(nintString(d.Argument))
	case MajorBytes:
		return base64.RawURLEncoding.EncodeToString(d.Bytes)
	case MajorText:
		return string(d.Bytes)
	case MajorArray:
		values := make([]any, len(d.Content))
		for i, element := range d.Content {
			values[i] = element.dataValue()
		}
		return values
	case MajorMap:
		pairs := make([]any, 0, len(d.Content)/2)
		for i := 0; i+1 < len(d.Content); i += 2 {
			pairs = append(pairs, map[string]any{
				"key":   d.Content[i].dataValue(),
				"value": d.Content[i+1].dataValue(),
			})
		}
		return pairs
	case MajorTag:
		return d.tagValue()
	case MajorOther:
		return d.otherValue()
	default:
		return nil
	}
}

func (d DataItem) tagValue() any {
	switch d.Argument {
	case 2:
		return new(big.Int).SetBytes(d.Content[0].Bytes).String()
	case 3:
		b := new(big.Int).SetBytes(d.Content[0].Bytes)
		b.Add(b, big.NewInt(1))
		b.Neg(b)
		return b.String()
	default:
		return d.Content[0].dataValue()
	}
}

func (d DataItem) otherValue() any {
	if d.FloatWidth == 0 {
		switch d.Argument {
		case 20:
			return false
		case 21:
			return true
		case 22:
			return nil
		case 23:
			return "undefined"
		default:
			return int(d.Argument)
		}
	}

	value := d.Float
	switch {
	case math.IsInf(value, 1):
		return "Infinity"
	case math.IsInf(value, -1):
		return "-Infinity"
	case math.IsNaN(value):
		return "NaN"
	case value == math.Trunc(value):
		return json.Number(fmt.Sprintf("%.1f", value))
	default:
		return value
	}
}

func nintString(n uint64) string {
	b := new(big.Int).SetUint64(n)
	b.Add(b, big.NewInt(1))
	b.Neg(b)
	return b.String()
}
