package runner

import (
	"io"
	"sync"
)

// duplexInput owns exactly one execute frame and at most one cancellation of
// that frame. EOF is a lifecycle signal, never a new execution/retry request.
type duplexInput struct {
	mu                sync.Mutex
	changed           *sync.Cond
	pending, cancel   []byte
	cancelled, closed bool
}

func newDuplexInput(execute, cancel []byte) *duplexInput {
	d := &duplexInput{pending: append(append([]byte{}, execute...), '\n'), cancel: append(append([]byte{}, cancel...), '\n')}
	d.changed = sync.NewCond(&d.mu)
	return d
}

func (d *duplexInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for len(d.pending) == 0 && !d.closed {
		d.changed.Wait()
	}
	if d.closed {
		return 0, io.EOF
	}
	n := copy(p, d.pending)
	d.pending = d.pending[n:]
	return n, nil
}

func (d *duplexInput) Cancel() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return io.ErrClosedPipe
	}
	if !d.cancelled {
		d.cancelled = true
		d.pending = append(d.pending, d.cancel...)
		d.changed.Broadcast()
	}
	return nil
}

func (d *duplexInput) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	d.pending = nil
	d.changed.Broadcast()
	return nil
}
