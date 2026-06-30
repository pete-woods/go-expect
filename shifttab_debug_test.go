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

// TEMPORARY DEBUG TEST. Diagnoses what bytes a TUI-style child actually
// receives when shift+tab (CSI Z, "\x1b[Z") is sent through the Console. The
// motivating question: a bubbletea CLI's shift+tab key binding does not fire
// on Windows under ConPTY. This isolates whether the sequence survives the
// Console -> ConPTY -> child stdin path (in which case the loss is downstream,
// in the child's input decoder) or is dropped/rewritten before the child sees
// it. Delete once answered.

package expect

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
	"gotest.tools/v3/assert"
)

// shiftTabSentinel terminates the helper's read loop. It is a plain byte that
// never appears in the "\x1b[Z" sequence under test.
const shiftTabSentinel = 'q'

// TestShiftTabHelperProcess is not a real test. Driven as a subprocess, it puts
// its stdin into raw mode — which on Windows enables
// ENABLE_VIRTUAL_TERMINAL_INPUT, exactly as a bubbletea/ultraviolet TUI does —
// then reads until the sentinel and reports the exact bytes received, so the
// parent can see what the Console delivered.
func TestShiftTabHelperProcess(t *testing.T) {
	if os.Getenv("GO_EXPECT_SHIFTTAB_HELPER_PROCESS") != "1" {
		return
	}

	if old, err := term.MakeRaw(os.Stdin.Fd()); err == nil {
		defer func() { _ = term.Restore(os.Stdin.Fd(), old) }()
	} else {
		fmt.Printf("MAKERAW_ERR:%v\n", err)
	}

	buf := make([]byte, 256)
	var got []byte
read:
	for {
		n, err := os.Stdin.Read(buf)
		for i := 0; i < n; i++ {
			if buf[i] == shiftTabSentinel {
				break read
			}
			got = append(got, buf[i])
		}
		if err != nil {
			break
		}
	}

	// Sentinels around the payload so the parent can extract it unambiguously
	// from any surrounding terminal output.
	fmt.Printf("RECEIVED<%q>END\n", string(got))
	os.Exit(0)
}

func shiftTabHelperCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestShiftTabHelperProcess$")
	cmd.Env = append(os.Environ(), "GO_EXPECT_SHIFTTAB_HELPER_PROCESS=1")
	return cmd
}

// TestShiftTabDelivery sends shift+tab (CSI Z) followed by the sentinel and
// reports what the child received. The assertion documents the expectation
// (the sequence arrives intact); on a platform where it is dropped or rewritten
// the failure message shows exactly what arrived instead.
func TestShiftTabDelivery(t *testing.T) {
	c, err := newTestConsole(t, WithDefaultTimeout(10*time.Second))
	assert.NilError(t, err)
	defer testCloser(t, c)

	cmd := shiftTabHelperCommand(t)
	assert.NilError(t, c.Start(cmd))

	// "\x1b[Z" is shift+tab; 'q' is the sentinel that ends the helper's read.
	_, err = c.Send("\x1b[Zq")
	assert.NilError(t, err)

	out, err := c.ExpectString("END")
	assert.NilError(t, err)

	received := extractReceived(out)
	t.Logf("[%s] shift+tab delivery — child received: %s", runtime.GOOS, received)

	if code := waitHelper(t, cmd); code != 0 {
		t.Errorf("Expected exit code 0 but got %d", code)
	}
	testCloser(t, c)
	_, _ = c.ExpectEOF()

	// Expectation: the CSI Z sequence reaches the child verbatim. If this fails,
	// the logged "received" value pinpoints where shift+tab is lost.
	assert.Check(t, strings.Contains(received, `\x1b[Z`),
		"shift+tab (\\x1b[Z) did not reach the child intact; got %s", received)
}

// extractReceived pulls the payload the helper printed between its RECEIVED<…>
// markers, falling back to the raw output if the markers are absent.
func extractReceived(out string) string {
	start := strings.Index(out, "RECEIVED<")
	if start < 0 {
		return fmt.Sprintf("%q", out)
	}
	rest := out[start+len("RECEIVED<"):]
	end := strings.Index(rest, ">END")
	if end < 0 {
		return fmt.Sprintf("%q", out)
	}
	return rest[:end]
}
