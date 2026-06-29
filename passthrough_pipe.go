// Copyright 2026 Pete Steyert-Woods
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package expect

import (
	"bytes"
	"io"
	"os"
	"sync"
	"time"
)

// PassthroughPipe pipes data from a io.Reader and allows setting a read
// deadline. If a timeout is reached the error is returned, otherwise the error
// from the provided io.Reader is passed through instead.
//
// The implementation is intentionally not backed by an os.Pipe so that it
// works with readers whose platform offers no deadline support (e.g. the
// ConPTY output pipe on Windows).
type PassthroughPipe struct {
	mu       sync.Mutex
	cond     *sync.Cond
	buf      bytes.Buffer
	err      error
	deadline time.Time
	timer    *time.Timer
	closed   bool
	// done is closed when the reader goroutine has consumed the underlying
	// reader to EOF (or error), so all available data has been buffered.
	done chan struct{}
}

// drainTimeout bounds how long waitDrained blocks for the reader goroutine to
// reach EOF on the underlying reader. In normal use the data source (e.g. a
// pty's child process) has already exited, so EOF is immediate; the timeout
// only guards against a caller draining while the source is still producing,
// so a drain can never block indefinitely.
const drainTimeout = 5 * time.Second

// NewPassthroughPipe returns a new pipe for a io.Reader that passes through
// non-timeout errors.
func NewPassthroughPipe(reader io.Reader) (*PassthroughPipe, error) {
	pp := &PassthroughPipe{done: make(chan struct{})}
	pp.cond = sync.NewCond(&pp.mu)

	go func() {
		defer close(pp.done)
		chunk := make([]byte, 32*1024)
		for {
			n, err := reader.Read(chunk)
			pp.mu.Lock()
			if n > 0 {
				pp.buf.Write(chunk[:n])
			}
			if err != nil {
				pp.err = err
				pp.cond.Broadcast()
				pp.mu.Unlock()
				return
			}
			pp.cond.Broadcast()
			pp.mu.Unlock()
		}
	}()

	return pp, nil
}

// waitDrained blocks until the reader goroutine has consumed the underlying
// reader to EOF (or error) — guaranteeing every available byte is buffered and
// retrievable by Read — or until drainTimeout elapses. Callers tearing down a
// pipe should call this, once the data source has stopped producing, before
// closing the underlying reader: closing it first can discard not-yet-consumed
// bytes (a closed pty master drops its buffer), intermittently losing output.
func (pp *PassthroughPipe) waitDrained() {
	select {
	case <-pp.done:
	case <-time.After(drainTimeout):
	}
}

func (pp *PassthroughPipe) Read(p []byte) (n int, err error) {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	for {
		// Match os.File semantics: once the deadline has passed all reads
		// fail, even if data is available.
		if !pp.deadline.IsZero() && !time.Now().Before(pp.deadline) {
			return 0, os.ErrDeadlineExceeded
		}
		if pp.buf.Len() > 0 {
			return pp.buf.Read(p)
		}
		if pp.err != nil {
			return 0, pp.err
		}
		if pp.closed {
			return 0, io.ErrClosedPipe
		}
		pp.cond.Wait()
	}
}

// Close unblocks any pending reads. It does not close the underlying reader.
func (pp *PassthroughPipe) Close() error {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	pp.closed = true
	if pp.timer != nil {
		pp.timer.Stop()
		pp.timer = nil
	}
	pp.cond.Broadcast()
	return nil
}

// SetReadDeadline sets the deadline for future and pending Read calls. A zero
// value for t means Read will not time out.
func (pp *PassthroughPipe) SetReadDeadline(t time.Time) error {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	pp.deadline = t
	if pp.timer != nil {
		pp.timer.Stop()
		pp.timer = nil
	}
	if !t.IsZero() {
		d := time.Until(t)
		if d < 0 {
			d = 0
		}
		// Wake up pending reads once the deadline expires.
		pp.timer = time.AfterFunc(d, func() {
			pp.mu.Lock()
			pp.cond.Broadcast()
			pp.mu.Unlock()
		})
	}
	pp.cond.Broadcast()
	return nil
}
