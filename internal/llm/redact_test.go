package llm

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantSub   string // substring that MUST appear in the output
		wantNoSub string // substring that MUST NOT appear in the output
	}{
		{
			name:      "env_secret_export",
			in:        `export ANTHROPIC_API_KEY=sk-ant-abc123def456ghi789jkl012`,
			wantNoSub: "sk-ant-abc123def456ghi789jkl012",
			wantSub:   "<redacted>",
		},
		{
			name:      "env_secret_password",
			in:        `db_password = "hunter2-correct-horse"`,
			wantNoSub: "hunter2-correct-horse",
			wantSub:   "<redacted>",
		},
		{
			name:      "bearer_header",
			in:        `Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.foo`,
			wantNoSub: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			wantSub:   "<redacted>",
		},
		{
			name:      "x_api_key_header",
			in:        `x-api-key: ak_live_aabbccddeeffgghhiijjkkll`,
			wantNoSub: "ak_live_aabbccddeeffgghhiijjkkll",
			wantSub:   "<redacted>",
		},
		{
			name:      "long_blob_unattributed",
			in:        `echo aGVsbG93b3JsZHRoaXNpc2FsbG9uZ2Jhc2U2NHN0cmluZ2luY29ubW9uIHVzZXNkZWxpYmVyYXRlbHkgbG9uZw==`,
			wantNoSub: "aGVsbG93b3JsZHRoaXNpc2FsbG9uZ2Jhc2U2NHN0cmluZ2luY29ubW9uIHVzZXNkZWxpYmVyYXRlbHkgbG9uZw",
			wantSub:   "<redacted: long blob>",
		},
		{
			name:      "pem_body_line",
			in:        "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDLY1vT8hG7fMDA",
			wantNoSub: "MIIEvQIBADANBgkqhkiG9w0BAQE",
			wantSub:   "<redacted: PEM body>",
		},
		{
			name:    "ordinary_command_unchanged",
			in:      "ls -la /var/log",
			wantSub: "ls -la /var/log",
		},
		{
			name:    "prose_with_apikey_word_unchanged",
			in:      "the apikey docs are in the wiki",
			wantSub: "the apikey docs are in the wiki",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := Redact(tc.in)
			if tc.wantSub != "" && !strings.Contains(got, tc.wantSub) {
				t.Errorf("missing %q in:\n  %s", tc.wantSub, got)
			}
			if tc.wantNoSub != "" && strings.Contains(got, tc.wantNoSub) {
				t.Errorf("did not redact %q from:\n  %s", tc.wantNoSub, got)
			}
		})
	}
}

func TestRedact_PreservesLineCount(t *testing.T) {
	in := "line1\nexport TOKEN=abc\nline3\nAuthorization: Bearer xyz\nline5"
	out, n := Redact(in)
	if n != 2 {
		t.Errorf("redaction count = %d, want 2", n)
	}
	if got, want := strings.Count(out, "\n"), strings.Count(in, "\n"); got != want {
		t.Errorf("line count changed: got %d newlines, want %d", got, want)
	}
}
