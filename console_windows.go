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

import "os/exec"

// ConPTY's line discipline treats carriage return as enter; a bare line feed
// does not terminate cooked-mode reads.
const lineSeparator = "\r"

// Start starts cmd attached to the Console's ConPTY pseudo console. The
// process is spawned with a pseudo console proc-thread attribute, which binds
// its console (and so its standard handles) to the Console — the ConPTY
// equivalent of making the pty the child's controlling terminal. Any
// cmd.Stdin/Stdout/Stderr assignments are ignored on Windows.
//
// cmd.Wait does not work for processes started this way; use WaitProcess
// instead.
func (c *Console) Start(cmd *exec.Cmd) error {
	return c.pty.Start(cmd)
}
