// Copyright 2018 Netflix, Inc.
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

// These tests drive the Console in-process through its slave file (Tty),
// which only exists on Unix. See expect_subprocess_test.go for the
// cross-platform tests that attach a real subprocess.

//go:build !windows

package expect

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestExpectf(t *testing.T) {
	t.Parallel()

	c, err := newTestConsole(t)
	if err != nil {
		t.Errorf("Expected no error but got'%s'", err)
	}
	defer testCloser(t, c)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := c.Expectf("What is 1+%d?", 1)
		assert.Check(t, err)
		_, err = c.SendLine("2")
		assert.Check(t, err)
		_, err = c.Expectf("What is %s backwards?", "Netflix")
		assert.Check(t, err)
		_, err = c.SendLine("xilfteN")
		assert.Check(t, err)
		_, err = c.ExpectEOF()
		assert.Check(t, err)
	}()

	err = Prompt(c.Tty(), c.Tty())
	if err != nil {
		t.Errorf("Expected no error but got '%s'", err)
	}
	testCloser(t, c.Tty())
	wg.Wait()
}

func TestExpect(t *testing.T) {
	t.Parallel()

	c, err := newTestConsole(t)
	if err != nil {
		t.Errorf("Expected no error but got'%s'", err)
	}
	defer testCloser(t, c)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := c.ExpectString("What is 1+1?")
		assert.Check(t, err)
		_, err = c.SendLine("2")
		assert.Check(t, err)
		_, err = c.ExpectString("What is Netflix backwards?")
		assert.Check(t, err)
		_, err = c.SendLine("xilfteN")
		assert.Check(t, err)
		_, err = c.ExpectEOF()
		assert.Check(t, err)
	}()

	err = Prompt(c.Tty(), c.Tty())
	if err != nil {
		t.Errorf("Expected no error but got '%s'", err)
	}
	// close the pts so we can expect EOF
	testCloser(t, c.Tty())
	wg.Wait()
}

func TestExpectOutput(t *testing.T) {
	t.Parallel()

	c, err := newTestConsole(t)
	if err != nil {
		t.Errorf("Expected no error but got'%s'", err)
	}
	defer testCloser(t, c)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := c.ExpectString("What is 1+1?")
		assert.Check(t, err)
		_, err = c.SendLine("3")
		assert.Check(t, err)
		_, err = c.ExpectEOF()
		assert.Check(t, err)
	}()

	err = Prompt(c.Tty(), c.Tty())
	if err == nil || !errors.Is(err, ErrWrongAnswer) {
		t.Errorf("Expected error '%s' but got '%s' instead", ErrWrongAnswer, err)
	}
	testCloser(t, c.Tty())
	wg.Wait()
}

func TestExpectDefaultTimeout(t *testing.T) {
	t.Parallel()

	c, err := NewTestConsole(t, WithDefaultTimeout(0))
	if err != nil {
		t.Errorf("Expected no error but got'%s'", err)
	}
	defer testCloser(t, c)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		Prompt(c.Tty(), c.Tty())
	}()

	_, err = c.ExpectString("What is 1+2?")
	if err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		t.Errorf("Expected error to contain 'i/o timeout' but got '%s' instead", err)
	}

	// Close to unblock Prompt and wait for the goroutine to exit.
	c.Tty().Close()
	wg.Wait()
}

func TestExpectTimeout(t *testing.T) {
	t.Parallel()

	c, err := NewTestConsole(t)
	if err != nil {
		t.Errorf("Expected no error but got'%s'", err)
	}
	defer testCloser(t, c)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		Prompt(c.Tty(), c.Tty())
	}()

	_, err = c.Expect(String("What is 1+2?"), WithTimeout(0))
	if err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		t.Errorf("Expected error to contain 'i/o timeout' but got '%s' instead", err)
	}

	// Close to unblock Prompt and wait for the goroutine to exit.
	c.Tty().Close()
	wg.Wait()
}

func TestExpectDefaultTimeoutOverride(t *testing.T) {
	t.Parallel()

	c, err := newTestConsole(t, WithDefaultTimeout(100*time.Millisecond))
	if err != nil {
		t.Errorf("Expected no error but got'%s'", err)
	}
	defer testCloser(t, c)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use a local err to avoid racing the Expect calls below on the
		// test's err variable.
		err := Prompt(c.Tty(), c.Tty())
		assert.Check(t, err)
		time.Sleep(200 * time.Millisecond)
		c.Tty().Close()
	}()

	_, err = c.ExpectString("What is 1+1?")
	assert.Check(t, err)
	_, err = c.SendLine("2")
	assert.Check(t, err)
	_, err = c.ExpectString("What is Netflix backwards?")
	assert.Check(t, err)
	_, err = c.SendLine("xilfteN")
	assert.Check(t, err)
	_, err = c.Expect(EOF, PTSClosed, WithTimeout(time.Second))
	assert.Check(t, err)

	wg.Wait()
}

func TestConsoleChain(t *testing.T) {
	t.Parallel()

	c1, err := NewConsole(expectNoError(t), sendNoError(t))
	if err != nil {
		t.Errorf("Expected no error but got'%s'", err)
	}
	defer testCloser(t, c1)

	var wg1 sync.WaitGroup
	wg1.Add(1)
	go func() {
		defer wg1.Done()
		_, err := c1.ExpectString("What is Netflix backwards?")
		assert.Check(t, err)
		_, err = c1.SendLine("xilfteN")
		assert.Check(t, err)
		_, err = c1.ExpectEOF()
		assert.Check(t, err)
	}()

	c2, err := newTestConsole(t, WithStdin(c1.Tty()), WithStdout(c1.Tty()))
	if err != nil {
		t.Errorf("Expected no error but got'%s'", err)
	}
	defer testCloser(t, c2)

	var wg2 sync.WaitGroup
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		_, err := c2.ExpectString("What is 1+1?")
		assert.Check(t, err)
		_, err = c2.SendLine("2")
		assert.Check(t, err)
		_, err = c2.ExpectEOF()
		assert.Check(t, err)
	}()

	err = Prompt(c2.Tty(), c2.Tty())
	if err != nil {
		t.Errorf("Expected no error but got '%s'", err)
	}

	testCloser(t, c2.Tty())
	wg2.Wait()

	testCloser(t, c1.Tty())
	wg1.Wait()
}

func TestStartControllingTty(t *testing.T) {
	t.Parallel()

	c, err := newTestConsole(t)
	if err != nil {
		t.Fatalf("Expected no error but got '%s'", err)
	}
	defer testCloser(t, c)

	// Opening /dev/tty only works if Start made the pty the child's
	// controlling terminal.
	cmd := exec.Command("sh", "-c", "echo answer-from-ctty > /dev/tty")
	if err := c.Start(cmd); err != nil {
		t.Fatalf("Expected no error but got '%s'", err)
	}

	_, err = c.ExpectString("answer-from-ctty")
	assert.Check(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := WaitProcess(ctx, cmd); err != nil {
		t.Errorf("Expected no error but got '%s'", err)
	}
}

func TestEditor(t *testing.T) {
	if _, err := exec.LookPath("vi"); err != nil {
		t.Skip("vi not found in PATH")
	}
	t.Parallel()

	c, err := NewConsole(expectNoError(t), sendNoError(t))
	if err != nil {
		t.Errorf("Expected no error but got '%s'", err)
	}
	defer testCloser(t, c)

	file, err := os.CreateTemp("", "")
	if err != nil {
		t.Errorf("Expected no error but got '%s'", err)
	}

	cmd := exec.Command("vi", file.Name())
	cmd.Stdin = c.Tty()
	cmd.Stdout = c.Tty()
	cmd.Stderr = c.Tty()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := c.Send("iHello world\x1b")
		assert.Check(t, err)
		_, err = c.SendLine(":wq!")
		assert.Check(t, err)
		_, err = c.ExpectEOF()
		assert.Check(t, err)
	}()

	err = cmd.Run()
	if err != nil {
		t.Errorf("Expected no error but got '%s'", err)
	}

	testCloser(t, c.Tty())
	wg.Wait()

	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Errorf("Expected no error but got '%s'", err)
	}
	if string(data) != "Hello world\n" {
		t.Errorf("Expected '%s' to equal '%s'", string(data), "Hello world\n")
	}
}

func ExampleConsole_echo() {
	c, err := NewConsole(WithStdout(os.Stdout))
	if err != nil {
		log.Fatal(err)
	}

	cmd := exec.Command("echo")
	cmd.Stdin = c.Tty()
	cmd.Stdout = c.Tty()
	cmd.Stderr = c.Tty()

	err = cmd.Start()
	if err != nil {
		log.Fatal(err)
	}

	c.Send("Hello world")
	c.ExpectString("Hello world")
	c.Tty().Close()
	c.ExpectEOF()

	err = cmd.Wait()
	if err != nil {
		log.Fatal(err)
	}

	c.Close()

	// Output: Hello world
}
