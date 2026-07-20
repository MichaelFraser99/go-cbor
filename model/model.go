package model

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
)

type MajorType int

const (
	MajorTypeInteger         = 0
	MajorTypeNegativeInteger = 1
	MajorTypeByteString      = 2
	MajorTypeTextString      = 3
	MajorTypeArray           = 4
	MajorTypeMap             = 5
	MajorTypeTag             = 6
	MajorTypeFloatOrSimple   = 7
)

type DataItem struct {
	MajorType int8
	Sign      int8
	Data      []byte
	Content   []*DataItem
}

func (d DataItem) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"majorType": d.MajorType,
		"data":      d.dataValue(),
	})
}

func (d DataItem) dataValue() any {
	switch d.MajorType {
	case MajorTypeInteger:
		return binary.BigEndian.Uint64(d.Data)
	case MajorTypeNegativeInteger:
		uData := binary.BigEndian.Uint64(d.Data)

		bData := new(big.Int).SetUint64(uData)
		bData.Add(bData, big.NewInt(1))
		bData.Mul(bData, big.NewInt(-1))

		return json.Number(bData.String())
	case MajorTypeByteString:
		return base64.RawURLEncoding.EncodeToString(d.Data)
	case MajorTypeTextString:
		return string(d.Data)
	case MajorTypeTag:
		return d.tagValue()
	case MajorTypeArray:
		values := make([]any, len(d.Content))
		for i, element := range d.Content {
			values[i] = element.dataValue()
		}
		return values
	case MajorTypeMap:
		pairs := make([]any, 0, len(d.Content)/2)
		for i := 0; i+1 < len(d.Content); i += 2 {
			pairs = append(pairs, map[string]any{
				"key":   d.Content[i].dataValue(),
				"value": d.Content[i+1].dataValue(),
			})
		}
		return pairs
	case MajorTypeFloatOrSimple:
		return d.simpleOrFloatValue()
	default:
		return nil
	}
}

func (d DataItem) tagValue() any {
	switch d.Sign {
	case 1:
		return json.Number(new(big.Int).SetBytes(d.Content[0].Data).String())
	case -1:
		bData := new(big.Int).SetBytes(d.Content[0].Data)
		bData.Add(bData, big.NewInt(1))
		bData.Mul(bData, big.NewInt(-1))

		return json.Number(bData.String())
	default:
		return d.Content[0].dataValue()
	}
}

func (d DataItem) simpleOrFloatValue() any {
	switch string(d.Data) {
	case "true":
		return true
	case "false":
		return false
	case "<nil>":
		return nil
	case "undefined":
		return "undefined"
	default:
		if len(d.Data) == 1 {
			return int(d.Data[0])
		}

		value := math.Float64frombits(binary.BigEndian.Uint64(d.Data))
		switch {
		case value == math.Inf(1):
			return "Infinity"
		case value == math.Inf(-1):
			return "-Infinity"
		case math.IsNaN(value):
			return "NaN"
		case value == math.Trunc(value):
			return json.Number(fmt.Sprintf("%.1f", value))
		default:
			return value
		}
	}
}
