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

package expect

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/xpty"
)

// Console is an interface to automate input and output for interactive
// applications. Console can block until a specified output is received and send
// input back on it's tty. Console can also multiplex other sources of input
// and multiplex its output to other writers.
type Console struct {
	opts            ConsoleOpts
	pty             xpty.Pty
	passthroughPipe *PassthroughPipe
	runeReader      *bufio.Reader
	closers         []io.Closer
}

// ConsoleOpt allows setting Console options.
type ConsoleOpt func(*ConsoleOpts) error

// ConsoleOpts provides additional options on creating a Console.
type ConsoleOpts struct {
	Logger          *log.Logger
	Stdins          []io.Reader
	Stdouts         []io.Writer
	Closers         []io.Closer
	ExpectObservers []Observer
	SendObservers   []SendObserver
	ReadTimeout     *time.Duration
	TermWidth       int
	TermHeight      int
}

// Observer provides an interface for a function callback that will
// be called after each Expect operation.
// matchers will be the list of active matchers when an error occurred, or a
// list of matchers that matched `buf` when err is nil.
// buf is the captured output that was matched against.
// err is error that might have occurred. May be nil.
type Observer func(matchers []Matcher, buf string, err error)

// SendObserver provides an interface for a function callback that will
// be called after each Send operation.
// msg is the string that was sent.
// num is the number of bytes actually sent.
// err is the error that might have occurred.  May be nil.
type SendObserver func(msg string, num int, err error)

// WithStdout adds writers that Console duplicates writes to, similar to the
// Unix tee(1) command.
//
// Each write is written to each listed writer, one at a time. Console is the
// last writer, writing to it's internal buffer for matching expects.
// If a listed writer returns an error, that overall write operation stops and
// returns the error; it does not continue down the list.
func WithStdout(writers ...io.Writer) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.Stdouts = append(opts.Stdouts, writers...)
		return nil
	}
}

// WithStdin adds readers that bytes read are written to Console's  tty. If a
// listed reader returns an error, that reader will not be continued to read.
func WithStdin(readers ...io.Reader) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.Stdins = append(opts.Stdins, readers...)
		return nil
	}
}

// WithCloser adds closers that are closed in order when Console is closed.
func WithCloser(closer ...io.Closer) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.Closers = append(opts.Closers, closer...)
		return nil
	}
}

// WithLogger adds a logger for Console to log debugging information to. By
// default Console will discard logs.
func WithLogger(logger *log.Logger) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.Logger = logger
		return nil
	}
}

// WithExpectObserver adds an Observer to allow monitoring Expect operations.
func WithExpectObserver(observers ...Observer) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.ExpectObservers = append(opts.ExpectObservers, observers...)
		return nil
	}
}

// WithSendObserver adds a SendObserver to allow monitoring Send operations.
func WithSendObserver(observers ...SendObserver) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.SendObservers = append(opts.SendObservers, observers...)
		return nil
	}
}

// WithDefaultTimeout sets a default read timeout during Expect statements.
func WithDefaultTimeout(timeout time.Duration) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.ReadTimeout = &timeout
		return nil
	}
}

// WithTermSize sets the width and height of the Console's terminal. The
// default is 80x24.
func WithTermSize(width, height int) ConsoleOpt {
	return func(opts *ConsoleOpts) error {
		opts.TermWidth = width
		opts.TermHeight = height
		return nil
	}
}

// openPty opens a new pseudo-terminal, retrying transient allocation
// failures: on macOS, concurrent opens of /dev/ptmx can spuriously fail with
// a bogus negative errno even when plenty of ptys are available. An
// immediate retry succeeds.
func openPty(width, height int) (xpty.Pty, error) {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		var pty xpty.Pty
		pty, err = xpty.NewPty(width, height)
		if err == nil {
			return pty, nil
		}
		var errno syscall.Errno
		if !errors.As(err, &errno) || int(errno) >= 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	return nil, err
}

// NewConsole returns a new Console with the given options.
func NewConsole(opts ...ConsoleOpt) (*Console, error) {
	options := ConsoleOpts{
		Logger:     log.New(io.Discard, "", 0),
		TermWidth:  80,
		TermHeight: 24,
	}

	for _, opt := range opts {
		if err := opt(&options); err != nil {
			return nil, err
		}
	}

	pty, err := openPty(options.TermWidth, options.TermHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to open pseudo-terminal: %w", err)
	}
	closers := make([]io.Closer, 0, len(options.Closers)+2)
	closers = append(closers, options.Closers...)
	closers = append(closers, pty)

	passthroughPipe, err := NewPassthroughPipe(pty)
	if err != nil {
		return nil, err
	}
	closers = append(closers, passthroughPipe)

	c := &Console{
		opts:            options,
		pty:             pty,
		passthroughPipe: passthroughPipe,
		runeReader:      bufio.NewReaderSize(passthroughPipe, utf8.UTFMax),
		closers:         closers,
	}

	for _, stdin := range options.Stdins {
		go func(stdin io.Reader) {
			_, err := io.Copy(c, stdin)
			if err != nil {
				c.Logf("failed to copy stdin: %s", err)
			}
		}(stdin)
	}

	return c, nil
}

// Pty returns the underlying cross-platform pseudo-terminal. On Unix this is
// a *xpty.UnixPty wrapping a classic pty pair, on Windows a *xpty.ConPty
// wrapping a ConPTY pseudo console.
func (c *Console) Pty() xpty.Pty {
	return c.pty
}

// WaitProcess waits for a process started with Console.Start to exit. This
// exists because on Windows cmd.Wait() does not work with processes spawned
// on a ConPTY. On other platforms it simply calls cmd.Wait().
func WaitProcess(ctx context.Context, cmd *exec.Cmd) error {
	return xpty.WaitProcess(ctx, cmd)
}

// Read reads bytes b from Console's tty.
func (c *Console) Read(b []byte) (int, error) {
	return c.pty.Read(b)
}

// Write writes bytes b to Console's tty.
func (c *Console) Write(b []byte) (int, error) {
	c.Logf("console write: %q", b)
	return c.pty.Write(b)
}

// Fd returns Console's file descriptor. On Unix this is the master part of
// its pty, on Windows the ConPTY handle.
func (c *Console) Fd() uintptr {
	return c.pty.Fd()
}

// Close closes Console's tty. Calling Close will unblock Expect and ExpectEOF.
//
// On Unix the data source's output may still be buffered in the pty when Close
// is called. Closing the master end first — as the generic closer loop does —
// discards that buffer, so output captured by a subsequent ExpectEOF is
// intermittently empty under load. To avoid this we close the slave end first:
// with no slave fd open anywhere a final master read returns the buffered bytes
// followed by EOF. We then wait for the passthrough reader to reach that EOF,
// so every byte lands in the pipe's buffer, before closing the master and the
// remaining closers.
func (c *Console) Close() error {
	if unixPty, ok := c.pty.(*xpty.UnixPty); ok {
		_ = unixPty.Slave().Close()
		c.passthroughPipe.waitDrained()
	}

	for _, fd := range c.closers {
		err := fd.Close()
		if err != nil {
			c.Logf("failed to close: %s", err)
		}
	}
	return nil
}

// Send writes string s to Console's tty.
func (c *Console) Send(s string) (int, error) {
	c.Logf("console send: %q", s)
	n, err := c.pty.Write([]byte(s))
	for _, observer := range c.opts.SendObservers {
		observer(s, n, err)
	}
	return n, err
}

// SendLine writes string s to Console's tty with a trailing line separator:
// "\n" on Unix, and "\r" on Windows where the ConPTY line discipline treats
// carriage return as enter.
func (c *Console) SendLine(s string) (int, error) {
	return c.Send(s + lineSeparator)
}

// Log prints to Console's logger.
// Arguments are handled in the manner of fmt.Print.
func (c *Console) Log(v ...interface{}) {
	c.opts.Logger.Print(v...)
}

// Logf prints to Console's logger.
// Arguments are handled in the manner of fmt.Printf.
func (c *Console) Logf(format string, v ...interface{}) {
	c.opts.Logger.Printf(format, v...)
}
