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
	"os"
	"os/exec"
	"syscall"

	"github.com/charmbracelet/x/xpty"
)

const lineSeparator = "\n"

// Start starts cmd attached to the Console's pseudo-terminal, with its
// standard input, output, and error connected to the terminal unless already
// set. Mirroring creack/pty's StartWithSize, the command is run in a new
// session with the pty as its controlling terminal, so that programs opening
// /dev/tty and job-control signals work.
//
// Wait for commands started this way with WaitProcess, which is also safe on
// Windows where cmd.Wait does not work with a ConPTY.
func (c *Console) Start(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
	cmd.SysProcAttr.Setctty = true
	return c.pty.Start(cmd)
}

// Tty returns Console's pts (slave part of a pty). A pseudoterminal, or pty is
// a pair of psuedo-devices, one of which, the slave, emulates a real text
// terminal device.
//
// Tty is only available on Unix; Windows ConPTY has no slave file. Use
// Console.Start to attach a command to the Console on all platforms.
func (c *Console) Tty() *os.File {
	return c.pty.(*xpty.UnixPty).Slave()
}
