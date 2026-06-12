# go-expect

![Go](https://github.com/pete-woods/go-expect/workflows/Go/badge.svg)
[![GoDoc](https://godoc.org/github.com/pete-woods/go-expect?status.svg)](https://godoc.org/github.com/pete-woods/go-expect)
[![OSS Lifecycle](https://img.shields.io/osslifecycle/pete-woods/go-expect.svg)]()

Package expect provides an expect-like interface to automate control of interactive applications through a pseudo-terminal.

It is cross-platform: on Unix the console is backed by a classic pty pair, on Windows by a [ConPTY](https://learn.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session) pseudo console, via [github.com/charmbracelet/x/xpty](https://github.com/charmbracelet/x/tree/main/xpty).

## Usage

### Cross-platform `exec.Cmd` example

```go
package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"time"

	expect "github.com/pete-woods/go-expect"
)

func main() {
	c, err := expect.NewConsole(
		expect.WithStdout(os.Stdout),
		expect.WithDefaultTimeout(10*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// Start attaches the command to the console's pseudo-terminal on all
	// platforms. (On Unix you can alternatively assign c.Tty() to the
	// command's Stdin/Stdout/Stderr yourself.)
	cmd := exec.Command("vi")
	if err = c.Start(cmd); err != nil {
		log.Fatal(err)
	}

	time.Sleep(time.Second)
	c.Send("iHello world\x1b")
	time.Sleep(time.Second)
	c.SendLine(":q!")

	// On Windows cmd.Wait does not work for processes attached to a ConPTY,
	// so use expect.WaitProcess instead.
	if err = expect.WaitProcess(context.Background(), cmd); err != nil {
		log.Fatal(err)
	}

	// The console holds the pseudo-terminal open, so the process exiting
	// does not produce an EOF by itself. Close the console to unblock
	// ExpectEOF once you are done.
	c.Close()
	c.ExpectEOF()
}
```

### Expecting output

```go
c.ExpectString("What is 1+1?")
c.SendLine("2")
c.ExpectEOF()
```

## Windows notes

- `Console.Tty()` only exists on Unix: ConPTY has no slave file. Use `Console.Start` to attach commands.
- `SendLine` terminates lines with `"\n"` on Unix and `"\r"` on Windows, where the ConPTY treats carriage return as enter.
- Wait for processes with `expect.WaitProcess` rather than `cmd.Wait`.
- ConPTY output contains VT escape sequences (cursor movement, colors) in addition to the raw program output. String matchers still work because the program's text appears literally within the stream, but exact-match assertions on full output will differ from Unix.
