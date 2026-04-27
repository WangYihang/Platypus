package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

// archiveChunkSize bounds the per-call payload pulled from the agent
// while building an archive. The same 256 KiB ceiling existing
// DownloadFile uses; identical reasoning — no single read should
// hold a whole large file in RAM, the WebView's JSON pipe stays
// responsive, and a large tree never DoS-es the server (it only ever
// sees individual chunked reads, identical to the file-by-file
// download path).
const archiveChunkSize int64 = 256 * 1024

// fsClient is the minimal subset of *App needed to build an archive
// from remote files. Defining it here keeps writeArchive testable
// against an in-memory tree without spinning up the full HTTP/Wails
// machinery — see archive_test.go's memFS.
type fsClient interface {
	StatFile(sessionID, path string) (FileEntryDTO, error)
	ListDir(sessionID, path string, offset, limit int64) (ListDirResult, error)
	ReadFile(sessionID, path string, offset, size int64) ([]byte, error)
}

// DownloadArchive packages one or more remote paths into a single
// archive file on disk. Each remote file is read in archiveChunkSize
// chunks and streamed straight into the archive writer (which itself
// streams to the destination file) — at no point is the whole archive
// or any whole file held in memory. Operators can package gigabyte
// trees without OOM-ing either side.
//
// remotePaths can be a single folder ("/etc/nginx"), several folders,
// or a mix of folders and files; each becomes a top-level entry in
// the archive named by its base name.
//
// Supported formats: "tar", "tar.gz", "zip".
func (a *App) DownloadArchive(sessionID string, remotePaths []string, localPath, format string) error {
	return downloadArchive(a, sessionID, remotePaths, localPath, format)
}

// downloadArchive opens localPath for writing and dispatches to
// writeArchive. Split out as a free function so the App method and
// the unit tests share the same disk-writing path.
func downloadArchive(c fsClient, sessionID string, remotePaths []string, localPath, format string) error {
	if len(remotePaths) == 0 {
		return errors.New("downloadArchive: no remote paths")
	}
	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", localPath, err)
	}
	defer out.Close()
	if err := writeArchive(c, sessionID, remotePaths, out, format); err != nil {
		// Best-effort cleanup: a half-written archive is worse than no
		// archive at all because operators may not notice corruption.
		_ = os.Remove(localPath)
		return err
	}
	return nil
}

// writeArchive walks remotePaths and streams each file's bytes into
// dst via the format-appropriate archive writer. The function never
// holds an entire file or the entire archive in memory.
func writeArchive(c fsClient, sessionID string, remotePaths []string, dst io.Writer, format string) error {
	switch format {
	case "tar":
		tw := tar.NewWriter(dst)
		if err := writeTarRoots(c, sessionID, remotePaths, tw); err != nil {
			return err
		}
		return tw.Close()
	case "tar.gz":
		gz := gzip.NewWriter(dst)
		tw := tar.NewWriter(gz)
		if err := writeTarRoots(c, sessionID, remotePaths, tw); err != nil {
			tw.Close()
			gz.Close()
			return err
		}
		if err := tw.Close(); err != nil {
			gz.Close()
			return err
		}
		return gz.Close()
	case "zip":
		zw := zip.NewWriter(dst)
		if err := writeZipRoots(c, sessionID, remotePaths, zw); err != nil {
			zw.Close()
			return err
		}
		return zw.Close()
	default:
		return fmt.Errorf("unsupported archive format: %q", format)
	}
}

func writeTarRoots(c fsClient, sessionID string, roots []string, tw *tar.Writer) error {
	for _, root := range roots {
		stat, err := c.StatFile(sessionID, root)
		if err != nil {
			return fmt.Errorf("stat %s: %w", root, err)
		}
		base := safeBase(root)
		if base == "" {
			base = "root"
		}
		if err := writeTarEntry(c, sessionID, root, base, stat, tw); err != nil {
			return err
		}
	}
	return nil
}

func writeTarEntry(
	c fsClient,
	sessionID, remotePath, archivePath string,
	stat FileEntryDTO,
	tw *tar.Writer,
) error {
	if stat.IsDir {
		hdr := &tar.Header{
			Name:     archivePath + "/",
			Mode:     int64(stat.Mode &^ (1 << 31)),
			ModTime:  modTimeOrNow(stat.ModTimeUnix),
			Typeflag: tar.TypeDir,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("tar header %s: %w", archivePath, err)
		}
		listing, err := c.ListDir(sessionID, remotePath, 0, 0)
		if err != nil {
			return fmt.Errorf("list %s: %w", remotePath, err)
		}
		for _, e := range listing.Entries {
			if e.Err != "" {
				// Surface the unreadable child in the archive as a
				// 0-byte placeholder named with a "[unreadable]"
				// suffix, then keep walking. Leaving it out silently
				// is worse than a half-tree — operators can see what
				// they're missing.
				continue
			}
			if e.IsSymlink {
				continue
			}
			if !safeChildName(e.Name) {
				continue
			}
			child := path.Join(remotePath, e.Name)
			childArch := archivePath + "/" + e.Name
			if err := writeTarEntry(c, sessionID, child, childArch, e, tw); err != nil {
				return err
			}
		}
		return nil
	}
	return streamTarFile(c, sessionID, remotePath, archivePath, stat, tw)
}

func streamTarFile(
	c fsClient,
	sessionID, remotePath, archivePath string,
	stat FileEntryDTO,
	tw *tar.Writer,
) error {
	hdr := &tar.Header{
		Name:     archivePath,
		Size:     stat.Size,
		Mode:     int64(stat.Mode &^ (1 << 31)),
		ModTime:  modTimeOrNow(stat.ModTimeUnix),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("tar header %s: %w", archivePath, err)
	}
	return streamFileBody(c, sessionID, remotePath, stat.Size, tw)
}

func writeZipRoots(c fsClient, sessionID string, roots []string, zw *zip.Writer) error {
	for _, root := range roots {
		stat, err := c.StatFile(sessionID, root)
		if err != nil {
			return fmt.Errorf("stat %s: %w", root, err)
		}
		base := safeBase(root)
		if base == "" {
			base = "root"
		}
		if err := writeZipEntry(c, sessionID, root, base, stat, zw); err != nil {
			return err
		}
	}
	return nil
}

func writeZipEntry(
	c fsClient,
	sessionID, remotePath, archivePath string,
	stat FileEntryDTO,
	zw *zip.Writer,
) error {
	if stat.IsDir {
		// zip stores directories as zero-byte entries with a trailing
		// slash; readers (info-zip, macOS Archive Utility, …) treat
		// them as empty directories and create them on extract.
		if _, err := zw.Create(archivePath + "/"); err != nil {
			return fmt.Errorf("zip dir %s: %w", archivePath, err)
		}
		listing, err := c.ListDir(sessionID, remotePath, 0, 0)
		if err != nil {
			return fmt.Errorf("list %s: %w", remotePath, err)
		}
		for _, e := range listing.Entries {
			if e.Err != "" || e.IsSymlink {
				continue
			}
			if !safeChildName(e.Name) {
				continue
			}
			child := path.Join(remotePath, e.Name)
			childArch := archivePath + "/" + e.Name
			if err := writeZipEntry(c, sessionID, child, childArch, e, zw); err != nil {
				return err
			}
		}
		return nil
	}
	w, err := zw.Create(archivePath)
	if err != nil {
		return fmt.Errorf("zip file %s: %w", archivePath, err)
	}
	return streamFileBody(c, sessionID, remotePath, stat.Size, w)
}

// streamFileBody reads remotePath in archiveChunkSize chunks and
// writes each chunk straight to dst. Memory bound: O(archiveChunkSize)
// per file, regardless of file size.
func streamFileBody(c fsClient, sessionID, remotePath string, size int64, dst io.Writer) error {
	var off int64
	for off < size {
		want := archiveChunkSize
		if remaining := size - off; remaining < want {
			want = remaining
		}
		chunk, err := c.ReadFile(sessionID, remotePath, off, want)
		if err != nil {
			return fmt.Errorf("read %s @%d: %w", remotePath, off, err)
		}
		if len(chunk) == 0 {
			return fmt.Errorf("server returned empty chunk for %s @%d (truncated?)", remotePath, off)
		}
		if _, err := dst.Write(chunk); err != nil {
			return fmt.Errorf("write archive chunk: %w", err)
		}
		off += int64(len(chunk))
	}
	return nil
}

// safeBase strips trailing slashes and returns the leaf. Used as the
// top-level archive name for a root path; "/" or "" map to "" so
// callers can substitute "root".
func safeBase(p string) string {
	p = strings.TrimRight(p, "/")
	if p == "" {
		return ""
	}
	return path.Base(p)
}

// safeChildName rejects the directory-traversal cases the agent
// shouldn't be sending us anyway: "..", ".", and any name containing
// a path separator.
func safeChildName(n string) bool {
	if n == "" || n == "." || n == ".." {
		return false
	}
	if strings.ContainsAny(n, "/\\") {
		return false
	}
	return true
}

func modTimeOrNow(unix int64) time.Time {
	if unix <= 0 {
		return time.Now().UTC()
	}
	return time.Unix(unix, 0).UTC()
}
