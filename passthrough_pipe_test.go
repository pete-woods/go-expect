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
