package protocol

import (
	"encoding/gob"
	"fmt"
	"io"
	"sync"

	"github.com/WangYihang/Platypus/internal/utils/message"
)

// Codec provides thread-safe encoding/decoding of protocol messages
// over a connection, wrapping gob encoder/decoder with proper locking
// and unified error handling.
type Codec struct {
	encoder     *gob.Encoder
	decoder     *gob.Decoder
	encoderLock sync.Mutex
	decoderLock sync.Mutex
}

// NewCodec creates a new Codec from a ReadWriter (typically a net.Conn).
func NewCodec(rw io.ReadWriter) *Codec {
	return &Codec{
		encoder: gob.NewEncoder(rw),
		decoder: gob.NewDecoder(rw),
	}
}

// NewCodecFromParts creates a Codec from separate reader and writer.
func NewCodecFromParts(r io.Reader, w io.Writer) *Codec {
	return &Codec{
		encoder: gob.NewEncoder(w),
		decoder: gob.NewDecoder(r),
	}
}

// Send encodes and sends a message in a thread-safe manner.
func (c *Codec) Send(msg message.Message) error {
	c.encoderLock.Lock()
	defer c.encoderLock.Unlock()
	if err := c.encoder.Encode(msg); err != nil {
		return fmt.Errorf("encode message (type=%d): %w", msg.Type, err)
	}
	return nil
}

// Recv decodes a message in a thread-safe manner.
func (c *Codec) Recv(msg *message.Message) error {
	c.decoderLock.Lock()
	defer c.decoderLock.Unlock()
	if err := c.decoder.Decode(msg); err != nil {
		return fmt.Errorf("decode message: %w", err)
	}
	return nil
}
