package cbor

import (
	"fmt"
	"reflect"
)

// SyntaxError reports malformed or not-well-formed CBOR input. Offset is the byte
// position at which the problem was detected.
type SyntaxError struct {
	Offset int64
	msg    string
}

func (e *SyntaxError) Error() string { return e.msg }

// UnmarshalTypeError reports a CBOR value that cannot be stored in the target Go
// type. Offset is the byte position of the value (0 if unknown).
type UnmarshalTypeError struct {
	CBORType string
	GoType   reflect.Type
	Offset   int64
}

func (e *UnmarshalTypeError) Error() string {
	return fmt.Sprintf("cbor: cannot unmarshal %s into Go value of type %s", e.CBORType, e.GoType)
}

// InvalidUnmarshalError reports an invalid argument to Unmarshal; the argument
// must be a non-nil pointer.
type InvalidUnmarshalError struct {
	Type reflect.Type
}

func (e *InvalidUnmarshalError) Error() string {
	if e.Type == nil {
		return "cbor: Unmarshal(nil)"
	}
	if e.Type.Kind() != reflect.Pointer {
		return "cbor: Unmarshal(non-pointer " + e.Type.String() + ")"
	}
	return "cbor: Unmarshal(nil " + e.Type.String() + ")"
}

// UnsupportedTypeError reports a Go type that cannot be encoded to CBOR.
type UnsupportedTypeError struct {
	Type reflect.Type
}

func (e *UnsupportedTypeError) Error() string {
	return "cbor: unsupported type: " + e.Type.String()
}
