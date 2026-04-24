package mesh

import (
	"io"
	"sync"
	"sync/atomic"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// envCodec is the mesh-layer framing helper. Thread-safe Send/Recv of
// v2pb.MeshEnvelope messages over an io.ReadWriter plus lifetime byte
// and message counters the keepalive loop reports to peers.
//
// Delegates to link.WriteFrame / link.ReadFrame so the on-wire format
// (4-byte BE length prefix + protobuf bytes) stays identical to what
// the legacy internal/protocol.ProtoCodec used to emit. That package
// is being retired — mesh is the last consumer, and envCodec here is
// its replacement.
type envCodec struct {
	r       io.Reader
	w       io.Writer
	writeMu sync.Mutex
	readMu  sync.Mutex

	bytesSent atomic.Uint64
	bytesRecv atomic.Uint64
	msgsSent  atomic.Uint64
	msgsRecv  atomic.Uint64
}

func newEnvCodec(rw io.ReadWriter) *envCodec {
	return &envCodec{r: rw, w: rw}
}

func newEnvCodecFromParts(r io.Reader, w io.Writer) *envCodec {
	return &envCodec{r: r, w: w}
}

// Send marshals and frames one MeshEnvelope. Serialised writes via
// writeMu so concurrent goroutines (handshake, keepalive, flood) can
// share one codec safely.
func (c *envCodec) Send(env *v2pb.MeshEnvelope) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	cw := &countingWriter{w: c.w}
	if err := link.WriteFrame(cw, env); err != nil {
		return err
	}
	c.bytesSent.Add(uint64(cw.n))
	c.msgsSent.Add(1)
	return nil
}

// Recv reads one framed MeshEnvelope. readMu gates concurrent reads
// (only the run loop reads, but the mutex matches the old codec's
// contract).
func (c *envCodec) Recv() (*v2pb.MeshEnvelope, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	cr := &countingReader{r: c.r}
	var env v2pb.MeshEnvelope
	if err := link.ReadFrame(cr, &env); err != nil {
		return nil, err
	}
	c.bytesRecv.Add(uint64(cr.n))
	c.msgsRecv.Add(1)
	return &env, nil
}

func (c *envCodec) BytesSent() uint64 { return c.bytesSent.Load() }
func (c *envCodec) BytesRecv() uint64 { return c.bytesRecv.Load() }
func (c *envCodec) MsgsSent() uint64  { return c.msgsSent.Load() }
func (c *envCodec) MsgsRecv() uint64  { return c.msgsRecv.Load() }

// countingWriter / countingReader wrap an io.Writer / io.Reader and
// record the exact byte count that crossed through — used by
// envCodec.Send / Recv to attribute wire bytes to its counters
// without re-marshalling the payload.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
