package process

import "sync"

// ringBuffer is a fixed-capacity byte buffer; once full, writes
// overwrite the oldest bytes. String() reconstructs the contents
// in chronological order. Used for stderr capture; we never need
// to read partial bytes mid-stream.
type ringBuffer struct {
	mu    sync.Mutex
	buf   []byte
	start int  // index of oldest byte
	full  bool // true once buf has wrapped at least once
	end   int  // next-write index
}

func newRingBuffer(cap int) *ringBuffer {
	if cap < 1 {
		cap = 1
	}
	return &ringBuffer{buf: make([]byte, cap)}
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(p) >= len(r.buf) {
		copy(r.buf, p[len(p)-len(r.buf):])
		r.start = 0
		r.end = 0
		r.full = true
		return len(p), nil
	}
	for _, b := range p {
		r.buf[r.end] = b
		r.end = (r.end + 1) % len(r.buf)
		if r.full {
			r.start = r.end
		} else if r.end == 0 {
			r.full = true
		}
	}
	return len(p), nil
}

func (r *ringBuffer) WriteString(s string) (int, error) {
	return r.Write([]byte(s))
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return string(r.buf[:r.end])
	}
	out := make([]byte, 0, len(r.buf))
	out = append(out, r.buf[r.start:]...)
	out = append(out, r.buf[:r.end]...)
	return string(out)
}
