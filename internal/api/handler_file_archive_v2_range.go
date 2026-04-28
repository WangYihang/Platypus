package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// byteRangeKind tells serveFsReadRange how to translate the parsed
// Range header into a concrete (offset, length) pair on the agent
// side. We model the three single-range forms separately because
// only the suffix form needs to peek the file size first.
type byteRangeKind int

const (
	rangeKindBoth      byteRangeKind = iota // bytes=A-B
	rangeKindOpenEnded                      // bytes=A-
	rangeKindSuffix                         // bytes=-N (last N bytes)
)

// byteRangeSpec is the decoded form of a single-range Range header.
// For rangeKindSuffix only `n` is meaningful; for the other two
// `start`/`end` are. We deliberately do not implement
// multipart/byteranges (rarely used by browsers and adds significant
// response-shaping complexity) — the parser returns ok=false on a
// multi-range header and the caller falls back to a full 200.
type byteRangeSpec struct {
	kind  byteRangeKind
	start int64
	end   int64 // inclusive; only valid for rangeKindBoth
	n     int64 // only valid for rangeKindSuffix
}

// parseSingleByteRange parses one of:
//
//	bytes=A-B    → kind=both,  start=A, end=B
//	bytes=A-     → kind=open,  start=A
//	bytes=-N     → kind=suffix, n=N
//
// Returns ok=false for multi-range, missing "bytes=" prefix,
// negative values, A > B, A < 0, or any field that fails strconv.
// The caller is expected to fall through to the full 200 path on
// ok=false rather than emitting 416 — multi-range is a valid client
// request, we just don't support it.
func parseSingleByteRange(h string) (byteRangeSpec, bool) {
	const prefix = "bytes="
	if !strings.HasPrefix(h, prefix) {
		return byteRangeSpec{}, false
	}
	v := strings.TrimPrefix(h, prefix)
	if strings.Contains(v, ",") {
		// Multi-range: don't try to serve as multipart/byteranges.
		return byteRangeSpec{}, false
	}
	dash := strings.IndexByte(v, '-')
	if dash < 0 {
		return byteRangeSpec{}, false
	}
	left, right := v[:dash], v[dash+1:]
	if left == "" {
		// Suffix range: bytes=-N.
		if right == "" {
			return byteRangeSpec{}, false
		}
		n, err := strconv.ParseInt(right, 10, 64)
		if err != nil || n <= 0 {
			return byteRangeSpec{}, false
		}
		return byteRangeSpec{kind: rangeKindSuffix, n: n}, true
	}
	start, err := strconv.ParseInt(left, 10, 64)
	if err != nil || start < 0 {
		return byteRangeSpec{}, false
	}
	if right == "" {
		return byteRangeSpec{kind: rangeKindOpenEnded, start: start}, true
	}
	end, err := strconv.ParseInt(right, 10, 64)
	if err != nil || end < start {
		return byteRangeSpec{}, false
	}
	return byteRangeSpec{kind: rangeKindBoth, start: start, end: end}, true
}

// serveFsReadRange serves a single-range Range request as a 206
// Partial Content. It opens an agent stream with the appropriate
// (offset, length), reads the FileReadResponse to learn the file's
// real size, and either streams the requested slice or — for ranges
// that fall entirely past EOF — returns 416 with `Content-Range:
// */<size>` per RFC 7233. TransferRecorder is intentionally bypassed:
// Range hits would otherwise flood the file_transfers drawer with
// dozens of rows for a single seek-heavy media preview.
func serveFsReadRange(
	c *gin.Context,
	deps FileArchiveDeps,
	sess *link.Session,
	agentID, path string,
	spec byteRangeSpec,
) {
	// Suffix range needs the file size before we can compute the
	// request offset, so peek with a one-byte read whose only purpose
	// is the FileReadResponse header. The agent ships the header as
	// the first frame regardless of the data payload.
	if spec.kind == rangeKindSuffix {
		size, err := peekFsReadSize(sess, agentID, path)
		if err != nil {
			c.String(http.StatusBadGateway, "peek size: %s", err)
			return
		}
		if size <= 0 {
			c.Header("Content-Range", fmt.Sprintf("bytes */%d", size))
			c.AbortWithStatus(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		n := spec.n
		if n > size {
			n = size
		}
		spec = byteRangeSpec{
			kind:  rangeKindBoth,
			start: size - n,
			end:   size - 1,
		}
	}

	// Translate the (possibly canonicalised) spec into the agent's
	// (offset, length) shape. Open-ended ranges map to length=0
	// because the agent treats Length=0 as "read to EOF".
	offset := spec.start
	var length int64
	if spec.kind == rangeKindBoth {
		length = spec.end - spec.start + 1
	}

	meta, _ := proto.Marshal(&v2pb.FileReadRequest{
		Path: path, Offset: offset, Length: length,
	})
	stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_FILE_READ, meta, "fs-read-range-"+agentID)
	if err != nil {
		c.String(http.StatusBadGateway, "open stream: %s", err)
		return
	}
	defer func() { _ = stream.Close() }()

	var hdr v2pb.FileReadResponse
	if err := link.ReadFrame(stream, &hdr); err != nil {
		c.String(http.StatusBadGateway, "read header: %s", err)
		return
	}
	if hdr.Error != "" {
		c.String(http.StatusBadGateway, "agent: %s", hdr.Error)
		return
	}

	size := hdr.Size
	if offset >= size {
		c.Header("Content-Range", fmt.Sprintf("bytes */%d", size))
		c.AbortWithStatus(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	// Clamp the inclusive end to the last real byte. This handles
	// `bytes=A-` (we picked up the file size from the header) AND
	// `bytes=A-B` where B exceeds the file (RFC 7233 says clients
	// MAY ask for one and the server clamps).
	var end int64
	if spec.kind == rangeKindBoth {
		end = spec.end
		if end >= size {
			end = size - 1
		}
	} else { // rangeKindOpenEnded
		end = size - 1
	}
	contentLen := end - offset + 1

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, end, size))
	c.Header("Content-Length", fmt.Sprintf("%d", contentLen))
	c.Status(http.StatusPartialContent)

	// Stream chunks from the agent, bounded by contentLen so a
	// length=0 ("to EOF") agent stream doesn't overshoot the
	// declared Content-Length when a client asked for a clamped
	// `bytes=A-B`.
	var written int64
	for {
		var ch v2pb.FileChunk
		if err := link.ReadFrame(stream, &ch); err != nil {
			return
		}
		if len(ch.Data) > 0 {
			toWrite := ch.Data
			if written+int64(len(toWrite)) > contentLen {
				toWrite = toWrite[:contentLen-written]
			}
			if _, werr := c.Writer.Write(toWrite); werr != nil {
				return
			}
			written += int64(len(toWrite))
			c.Writer.Flush()
		}
		if ch.Eof || written >= contentLen {
			return
		}
	}
}

// peekFsReadSize opens a tiny FileRead stream just to read the
// FileReadResponse header (size + mode). Used by suffix-range
// requests (`bytes=-N`) which need the file size before they can
// translate to an absolute offset. We pass length=1 instead of 0 so
// the agent doesn't stream the whole file when the caller will close
// after the header.
func peekFsReadSize(sess *link.Session, agentID, path string) (int64, error) {
	meta, _ := proto.Marshal(&v2pb.FileReadRequest{Path: path, Length: 1})
	stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_FILE_READ, meta, "fs-read-peek-"+agentID)
	if err != nil {
		return 0, fmt.Errorf("open: %w", err)
	}
	defer func() { _ = stream.Close() }()
	var hdr v2pb.FileReadResponse
	if err := link.ReadFrame(stream, &hdr); err != nil {
		return 0, fmt.Errorf("read header: %w", err)
	}
	if hdr.Error != "" {
		return 0, fmt.Errorf("%s", hdr.Error)
	}
	return hdr.Size, nil
}
