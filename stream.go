package cbor

import (
	"bytes"
	"encoding/binary"
	"io"
	"reflect"
)

// Encoder writes a sequence of CBOR items to an io.Writer. It reuses its encoding
// state and buffer across Encode calls, so a high-rate stream allocates nothing
// per item beyond what encoding the value itself requires.
type Encoder struct {
	w  io.Writer
	es encodeState
}

// NewEncoder returns an Encoder that writes to w using the default (canonical)
// encoding. Use EncoderOptions.NewEncoder to configure it.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// NewEncoder returns an Encoder writing to w with the configured options.
func (o EncoderOptions) NewEncoder(w io.Writer) (*Encoder, error) {
	em, err := o.Encoding()
	if err != nil {
		return nil, err
	}
	m := em.(encoding)
	return &Encoder{w: w, es: encodeState{sort: m.sort, time: m.time, float: m.float, nan: m.nan, bigInt: m.bigInt}}, nil
}

// Encode writes the CBOR encoding of v to the stream. Nothing is written if
// encoding v fails, so a failed item cannot corrupt the stream.
func (e *Encoder) Encode(v any) error {
	e.es.buf = e.es.buf[:0]
	if err := e.es.encode(v); err != nil {
		return err
	}
	_, err := e.w.Write(e.es.buf)
	return err
}

// Decoder reads a sequence of CBOR items from an io.Reader. It reads only as many
// bytes as each item requires, so it is suitable for streaming from a connection
// that stays open between items: Decode returns as soon as one complete item has
// arrived rather than waiting for the reader to reach EOF.
type Decoder struct {
	r        io.Reader
	buf      []byte
	readBuf  []byte
	pos      int
	consumed int64
	dm       decoding
	hasDM    bool
	stack    []frame
	frameOff int
	depth    int
}

// InputOffset returns the number of bytes consumed from the stream so far — the
// position at which the next item begins.
func (d *Decoder) InputOffset() int64 { return d.consumed }

// Buffered returns a reader over bytes read from the underlying reader but not yet
// consumed by Decode. Use it to hand a connection to another protocol after reading
// a known number of CBOR items.
func (d *Decoder) Buffered() io.Reader {
	return bytes.NewReader(d.buf[d.pos:])
}

// NewDecoder returns a Decoder that reads from r with default decoding. Use
// DecoderOptions.NewDecoder to configure it.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: r}
}

// NewDecoder returns a Decoder reading from r with the configured options.
func (o DecoderOptions) NewDecoder(r io.Reader) (*Decoder, error) {
	dm, err := o.Decoding()
	if err != nil {
		return nil, err
	}
	return &Decoder{r: r, dm: dm.(decoding), hasDM: true}, nil
}

// Decode reads the next CBOR item from the stream into v, returning io.EOF once
// the stream is exhausted. It reads exactly enough bytes to frame one item, so a
// reader that stays open after the item does not block the call. A truncated final
// item returns io.ErrUnexpectedEOF. A malformed item is a terminal condition: the
// error recurs on each subsequent call rather than resyncing into the garbage, so
// treat a non-EOF error as the end of the stream and stop reading. A Decoder is not
// safe for concurrent use.
func (d *Decoder) Decode(v any) error {
	rv := reflect.ValueOf(v)
	if v == nil || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &InvalidUnmarshalError{Type: reflect.TypeOf(v)}
	}

	n, err := d.nextItem()
	if err != nil {
		return err
	}
	item := d.buf[d.pos : d.pos+n]

	var s *decodeState
	if d.hasDM {
		s = d.dm.newState()
	} else {
		s = newDecodeState()
	}
	s.data = item
	if _, err := s.reflect(item, rv.Elem()); err != nil {
		return err
	}
	d.pos += n
	d.consumed += int64(n)
	return nil
}

func (d *Decoder) nextItem() (int, error) {
	lim := scanLimits{maxDepth: defaultMaxDepth}
	if d.hasDM {
		lim = scanLimits{
			maxDepth:    d.dm.maxDepth,
			maxString:   d.dm.maxString,
			maxArray:    d.dm.maxArray,
			maxMap:      d.dm.maxMap,
			rejectIndef: d.dm.rejectIndef,
		}
	}
	d.stack = append(d.stack[:0], frame{kind: frameSeq, need: 1})
	d.frameOff = 0
	d.depth = 0
	for {
		n, st := d.frameNext(d.buf[d.pos:], lim)
		switch st {
		case scanComplete:
			return n, nil
		case scanInvalid:
			return 0, d.frameError()
		}
		if d.pos > 0 {
			d.buf = append(d.buf[:0], d.buf[d.pos:]...)
			d.pos = 0
		}
		if d.readBuf == nil {
			d.readBuf = make([]byte, 4096)
		}
		m, err := d.r.Read(d.readBuf)
		if m > 0 {
			d.buf = append(d.buf, d.readBuf[:m]...)
		}
		if err != nil {
			if err == io.EOF {
				if len(d.buf) == d.pos {
					return 0, io.EOF
				}
				return 0, io.ErrUnexpectedEOF
			}
			return 0, err
		}
	}
}

func (d *Decoder) frameError() error {
	var s *decodeState
	if d.hasDM {
		s = d.dm.newState()
	} else {
		s = newDecodeState()
	}
	s.data = d.buf[d.pos:]
	if _, _, err := s.decodeItem(d.buf[d.pos:]); err != nil {
		if se, ok := err.(*SyntaxError); ok {
			se.Offset += d.consumed
		}
		return err
	}
	return &SyntaxError{Offset: d.consumed, msg: "cbor: malformed item in stream"}
}

type scanStatus int

const (
	scanComplete scanStatus = iota
	scanIncomplete
	scanInvalid
)

type scanLimits struct {
	maxDepth    int
	maxString   int
	maxArray    int
	maxMap      int
	rejectIndef bool
}

const (
	frameSeq uint8 = iota
	frameIndefArray
	frameIndefMap
	frameIndefStr
)

type frame struct {
	kind     uint8
	descend  bool
	strMajor byte
	need     uint64
	seen     uint64
}

func parseHead(buf []byte) (major, ai byte, arg uint64, headLen int, st scanStatus) {
	if len(buf) == 0 {
		return 0, 0, 0, 0, scanIncomplete
	}
	major = buf[0] >> 5
	ai = buf[0] & 0x1f
	headLen = 1
	switch {
	case ai < 24:
		arg = uint64(ai)
	case ai == 24:
		if len(buf) < 2 {
			return 0, 0, 0, 0, scanIncomplete
		}
		arg, headLen = uint64(buf[1]), 2
	case ai == 25:
		if len(buf) < 3 {
			return 0, 0, 0, 0, scanIncomplete
		}
		arg, headLen = uint64(binary.BigEndian.Uint16(buf[1:3])), 3
	case ai == 26:
		if len(buf) < 5 {
			return 0, 0, 0, 0, scanIncomplete
		}
		arg, headLen = uint64(binary.BigEndian.Uint32(buf[1:5])), 5
	case ai == 27:
		if len(buf) < 9 {
			return 0, 0, 0, 0, scanIncomplete
		}
		arg, headLen = binary.BigEndian.Uint64(buf[1:9]), 9
	case ai == 31:
	default:
		return major, ai, 0, 0, scanInvalid
	}
	return major, ai, arg, headLen, scanComplete
}

func (d *Decoder) descendFrame(lim scanLimits, f frame) scanStatus {
	if d.depth+1 > lim.maxDepth {
		return scanInvalid
	}
	d.depth++
	d.stack = append(d.stack, f)
	return scanComplete
}

func (d *Decoder) popFrame() {
	last := len(d.stack) - 1
	if d.stack[last].descend {
		d.depth--
	}
	d.stack = d.stack[:last]
}

func (d *Decoder) frameNext(buf []byte, lim scanLimits) (int, scanStatus) {
	off := d.frameOff
	for {
		if len(d.stack) == 0 {
			d.frameOff = off
			return off, scanComplete
		}
		ti := len(d.stack) - 1
		top := &d.stack[ti]
		if top.kind != frameSeq {
			if off >= len(buf) {
				d.frameOff = off
				return 0, scanIncomplete
			}
			b := buf[off]
			if b == breakByte {
				if top.kind == frameIndefMap && top.seen%2 != 0 {
					return 0, scanInvalid
				}
				off++
				d.popFrame()
				continue
			}
			switch top.kind {
			case frameIndefStr:
				if b>>5 != top.strMajor || b&0x1f == 0x1f {
					return 0, scanInvalid
				}
			case frameIndefArray:
				if lim.maxArray > 0 && top.seen >= uint64(lim.maxArray) {
					return 0, scanInvalid
				}
			case frameIndefMap:
				if top.seen%2 == 0 && lim.maxMap > 0 && top.seen/2 >= uint64(lim.maxMap) {
					return 0, scanInvalid
				}
			}
		} else if top.need == 0 {
			d.popFrame()
			continue
		}
		n, st := d.frameChild(buf, off, lim)
		if st != scanComplete {
			d.frameOff = off
			return 0, st
		}
		top = &d.stack[ti]
		if top.kind == frameSeq {
			top.need--
		} else {
			top.seen++
		}
		off += n
	}
}

func (d *Decoder) frameChild(buf []byte, off int, lim scanLimits) (int, scanStatus) {
	major, ai, arg, headLen, st := parseHead(buf[off:])
	if st != scanComplete {
		return 0, st
	}
	switch major {
	case 0, 1:
		if ai == 0x1f {
			return 0, scanInvalid
		}
		return headLen, scanComplete
	case 2, 3:
		if ai == 0x1f {
			if lim.rejectIndef {
				return 0, scanInvalid
			}
			d.stack = append(d.stack, frame{kind: frameIndefStr, strMajor: major})
			return headLen, scanComplete
		}
		if lim.maxString > 0 && arg > uint64(lim.maxString) {
			return 0, scanInvalid
		}
		if arg > uint64(len(buf)-off-headLen) {
			return 0, scanIncomplete
		}
		return headLen + int(arg), scanComplete
	case 4:
		if ai == 0x1f {
			if lim.rejectIndef {
				return 0, scanInvalid
			}
			if st := d.descendFrame(lim, frame{kind: frameIndefArray, descend: true}); st != scanComplete {
				return 0, st
			}
			return headLen, scanComplete
		}
		if lim.maxArray > 0 && arg > uint64(lim.maxArray) {
			return 0, scanInvalid
		}
		if st := d.descendFrame(lim, frame{kind: frameSeq, descend: true, need: arg}); st != scanComplete {
			return 0, st
		}
		return headLen, scanComplete
	case 5:
		if ai == 0x1f {
			if lim.rejectIndef {
				return 0, scanInvalid
			}
			if st := d.descendFrame(lim, frame{kind: frameIndefMap, descend: true}); st != scanComplete {
				return 0, st
			}
			return headLen, scanComplete
		}
		if lim.maxMap > 0 && arg > uint64(lim.maxMap) {
			return 0, scanInvalid
		}
		need := arg * 2
		if arg >= 1<<63 {
			need = ^uint64(0)
		}
		if st := d.descendFrame(lim, frame{kind: frameSeq, descend: true, need: need}); st != scanComplete {
			return 0, st
		}
		return headLen, scanComplete
	case 6:
		if ai == 0x1f {
			return 0, scanInvalid
		}
		if st := d.descendFrame(lim, frame{kind: frameSeq, descend: true, need: 1}); st != scanComplete {
			return 0, st
		}
		return headLen, scanComplete
	default:
		if ai == 0x1f {
			return 0, scanInvalid
		}
		return headLen, scanComplete
	}
}
