package cbor

import "math/big"

// Native converts the item to ordinary Go values: int64/uint64/*big.Int, float64,
// string, []byte, bool, nil, []any, Map, Tag, and SimpleValue.
func (d DataItem) Native() any {
	switch d.Major {
	case MajorUint:
		if d.Argument < 1<<63 {
			return int64(d.Argument)
		}
		return d.Argument
	case MajorNint:
		if d.Argument < 1<<63 {
			return int64(-1) - int64(d.Argument)
		}
		b := new(big.Int).SetUint64(d.Argument)
		b.Add(b, big.NewInt(1))
		b.Neg(b)
		return b
	case MajorBytes:
		return d.Bytes
	case MajorText:
		return string(d.Bytes)
	case MajorArray:
		out := make([]any, len(d.Content))
		for i, element := range d.Content {
			out[i] = element.Native()
		}
		return out
	case MajorMap:
		out := make(Map, 0, len(d.Content)/2)
		for i := 0; i+1 < len(d.Content); i += 2 {
			out = append(out, MapEntry{Key: d.Content[i].Native(), Value: d.Content[i+1].Native()})
		}
		return out
	case MajorTag:
		if (d.Argument == 2 || d.Argument == 3) && len(d.Content) == 1 && d.Content[0].Major == MajorBytes {
			b := new(big.Int).SetBytes(d.Content[0].Bytes)
			if d.Argument == 3 {
				b.Add(b, big.NewInt(1))
				b.Neg(b)
			}
			return b
		}
		return Tag{Number: d.Argument, Content: d.Content[0].Native()}
	case MajorOther:
		if d.FloatWidth != 0 {
			return d.Float
		}
		switch d.Argument {
		case 20:
			return false
		case 21:
			return true
		case 22:
			return nil
		case 23:
			return Undefined{}
		default:
			return SimpleValue(d.Argument)
		}
	default:
		return nil
	}
}
