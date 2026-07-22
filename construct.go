package cbor

// Uint builds an unsigned-integer data item.
func Uint(n uint64) *DataItem { return &DataItem{Major: MajorUint, Argument: n} }

// Nint builds a negative-integer data item whose value is -1 - n (so Nint(0) is
// -1 and Nint(6) is -7). For a signed value, prefer Int.
func Nint(n uint64) *DataItem { return &DataItem{Major: MajorNint, Argument: n} }

// Int builds an integer data item from a signed value.
func Int(i int64) *DataItem {
	if i >= 0 {
		return &DataItem{Major: MajorUint, Argument: uint64(i)}
	}
	return &DataItem{Major: MajorNint, Argument: uint64(-1 - i)}
}

// Text builds a text-string data item.
func Text(s string) *DataItem { return &DataItem{Major: MajorText, Bytes: []byte(s)} }

// ByteString builds a byte-string data item.
func ByteString(b []byte) *DataItem { return &DataItem{Major: MajorBytes, Bytes: b} }

// ArrayOf builds an array data item from the given elements.
func ArrayOf(items ...*DataItem) *DataItem {
	return &DataItem{Major: MajorArray, Content: items}
}

// MapOf builds a map data item from a flat key, value, key, value… sequence.
func MapOf(pairs ...*DataItem) *DataItem {
	return &DataItem{Major: MajorMap, Content: pairs}
}

// TagOf wraps content in a tag with the given number.
func TagOf(number uint64, content *DataItem) *DataItem {
	return &DataItem{Major: MajorTag, Argument: number, Content: []*DataItem{content}}
}

// Float builds a double-precision (8-byte) float data item. It does not select the
// shortest width; set FloatWidth yourself, or encode via the reflect path (a plain
// float64), for canonical shortest-form floats.
func Float(f float64) *DataItem {
	return &DataItem{Major: MajorOther, Float: f, FloatWidth: 8}
}

// Bool builds a boolean simple-value data item.
func Bool(b bool) *DataItem {
	arg := uint64(20)
	if b {
		arg = 21
	}
	return &DataItem{Major: MajorOther, Argument: arg}
}

// Null builds the null simple value.
func Null() *DataItem { return &DataItem{Major: MajorOther, Argument: 22} }

// Simple builds a simple value (0–19 or 32–255).
func Simple(v byte) *DataItem { return &DataItem{Major: MajorOther, Argument: uint64(v)} }
