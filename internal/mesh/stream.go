package mesh

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

const meshStreamChunkSize = 16 * 1024

type streamKey struct {
	initiator string
	id        uint64
}

type streamState struct {
	key      streamKey
	peerNode string
	conn     net.Conn

	openCh   chan error
	opened   bool
	closeMu  sync.Mutex
	closed   bool
	closeErr string
}

type streamManager struct {
	node *Node

	mu      sync.Mutex
	nextID  uint64
	streams map[streamKey]*streamState
}

func newStreamManager(node *Node) *streamManager {
	return &streamManager{
		node:    node,
		streams: map[streamKey]*streamState{},
	}
}

func (m *streamManager) DialBootstrap(ctx context.Context, targetNodeID string) (net.Conn, error) {
	if targetNodeID == "" {
		return nil, fmt.Errorf("mesh: empty bootstrap target node")
	}

	local, internal := net.Pipe()
	st := m.newState(streamKey{
		initiator: m.node.NodeID(),
		id:        m.nextStreamID(),
	}, targetNodeID, internal, true)

	open := &agentpb.MeshStreamOpen{
		InitiatorNodeId: m.node.NodeID(),
		StreamId:        st.key.id,
		TargetNodeId:    targetNodeID,
		Kind:            agentpb.MeshStreamKind_MESH_STREAM_KIND_BOOTSTRAP_SERVER,
	}
	if err := m.node.SendTo(targetNodeID, &agentpb.Envelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload:   &agentpb.Envelope_MeshStreamOpen{MeshStreamOpen: open},
	}); err != nil {
		closeConns(local, internal)
		m.drop(st.key)
		return nil, err
	}

	select {
	case err := <-st.openCh:
		if err != nil {
			closeConns(local, internal)
			m.drop(st.key)
			return nil, err
		}
	case <-ctx.Done():
		closeConns(local, internal)
		m.closeStream(st, "dial timeout", false)
		return nil, ctx.Err()
	}

	go m.pumpOutbound(st)
	return local, nil
}

func (m *streamManager) handleOpen(env *agentpb.Envelope) {
	msg := env.GetMeshStreamOpen()
	if msg == nil {
		return
	}
	if msg.Kind != agentpb.MeshStreamKind_MESH_STREAM_KIND_BOOTSTRAP_SERVER {
		m.sendOpenAck(msg.InitiatorNodeId, msg.StreamId, false, "unsupported stream kind")
		return
	}
	if !m.node.cfg.BootstrapEnabled || m.node.cfg.BootstrapTarget == "" {
		m.sendOpenAck(msg.InitiatorNodeId, msg.StreamId, false, "bootstrap service disabled")
		return
	}
	if msg.TargetNodeId != "" && msg.TargetNodeId != m.node.NodeID() {
		m.sendOpenAck(msg.InitiatorNodeId, msg.StreamId, false, "wrong bootstrap target")
		return
	}

	conn, err := net.DialTimeout("tcp", m.node.cfg.BootstrapTarget, 10*time.Second)
	if err != nil {
		m.sendOpenAck(msg.InitiatorNodeId, msg.StreamId, false, err.Error())
		return
	}

	key := streamKey{initiator: msg.InitiatorNodeId, id: msg.StreamId}
	st := m.newState(key, msg.InitiatorNodeId, conn, false)
	st.opened = true
	m.sendOpenAck(msg.InitiatorNodeId, msg.StreamId, true, "")
	go m.pumpOutbound(st)
}

func (m *streamManager) handleOpenAck(env *agentpb.Envelope) {
	msg := env.GetMeshStreamOpenAck()
	if msg == nil {
		return
	}
	key := streamKey{initiator: msg.InitiatorNodeId, id: msg.StreamId}
	st := m.get(key)
	if st == nil || st.openCh == nil {
		return
	}
	if msg.Ok {
		st.openCh <- nil
	} else {
		st.openCh <- fmt.Errorf("mesh bootstrap open failed: %s", msg.Error)
	}
	close(st.openCh)
	st.openCh = nil
	st.opened = msg.Ok
}

func (m *streamManager) handleData(env *agentpb.Envelope) {
	msg := env.GetMeshStreamData()
	if msg == nil {
		return
	}
	key := streamKey{initiator: msg.InitiatorNodeId, id: msg.StreamId}
	st := m.get(key)
	if st == nil {
		return
	}
	if len(msg.Chunk) > 0 {
		if _, err := st.conn.Write(msg.Chunk); err != nil {
			m.node.logger.Debug("mesh stream write failed", slog.String("error", err.Error()))
			m.closeStream(st, err.Error(), true)
			return
		}
	}
	if msg.Eof {
		closeConn(st.conn)
		m.drop(key)
	}
}

func (m *streamManager) handleClose(env *agentpb.Envelope) {
	msg := env.GetMeshStreamClose()
	if msg == nil {
		return
	}
	key := streamKey{initiator: msg.InitiatorNodeId, id: msg.StreamId}
	st := m.get(key)
	if st == nil {
		return
	}
	st.closeErr = msg.Reason
	closeConn(st.conn)
	m.drop(key)
}

func (m *streamManager) pumpOutbound(st *streamState) {
	buf := make([]byte, meshStreamChunkSize)
	for {
		n, err := st.conn.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			if sendErr := m.sendData(st, chunk, false); sendErr != nil {
				m.closeStream(st, sendErr.Error(), true)
				return
			}
		}
		if err != nil {
			if err == io.EOF {
				_ = m.sendData(st, nil, true)
			} else if !isClosedConnErr(err) {
				m.node.logger.Debug("mesh stream read failed", slog.String("error", err.Error()))
				m.sendClose(st, err.Error())
			}
			closeConn(st.conn)
			m.drop(st.key)
			return
		}
	}
}

func (m *streamManager) newState(key streamKey, peerNode string, conn net.Conn, waitAck bool) *streamState {
	st := &streamState{
		key:      key,
		peerNode: peerNode,
		conn:     conn,
	}
	if waitAck {
		st.openCh = make(chan error, 1)
	}
	m.mu.Lock()
	m.streams[key] = st
	m.mu.Unlock()
	return st
}

func (m *streamManager) get(key streamKey) *streamState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streams[key]
}

func (m *streamManager) drop(key streamKey) {
	m.mu.Lock()
	delete(m.streams, key)
	m.mu.Unlock()
}

func (m *streamManager) nextStreamID() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	return m.nextID
}

func (m *streamManager) closeStream(st *streamState, reason string, notifyPeer bool) {
	st.closeMu.Lock()
	if st.closed {
		st.closeMu.Unlock()
		return
	}
	st.closed = true
	st.closeMu.Unlock()
	if notifyPeer {
		m.sendClose(st, reason)
	}
	closeConn(st.conn)
	m.drop(st.key)
}

func (m *streamManager) sendOpenAck(initiator string, streamID uint64, ok bool, errText string) {
	if initiator == "" {
		return
	}
	_ = m.node.SendTo(initiator, &agentpb.Envelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload: &agentpb.Envelope_MeshStreamOpenAck{MeshStreamOpenAck: &agentpb.MeshStreamOpenAck{
			InitiatorNodeId: initiator,
			StreamId:        streamID,
			Ok:              ok,
			Error:           errText,
		}},
	})
}

func (m *streamManager) sendData(st *streamState, chunk []byte, eof bool) error {
	return m.node.SendTo(st.peerNode, &agentpb.Envelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload: &agentpb.Envelope_MeshStreamData{MeshStreamData: &agentpb.MeshStreamData{
			InitiatorNodeId: st.key.initiator,
			StreamId:        st.key.id,
			Chunk:           chunk,
			Eof:             eof,
		}},
	})
}

func (m *streamManager) sendClose(st *streamState, reason string) {
	_ = m.node.SendTo(st.peerNode, &agentpb.Envelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload: &agentpb.Envelope_MeshStreamClose{MeshStreamClose: &agentpb.MeshStreamClose{
			InitiatorNodeId: st.key.initiator,
			StreamId:        st.key.id,
			Reason:          reason,
		}},
	})
}

func isClosedConnErr(err error) bool {
	if err == nil {
		return false
	}
	return err == net.ErrClosed
}

// DialBootstrap opens a routed byte stream to the target mesh node and
// returns a net.Conn that callers can wrap with TLS.
func (n *Node) DialBootstrap(ctx context.Context, targetNodeID string) (net.Conn, error) {
	if n == nil || n.streams == nil {
		return nil, fmt.Errorf("mesh: node not initialised")
	}
	return n.streams.DialBootstrap(ctx, targetNodeID)
}
