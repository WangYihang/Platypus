package api

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// setupArchiveAgentWithSlicableRead is the offset/length-honouring
// twin of setupArchiveAgentWithRead. The existing helper always
// streams the full body regardless of FileReadRequest.Offset/Length,
// which is fine for the legacy full-download tests but useless for
// Range coverage where the whole point is that the agent serves a
// slice. The agent here mirrors the production agent's Clamp +
// Length semantics exactly so the HTTP-side translation logic can
// be checked end-to-end.
func setupArchiveAgentWithSlicableRead(t *testing.T, agentID string, body []byte) *archiveTestAgent {
	t.Helper()
	fixture := newAgentRouteFixture(t, agentID)

	svc := core.NewAgentLinkService()
	clientConn, serverConn := net.Pipe()
	serverCh := make(chan *link.Session, 1)
	go func() {
		s, err := link.NewServerSession(serverConn)
		if err != nil {
			t.Errorf("server session: %v", err)
			return
		}
		serverCh <- s
	}()
	agentSess, err := link.NewClientSession(clientConn)
	if err != nil {
		t.Fatalf("client session: %v", err)
	}
	peer := <-serverCh
	svc.Register(agentID, agentSess)

	go func() {
		for {
			hdr, stream, err := peer.Accept()
			if err != nil {
				return
			}
			switch hdr.Type {
			case v2pb.StreamType_STREAM_TYPE_FILE_READ:
				var req v2pb.FileReadRequest
				_ = proto.Unmarshal(hdr.Metadata, &req)
				go func(s io.ReadWriteCloser, r v2pb.FileReadRequest) {
					defer s.Close()
					size := int64(len(body))
					off := r.Offset
					if off < 0 {
						off = 0
					}
					if off > size {
						off = size
					}
					remaining := size - off
					if r.Length > 0 && r.Length < remaining {
						remaining = r.Length
					}
					_ = link.WriteFrame(s, &v2pb.FileReadResponse{
						Size: size, Mode: 0o644,
					})
					if remaining == 0 {
						_ = link.WriteFrame(s, &v2pb.FileChunk{Eof: true})
						return
					}
					_ = link.WriteFrame(s, &v2pb.FileChunk{
						Data: body[off : off+remaining],
						Eof:  true,
					})
				}(stream, req)
			default:
				_ = stream.Close()
			}
		}
	}()

	t.Cleanup(func() {
		agentSess.Close()
		peer.Close()
	})

	return &archiveTestAgent{
		svc:      svc,
		peer:     peer,
		fixture:  fixture,
		cancels:  NewTransferCancelRegistry(),
		recorder: &FakeTransferRecorder{},
	}
}

// rangeFixture100 is the 100-byte canonical fixture used by every
// Range test below. Distinct ASCII chars keep the slice assertions
// readable in failure output.
var rangeFixture100 = func() []byte {
	b := make([]byte, 100)
	for i := range b {
		b[i] = byte('A' + (i % 26))
	}
	return b
}()

func newRangeAgent(t *testing.T, name string) *archiveTestAgent {
	return setupArchiveAgentWithSlicableRead(t, name, rangeFixture100)
}

func mountRangeRoutes(t *testing.T, a *archiveTestAgent) *httptest.Server {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// fsReadGet is a small helper that issues a GET to /fs/read with the
// fixture's bearer and an optional Range header.
func fsReadGet(t *testing.T, srvURL, fsReadURL, bearer, rangeHdr string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srvURL+fsReadURL, nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	if rangeHdr != "" {
		req.Header.Set("Range", rangeHdr)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

// TestFileRead_Range_BytesRange_Returns206 covers the canonical
// `bytes=A-B` form most media players use.
func TestFileRead_Range_BytesRange_Returns206(t *testing.T) {
	a := newRangeAgent(t, "agent-range-ab")
	srv := mountRangeRoutes(t, a)

	resp := fsReadGet(t, srv.URL, a.fixture.URL("/fs/read?path=/x"), a.fixture.Token, "bytes=10-19")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 206; body=%s", resp.StatusCode, b)
	}
	if got, want := resp.Header.Get("Content-Range"), "bytes 10-19/100"; got != want {
		t.Errorf("Content-Range = %q; want %q", got, want)
	}
	if got, want := resp.Header.Get("Content-Length"), "10"; got != want {
		t.Errorf("Content-Length = %q; want %q", got, want)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, rangeFixture100[10:20]) {
		t.Errorf("body = %q; want %q", body, rangeFixture100[10:20])
	}
}

// TestFileRead_Range_OpenEnded_Returns206 covers `bytes=A-` (read
// from A to the end of the file).
func TestFileRead_Range_OpenEnded_Returns206(t *testing.T) {
	a := newRangeAgent(t, "agent-range-open")
	srv := mountRangeRoutes(t, a)

	resp := fsReadGet(t, srv.URL, a.fixture.URL("/fs/read?path=/x"), a.fixture.Token, "bytes=50-")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 206; body=%s", resp.StatusCode, b)
	}
	if got, want := resp.Header.Get("Content-Range"), "bytes 50-99/100"; got != want {
		t.Errorf("Content-Range = %q; want %q", got, want)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, rangeFixture100[50:]) {
		t.Errorf("body = %q; want %q", body, rangeFixture100[50:])
	}
}

// TestFileRead_Range_Suffix_Returns206 covers the `bytes=-N` (last N
// bytes) form. Media players that read the moov atom from the tail
// of an MP4 use this; without coverage we'd silently 200 the whole
// file and force a re-download.
func TestFileRead_Range_Suffix_Returns206(t *testing.T) {
	a := newRangeAgent(t, "agent-range-suffix")
	srv := mountRangeRoutes(t, a)

	resp := fsReadGet(t, srv.URL, a.fixture.URL("/fs/read?path=/x"), a.fixture.Token, "bytes=-20")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 206; body=%s", resp.StatusCode, b)
	}
	if got, want := resp.Header.Get("Content-Range"), "bytes 80-99/100"; got != want {
		t.Errorf("Content-Range = %q; want %q", got, want)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, rangeFixture100[80:]) {
		t.Errorf("body = %q; want %q", body, rangeFixture100[80:])
	}
}

// TestFileRead_Range_Unsatisfiable_Returns416 covers the case where
// the client asks for a range entirely past EOF — the spec mandates
// 416 with `Content-Range: bytes */<size>` so the client knows the
// real upper bound.
func TestFileRead_Range_Unsatisfiable_Returns416(t *testing.T) {
	a := newRangeAgent(t, "agent-range-416")
	srv := mountRangeRoutes(t, a)

	resp := fsReadGet(t, srv.URL, a.fixture.URL("/fs/read?path=/x"), a.fixture.Token, "bytes=200-300")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 416; body=%s", resp.StatusCode, b)
	}
	if got, want := resp.Header.Get("Content-Range"), "bytes */100"; got != want {
		t.Errorf("Content-Range = %q; want %q", got, want)
	}
}

// TestFileRead_Range_Multipart_FallsBackTo200 documents that
// multi-range requests intentionally fall back to a full 200 — we
// don't implement multipart/byteranges (rarely used by browsers) and
// silently degrading to a single 200 is the standard, spec-compliant
// fallback.
func TestFileRead_Range_Multipart_FallsBackTo200(t *testing.T) {
	a := newRangeAgent(t, "agent-range-multi")
	srv := mountRangeRoutes(t, a)

	resp := fsReadGet(t, srv.URL, a.fixture.URL("/fs/read?path=/x"), a.fixture.Token, "bytes=0-9,20-29")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 200; body=%s", resp.StatusCode, b)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, rangeFixture100) {
		t.Errorf("multi-range fallback body did not equal full file (len got=%d want=%d)",
			len(body), len(rangeFixture100))
	}
}

// TestFileRead_NoRange_StillReturns200 is the regression pin for the
// existing full-download path: adding Range support must not change
// any byte of the no-Range response.
func TestFileRead_NoRange_StillReturns200(t *testing.T) {
	a := newRangeAgent(t, "agent-range-norange")
	srv := mountRangeRoutes(t, a)

	resp := fsReadGet(t, srv.URL, a.fixture.URL("/fs/read?path=/x"), a.fixture.Token, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 200; body=%s", resp.StatusCode, b)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, rangeFixture100) {
		t.Errorf("body length = %d; want %d", len(body), len(rangeFixture100))
	}
}

// TestFileRead_Range_DoesNotRecordTransfer documents the policy
// choice that Range hits stay out of the file_transfers drawer:
// otherwise a single video preview would explode into hundreds of
// rows as the player seeked. The full-download path keeps recording.
func TestFileRead_Range_DoesNotRecordTransfer(t *testing.T) {
	a := newRangeAgent(t, "agent-range-norec")
	srv := mountRangeRoutes(t, a)

	resp := fsReadGet(t, srv.URL, a.fixture.URL("/fs/read?path=/x"), a.fixture.Token, "bytes=0-9")
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	a.recorder.mu.Lock()
	createdCount := len(a.recorder.created)
	a.recorder.mu.Unlock()
	if createdCount != 0 {
		t.Fatalf("recorder.Create called %d times for a Range hit; want 0", createdCount)
	}
}

// TestFileRead_Range_AcceptRangesHeader covers the affordance side:
// every full-200 response advertises Range support so well-behaved
// clients (pdf.js, video.js) opt into Range on the second request.
func TestFileRead_Range_AcceptRangesHeader(t *testing.T) {
	a := newRangeAgent(t, "agent-range-accept")
	srv := mountRangeRoutes(t, a)

	resp := fsReadGet(t, srv.URL, a.fixture.URL("/fs/read?path=/x"), a.fixture.Token, "")
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	if got := resp.Header.Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges = %q; want %q", got, "bytes")
	}
}

// TestFileRead_Range_PreviewToken_Works pins that the Range path
// composes with the signed-URL path: a browser <video> can issue
// Range requests against a preview-token URL without losing the
// alternative auth.
func TestFileRead_Range_PreviewToken_Works(t *testing.T) {
	a := newRangeAgent(t, "agent-range-prev")
	signer := signerForTests(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutesWithSigner(r, signer)
	srv := httptest.NewServer(r)
	defer srv.Close()

	tok, exp := signer.Sign(a.fixture.ProjectID, a.fixture.AgentID, "/x")

	url := fsReadURLWithToken(srv.URL, a.fixture.URL(""), "/x", tok, exp)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Range", "bytes=10-19")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 206; body=%s", resp.StatusCode, b)
	}
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, rangeFixture100[10:20]) {
		t.Errorf("body = %q; want %q", got, rangeFixture100[10:20])
	}
}

