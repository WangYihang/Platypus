package mesh

import (
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

)

const (
	meshStreamChunkSize    = 16 * 1024
	meshStreamInboundQueue = 32
)

type streamKey struct {
	initiator string
	id        uint64
}

type inboundFrame struct {
	chunk  []byte
	eof    bool
	reason string
}

type streamState struct {
	key      streamKey
	peerNode string
	conn     net.Conn

	openCh   chan error
	opened   bool
	inbound  chan inboundFrame
	done     chan struct{}
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

	open := &v2pb.MeshStreamOpen{
		InitiatorNodeId: m.node.NodeID(),
		StreamId:        st.key.id,
		TargetNodeId:    targetNodeID,
		Kind:            v2pb.MeshStreamKind_MESH_STREAM_KIND_BOOTSTRAP_SERVER,
	}
	if err := m.node.SendTo(targetNodeID, &v2pb.MeshEnvelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload:   &v2pb.MeshEnvelope_StreamOpen{StreamOpen: open},
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

func (m *streamManager) handleOpen(env *v2pb.MeshEnvelope) {
	msg := env.GetStreamOpen()
	if msg == nil {
		return
	}
	if msg.Kind != v2pb.MeshStreamKind_MESH_STREAM_KIND_BOOTSTRAP_SERVER {
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

func (m *streamManager) handleOpenAck(env *v2pb.MeshEnvelope) {
	msg := env.GetStreamOpenAck()
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

func (m *streamManager) handleData(env *v2pb.MeshEnvelope) {
	msg := env.GetStreamData()
	if msg == nil {
		return
	}
	key := streamKey{initiator: msg.InitiatorNodeId, id: msg.StreamId}
	st := m.get(key)
	if st == nil {
		return
	}
	frame := inboundFrame{eof: msg.Eof}
	if len(msg.Chunk) > 0 {
		frame.chunk = append([]byte(nil), msg.Chunk...)
	}
	if !m.enqueueInbound(st, frame) {
		m.node.logger.Warn("mesh stream inbound queue overflow",
			slog.String("initiator", key.initiator),
			slog.Uint64("stream_id", key.id))
		go m.closeStream(st, "inbound queue overflow", true)
	}
}

func (m *streamManager) handleClose(env *v2pb.MeshEnvelope) {
	msg := env.GetStreamClose()
	if msg == nil {
		return
	}
	key := streamKey{initiator: msg.InitiatorNodeId, id: msg.StreamId}
	st := m.get(key)
	if st == nil {
		return
	}
	st.closeErr = msg.Reason
	_ = m.enqueueInbound(st, inboundFrame{reason: msg.Reason})
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
			m.closeStream(st, "", false)
			return
		}
	}
}

func (m *streamManager) pumpInbound(st *streamState) {
	for {
		select {
		case <-st.done:
			return
		case frame, ok := <-st.inbound:
			if !ok {
				return
			}
			if len(frame.chunk) > 0 {
				if _, err := st.conn.Write(frame.chunk); err != nil {
					if !isClosedConnErr(err) {
						m.node.logger.Debug("mesh stream write failed", slog.String("error", err.Error()))
						m.closeStream(st, err.Error(), true)
					}
					return
				}
			}
			if frame.eof || frame.reason != "" {
				m.closeStream(st, frame.reason, false)
				return
			}
		}
	}
}

func (m *streamManager) enqueueInbound(st *streamState, frame inboundFrame) bool {
	select {
	case <-st.done:
		return false
	default:
	}
	select {
	case st.inbound <- frame:
		return true
	default:
		return false
	}
}

func (m *streamManager) newState(key streamKey, peerNode string, conn net.Conn, waitAck bool) *streamState {
	st := &streamState{
		key:      key,
		peerNode: peerNode,
		conn:     conn,
		inbound:  make(chan inboundFrame, meshStreamInboundQueue),
		done:     make(chan struct{}),
	}
	if waitAck {
		st.openCh = make(chan error, 1)
	}
	m.mu.Lock()
	m.streams[key] = st
	m.mu.Unlock()
	go m.pumpInbound(st)
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
	close(st.done)
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
	_ = m.node.SendTo(initiator, &v2pb.MeshEnvelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload: &v2pb.MeshEnvelope_StreamOpenAck{StreamOpenAck: &v2pb.MeshStreamOpenAck{
			InitiatorNodeId: initiator,
			StreamId:        streamID,
			Ok:              ok,
			Error:           errText,
		}},
	})
}

func (m *streamManager) sendData(st *streamState, chunk []byte, eof bool) error {
	return m.node.SendTo(st.peerNode, &v2pb.MeshEnvelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload: &v2pb.MeshEnvelope_StreamData{StreamData: &v2pb.MeshStreamData{
			InitiatorNodeId: st.key.initiator,
			StreamId:        st.key.id,
			Chunk:           chunk,
			Eof:             eof,
		}},
	})
}

func (m *streamManager) sendClose(st *streamState, reason string) {
	_ = m.node.SendTo(st.peerNode, &v2pb.MeshEnvelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload: &v2pb.MeshEnvelope_StreamClose{StreamClose: &v2pb.MeshStreamClose{
			InitiatorNodeId: st.key.initiator,
			StreamId:        st.key.id,
			Reason:          reason,
		}},
	})
}

func isClosedConnErr(err error) bool {
	return err != nil && (errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "use of closed network connection"))
}
