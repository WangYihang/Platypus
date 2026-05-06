// sys-files-read-go is the TinyGo port of
// example/plugins/system/sys-files-read. Five entry points share
// the same plugin id (one wasm module):
//
//   list_dir + stat                 ← cap fs.read (RPCs)
//   read (stream)                   ← cap fs.read (FILE_READ)
//   scan (stream)                   ← cap fs.read (FILE_SCAN)
//   archive (stream)                ← cap fs.read (FILE_ARCHIVE)
//
// Wire formats stay byte-for-byte identical to the Rust crate's
// output. Different plugin id (-go suffix) so both ship side-by-side.
//
// Archive format support:
//   - ARCHIVE_FORMAT_TAR    — supported (archive/tar stdlib)
//   - ARCHIVE_FORMAT_TAR_GZ — supported (archive/tar + compress/gzip)
//   - ARCHIVE_FORMAT_ZIP    — NOT supported (parity gap with Rust crate)
//
// Build: tinygo build -target wasi -o sys_files_read.wasm .
package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

const (
	wireVarint = 0
	wireLen    = 2

	fileChunkSize int64 = 256 * 1024
	flushBytes    int   = 256 * 1024

	archiveFormatUnspecified uint32 = 0
	archiveFormatTar         uint32 = 1
	archiveFormatTarGz       uint32 = 2
	archiveFormatZip         uint32 = 3
)

// ============================================================
// list_dir + stat (RPCs)
// ============================================================
//
// JSON in / JSON out — the bridge marshals the proto request to
// JSON and unmarshals the JSON response back. Same wire shape as
// the Rust crate (camelCase keys preserved by the bridge wrappers
// in internal/agent/plugin/bridge/listdir.go + stat.go).

//export list_dir
func listDir() int32 {
	path := decodeJSONStringField(pdk.Input(), "path")
	entries, err := platypus.HostFSListDir(path)
	if err != nil {
		emitListDirResponse(nil, err.Error())
		return 0
	}
	emitListDirResponse(entries, "")
	return 0
}

//export stat
func statEntry() int32 {
	path := decodeJSONStringField(pdk.Input(), "path")
	e, err := platypus.HostFSStat(path)
	if err != nil {
		emitStatResponse(nil, err.Error())
		return 0
	}
	emitStatResponse(&e, "")
	return 0
}

func emitListDirResponse(entries []platypus.FSListEntry, errMsg string) {
	var b strings.Builder
	b.WriteString(`{"entries":[`)
	for i, e := range entries {
		if i > 0 {
			b.WriteByte(',')
		}
		writeFileEntryJSON(&b, e)
	}
	b.WriteByte(']')
	if errMsg != "" {
		b.WriteString(`,"error":`)
		b.WriteString(platypus.EncodeJSONString(errMsg))
	}
	b.WriteByte('}')
	pdk.OutputString(b.String())
}

func emitStatResponse(entry *platypus.FSListEntry, errMsg string) {
	var b strings.Builder
	b.WriteByte('{')
	if entry != nil {
		b.WriteString(`"entry":`)
		writeFileEntryJSON(&b, *entry)
	}
	if errMsg != "" {
		if entry != nil {
			b.WriteByte(',')
		}
		b.WriteString(`"error":`)
		b.WriteString(platypus.EncodeJSONString(errMsg))
	}
	b.WriteByte('}')
	pdk.OutputString(b.String())
}

// writeFileEntryJSON encodes one FSListEntry into the FileEntry
// shape the Rust crate emits: name, mode, size, mtime_unix_nano,
// symlink_target.  Mode is synthesized from is_dir like the Rust
// crate does (S_IFDIR bit when directory, S_IFREG otherwise).
func writeFileEntryJSON(b *strings.Builder, e platypus.FSListEntry) {
	mode := uint32(0o100000)
	if e.IsDir {
		mode = 0o040000
	}
	b.WriteByte('{')
	b.WriteString(`"name":`)
	b.WriteString(platypus.EncodeJSONString(e.Name))
	b.WriteString(`,"mode":`)
	b.WriteString(uint32ToStr(mode))
	b.WriteString(`,"size":`)
	b.WriteString(int64ToStr(e.Size))
	b.WriteString(`,"mtime_unix_nano":`)
	b.WriteString(int64ToStr(e.MTimeUnix * 1_000_000_000))
	b.WriteByte('}')
}

// ============================================================
// read (stream)
// ============================================================

//export read
func readEntry() int32 {
	req, err := parseFileReadRequest(pdk.Input())
	if err != nil {
		_ = writeReadHeader(0, 0, "parse FileReadRequest")
		return 0
	}
	if req.Path == "" {
		_ = writeReadHeader(0, 0, "empty path")
		return 0
	}

	// Probe size+mode with a zero-length read.
	probe, err := platypus.HostFSReadRange(req.Path, 0, 0)
	if err != nil {
		_ = writeReadHeader(0, 0, err.Error())
		return 0
	}
	totalSize := probe.Size
	mode := probe.Mode

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > totalSize {
		offset = totalSize
	}
	remaining := totalSize - offset
	if req.Length > 0 && req.Length < remaining {
		remaining = req.Length
	}

	if err := writeReadHeader(totalSize, mode, ""); err != nil {
		platypus.LogErrorf("sys-files-read-go: write read header: %s", err.Error())
		return 1
	}

	for remaining > 0 {
		want := remaining
		if want > fileChunkSize {
			want = fileChunkSize
		}
		r, err := platypus.HostFSReadRange(req.Path, offset, want)
		if err != nil {
			_ = writeFileChunk(nil, true, err.Error(), offset)
			return 0
		}
		n := int64(len(r.Data))
		offset += n
		remaining -= n
		isEOF := remaining == 0 || r.EOF || n == 0
		if err := writeFileChunk(r.Data, isEOF, "", offset); err != nil {
			return 1
		}
		if isEOF {
			return 0
		}
	}
	_ = writeFileChunk(nil, true, "", offset)
	return 0
}

type fileReadRequest struct {
	Path   string
	Offset int64
	Length int64
}

func parseFileReadRequest(buf []byte) (fileReadRequest, error) {
	var req fileReadRequest
	for len(buf) > 0 {
		tag, n := binary.Uvarint(buf)
		if n <= 0 {
			return req, errors.New("truncated tag")
		}
		buf = buf[n:]
		field := uint32(tag >> 3)
		wire := uint32(tag & 0x7)
		switch {
		case field == 1 && wire == wireLen:
			s, rest, ok := readLenString(buf)
			if !ok {
				return req, errors.New("truncated string")
			}
			req.Path = s
			buf = rest
		case field == 2 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				return req, errors.New("truncated varint")
			}
			req.Offset = int64(v)
			buf = buf[m:]
		case field == 3 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				return req, errors.New("truncated varint")
			}
			req.Length = int64(v)
			buf = buf[m:]
		default:
			b, err := skipField(buf, wire)
			if err != nil {
				return req, err
			}
			buf = b
		}
	}
	return req, nil
}

func writeReadHeader(size int64, mode uint32, errMsg string) error {
	buf := make([]byte, 0, 32)
	if size != 0 {
		buf = appendTag(buf, 1, wireVarint)
		buf = binary.AppendUvarint(buf, uint64(size))
	}
	if mode != 0 {
		buf = appendTag(buf, 2, wireVarint)
		buf = binary.AppendUvarint(buf, uint64(mode))
	}
	if errMsg != "" {
		buf = appendTag(buf, 3, wireLen)
		buf = binary.AppendUvarint(buf, uint64(len(errMsg)))
		buf = append(buf, errMsg...)
	}
	return platypus.HostStreamWrite(buf)
}

// writeFileChunk emits one FileChunk frame. data=1 bytes, eof=2 bool,
// error=3 string, source_bytes_so_far=4 int64. Shared between read
// and archive entry points (identical proto schema).
func writeFileChunk(data []byte, eof bool, errMsg string, sourceBytesSoFar int64) error {
	buf := make([]byte, 0, len(data)+32)
	if len(data) > 0 {
		buf = appendTag(buf, 1, wireLen)
		buf = binary.AppendUvarint(buf, uint64(len(data)))
		buf = append(buf, data...)
	}
	if eof {
		buf = appendTag(buf, 2, wireVarint)
		buf = binary.AppendUvarint(buf, 1)
	}
	if errMsg != "" {
		buf = appendTag(buf, 3, wireLen)
		buf = binary.AppendUvarint(buf, uint64(len(errMsg)))
		buf = append(buf, errMsg...)
	}
	if sourceBytesSoFar != 0 {
		buf = appendTag(buf, 4, wireVarint)
		buf = binary.AppendUvarint(buf, uint64(sourceBytesSoFar))
	}
	return platypus.HostStreamWrite(buf)
}

// ============================================================
// scan (stream)
// ============================================================

//export scan
func scanEntry() int32 {
	req := parseScanRequest(pdk.Input())
	if len(req.Paths) == 0 {
		_ = writeScanResponse(0, 0, 0, "no paths to scan")
		return 0
	}
	var files, dirs, totalBytes int64
	for _, root := range req.Paths {
		st, err := platypus.HostFSStat(root)
		if err != nil {
			_ = writeScanResponse(0, 0, 0, "stat "+root+": "+err.Error())
			return 0
		}
		if !st.IsDir {
			files++
			totalBytes += st.Size
			continue
		}
		dirs++
		f, d, b := scanWalk(root)
		files += f
		dirs += d
		totalBytes += b
	}
	_ = writeScanResponse(files, dirs, totalBytes, "")
	return 0
}

// scanWalk is the iterative directory walker.  Stack-based to avoid
// wasm-stack overflow on adversarially deep trees.
func scanWalk(root string) (files, dirs, totalBytes int64) {
	stack := []string{root}
	for len(stack) > 0 {
		dir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		entries, err := platypus.HostFSListDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			path := dir
			if !strings.HasSuffix(path, "/") {
				path += "/"
			}
			path += e.Name
			if e.IsDir {
				dirs++
				stack = append(stack, path)
			} else {
				files++
				totalBytes += e.Size
			}
		}
	}
	return files, dirs, totalBytes
}

type scanRequest struct {
	Paths          []string
	FollowSymlinks bool
}

func parseScanRequest(buf []byte) scanRequest {
	var req scanRequest
	for len(buf) > 0 {
		tag, n := binary.Uvarint(buf)
		if n <= 0 {
			return req
		}
		buf = buf[n:]
		field := uint32(tag >> 3)
		wire := uint32(tag & 0x7)
		switch {
		case field == 1 && wire == wireLen:
			s, rest, ok := readLenString(buf)
			if !ok {
				return req
			}
			req.Paths = append(req.Paths, s)
			buf = rest
		case field == 2 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				return req
			}
			req.FollowSymlinks = v != 0
			buf = buf[m:]
		default:
			b, err := skipField(buf, wire)
			if err != nil {
				return req
			}
			buf = b
		}
	}
	return req
}

func writeScanResponse(fileCount, dirCount, totalBytes int64, errMsg string) error {
	buf := make([]byte, 0, 48)
	if fileCount != 0 {
		buf = appendTag(buf, 1, wireVarint)
		buf = binary.AppendUvarint(buf, uint64(fileCount))
	}
	if dirCount != 0 {
		buf = appendTag(buf, 2, wireVarint)
		buf = binary.AppendUvarint(buf, uint64(dirCount))
	}
	if totalBytes != 0 {
		buf = appendTag(buf, 3, wireVarint)
		buf = binary.AppendUvarint(buf, uint64(totalBytes))
	}
	if errMsg != "" {
		buf = appendTag(buf, 4, wireLen)
		buf = binary.AppendUvarint(buf, uint64(len(errMsg)))
		buf = append(buf, errMsg...)
	}
	return platypus.HostStreamWrite(buf)
}

// ============================================================
// archive (stream)
// ============================================================
//
// Walks the requested paths via host_fs_listdir + host_fs_stat,
// reads file contents via host_fs_read_range, streams a tar (or
// tar.gz) archive out via host_stream_write FileChunk frames.
// Same flushing rhythm as the Rust crate (FLUSH_BYTES = 256 KiB).

//export archive
func archiveEntry() int32 {
	req := parseArchiveRequest(pdk.Input())
	if len(req.Paths) == 0 {
		_ = writeArchiveHeader("no paths to archive")
		_ = writeFileChunk(nil, true, "", 0)
		return 0
	}
	if req.Format == archiveFormatUnspecified {
		req.Format = archiveFormatTar
	}
	if req.Format == archiveFormatZip {
		_ = writeArchiveHeader("zip format not supported")
		_ = writeFileChunk(nil, true, "", 0)
		return 0
	}
	if req.Format != archiveFormatTar && req.Format != archiveFormatTarGz {
		_ = writeArchiveHeader("unsupported archive format")
		_ = writeFileChunk(nil, true, "", 0)
		return 0
	}

	if err := writeArchiveHeader(""); err != nil {
		return 1
	}

	fb := newFrameBuffer()

	// Optional gzip layer between tar and the chunked frame buffer.
	var sink io.Writer = fb
	var gz *gzip.Writer
	if req.Format == archiveFormatTarGz {
		gz = gzip.NewWriter(fb)
		sink = gz
	}
	tw := tar.NewWriter(sink)

	for _, root := range req.Paths {
		st, err := platypus.HostFSStat(root)
		if err != nil {
			fb.fail("stat " + root + ": " + err.Error())
			break
		}
		if !st.IsDir {
			if err := archiveAddFile(tw, fb, root, root, st); err != nil {
				fb.fail(err.Error())
				break
			}
			continue
		}
		if err := archiveAddTree(tw, fb, root); err != nil {
			fb.fail(err.Error())
			break
		}
	}

	tarErr := tw.Close()
	var gzErr error
	if gz != nil {
		gzErr = gz.Close()
	}

	finalErr := fb.failed
	if finalErr == "" && tarErr != nil {
		finalErr = "tar close: " + tarErr.Error()
	}
	if finalErr == "" && gzErr != nil {
		finalErr = "gzip close: " + gzErr.Error()
	}
	_ = fb.flushAndTerminate(finalErr)
	return 0
}

// frameBuffer accumulates archive bytes from tar.Writer (or gzip-
// wrapped tar.Writer) and flushes a FileChunk every 256 KiB. Mirrors
// the Rust crate's FrameBuffer.
type frameBuffer struct {
	buf              []byte
	sourceBytesSoFar int64
	failed           string
}

func newFrameBuffer() *frameBuffer {
	return &frameBuffer{buf: make([]byte, 0, flushBytes+4096)}
}

func (b *frameBuffer) Write(p []byte) (int, error) {
	if b.failed != "" {
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	for len(b.buf) >= flushBytes {
		chunk := b.buf[:flushBytes]
		if err := writeFileChunk(chunk, false, "", b.sourceBytesSoFar); err != nil {
			b.failed = err.Error()
			return len(p), nil
		}
		b.buf = b.buf[flushBytes:]
	}
	return len(p), nil
}

func (b *frameBuffer) addSource(n int64) { b.sourceBytesSoFar += n }

func (b *frameBuffer) fail(msg string) {
	if b.failed == "" {
		b.failed = msg
	}
}

func (b *frameBuffer) flushAndTerminate(errMsg string) error {
	if len(b.buf) > 0 {
		if err := writeFileChunk(b.buf, false, "", b.sourceBytesSoFar); err != nil {
			return err
		}
		b.buf = b.buf[:0]
	}
	return writeFileChunk(nil, true, errMsg, b.sourceBytesSoFar)
}

func writeArchiveHeader(errMsg string) error {
	// FileReadResponse same shape — size=1:int64, mode=2:uint32,
	// error=3:string. Archive doesn't fill size (its scope is
	// streaming, not bounded), only the error if any.
	buf := make([]byte, 0, len(errMsg)+8)
	if errMsg != "" {
		buf = appendTag(buf, 3, wireLen)
		buf = binary.AppendUvarint(buf, uint64(len(errMsg)))
		buf = append(buf, errMsg...)
	}
	return platypus.HostStreamWrite(buf)
}

// archiveAddFile reads `path` via HostFSReadRange in fileChunkSize-
// sized chunks and feeds them through tw. Symlinks aren't supported
// by the Rust crate either; we mirror that.
func archiveAddFile(tw *tar.Writer, fb *frameBuffer, archiveName, path string, st platypus.FSListEntry) error {
	mode := int64(0o644)
	if st.Mode != 0 {
		mode = int64(st.Mode & 0o7777)
	}
	hdr := &tar.Header{
		Name:    strings.TrimPrefix(archiveName, "/"),
		Mode:    mode,
		Size:    st.Size,
		ModTime: tarTime(st.MTimeUnix),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	var offset int64
	for offset < st.Size {
		want := st.Size - offset
		if want > fileChunkSize {
			want = fileChunkSize
		}
		r, err := platypus.HostFSReadRange(path, offset, want)
		if err != nil {
			return err
		}
		n := int64(len(r.Data))
		if n == 0 {
			break
		}
		if _, err := tw.Write(r.Data); err != nil {
			return err
		}
		fb.addSource(n)
		offset += n
		if r.EOF {
			break
		}
	}
	return nil
}

func archiveAddTree(tw *tar.Writer, fb *frameBuffer, root string) error {
	stack := []string{root}
	for len(stack) > 0 {
		dir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		// Add a directory tar entry first.
		dst, err := platypus.HostFSStat(dir)
		if err == nil {
			mode := int64(0o755)
			if dst.Mode != 0 {
				mode = int64(dst.Mode & 0o7777) | 0o111
			}
			hdr := &tar.Header{
				Name:     strings.TrimPrefix(dir, "/") + "/",
				Mode:     mode,
				ModTime:  tarTime(dst.MTimeUnix),
				Typeflag: tar.TypeDir,
			}
			_ = tw.WriteHeader(hdr)
		}
		entries, err := platypus.HostFSListDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			path := dir
			if !strings.HasSuffix(path, "/") {
				path += "/"
			}
			path += e.Name
			if e.IsDir {
				stack = append(stack, path)
				continue
			}
			if err := archiveAddFile(tw, fb, path, path, e); err != nil {
				return err
			}
		}
	}
	return nil
}

type archiveRequest struct {
	Paths          []string
	Format         uint32
	FollowSymlinks bool
}

func parseArchiveRequest(buf []byte) archiveRequest {
	var req archiveRequest
	for len(buf) > 0 {
		tag, n := binary.Uvarint(buf)
		if n <= 0 {
			return req
		}
		buf = buf[n:]
		field := uint32(tag >> 3)
		wire := uint32(tag & 0x7)
		switch {
		case field == 1 && wire == wireLen:
			s, rest, ok := readLenString(buf)
			if !ok {
				return req
			}
			req.Paths = append(req.Paths, s)
			buf = rest
		case field == 2 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				return req
			}
			req.Format = uint32(v)
			buf = buf[m:]
		case field == 3 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				return req
			}
			req.FollowSymlinks = v != 0
			buf = buf[m:]
		default:
			b, err := skipField(buf, wire)
			if err != nil {
				return req
			}
			buf = b
		}
	}
	return req
}

// ============================================================
// shared helpers
// ============================================================

func main() {}

func appendTag(buf []byte, field, wire uint32) []byte {
	return binary.AppendUvarint(buf, uint64((field<<3)|wire))
}

func skipField(buf []byte, wire uint32) ([]byte, error) {
	switch wire {
	case wireVarint:
		_, n := binary.Uvarint(buf)
		if n <= 0 {
			return nil, errors.New("truncated varint")
		}
		return buf[n:], nil
	case wireLen:
		ln, n := binary.Uvarint(buf)
		if n <= 0 {
			return nil, errors.New("truncated len")
		}
		buf = buf[n:]
		if uint64(len(buf)) < ln {
			return nil, errors.New("truncated body")
		}
		return buf[ln:], nil
	default:
		return nil, errors.New("unsupported wire type")
	}
}

func readLenString(buf []byte) (string, []byte, bool) {
	body, rest, ok := readLenBytes(buf)
	if !ok {
		return "", buf, false
	}
	return string(body), rest, true
}

func readLenBytes(buf []byte) ([]byte, []byte, bool) {
	ln, n := binary.Uvarint(buf)
	if n <= 0 {
		return nil, buf, false
	}
	buf = buf[n:]
	if uint64(len(buf)) < ln {
		return nil, buf, false
	}
	return buf[:ln], buf[ln:], true
}

// decodeJSONStringField extracts the string value of a top-level
// field from a JSON object. Used by list_dir / stat to read the
// `path` field without dragging encoding/json into the binary's
// reflect type cache (see SDK util.go for the TinyGo rationale).
func decodeJSONStringField(buf []byte, fieldName string) string {
	// Brute-force scan: find `"<fieldName>"` then the colon then a
	// JSON string. Sufficient for our flat single-field requests.
	needle := `"` + fieldName + `"`
	idx := strings.Index(string(buf), needle)
	if idx < 0 {
		return ""
	}
	i := idx + len(needle)
	// Skip whitespace + colon + whitespace.
	for i < len(buf) && (buf[i] == ' ' || buf[i] == '\t' || buf[i] == ':' || buf[i] == '\n' || buf[i] == '\r') {
		i++
	}
	if i >= len(buf) || buf[i] != '"' {
		return ""
	}
	i++
	var b strings.Builder
	for i < len(buf) {
		c := buf[i]
		if c == '"' {
			return b.String()
		}
		if c == '\\' && i+1 < len(buf) {
			i++
			switch buf[i] {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '"', '\\', '/':
				b.WriteByte(buf[i])
			}
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	return ""
}

func uint32ToStr(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func int64ToStr(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// tarTime converts a unix-seconds timestamp to time.Time for the
// archive/tar Header.ModTime field. tar requires a non-zero time
// for cross-reader portability, so we can't dodge the time import.
func tarTime(unix int64) time.Time { return time.Unix(unix, 0) }
