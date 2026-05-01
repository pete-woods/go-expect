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
