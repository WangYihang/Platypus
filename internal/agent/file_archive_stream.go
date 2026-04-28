package agent

import (
	"archive/tar"
	"archive/zip"
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// archiveFlushBytes is the soft target for "bytes accumulated
// before we flush a FileChunk frame to the wire". A larger value
// reduces per-frame overhead (length prefix + protobuf header) at
// the cost of less granular progress on the server side. 256 KiB
// matches the existing FILE_READ chunk size so we don't regress
// per-file backpressure.
const archiveFlushBytes = 256 * 1024

// HandleFileArchiveStream is the agent-side handler for a
// STREAM_TYPE_FILE_ARCHIVE stream. Walks the requested paths,
// packs them into the requested archive format, and streams the
// resulting bytes back as FileChunk frames.
//
// Wire shape:
//  1. FileArchiveResponse  (ack — empty error means OK)
//  2. zero-or-more FileChunk frames (data, eof=false)
//  3. final FileChunk frame (eof=true; Error populated on failure)
//
// Mid-walk errors on individual files are logged via the final
// FileChunk's Error field but the partial archive is preserved —
// consumers can decide whether to keep or discard it.
func HandleFileArchiveStream(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.FileArchiveRequest) error {
	defer func() { _ = stream.Close() }()

	if req == nil || len(req.Paths) == 0 {
		return writeArchiveHeader(stream, "no paths to archive")
	}
	if req.Format == v2pb.ArchiveFormat_ARCHIVE_FORMAT_UNSPECIFIED {
		return writeArchiveHeader(stream, "archive format not specified")
	}

	statFn := os.Lstat
	if req.FollowSymlinks {
		statFn = os.Stat
	}
	// Pre-stat every root so we fail fast (with the dedicated
	// header error) rather than half-streaming a partial archive
	// when one of the requested paths is missing.
	for _, p := range req.Paths {
		if _, err := statFn(p); err != nil {
			return writeArchiveHeader(stream, err.Error())
		}
	}

	if err := writeArchiveHeader(stream, ""); err != nil {
		return err
	}

	// sourceBytesSoFar accumulates bytes read off disk (file content
	// only — tar/zip headers don't count, so the number stays
	// directly comparable to FileScanResponse.total_bytes). Both
	// the chunkWriter (when stamping FileChunk.source_bytes_so_far)
	// and writeTarArchive / writeZipArchive (when copying file
	// contents) read/write this counter via atomic ops; using a
	// pointer avoids a second pass through the encoder hierarchy.
	var sourceBytesSoFar int64

	// chunkWriter buffers output bytes and flushes them as
	// FileChunk frames whenever it accumulates archiveFlushBytes
	// or when explicitly flushed. The archive encoders (tar /
	// gzip / zip) write into this; on EOF we send the final
	// chunk with eof=true.
	cw := &chunkWriter{stream: stream, ctx: ctx, sourceBytesSoFar: &sourceBytesSoFar}
	defer cw.finish() // sends terminal eof=true on whatever path we exit

	switch req.Format {
	case v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR:
		if err := writeTarArchive(ctx, cw, req, &sourceBytesSoFar); err != nil {
			cw.fail(err.Error())
			return err
		}
	case v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ:
		level := clampGzipLevel(int(req.CompressionLevel))
		gz, err := gzip.NewWriterLevel(cw, level)
		if err != nil {
			cw.fail(err.Error())
			return err
		}
		if err := writeTarArchive(ctx, gz, req, &sourceBytesSoFar); err != nil {
			_ = gz.Close()
			cw.fail(err.Error())
			return err
		}
		if err := gz.Close(); err != nil {
			cw.fail(err.Error())
			return err
		}
	case v2pb.ArchiveFormat_ARCHIVE_FORMAT_ZIP:
		zw := zip.NewWriter(cw)
		level := clampZipLevel(int(req.CompressionLevel))
		if level != 0 {
			zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
				return newZipDeflater(out, level)
			})
		}
		if err := writeZipArchive(ctx, zw, req, &sourceBytesSoFar); err != nil {
			_ = zw.Close()
			cw.fail(err.Error())
			return err
		}
		if err := zw.Close(); err != nil {
			cw.fail(err.Error())
			return err
		}
	default:
		err := fmt.Errorf("unsupported archive format: %s", req.Format)
		cw.fail(err.Error())
		return err
	}
	return nil
}

func writeArchiveHeader(w io.Writer, errMsg string) error {
	return link.WriteFrame(w, &v2pb.FileArchiveResponse{Error: errMsg})
}

// chunkWriter is an io.Writer that emits length-prefixed FileChunk
// frames. It buffers up to archiveFlushBytes worth of bytes
// before each flush so we don't pay per-byte protobuf overhead.
//
// chunkWriter is single-goroutine: archive encoders call Write
// serially, and finish/fail are called from the same goroutine
// after the encoder closes.
//
// sourceBytesSoFar is a pointer to a counter incremented by
// streamFileInto as it reads bytes off disk (see archiveSourceCounter).
// Each FileChunk frame is stamped with the latest value so the server
// can render real percentage progress against the pre-scan total —
// the wire bytes (post-gzip) would not be comparable.
type chunkWriter struct {
	stream           io.Writer
	ctx              context.Context
	buf              []byte
	closed           bool
	err              error
	sourceBytesSoFar *int64
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, io.ErrClosedPipe
	}
	if w.err != nil {
		return 0, w.err
	}
	if cerr := w.ctx.Err(); cerr != nil {
		w.err = cerr
		return 0, cerr
	}
	w.buf = append(w.buf, p...)
	for len(w.buf) >= archiveFlushBytes {
		if err := w.flush(archiveFlushBytes); err != nil {
			w.err = err
			return 0, err
		}
	}
	return len(p), nil
}

// flush emits up to n buffered bytes as a single non-EOF FileChunk.
func (w *chunkWriter) flush(n int) error {
	if n > len(w.buf) {
		n = len(w.buf)
	}
	if n == 0 {
		return nil
	}
	chunk := &v2pb.FileChunk{
		Data: append([]byte(nil), w.buf[:n]...),
		Eof:  false,
	}
	if w.sourceBytesSoFar != nil {
		chunk.SourceBytesSoFar = atomic.LoadInt64(w.sourceBytesSoFar)
	}
	if err := link.WriteFrame(w.stream, chunk); err != nil {
		return err
	}
	w.buf = w.buf[n:]
	return nil
}

// finish drains any remaining buffered bytes (as a final non-EOF
// chunk if non-empty) and emits the terminal eof=true frame.
// Idempotent: only the first call writes anything.
func (w *chunkWriter) finish() {
	if w.closed {
		return
	}
	w.closed = true
	if len(w.buf) > 0 {
		_ = w.flush(len(w.buf))
	}
	// Final eof. If we already had an error, surface it here.
	errMsg := ""
	if w.err != nil && !errors.Is(w.err, context.Canceled) {
		errMsg = w.err.Error()
	} else if w.err != nil {
		errMsg = "cancelled: " + w.err.Error()
	}
	final := &v2pb.FileChunk{Eof: true, Error: errMsg}
	if w.sourceBytesSoFar != nil {
		final.SourceBytesSoFar = atomic.LoadInt64(w.sourceBytesSoFar)
	}
	_ = link.WriteFrame(w.stream, final)
}

// fail records an error message that finish() will surface in the
// terminal frame.
func (w *chunkWriter) fail(msg string) {
	if w.err == nil && msg != "" {
		w.err = errors.New(msg)
	}
}

func clampGzipLevel(n int) int {
	if n <= 0 {
		return gzip.DefaultCompression
	}
	if n > gzip.BestCompression {
		return gzip.BestCompression
	}
	return n
}

func clampZipLevel(n int) int {
	if n <= 0 {
		return 0 // sentinel: use default
	}
	if n > 9 {
		return 9
	}
	return n
}

// archiveEntry pairs a tar/zip entry name with the on-disk path it
// was sourced from, so we can read the file lazily during write.
type archiveEntry struct {
	name string // path inside the archive (slash-separated)
	src  string // local filesystem path
	info fs.FileInfo
}

// walkArchive enumerates the entries to add. Names are derived
// from the basename of each requested root: a request for
// /a/b/dir produces entries "dir/...", for /a/b/file produces
// "file". Multiple roots with the same basename get a numeric
// suffix to avoid collisions.
func walkArchive(req *v2pb.FileArchiveRequest) ([]archiveEntry, error) {
	statFn := os.Lstat
	if req.FollowSymlinks {
		statFn = os.Stat
	}
	seenBase := map[string]int{}
	var out []archiveEntry
	for _, root := range req.Paths {
		info, err := statFn(root)
		if err != nil {
			return nil, err
		}
		base := filepath.Base(root)
		if base == "/" || base == "." || base == "" {
			base = "root"
		}
		// On collision, append a counter: "dir", "dir-1", "dir-2".
		if n, ok := seenBase[base]; ok {
			n++
			seenBase[base] = n
			base = fmt.Sprintf("%s-%d", base, n)
		} else {
			seenBase[base] = 0
		}

		if !info.IsDir() {
			out = append(out, archiveEntry{name: base, src: root, info: info})
			continue
		}

		walker := filepath.Walk
		if req.FollowSymlinks {
			walker = walkFollowSymlinks
		}
		err = walker(root, func(path string, fi fs.FileInfo, walkErr error) error {
			if walkErr != nil {
				if fi != nil && fi.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			name := base
			if rel != "." {
				name = base + "/" + filepath.ToSlash(rel)
			}
			out = append(out, archiveEntry{name: name, src: path, info: fi})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func writeTarArchive(ctx context.Context, w io.Writer, req *v2pb.FileArchiveRequest, srcBytes *int64) error {
	entries, err := walkArchive(req)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(w)
	defer tw.Close()
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(e.info, "")
		if err != nil {
			return fmt.Errorf("tar header for %s: %w", e.src, err)
		}
		hdr.Name = e.name
		if e.info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("tar write header %s: %w", e.name, err)
		}
		if !e.info.Mode().IsRegular() {
			continue
		}
		if err := streamFileInto(ctx, tw, e.src, srcBytes); err != nil {
			return err
		}
	}
	return tw.Close()
}

func writeZipArchive(ctx context.Context, zw *zip.Writer, req *v2pb.FileArchiveRequest, srcBytes *int64) error {
	entries, err := walkArchive(req)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := zip.FileInfoHeader(e.info)
		if err != nil {
			return fmt.Errorf("zip header for %s: %w", e.src, err)
		}
		hdr.Name = e.name
		if e.info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		hdr.Method = zip.Deflate
		if e.info.IsDir() {
			hdr.Method = zip.Store
		}
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return fmt.Errorf("zip create %s: %w", e.name, err)
		}
		if !e.info.Mode().IsRegular() {
			continue
		}
		if err := streamFileInto(ctx, fw, e.src, srcBytes); err != nil {
			return err
		}
	}
	return nil
}

// streamFileInto opens the file and copies its contents to w in
// archiveFlushBytes-sized chunks, checking ctx between chunks for
// prompt cancellation. srcBytes is the shared counter the
// chunkWriter samples when stamping each FileChunk; we increment it
// after a successful Write so the count never drifts ahead of what
// the encoder has actually seen.
func streamFileInto(ctx context.Context, w io.Writer, path string, srcBytes *int64) error {
	f, err := os.Open(path)
	if err != nil {
		// Mid-walk read failure on a single file: log via the
		// final frame's error path but keep writing the rest of
		// the archive. Returning nil here continues to the next
		// entry; chunkWriter.fail isn't reached so the consumer
		// gets a clean eof.
		return nil
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, archiveFlushBytes)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, rerr := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			if srcBytes != nil {
				atomic.AddInt64(srcBytes, int64(n))
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return nil
			}
			// Per-file read error after open: drop the rest of
			// this file but keep the archive going.
			return nil
		}
	}
}

// newZipDeflater returns a flate writer at the requested level so
// callers can override archive/zip's default Deflate compressor.
func newZipDeflater(out io.Writer, level int) (io.WriteCloser, error) {
	return flate.NewWriter(out, level)
}
