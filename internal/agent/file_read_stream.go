package agent

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// fileReadChunkSize is the max bytes emitted per FileChunk. Tuned
// well under v2 FrameMaxBytes (1 MiB) so a single chunk + its
// length prefix + protobuf overhead always fits. Smaller chunks
// give finer-grained progress but trade throughput; 256 KiB is a
// sensible middle for local-network transfers.
const fileReadChunkSize = 256 * 1024

// HandleFileReadStream is the agent-side handler for a
// STREAM_TYPE_FILE_READ stream. Emits exactly one FileReadResponse
// header (populated with either size+mode or error), followed by
// zero or more FileChunk frames carrying data, the last of which
// has eof=true. A mid-transfer error lands in the final chunk's
// Error field; previous chunks may contain partial data.
func HandleFileReadStream(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.FileReadRequest) error {
	defer func() { _ = stream.Close() }()
	if req == nil || req.Path == "" {
		return writeFileReadHeader(stream, 0, 0, "empty path")
	}

	f, err := os.Open(req.Path)
	if err != nil {
		return writeFileReadHeader(stream, 0, 0, err.Error())
	}
	defer func() { _ = f.Close() }()

	stat, err := f.Stat()
	if err != nil {
		return writeFileReadHeader(stream, 0, 0, err.Error())
	}

	// Clamp Offset and Length to the file's current size.
	size := stat.Size()
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > size {
		offset = size
	}
	remaining := size - offset
	if req.Length > 0 && req.Length < remaining {
		remaining = req.Length
	}

	if err := writeFileReadHeader(stream, size, uint32(stat.Mode().Perm()), ""); err != nil {
		return err
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return writeFileReadEOF(stream, err.Error())
		}
	}

	buf := make([]byte, fileReadChunkSize)
	for remaining > 0 {
		if ctx.Err() != nil {
			return writeFileReadEOF(stream, "cancelled: "+ctx.Err().Error())
		}
		toRead := int64(len(buf))
		if toRead > remaining {
			toRead = remaining
		}
		n, readErr := f.Read(buf[:toRead])
		if n > 0 {
			remaining -= int64(n)
			chunk := &v2pb.FileChunk{
				Data: append([]byte(nil), buf[:n]...),
				Eof:  remaining == 0 && readErr == nil,
			}
			if err := link.WriteFrame(stream, chunk); err != nil {
				return err
			}
			if chunk.Eof {
				return nil
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return writeFileReadEOF(stream, "")
			}
			return writeFileReadEOF(stream, readErr.Error())
		}
	}
	// Loop exits when remaining==0 but last chunk wasn't marked eof
	// (e.g. the write above ran with remaining > 0 and readErr nil,
	// then remaining hit zero mid-read). Emit an explicit eof.
	return writeFileReadEOF(stream, "")
}

func writeFileReadHeader(stream io.Writer, size int64, mode uint32, errMsg string) error {
	return link.WriteFrame(stream, &v2pb.FileReadResponse{
		Size:  size,
		Mode:  mode,
		Error: errMsg,
	})
}

func writeFileReadEOF(stream io.Writer, errMsg string) error {
	return link.WriteFrame(stream, &v2pb.FileChunk{Eof: true, Error: errMsg})
}
