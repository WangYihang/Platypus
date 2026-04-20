package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// downloadChunkSize and uploadChunkSize bound the per-call payload so we
// don't blow up the WebView's JSON pipe with a single huge buffer.
const (
	downloadChunkSize int64 = 256 * 1024
	uploadChunkSize   int64 = 256 * 1024
)

// PickFileToUpload opens a native file picker. Returns "" if the user
// cancelled. Wails-only — no-ops outside the running app.
func (a *App) PickFileToUpload(title string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("file dialog requires running Wails runtime")
	}
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{Title: title})
}

// PickSaveLocation opens a native save-as dialog. Returns "" if cancelled.
func (a *App) PickSaveLocation(title, defaultFilename string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("file dialog requires running Wails runtime")
	}
	return wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: defaultFilename,
	})
}

// FileSize asks the agent how many bytes a file occupies. Returns -1 with
// an error if the agent can't stat the file (e.g. permission denied).
func (a *App) FileSize(sessionID, path string) (int64, error) {
	c, err := a.client()
	if err != nil {
		return 0, err
	}
	q := url.Values{"path": []string{path}}
	body, err := c.Get(context.Background(),
		"/api/v1/sessions/"+url.PathEscape(sessionID)+"/files/size", q)
	if err != nil {
		return 0, err
	}
	var resp struct {
		Status bool   `json:"status"`
		Size   int64  `json:"size"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse size: %w", err)
	}
	if !resp.Status {
		return 0, fmt.Errorf("server: %s", resp.Error)
	}
	return resp.Size, nil
}

// ReadFile returns up to size bytes from offset. size==0 means "to end of file".
func (a *App) ReadFile(sessionID, path string, offset, size int64) ([]byte, error) {
	c, err := a.client()
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"path":   []string{path},
		"offset": []string{strconv.FormatInt(offset, 10)},
		"size":   []string{strconv.FormatInt(size, 10)},
	}
	return c.Get(context.Background(),
		"/api/v1/sessions/"+url.PathEscape(sessionID)+"/files", q)
}

// WriteFile uploads bytes to the remote path. If appendMode is true, bytes
// are appended; otherwise the file is overwritten.
func (a *App) WriteFile(sessionID, path string, data []byte, appendMode bool) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	q := url.Values{
		"path":   []string{path},
		"append": []string{strconv.FormatBool(appendMode)},
	}
	uri := "/api/v1/sessions/" + url.PathEscape(sessionID) + "/files?" + q.Encode()
	_, err = c.PostRaw(context.Background(), uri, "application/octet-stream", data)
	return err
}

// DownloadFile streams a remote file to localPath in 256 KiB chunks.
// Designed for arbitrary-size files — never holds the whole payload in
// memory.
func (a *App) DownloadFile(sessionID, remotePath, localPath string) error {
	size, err := a.FileSize(sessionID, remotePath)
	if err != nil {
		return fmt.Errorf("size: %w", err)
	}
	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local: %w", err)
	}
	defer f.Close()

	var off int64
	for off < size {
		want := downloadChunkSize
		if remaining := size - off; remaining < want {
			want = remaining
		}
		chunk, err := a.ReadFile(sessionID, remotePath, off, want)
		if err != nil {
			return fmt.Errorf("read at %d: %w", off, err)
		}
		if len(chunk) == 0 {
			return fmt.Errorf("server returned empty chunk at offset %d (truncated?)", off)
		}
		if _, err := f.Write(chunk); err != nil {
			return fmt.Errorf("write local: %w", err)
		}
		off += int64(len(chunk))
	}
	return nil
}

// UploadFile streams localPath to the remote path, again in 256 KiB chunks.
// First chunk overwrites; subsequent chunks append.
func (a *App) UploadFile(sessionID, remotePath, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local: %w", err)
	}
	defer f.Close()

	buf := make([]byte, uploadChunkSize)
	first := true
	for {
		n, err := io.ReadFull(f, buf)
		if n == 0 {
			if first {
				// Empty source file — write empty payload to truncate dest.
				return a.WriteFile(sessionID, remotePath, nil, false)
			}
			break
		}
		if err := a.WriteFile(sessionID, remotePath, buf[:n], !first); err != nil {
			return err
		}
		first = false
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil && err != io.ErrUnexpectedEOF {
			return fmt.Errorf("read local: %w", err)
		}
	}
	return nil
}
