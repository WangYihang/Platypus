package api

import (
	"encoding/json"
	"os"
	"testing"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func TestMimeFromEntry(t *testing.T) {
	tests := []struct {
		name          string
		entryName     string
		mode          uint32
		symlinkTarget string
		want          string
	}{
		// Directories take precedence over name.
		{"plain dir", "src", uint32(os.ModeDir | 0o755), "", "inode/directory"},
		{"dir with dotted name", "foo.png", uint32(os.ModeDir | 0o755), "", "inode/directory"},

		// Symlinks (unresolved) reported as inode/symlink.
		{"symlink", "link", uint32(os.ModeSymlink | 0o777), "/etc/hosts", "inode/symlink"},

		// Common image types.
		{"png", "logo.png", 0o644, "", "image/png"},
		{"jpeg", "photo.jpg", 0o644, "", "image/jpeg"},
		{"jpeg alt ext", "photo.jpeg", 0o644, "", "image/jpeg"},
		{"gif", "anim.gif", 0o644, "", "image/gif"},
		{"webp", "pic.webp", 0o644, "", "image/webp"},
		{"svg", "icon.svg", 0o644, "", "image/svg+xml"},
		{"bmp", "old.bmp", 0o644, "", "image/bmp"},
		{"ico", "favicon.ico", 0o644, "", "image/x-icon"},

		// PDF.
		{"pdf", "spec.pdf", 0o644, "", "application/pdf"},

		// Audio / video.
		{"mp3", "song.mp3", 0o644, "", "audio/mpeg"},
		{"wav", "voice.wav", 0o644, "", "audio/wav"},
		{"ogg audio", "tone.ogg", 0o644, "", "audio/ogg"},
		{"mp4", "clip.mp4", 0o644, "", "video/mp4"},
		{"webm", "clip.webm", 0o644, "", "video/webm"},
		{"mkv", "movie.mkv", 0o644, "", "video/x-matroska"},

		// Text-ish.
		{"plain text", "notes.txt", 0o644, "", "text/plain"},
		{"markdown", "README.md", 0o644, "", "text/markdown"},
		{"json", "data.json", 0o644, "", "application/json"},
		{"yaml", "config.yaml", 0o644, "", "application/yaml"},
		{"yaml short", "config.yml", 0o644, "", "application/yaml"},
		{"toml", "Cargo.toml", 0o644, "", "application/toml"},
		{"xml", "doc.xml", 0o644, "", "application/xml"},
		{"html", "index.html", 0o644, "", "text/html"},
		{"css", "main.css", 0o644, "", "text/css"},
		{"javascript", "app.js", 0o644, "", "text/javascript"},
		{"typescript", "app.ts", 0o644, "", "text/typescript"},
		{"tsx", "App.tsx", 0o644, "", "text/typescript"},
		{"go source", "main.go", 0o644, "", "text/x-go"},
		{"python", "main.py", 0o644, "", "text/x-python"},
		{"shell", "run.sh", 0o644, "", "text/x-shellscript"},
		{"csv", "rows.csv", 0o644, "", "text/csv"},
		{"log", "app.log", 0o644, "", "text/plain"},

		// Archives.
		{"zip", "code.zip", 0o644, "", "application/zip"},
		{"tar", "code.tar", 0o644, "", "application/x-tar"},
		{"gzip", "code.gz", 0o644, "", "application/gzip"},
		{"tar.gz", "code.tar.gz", 0o644, "", "application/gzip"},
		{"7z", "code.7z", 0o644, "", "application/x-7z-compressed"},

		// Case-insensitive.
		{"upper case ext", "PHOTO.JPG", 0o644, "", "image/jpeg"},
		{"mixed case ext", "Notes.MD", 0o644, "", "text/markdown"},

		// Unknown → octet-stream.
		{"no extension", "Makefile", 0o644, "", "application/octet-stream"},
		{"unknown ext", "blob.weirdext", 0o644, "", "application/octet-stream"},
		{"dotfile", ".bashrc", 0o644, "", "application/octet-stream"},
		{"empty name", "", 0o644, "", "application/octet-stream"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MimeFromEntry(tc.entryName, tc.mode, tc.symlinkTarget)
			if got != tc.want {
				t.Errorf("MimeFromEntry(%q, %#o, %q) = %q, want %q",
					tc.entryName, tc.mode, tc.symlinkTarget, got, tc.want)
			}
		})
	}
}

func TestEnrichFileEntry(t *testing.T) {
	in := &v2pb.FileEntry{
		Name:          "logo.png",
		Mode:          0o644,
		Size:          12345,
		MtimeUnixNano: 1700000000000000000,
	}
	out := EnrichFileEntry(in)

	if out.Name != "logo.png" || out.Size != 12345 || out.Mode != 0o644 ||
		out.MtimeUnixNano != 1700000000000000000 {
		t.Fatalf("base fields not preserved: %+v", out)
	}
	if out.Mime != "image/png" {
		t.Errorf("mime = %q, want image/png", out.Mime)
	}
}

func TestEnrichFileEntryNilSafe(t *testing.T) {
	if got := EnrichFileEntry(nil); got != nil {
		t.Errorf("EnrichFileEntry(nil) = %+v, want nil", got)
	}
}

func TestEnrichFileEntryJSONShape(t *testing.T) {
	out := EnrichFileEntry(&v2pb.FileEntry{
		Name: "spec.pdf",
		Mode: 0o644,
		Size: 99,
	})
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	got := string(b)
	// Must include the keys consumed by the existing frontend plus the
	// new mime key. Field names match the proto-generated tags so the
	// frontend keeps working unchanged for non-mime fields.
	for _, want := range []string{
		`"name":"spec.pdf"`,
		`"size":99`,
		`"mime":"application/pdf"`,
	} {
		if !contains(got, want) {
			t.Errorf("json missing %q: %s", want, got)
		}
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
