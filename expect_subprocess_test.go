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
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

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

	c.ExpectString("What is 1+1?")
	c.SendLine("2")
	c.ExpectString("What is Netflix backwards?")
	c.SendLine("xilfteN")

	if code := waitHelper(t, cmd); code != 0 {
		t.Errorf("Expected exit code 0 but got %d", code)
	}

	// The Console holds the pseudo-terminal open, so the subprocess exiting
	// does not produce an EOF; close the Console to unblock ExpectEOF.
	testCloser(t, c)
	c.ExpectEOF()
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

	c.ExpectString("What is 1+1?")
	c.SendLine("3")

	if code := waitHelper(t, cmd); code != 1 {
		t.Errorf("Expected exit code 1 but got %d", code)
	}

	testCloser(t, c)
	c.ExpectEOF()
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
