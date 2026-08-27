package llm

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// MaxExtractBytes caps the prompt size to keep input-token cost
// bounded and fit comfortably in any model's context. Tail-trim
// (see ExtractFromCast) keeps the LAST 50 KB because the
// operator's intent is usually clearest near the end of the
// session — final commands + final outputs.
const MaxExtractBytes = 50 * 1024

// ExtractFromCast reads an asciinema v2 .cast file and returns the
// human-readable terminal text suitable for an LLM prompt:
//
//   - drops the JSON header and resize ("r") events
//   - decodes "o" output events into their raw bytes
//   - strips ANSI / DEC private-mode escape sequences
//   - tail-trims to MaxExtractBytes
//
// Errors propagate from the reader; a malformed event line is
// skipped silently (matches asciinema-player's tolerant parser).
func ExtractFromCast(r io.Reader) (string, error) {
	var buf strings.Builder
	scanner := bufio.NewScanner(r)
	// Cast lines can be long-ish — bash prompts with PROMPT_COMMAND
	// hit ~1 KB easily. Bump the buffer to the same cap as the
	// extract output so we never short-read.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	first := true
	for scanner.Scan() {
		line := scanner.Bytes()
		if first {
			// Header line (JSON object). Skip.
			first = false
			continue
		}
		if len(line) == 0 || line[0] != '[' {
			continue
		}
		var ev []json.RawMessage
		if err := json.Unmarshal(line, &ev); err != nil || len(ev) < 3 {
			continue
		}
		var kind string
		if err := json.Unmarshal(ev[1], &kind); err != nil {
			continue
		}
		if kind != "o" {
			// Skip resize / input / marker events.
			continue
		}
		var payload string
		if err := json.Unmarshal(ev[2], &payload); err != nil {
			continue
		}
		buf.WriteString(payload)
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	cleaned := stripANSI(buf.String())
	if len(cleaned) > MaxExtractBytes {
		cleaned = cleaned[len(cleaned)-MaxExtractBytes:]
	}
	return cleaned, nil
}

// stripANSI removes the escape sequences a normal bash session
// emits (CSI colours, cursor moves, mode toggles). We don't need
// terminal-grade fidelity — the LLM only consumes plain text. The
// hand-rolled state machine handles:
//
//   - CSI (ESC [ ... letter)
//   - OSC (ESC ] ... ESC \ or BEL)
//   - DEC private-mode (ESC [ ? ... letter)  → falls into CSI
//   - single-char ESC sequences (ESC = / ESC > etc.)
//
// We also drop \r so the LLM sees clean line breaks.
func stripANSI(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case 0x1b: // ESC
			if i+1 >= len(s) {
				return out.String()
			}
			next := s[i+1]
			switch next {
			case '[':
				// CSI: skip until a letter (final byte 0x40-0x7e).
				j := i + 2
				for j < len(s) && !isCSIFinal(s[j]) {
					j++
				}
				if j < len(s) {
					j++ // consume the final byte
				}
				i = j
			case ']':
				// OSC: skip until BEL (0x07) or ESC \ (ST).
				j := i + 2
				for j < len(s) && s[j] != 0x07 {
					if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				if j < len(s) && s[j] == 0x07 {
					j++
				}
				i = j
			default:
				// Single-char sequence (ESC = / ESC > / ESC 7 / ESC 8 / etc.).
				i += 2
			}
		case '\r':
			i++
		case '\b':
			// Backspace: drop both the backspace and the previous
			// char in our output. Keeps `cd /usr/local/bin` clean
			// from interactive line editing.
			s := out.String()
			if len(s) > 0 {
				out.Reset()
				out.WriteString(s[:len(s)-1])
			}
			i++
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.String()
}

func isCSIFinal(b byte) bool {
	return b >= 0x40 && b <= 0x7e
}
