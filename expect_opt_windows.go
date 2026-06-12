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

//go:build windows

package expect

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// isClosedErr reports whether err indicates that the pseudo-terminal was
// closed. Closing a ConPTY terminates its conhost, so a pending read on the
// output pipe fails with a broken-pipe class error. Reads after the Console
// itself is closed return os.ErrClosed or io.ErrClosedPipe.
func isClosedErr(err error) bool {
	return errors.Is(err, windows.ERROR_BROKEN_PIPE) ||
		errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED) ||
		errors.Is(err, windows.ERROR_OPERATION_ABORTED) ||
		errors.Is(err, windows.ERROR_INVALID_HANDLE) ||
		errors.Is(err, windows.ERROR_HANDLE_EOF) ||
		errors.Is(err, os.ErrClosed) ||
		errors.Is(err, io.ErrClosedPipe)
}
