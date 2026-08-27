package llm

import (
	"strings"
	"testing"
)

func TestExtractFromCast_PlainSession(t *testing.T) {
	cast := `{"version":2,"width":80,"height":24,"timestamp":1700000000}` + "\n" +
		`[0.1, "o", "hello\r\n"]` + "\n" +
		`[0.2, "r", "120x40"]` + "\n" +
		`[0.3, "o", "world\r\n"]` + "\n"
	got, err := ExtractFromCast(strings.NewReader(cast))
	if err != nil {
		t.Fatal(err)
	}
	want := "hello\nworld\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractFromCast_StripsANSI(t *testing.T) {
	// Bash-style coloured prompt + clear-to-EOL.
	cast := `{"version":2,"width":80,"height":24,"timestamp":1}` + "\n" +
		`[0.1, "o", "\u001b[1;32m$ \u001b[0m\u001b[Kls\r\n"]` + "\n"
	got, err := ExtractFromCast(strings.NewReader(cast))
	if err != nil {
		t.Fatal(err)
	}
	want := "$ ls\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractFromCast_TailTrim(t *testing.T) {
	// Build a cast with > MaxExtractBytes of output. Each "o" event
	// emits 1 KB of 'a's; produce 100 of them = 100 KB.
	var b strings.Builder
	b.WriteString(`{"version":2,"width":80,"height":24,"timestamp":1}` + "\n")
	chunk := strings.Repeat("a", 1024)
	for i := 0; i < 100; i++ {
		b.WriteString(`[`)
		b.WriteString("0.001,")
		b.WriteString(` "o", "`)
		b.WriteString(chunk)
		b.WriteString(`"]`)
		b.WriteByte('\n')
	}
	got, err := ExtractFromCast(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxExtractBytes {
		t.Errorf("len = %d, want %d (MaxExtractBytes)", len(got), MaxExtractBytes)
	}
	// And it should be the TAIL — last byte should be 'a' from the
	// final chunk, not e.g. a header byte.
	if got[len(got)-1] != 'a' {
		t.Errorf("tail byte = %c, want 'a'", got[len(got)-1])
	}
}

func TestExtractFromCast_SkipsResizeAndInput(t *testing.T) {
	cast := `{"version":2,"width":80,"height":24,"timestamp":1}` + "\n" +
		`[0.1, "i", "ls\n"]` + "\n" +
		`[0.2, "r", "120x40"]` + "\n" +
		`[0.3, "o", "real output\n"]` + "\n"
	got, err := ExtractFromCast(strings.NewReader(cast))
	if err != nil {
		t.Fatal(err)
	}
	if got != "real output\n" {
		t.Errorf("got %q, want %q", got, "real output\n")
	}
}

func TestStripANSI_OSCBeyondCSI(t *testing.T) {
	// OSC 0 (set window title) terminated by BEL.
	in := "before\x1b]0;my title\x07after"
	got := stripANSI(in)
	if got != "beforeafter" {
		t.Errorf("got %q", got)
	}
}

func TestStripANSI_BackspaceClearsPriorChar(t *testing.T) {
	in := "abc\bd"
	got := stripANSI(in)
	if got != "abd" {
		t.Errorf("got %q, want %q", got, "abd")
	}
}
