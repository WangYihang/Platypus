package agent

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleFileWriteStream is the agent-side handler for a
// STREAM_TYPE_FILE_WRITE stream:
//
//  1. Open destination (creating parents if req.Mkdirs, respecting
//     req.Append). Mode defaults to 0o644.
//  2. Write ack FileWriteResponse — populated with Error on open
//     failure, empty otherwise.
//  3. Consume FileChunk frames until one arrives with eof=true,
//     writing each chunk's Data to the file.
//  4. Emit FileWriteResult with BytesWritten and Error (if any
//     chunk carried one, or if a write syscall failed).
func HandleFileWriteStream(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.FileWriteRequest) error {
	defer func() { _ = stream.Close() }()
	if req == nil || req.Path == "" {
		return writeFileWriteAck(stream, "empty path")
	}

	mode := os.FileMode(0o644)
	if req.Mode != 0 {
		mode = os.FileMode(req.Mode) & os.ModePerm
	}

	if req.Mkdirs {
		if err := os.MkdirAll(filepath.Dir(req.Path), 0o755); err != nil {
			return writeFileWriteAck(stream, "mkdirs: "+err.Error())
		}
	}

	flag := os.O_CREATE | os.O_WRONLY
	if req.Append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	f, err := os.OpenFile(req.Path, flag, mode)
	if err != nil {
		return writeFileWriteAck(stream, err.Error())
	}
	// Close happens in the deferred stream close via the defer on
	// the helper below so we can surface Close errors in the
	// result frame.

	// Ack.
	if err := writeFileWriteAck(stream, ""); err != nil {
		_ = f.Close()
		return err
	}

	var written int64
	var chunkErr string
	for {
		if ctx.Err() != nil {
			chunkErr = "cancelled: " + ctx.Err().Error()
			break
		}
		var ch v2pb.FileChunk
		if err := link.ReadFrame(stream, &ch); err != nil {
			if !errors.Is(err, io.EOF) {
				chunkErr = "read chunk: " + err.Error()
			}
			break
		}
		if len(ch.Data) > 0 {
			n, werr := f.Write(ch.Data)
			written += int64(n)
			if werr != nil {
				chunkErr = "write: " + werr.Error()
				break
			}
		}
		if ch.Error != "" {
			chunkErr = "sender: " + ch.Error
		}
		if ch.Eof {
			break
		}
	}

	// Close before emitting the result so fsync errors show up as
	// result.Error rather than getting lost.
	if closeErr := f.Close(); closeErr != nil && chunkErr == "" {
		chunkErr = "close: " + closeErr.Error()
	}

	return link.WriteFrame(stream, &v2pb.FileWriteResult{
		BytesWritten: written,
		Error:        chunkErr,
	})
}

func writeFileWriteAck(stream io.Writer, errMsg string) error {
	return link.WriteFrame(stream, &v2pb.FileWriteResponse{Error: errMsg})
}
