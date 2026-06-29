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
//
// SPDX-License-Identifier: Apache-2.0

package expect

import (
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func TestPassthroughPipe(t *testing.T) {
	r, w := io.Pipe()

	passthroughPipe, err := NewPassthroughPipe(r)
	assert.NilError(t, err)

	err = passthroughPipe.SetReadDeadline(time.Now().Add(time.Hour))
	assert.NilError(t, err)

	pipeError := errors.New("pipe error")
	err = w.CloseWithError(pipeError)
	assert.NilError(t, err)

	p := make([]byte, 1)
	_, err = passthroughPipe.Read(p)
	assert.Check(t, is.ErrorIs(err, pipeError))
}

// gatedReader blocks every Read until gate is closed, then returns its data on
// the first read and io.EOF thereafter. It models a source (like a pty master)
// holding buffered bytes the passthrough goroutine has not yet consumed when
// the pipe is torn down.
type gatedReader struct {
	gate <-chan struct{}
	data []byte
	sent bool
}

func (r *gatedReader) Read(p []byte) (int, error) {
	<-r.gate
	if r.sent {
		return 0, io.EOF
	}
	r.sent = true
	return copy(p, r.data), nil
}

// TestPassthroughPipeReadShortCircuitsOnCloseBeforeDrain documents the hazard
// the drain fix addresses: with the source's bytes still unconsumed, a Read
// after Close short-circuits to ErrClosedPipe and the data is lost.
func TestPassthroughPipeReadShortCircuitsOnCloseBeforeDrain(t *testing.T) {
	gate := make(chan struct{}) // never closed: the reader goroutine stays blocked
	defer close(gate)

	pp, err := NewPassthroughPipe(&gatedReader{gate: gate, data: []byte("buffered output")})
	assert.NilError(t, err)
	assert.NilError(t, pp.Close())

	// Nothing has been buffered yet, so Read sees closed and gives up — the
	// bytes the source is about to produce would never be read.
	_, err = pp.Read(make([]byte, 16))
	assert.Check(t, is.ErrorIs(err, io.ErrClosedPipe))
}

// TestPassthroughPipeWaitDrainedCapturesBufferedData verifies the fix: even
// when Close runs before the source's bytes are consumed, waitDrained blocks
// until they are buffered, so they remain readable.
func TestPassthroughPipeWaitDrainedCapturesBufferedData(t *testing.T) {
	gate := make(chan struct{})

	pp, err := NewPassthroughPipe(&gatedReader{gate: gate, data: []byte("buffered output")})
	assert.NilError(t, err)

	// Close while the reader goroutine is still blocked on the gate — the
	// teardown ordering that used to drop output.
	assert.NilError(t, pp.Close())

	// The source now yields its buffered bytes followed by EOF.
	close(gate)

	// waitDrained must block until those bytes land in the buffer.
	pp.waitDrained()

	out, err := io.ReadAll(pp)
	assert.NilError(t, err)
	assert.Check(t, is.Equal(string(out), "buffered output"))
}

func TestPassthroughPipeTimeout(t *testing.T) {
	r, w := io.Pipe()

	passthroughPipe, err := NewPassthroughPipe(r)
	assert.NilError(t, err)

	err = passthroughPipe.SetReadDeadline(time.Now())
	assert.NilError(t, err)

	_, err = w.Write([]byte("a"))
	assert.NilError(t, err)

	p := make([]byte, 1)
	_, err = passthroughPipe.Read(p)
	assert.Assert(t, os.IsTimeout(err))

	err = passthroughPipe.SetReadDeadline(time.Time{})
	assert.NilError(t, err)

	n, err := passthroughPipe.Read(p)
	assert.Equal(t, 1, n)
	assert.NilError(t, err)
}
