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

//go:build !windows

package expect

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// isClosedErr reports whether err indicates that the pseudo-terminal was
// closed. On Linux reading from the ptm after the pts is closed returns EIO.
// Reads after the Console itself is closed return os.ErrClosed (from the
// master file) or io.ErrClosedPipe (from the PassthroughPipe).
func isClosedErr(err error) bool {
	return errors.Is(err, syscall.EIO) ||
		errors.Is(err, os.ErrClosed) ||
		errors.Is(err, io.ErrClosedPipe)
}
