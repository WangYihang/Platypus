package api

import (
	"os"
	"path/filepath"
	"strings"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// RestFileEntry is the JSON-facing wrapper for the proto FileEntry that
// adds an extension-derived mime type. Field tags mirror the proto's
// generated JSON tags so the frontend keeps consuming the same shape.
type RestFileEntry struct {
	Name          string `json:"name,omitempty"`
	Mode          uint32 `json:"mode,omitempty"`
	Size          int64  `json:"size,omitempty"`
	MtimeUnixNano int64  `json:"mtime_unix_nano,omitempty"`
	SymlinkTarget string `json:"symlink_target,omitempty"`
	Mime          string `json:"mime,omitempty"`
}

// EnrichFileEntry converts a proto FileEntry into its REST DTO and
// fills in the mime field. Returns nil for a nil input so callers can
// pass through StatResponse.Entry without a guard.
func EnrichFileEntry(e *v2pb.FileEntry) *RestFileEntry {
	if e == nil {
		return nil
	}
	return &RestFileEntry{
		Name:          e.GetName(),
		Mode:          e.GetMode(),
		Size:          e.GetSize(),
		MtimeUnixNano: e.GetMtimeUnixNano(),
		SymlinkTarget: e.GetSymlinkTarget(),
		Mime:          MimeFromEntry(e.GetName(), e.GetMode(), e.GetSymlinkTarget()),
	}
}

// EnrichFileEntries maps EnrichFileEntry over a slice.
func EnrichFileEntries(entries []*v2pb.FileEntry) []*RestFileEntry {
	out := make([]*RestFileEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, EnrichFileEntry(e))
	}
	return out
}

// MimeFromEntry returns a MIME type for a directory entry based on its
// name and mode. Detection is intentionally extension-only so it can run
// over a directory listing without extra I/O. Directories and symlinks
// are reported with the "inode/" prefix used by file(1) so the frontend
// can short-circuit non-file rendering.
//
// Unknown types fall back to application/octet-stream.
func MimeFromEntry(name string, mode uint32, symlinkTarget string) string {
	fm := os.FileMode(mode)
	if fm.IsDir() {
		return "inode/directory"
	}
	if fm&os.ModeSymlink != 0 || symlinkTarget != "" {
		return "inode/symlink"
	}

	// Match against full lower-cased name to support compound extensions
	// like "tar.gz" before falling back to filepath.Ext.
	lower := strings.ToLower(name)
	for _, suffix := range compoundSuffixes {
		if strings.HasSuffix(lower, suffix.ext) {
			return suffix.mime
		}
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return "application/octet-stream"
	}
	if mt, ok := extToMime[ext]; ok {
		return mt
	}
	return "application/octet-stream"
}

var compoundSuffixes = []struct {
	ext  string
	mime string
}{
	{".tar.gz", "application/gzip"},
	{".tar.bz2", "application/x-bzip2"},
	{".tar.xz", "application/x-xz"},
}

var extToMime = map[string]string{
	// Images
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".tif":  "image/tiff",
	".tiff": "image/tiff",
	".avif": "image/avif",
	".heic": "image/heic",

	// Documents
	".pdf": "application/pdf",

	// Audio
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".aac":  "audio/aac",
	".opus": "audio/opus",

	// Video
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".mov":  "video/quicktime",
	".webm": "video/webm",
	".mkv":  "video/x-matroska",
	".avi":  "video/x-msvideo",

	// Text / source
	".txt":  "text/plain",
	".log":  "text/plain",
	".md":   "text/markdown",
	".markdown": "text/markdown",
	".json": "application/json",
	".jsonc": "application/json",
	".yaml": "application/yaml",
	".yml":  "application/yaml",
	".toml": "application/toml",
	".xml":  "application/xml",
	".html": "text/html",
	".htm":  "text/html",
	".css":  "text/css",
	".js":   "text/javascript",
	".mjs":  "text/javascript",
	".cjs":  "text/javascript",
	".ts":   "text/typescript",
	".tsx":  "text/typescript",
	".jsx":  "text/javascript",
	".go":   "text/x-go",
	".py":   "text/x-python",
	".rb":   "text/x-ruby",
	".rs":   "text/x-rust",
	".java": "text/x-java",
	".c":    "text/x-c",
	".h":    "text/x-c",
	".cpp":  "text/x-c++",
	".cc":   "text/x-c++",
	".hpp":  "text/x-c++",
	".cs":   "text/x-csharp",
	".php":  "application/x-php",
	".sh":   "text/x-shellscript",
	".bash": "text/x-shellscript",
	".zsh":  "text/x-shellscript",
	".fish": "text/x-shellscript",
	".ps1":  "application/x-powershell",
	".sql":  "application/sql",
	".csv":  "text/csv",
	".tsv":  "text/tab-separated-values",
	".ini":  "text/plain",
	".conf": "text/plain",
	".env":  "text/plain",

	// Archives
	".zip": "application/zip",
	".tar": "application/x-tar",
	".gz":  "application/gzip",
	".bz2": "application/x-bzip2",
	".xz":  "application/x-xz",
	".7z":  "application/x-7z-compressed",
	".rar": "application/vnd.rar",
}
