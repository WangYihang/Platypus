package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// fakeCodec captures Send calls so tests can inspect the envelopes each
// handler emits without needing a real TLS connection.
type fakeCodec struct{ sent []*agentpb.Envelope }

func (f *fakeCodec) Send(e *agentpb.Envelope) error { f.sent = append(f.sent, e); return nil }
func (f *fakeCodec) Recv() (*agentpb.Envelope, error) {
	return nil, nil
}

func newTestClient() (*Client, *fakeCodec) {
	codec := &fakeCodec{}
	return &Client{Codec: codec}, codec
}

func TestHandleMkdirAndListAndStatAndDelete(t *testing.T) {
	dir := t.TempDir()
	c, codec := newTestClient()

	// mkdir a/b/c with parents
	nested := filepath.Join(dir, "a", "b", "c")
	handleMkdir(c, "req-mk", &agentpb.MkdirRequest{Path: nested, Parents: true, Mode: 0o755})
	if got := codec.sent[0].GetMkdirResponse(); got == nil || got.Error != "" {
		t.Fatalf("mkdir failed: %+v", got)
	}
	if fi, err := os.Stat(nested); err != nil || !fi.IsDir() {
		t.Fatalf("mkdir didn't create %s: %v", nested, err)
	}

	// write a couple of files so ListDir has content
	files := []string{"alpha.txt", "beta.bin"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// ListDir — expect 3 entries (alpha.txt, beta.bin, a) in alphabetical order.
	codec.sent = nil
	handleListDir(c, "req-ls", &agentpb.ListDirRequest{Path: dir})
	ls := codec.sent[0].GetListDirResponse()
	if ls == nil || ls.Error != "" {
		t.Fatalf("list failed: %+v", ls)
	}
	if ls.Total != 3 {
		t.Fatalf("total = %d, want 3", ls.Total)
	}
	names := make([]string, 0, len(ls.Entries))
	for _, e := range ls.Entries {
		names = append(names, e.Name)
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("entries not sorted: %v", names)
	}
	if !ls.Eof {
		t.Fatal("expected eof=true for small listing")
	}

	// Paging sanity: offset=2, limit=1 → exactly one entry, not eof unless
	// the third happens to be the tail.
	codec.sent = nil
	handleListDir(c, "req-ls2", &agentpb.ListDirRequest{Path: dir, Offset: 2, Limit: 1})
	ls2 := codec.sent[0].GetListDirResponse()
	if len(ls2.Entries) != 1 {
		t.Fatalf("paging len = %d, want 1", len(ls2.Entries))
	}

	// Stat alpha.txt — should be a regular file with non-zero size.
	codec.sent = nil
	handleStat(c, "req-st", &agentpb.StatRequest{Path: filepath.Join(dir, "alpha.txt")})
	st := codec.sent[0].GetStatResponse()
	if st.Error != "" || st.Entry == nil {
		t.Fatalf("stat failed: %+v", st)
	}
	if st.Entry.IsDir {
		t.Fatal("alpha.txt shouldn't be a directory")
	}
	if int(st.Entry.Size) != len("alpha.txt") {
		t.Fatalf("size = %d, want %d", st.Entry.Size, len("alpha.txt"))
	}

	// Delete a non-empty dir without recursive should fail; with recursive
	// should succeed.
	codec.sent = nil
	handleDelete(c, "req-rm", &agentpb.DeleteRequest{Path: filepath.Join(dir, "a")})
	delResp := codec.sent[0].GetDeleteResponse()
	if delResp.Error == "" {
		t.Fatal("expected non-recursive delete of non-empty dir to fail")
	}
	codec.sent = nil
	handleDelete(c, "req-rm2", &agentpb.DeleteRequest{Path: filepath.Join(dir, "a"), Recursive: true})
	delResp = codec.sent[0].GetDeleteResponse()
	if delResp.Error != "" {
		t.Fatalf("recursive delete failed: %s", delResp.Error)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatalf("dir still exists after recursive delete: %v", err)
	}
}

func TestHandleRename(t *testing.T) {
	dir := t.TempDir()
	c, codec := newTestClient()

	src := filepath.Join(dir, "old.txt")
	dst := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	handleRename(c, "req", &agentpb.RenameRequest{From: src, To: dst})
	resp := codec.sent[0].GetRenameResponse()
	if resp.Error != "" {
		t.Fatalf("rename failed: %s", resp.Error)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("dst missing after rename: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src still present after rename: %v", err)
	}
}

func TestHandleRenameRejectsEmpty(t *testing.T) {
	c, codec := newTestClient()
	handleRename(c, "req", &agentpb.RenameRequest{From: "", To: "/tmp/x"})
	if resp := codec.sent[0].GetRenameResponse(); resp.Error == "" {
		t.Fatal("empty 'from' should fail")
	}
}

func TestHandleChmodOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics differ on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "perm.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, codec := newTestClient()

	handleChmod(c, "req", &agentpb.ChmodRequest{Path: path, Mode: 0o751})
	if resp := codec.sent[0].GetChmodResponse(); resp.Error != "" {
		t.Fatalf("chmod failed: %s", resp.Error)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o751 {
		t.Fatalf("mode = %o, want 751", fi.Mode().Perm())
	}
}

func TestHandleListDirSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need admin on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	c, codec := newTestClient()
	handleListDir(c, "req", &agentpb.ListDirRequest{Path: dir})
	ls := codec.sent[0].GetListDirResponse()

	var linkEntry *agentpb.FileEntry
	for _, e := range ls.Entries {
		if e.Name == "link" {
			linkEntry = e
			break
		}
	}
	if linkEntry == nil {
		t.Fatalf("link not in listing (names=%v)", entryNames(ls.Entries))
	}
	if !linkEntry.IsSymlink {
		t.Fatal("link entry not flagged as symlink")
	}
	if !strings.HasSuffix(linkEntry.SymlinkTarget, "target.txt") {
		t.Fatalf("symlink_target = %q, expected to end with target.txt", linkEntry.SymlinkTarget)
	}
}

func entryNames(entries []*agentpb.FileEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return out
}

func TestHandleListDirRejectsEmptyPath(t *testing.T) {
	c, codec := newTestClient()
	handleListDir(c, "req", &agentpb.ListDirRequest{})
	if resp := codec.sent[0].GetListDirResponse(); resp.Error == "" {
		t.Fatal("empty path should fail")
	}
}
