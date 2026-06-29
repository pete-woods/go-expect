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

// Cross-platform tests that attach a real subprocess to the Console via
// Console.Start. Unlike the in-process tests these also run on Windows,
// where the ConPTY has no slave file and processes must be spawned attached
// to the pseudo console.

package expect

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

// echoPayload is the output the echo helper process writes before exiting. It
// spans several lines so a dropped read is obvious, and is small enough to fit
// the pty buffer in a single write.
const echoPayload = "line one\nline two\nline three\n"

// TestHelperProcess is not a real test. It is the subprocess driven by the
// tests below, answering the Prompt survey on its stdio.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_EXPECT_HELPER_PROCESS") != "1" {
		return
	}

	if err := Prompt(os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

// TestEchoHelperProcess is not a real test. It writes echoPayload to stdout and
// exits immediately, modelling a command whose entire output is buffered in the
// pty by the time it exits.
func TestEchoHelperProcess(t *testing.T) {
	if os.Getenv("GO_EXPECT_ECHO_HELPER_PROCESS") != "1" {
		return
	}

	fmt.Print(echoPayload)
	os.Exit(0)
}

func echoHelperCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestEchoHelperProcess$")
	cmd.Env = append(os.Environ(), "GO_EXPECT_ECHO_HELPER_PROCESS=1")
	return cmd
}

func helperCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(os.Environ(), "GO_EXPECT_HELPER_PROCESS=1")
	return cmd
}

func waitHelper(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// On a non-zero exit WaitProcess returns an *exec.ExitError on Unix but
	// nil on Windows, so assert on the exit code instead of the error.
	_ = WaitProcess(ctx, cmd)
	if cmd.ProcessState == nil {
		t.Fatal("process state not populated after wait")
	}
	return cmd.ProcessState.ExitCode()
}

func TestSubprocess(t *testing.T) {
	t.Parallel()

	c, err := newTestConsole(t, WithDefaultTimeout(10*time.Second))
	if err != nil {
		t.Fatalf("Expected no error but got '%s'", err)
	}
	defer testCloser(t, c)

	cmd := helperCommand(t)
	if err := c.Start(cmd); err != nil {
		t.Fatalf("Expected no error but got '%s'", err)
	}

	_, err = c.ExpectString("What is 1+1?")
	assert.Check(t, err)
	_, err = c.SendLine("2")
	assert.Check(t, err)
	_, err = c.ExpectString("What is Netflix backwards?")
	assert.Check(t, err)
	_, err = c.SendLine("xilfteN")
	assert.Check(t, err)

	if code := waitHelper(t, cmd); code != 0 {
		t.Errorf("Expected exit code 0 but got %d", code)
	}

	// The Console holds the pseudo-terminal open, so the subprocess exiting
	// does not produce an EOF; close the Console to unblock ExpectEOF.
	testCloser(t, c)
	_, err = c.ExpectEOF()
	assert.Check(t, err)
}

func TestSubprocessWrongAnswer(t *testing.T) {
	t.Parallel()

	c, err := newTestConsole(t, WithDefaultTimeout(10*time.Second))
	if err != nil {
		t.Fatalf("Expected no error but got '%s'", err)
	}
	defer testCloser(t, c)

	cmd := helperCommand(t)
	if err := c.Start(cmd); err != nil {
		t.Fatalf("Expected no error but got '%s'", err)
	}

	_, err = c.ExpectString("What is 1+1?")
	assert.Check(t, err)
	_, err = c.SendLine("3")
	assert.Check(t, err)

	if code := waitHelper(t, cmd); code != 1 {
		t.Errorf("Expected exit code 1 but got %d", code)
	}

	testCloser(t, c)
	_, err = c.ExpectEOF()
	assert.Check(t, err)
}

// TestSubprocessCloseDrainsBufferedOutput reproduces the close/drain race that
// intermittently produced empty captured output under load: a command writes
// its output, exits, and only then is the Console closed and drained. Closing
// the pty master before that output is read would discard it; this test asserts
// every byte survives. It runs many iterations because the failure is a
// goroutine-scheduling race that a single run can mask.
//
// The race and its fix are specific to the Unix pty master/slave pair. The
// Windows ConPTY has no slave file and a different teardown path (covered by
// TestSubprocess), where a post-close read can surface ERROR_INVALID_FUNCTION
// rather than a recognised EOF; this stress test does not target that.
func TestSubprocessCloseDrainsBufferedOutput(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("targets the Unix pty master/slave drain race; ConPTY teardown differs")
	}

	for i := range 64 {
		t.Run(fmt.Sprintf("iteration-%d", i), func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			c, err := newTestConsole(t, WithStdout(&stdout))
			assert.NilError(t, err)

			cmd := echoHelperCommand(t)
			assert.NilError(t, c.Start(cmd))
			assert.Equal(t, waitHelper(t, cmd), 0)

			// Mirror the consumer pattern: the process has exited, so close
			// the Console to unblock the drain, then read everything it wrote.
			testCloser(t, c)
			out, err := c.ExpectEOF()
			assert.NilError(t, err)

			// The pty line discipline translates \n to \r\n; strip the
			// carriage returns before comparing against the payload.
			gotEOF := strings.ReplaceAll(out, "\r\n", "\n")
			gotStdout := strings.ReplaceAll(stdout.String(), "\r\n", "\n")
			assert.Check(t, is.Contains(gotEOF, echoPayload))
			assert.Check(t, is.Contains(gotStdout, echoPayload))
		})
	}
}

func TestSubprocessReadTimeout(t *testing.T) {
	t.Parallel()

	c, err := NewTestConsole(t)
	if err != nil {
		t.Fatalf("Expected no error but got '%s'", err)
	}
	defer testCloser(t, c)

	_, err = c.Expect(String("this will never be printed"), WithTimeout(100*time.Millisecond))
	if err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		t.Errorf("Expected error to contain 'i/o timeout' but got '%s' instead", err)
	}
}
