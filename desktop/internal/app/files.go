package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

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

// PickDirectoryToSave opens a native directory picker. Returns "" if the
// user cancelled. Used by multi-file and recursive folder downloads
// where we'd otherwise hammer the user with one save-as dialog per
// file.
func (a *App) PickDirectoryToSave(title string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("directory dialog requires running Wails runtime")
	}
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: title,
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

// FileEntryDTO is the TS-facing shape for a ListDir / StatFile entry.
// Field names must match internal/api.fileEntryDTO so the JSON unmarshal
// is symmetric; keep them in sync when touching either.
type FileEntryDTO struct {
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	Mode          uint32 `json:"mode"`
	ModTimeUnix   int64  `json:"modTimeUnix"`
	IsDir         bool   `json:"isDir"`
	IsSymlink     bool   `json:"isSymlink"`
	SymlinkTarget string `json:"symlinkTarget,omitempty"`
	Err           string `json:"error,omitempty"`
}

// ListDirResult is returned to the TS layer verbatim so callers can page
// through a large directory without guessing at total/eof semantics.
type ListDirResult struct {
	Entries []FileEntryDTO `json:"entries"`
	Total   int64          `json:"total"`
	EOF     bool           `json:"eof"`
}

// ListDir returns a page of directory entries. offset=0, limit=0 asks
// the agent for the first page at its default size (currently 5000).
func (a *App) ListDir(sessionID, path string, offset, limit int64) (ListDirResult, error) {
	c, err := a.client()
	if err != nil {
		return ListDirResult{}, err
	}
	q := url.Values{
		"path":   []string{path},
		"offset": []string{strconv.FormatInt(offset, 10)},
		"limit":  []string{strconv.FormatInt(limit, 10)},
	}
	body, err := c.Get(context.Background(),
		"/api/v1/sessions/"+url.PathEscape(sessionID)+"/files/list", q)
	if err != nil {
		return ListDirResult{}, err
	}
	var resp struct {
		Status  bool           `json:"status"`
		Entries []FileEntryDTO `json:"entries"`
		Total   int64          `json:"total"`
		EOF     bool           `json:"eof"`
		Error   string         `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ListDirResult{}, fmt.Errorf("parse list: %w", err)
	}
	if !resp.Status {
		return ListDirResult{}, fmt.Errorf("server: %s", resp.Error)
	}
	return ListDirResult{Entries: resp.Entries, Total: resp.Total, EOF: resp.EOF}, nil
}

// StatFile returns metadata for a single path.
func (a *App) StatFile(sessionID, path string) (FileEntryDTO, error) {
	c, err := a.client()
	if err != nil {
		return FileEntryDTO{}, err
	}
	q := url.Values{"path": []string{path}}
	body, err := c.Get(context.Background(),
		"/api/v1/sessions/"+url.PathEscape(sessionID)+"/files/stat", q)
	if err != nil {
		return FileEntryDTO{}, err
	}
	var resp struct {
		Status bool         `json:"status"`
		Entry  FileEntryDTO `json:"entry"`
		Error  string       `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return FileEntryDTO{}, fmt.Errorf("parse stat: %w", err)
	}
	if !resp.Status {
		return FileEntryDTO{}, fmt.Errorf("server: %s", resp.Error)
	}
	return resp.Entry, nil
}

// DeleteFile removes a file or — with recursive=true — a whole subtree.
func (a *App) DeleteFile(sessionID, path string, recursive bool) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	q := url.Values{
		"path":      []string{path},
		"recursive": []string{strconv.FormatBool(recursive)},
	}
	uri := "/api/v1/sessions/" + url.PathEscape(sessionID) + "/files?" + q.Encode()
	_, err = c.Delete(context.Background(), uri)
	return err
}

// RenameFile moves (renames) a path.
func (a *App) RenameFile(sessionID, from, to string) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	_, err = c.Post(context.Background(),
		"/api/v1/sessions/"+url.PathEscape(sessionID)+"/files/rename",
		map[string]string{"from": from, "to": to})
	return err
}

// Mkdir creates a directory. parents=true is mkdir -p. mode is the unix
// permission bits (0755 if 0 is passed).
func (a *App) Mkdir(sessionID, path string, parents bool, mode uint32) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	q := url.Values{
		"path":    []string{path},
		"parents": []string{strconv.FormatBool(parents)},
	}
	if mode != 0 {
		q.Set("mode", strconv.FormatUint(uint64(mode), 8))
	}
	uri := "/api/v1/sessions/" + url.PathEscape(sessionID) + "/files/mkdir?" + q.Encode()
	_, err = c.PostRaw(context.Background(), uri, "application/json", nil)
	return err
}

// Chmod sets permission bits on a path. mode is passed as the unix
// octal mode (e.g. 0o644). On Windows only the owner-write bit is
// meaningful — that's a platform quirk, not a bug.
func (a *App) Chmod(sessionID, path string, mode uint32) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	q := url.Values{
		"path": []string{path},
		"mode": []string{strconv.FormatUint(uint64(mode), 8)},
	}
	uri := "/api/v1/sessions/" + url.PathEscape(sessionID) + "/files/chmod?" + q.Encode()
	_, err = c.PostRaw(context.Background(), uri, "application/json", nil)
	return err
}

// DownloadFolder mirrors a remote directory tree onto the local
// filesystem. localDir is treated as the parent into which we
// re-create the remote folder — i.e. DownloadFolder(sid, "/etc/nginx",
// "/home/me/Downloads") writes files under "/home/me/Downloads/nginx".
//
// Symlinks are skipped (the downloaded copy would dangle on the
// destination machine anyway). Files larger than the per-chunk size
// stream through DownloadFile's existing chunk loop. Errors abort the
// walk so a partial mirror surfaces immediately rather than silently.
func (a *App) DownloadFolder(sessionID, remotePath, localDir string) error {
	remotePath = path.Clean(remotePath)
	if remotePath == "" || remotePath == "." {
		remotePath = "/"
	}
	root, err := a.StatFile(sessionID, remotePath)
	if err != nil {
		return fmt.Errorf("stat root: %w", err)
	}
	if !root.IsDir {
		return fmt.Errorf("%s is not a directory", remotePath)
	}
	rootName := path.Base(remotePath)
	if rootName == "/" || rootName == "." || rootName == "" {
		rootName = "root"
	}
	localRoot := filepath.Join(localDir, rootName)
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", localRoot, err)
	}
	return a.downloadFolderRecursive(sessionID, remotePath, localRoot)
}

func (a *App) downloadFolderRecursive(sessionID, remoteDir, localDir string) error {
	const pageLimit = int64(0) // 0 == agent default page size
	listing, err := a.ListDir(sessionID, remoteDir, 0, pageLimit)
	if err != nil {
		return fmt.Errorf("list %s: %w", remoteDir, err)
	}
	for _, e := range listing.Entries {
		if e.Err != "" {
			// Surface unreadable children but keep walking — typical
			// causes are EACCES on /proc or /root and aborting on
			// the first one would leave most of the tree unwritten.
			continue
		}
		if e.IsSymlink {
			continue
		}
		// Reject path traversal attempts in entry names. The agent
		// shouldn't be sending us "../" but if it does we'd write
		// outside localDir.
		if strings.ContainsAny(e.Name, "/\\") || e.Name == ".." || e.Name == "." {
			continue
		}
		remoteChild := path.Join(remoteDir, e.Name)
		localChild := filepath.Join(localDir, e.Name)
		if e.IsDir {
			if err := os.MkdirAll(localChild, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", localChild, err)
			}
			if err := a.downloadFolderRecursive(sessionID, remoteChild, localChild); err != nil {
				return err
			}
			continue
		}
		if err := a.DownloadFile(sessionID, remoteChild, localChild); err != nil {
			return fmt.Errorf("download %s: %w", remoteChild, err)
		}
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
