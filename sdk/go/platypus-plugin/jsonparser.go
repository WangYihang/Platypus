package platypus

import (
	"errors"
	"fmt"
	"strconv"
	"unicode/utf16"
)

// jsonParser is a tiny hand-written JSON tokeniser used by the SDK
// to decode envelope responses without pulling in TinyGo's
// reflect-based encoding/json (which panics on Type.Implements
// during its type-cache init for any reachable map / RawMessage —
// see envelope.go for the full rationale).
//
// Supports the well-formed JSON subset host responses use:
//
//   - objects (recursive skipping)
//   - arrays (recursive skipping)
//   - strings (with the standard escapes + \uHHHH including UTF-16 surrogate pairs)
//   - numbers (skipped, never typed-decoded)
//   - true / false / null literals
//
// No comments, no trailing commas, no NaN/Infinity. The agent's
// envelope encoder is well-behaved Go encoding/json; we don't need
// the relaxed forms.
type jsonParser struct {
	buf []byte
	pos int
}

func parseErr(msg string, pos int) error {
	return fmt.Errorf("platypus: envelope parse: %s at offset %d", msg, pos)
}

func (p *jsonParser) skipWhitespace() {
	for p.pos < len(p.buf) {
		switch p.buf[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *jsonParser) consume(c byte) bool {
	if p.pos < len(p.buf) && p.buf[p.pos] == c {
		p.pos++
		return true
	}
	return false
}

func (p *jsonParser) readBool() (bool, error) {
	if p.match("true") {
		return true, nil
	}
	if p.match("false") {
		return false, nil
	}
	return false, parseErr("expect bool", p.pos)
}

func (p *jsonParser) match(s string) bool {
	if p.pos+len(s) > len(p.buf) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if p.buf[p.pos+i] != s[i] {
			return false
		}
	}
	p.pos += len(s)
	return true
}

// readUint64 decodes a non-negative integer literal. Numbers with
// fractional/exponent parts return an error so the caller can fall
// back to a float decoder if needed.
func (p *jsonParser) readUint64() (uint64, error) {
	start := p.pos
	for p.pos < len(p.buf) && p.buf[p.pos] >= '0' && p.buf[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start {
		return 0, parseErr("expect uint", p.pos)
	}
	v, err := strconv.ParseUint(string(p.buf[start:p.pos]), 10, 64)
	if err != nil {
		return 0, parseErr("bad uint", start)
	}
	return v, nil
}

// readInt64 decodes a signed integer literal (optional leading '-').
func (p *jsonParser) readInt64() (int64, error) {
	neg := false
	if p.consume('-') {
		neg = true
	}
	start := p.pos
	for p.pos < len(p.buf) && p.buf[p.pos] >= '0' && p.buf[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start {
		return 0, parseErr("expect int", p.pos)
	}
	v, err := strconv.ParseInt(string(p.buf[start:p.pos]), 10, 64)
	if err != nil {
		return 0, parseErr("bad int", start)
	}
	if neg {
		v = -v
	}
	return v, nil
}

// fieldHandler is one (key, callback) pair for parseObject. The
// callback parses the value at p.pos for the matching key; unknown
// keys are skipped automatically.
type fieldHandler struct {
	Key     string
	Handler func(p *jsonParser) error
}

// parseObject visits each field of a JSON object {…}. Fields not
// listed in `fields` are skipped via skipValue. Returns on the
// closing '}'.
func parseObject(p *jsonParser, fields []fieldHandler) error {
	p.skipWhitespace()
	if !p.consume('{') {
		return parseErr("expect '{'", p.pos)
	}
	for {
		p.skipWhitespace()
		if p.consume('}') {
			return nil
		}
		key, err := p.readString()
		if err != nil {
			return err
		}
		p.skipWhitespace()
		if !p.consume(':') {
			return parseErr("expect ':'", p.pos)
		}
		p.skipWhitespace()
		handled := false
		for _, fh := range fields {
			if fh.Key == key {
				if err := fh.Handler(p); err != nil {
					return err
				}
				handled = true
				break
			}
		}
		if !handled {
			if err := p.skipValue(); err != nil {
				return err
			}
		}
		p.skipWhitespace()
		if p.consume(',') {
			continue
		}
		p.skipWhitespace()
		if !p.consume('}') {
			return parseErr("expect ',' or '}'", p.pos)
		}
		return nil
	}
}

// readString decodes a JSON string literal starting at p.pos.
// Handles the standard escape set + \uHHHH (with UTF-16 surrogate
// pair recombination for code points above U+FFFF).
func (p *jsonParser) readString() (string, error) {
	if p.pos >= len(p.buf) || p.buf[p.pos] != '"' {
		return "", parseErr("expect string", p.pos)
	}
	p.pos++ // opening quote
	out := make([]byte, 0, 32)
	for p.pos < len(p.buf) {
		c := p.buf[p.pos]
		switch c {
		case '"':
			p.pos++
			return string(out), nil
		case '\\':
			p.pos++
			if p.pos >= len(p.buf) {
				return "", parseErr("truncated escape", p.pos)
			}
			esc := p.buf[p.pos]
			p.pos++
			switch esc {
			case '"', '\\', '/':
				out = append(out, esc)
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'u':
				r, err := p.readHex4()
				if err != nil {
					return "", err
				}
				// Surrogate pair: high surrogate followed by \uDxxx.
				if utf16.IsSurrogate(rune(r)) && p.pos+6 <= len(p.buf) &&
					p.buf[p.pos] == '\\' && p.buf[p.pos+1] == 'u' {
					p.pos += 2
					r2, err := p.readHex4()
					if err != nil {
						return "", err
					}
					out = appendRune(out, utf16.DecodeRune(rune(r), rune(r2)))
				} else {
					out = appendRune(out, rune(r))
				}
			default:
				return "", parseErr("bad escape", p.pos)
			}
		default:
			out = append(out, c)
			p.pos++
		}
	}
	return "", parseErr("unterminated string", p.pos)
}

func (p *jsonParser) readHex4() (uint32, error) {
	if p.pos+4 > len(p.buf) {
		return 0, parseErr("truncated \\u escape", p.pos)
	}
	v, err := strconv.ParseUint(string(p.buf[p.pos:p.pos+4]), 16, 32)
	if err != nil {
		return 0, parseErr("bad \\u escape", p.pos)
	}
	p.pos += 4
	return uint32(v), nil
}

func appendRune(dst []byte, r rune) []byte {
	switch {
	case r < 0x80:
		return append(dst, byte(r))
	case r < 0x800:
		return append(dst,
			byte(0xc0|(r>>6)),
			byte(0x80|(r&0x3f)),
		)
	case r < 0x10000:
		return append(dst,
			byte(0xe0|(r>>12)),
			byte(0x80|((r>>6)&0x3f)),
			byte(0x80|(r&0x3f)),
		)
	default:
		return append(dst,
			byte(0xf0|(r>>18)),
			byte(0x80|((r>>12)&0x3f)),
			byte(0x80|((r>>6)&0x3f)),
			byte(0x80|(r&0x3f)),
		)
	}
}

// skipValue advances p.pos past one complete JSON value. Used to
// extract the raw bytes for the envelope's `data` field — the
// caller records p.pos before + after the call and slices the
// buffer.
func (p *jsonParser) skipValue() error {
	p.skipWhitespace()
	if p.pos >= len(p.buf) {
		return errors.New("platypus: envelope parse: unexpected end")
	}
	c := p.buf[p.pos]
	switch {
	case c == '"':
		return p.skipString()
	case c == '{':
		return p.skipObject()
	case c == '[':
		return p.skipArray()
	case c == 't':
		if p.match("true") {
			return nil
		}
	case c == 'f':
		if p.match("false") {
			return nil
		}
	case c == 'n':
		if p.match("null") {
			return nil
		}
	case c == '-' || (c >= '0' && c <= '9'):
		return p.skipNumber()
	}
	return parseErr("expect value", p.pos)
}

func (p *jsonParser) skipString() error {
	if !p.consume('"') {
		return parseErr("expect string", p.pos)
	}
	for p.pos < len(p.buf) {
		c := p.buf[p.pos]
		switch c {
		case '"':
			p.pos++
			return nil
		case '\\':
			p.pos++
			if p.pos < len(p.buf) {
				p.pos++ // skip the escaped char
			}
		default:
			p.pos++
		}
	}
	return parseErr("unterminated string", p.pos)
}

func (p *jsonParser) skipNumber() error {
	for p.pos < len(p.buf) {
		c := p.buf[p.pos]
		if c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' ||
			(c >= '0' && c <= '9') {
			p.pos++
			continue
		}
		break
	}
	return nil
}

func (p *jsonParser) skipObject() error {
	if !p.consume('{') {
		return parseErr("expect '{'", p.pos)
	}
	for {
		p.skipWhitespace()
		if p.consume('}') {
			return nil
		}
		if _, err := p.readString(); err != nil {
			return err
		}
		p.skipWhitespace()
		if !p.consume(':') {
			return parseErr("expect ':'", p.pos)
		}
		if err := p.skipValue(); err != nil {
			return err
		}
		p.skipWhitespace()
		if p.consume(',') {
			continue
		}
		p.skipWhitespace()
		if !p.consume('}') {
			return parseErr("expect ',' or '}'", p.pos)
		}
		return nil
	}
}

func (p *jsonParser) skipArray() error {
	if !p.consume('[') {
		return parseErr("expect '['", p.pos)
	}
	for {
		p.skipWhitespace()
		if p.consume(']') {
			return nil
		}
		if err := p.skipValue(); err != nil {
			return err
		}
		p.skipWhitespace()
		if p.consume(',') {
			continue
		}
		p.skipWhitespace()
		if !p.consume(']') {
			return parseErr("expect ',' or ']'", p.pos)
		}
		return nil
	}
}
