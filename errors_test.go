package cbor_test

import (
	"errors"
	"strings"
	"testing"

	cbor "github.com/MichaelFraser99/go-cbor"
)

func TestInvalidUnmarshalErrorMessages(t *testing.T) {
	var target *cbor.InvalidUnmarshalError

	err := cbor.Unmarshal([]byte{0x00}, nil)
	if !errors.As(err, &target) || err.Error() != "cbor: Unmarshal(nil)" {
		t.Errorf("Unmarshal(nil) = %v", err)
	}

	var i int
	err = cbor.Unmarshal([]byte{0x00}, i)
	if !errors.As(err, &target) || !strings.Contains(err.Error(), "non-pointer int") {
		t.Errorf("Unmarshal(non-pointer) = %v", err)
	}

	var p *int
	err = cbor.Unmarshal([]byte{0x00}, p)
	if !errors.As(err, &target) || !strings.Contains(err.Error(), "nil *int") {
		t.Errorf("Unmarshal(nil pointer) = %v", err)
	}
}

func TestUnsupportedTypeErrorMessage(t *testing.T) {
	var target *cbor.UnsupportedTypeError
	_, err := cbor.Marshal(make(chan int))
	if !errors.As(err, &target) {
		t.Fatalf("Marshal(chan) = %v, want *UnsupportedTypeError", err)
	}
	if !strings.Contains(err.Error(), "unsupported type") || !strings.Contains(err.Error(), "chan int") {
		t.Errorf("message = %q", err.Error())
	}
}

func TestSyntaxErrorMessage(t *testing.T) {
	var target *cbor.SyntaxError
	var v any
	err := cbor.Unmarshal([]byte{0x1c}, &v)
	if !errors.As(err, &target) {
		t.Fatalf("err = %T %v, want *SyntaxError", err, err)
	}
	if target.Error() == "" {
		t.Error("empty SyntaxError message")
	}
}

func TestUnmarshalTypeErrorMessage(t *testing.T) {
	var target *cbor.UnmarshalTypeError
	var i int
	err := cbor.Unmarshal([]byte{0x61, 0x61}, &i) // text string into int
	if !errors.As(err, &target) {
		t.Fatalf("err = %T %v, want *UnmarshalTypeError", err, err)
	}
	if !strings.Contains(target.Error(), "cannot unmarshal") {
		t.Errorf("message = %q", target.Error())
	}
}
