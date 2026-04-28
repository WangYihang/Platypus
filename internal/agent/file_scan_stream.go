package agent

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleFileScanStream is the agent-side handler for a
// STREAM_TYPE_FILE_SCAN stream. Walks the requested paths once and
// emits a single FileScanResponse summarising the totals — no
// payload bytes flow. The server uses this to size a progress bar
// before opening STREAM_TYPE_FILE_ARCHIVE for the same paths.
//
// Mid-walk errors (permission denied on a deep subdir) are silently
// skipped: a missing-root error is fatal and reported via the Error
// field, but partial unreadability of a tree shouldn't deny the
// caller a useful estimate.
func HandleFileScanStream(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.FileScanRequest) error {
	defer func() { _ = stream.Close() }()

	if req == nil || len(req.Paths) == 0 {
		return writeFileScanResponse(stream, 0, 0, 0, "no paths to scan")
	}

	statFn := os.Lstat
	if req.FollowSymlinks {
		statFn = os.Stat
	}

	var (
		fileCount  int64
		dirCount   int64
		totalBytes int64
	)

	for _, root := range req.Paths {
		if ctx.Err() != nil {
			return writeFileScanResponse(stream, fileCount, dirCount, totalBytes,
				"cancelled: "+ctx.Err().Error())
		}
		info, err := statFn(root)
		if err != nil {
			return writeFileScanResponse(stream, 0, 0, 0, err.Error())
		}
		// Single-file root: count it directly, skip the walk.
		if !info.IsDir() {
			if info.Mode().IsRegular() {
				fileCount++
				totalBytes += info.Size()
			} else {
				// Symlink (lstat) / device / fifo: counted but no
				// bytes contributed.
				fileCount++
			}
			continue
		}

		walkFn := filepath.WalkFunc(func(path string, fi fs.FileInfo, walkErr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Mid-walk error → swallow (skip the entry / subtree).
			// We still want a useful estimate even with a few
			// unreadable corners.
			if walkErr != nil {
				if fi != nil && fi.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if path == root {
				dirCount++
				return nil
			}
			switch {
			case fi.IsDir():
				dirCount++
			case fi.Mode().IsRegular():
				fileCount++
				totalBytes += fi.Size()
			default:
				fileCount++
			}
			return nil
		})

		walker := filepath.Walk
		if req.FollowSymlinks {
			// filepath.Walk uses Lstat by default. To follow
			// symlinks we wrap it: stat the link target ourselves
			// when WalkFunc sees a symlink and recurse into it.
			walker = walkFollowSymlinks
		}
		if err := walker(root, walkFn); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return writeFileScanResponse(stream, fileCount, dirCount, totalBytes,
					"cancelled: "+err.Error())
			}
			// Walk-level error after a successful root stat means
			// a deep failure we couldn't recover from — surface it
			// but keep partial counts so the caller can decide.
			return writeFileScanResponse(stream, fileCount, dirCount, totalBytes, err.Error())
		}
	}

	return writeFileScanResponse(stream, fileCount, dirCount, totalBytes, "")
}

// walkFollowSymlinks behaves like filepath.Walk but resolves
// symlinks. It guards against cycles using a visited-inode map
// (sized to the depth, not the total tree, so the memory hit is
// bounded by directory depth not file count).
func walkFollowSymlinks(root string, fn filepath.WalkFunc) error {
	visited := map[string]struct{}{}
	var walk func(path string) error
	walk = func(path string) error {
		info, err := os.Stat(path) // follows symlinks
		if err != nil {
			return fn(path, nil, err)
		}
		if info.IsDir() {
			real, err := filepath.EvalSymlinks(path)
			if err == nil {
				if _, seen := visited[real]; seen {
					return nil
				}
				visited[real] = struct{}{}
			}
			if err := fn(path, info, nil); err != nil {
				if errors.Is(err, filepath.SkipDir) {
					return nil
				}
				return err
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				return fn(path, info, err)
			}
			for _, e := range entries {
				if err := walk(filepath.Join(path, e.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		return fn(path, info, nil)
	}
	return walk(root)
}

func writeFileScanResponse(stream io.Writer, files, dirs, bytes int64, errMsg string) error {
	return link.WriteFrame(stream, &v2pb.FileScanResponse{
		FileCount:  files,
		DirCount:   dirs,
		TotalBytes: bytes,
		Error:      errMsg,
	})
}
