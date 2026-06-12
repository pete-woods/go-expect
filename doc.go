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

// Package expect provides an expect-like interface to automate control of
// interactive applications through a pseudo-terminal. It works
// cross-platform: on Unix the Console is backed by a classic pty pair, on
// Windows by a ConPTY pseudo console (via github.com/charmbracelet/x/xpty).
//
// Attach a command to the Console with Console.Start. On Unix the slave file
// is also available via Console.Tty for in-process use or manual stdio
// assignment; Windows ConPTY has no slave file, so Start is the only way to
// attach a process there, and processes started this way must be waited on
// with WaitProcess instead of exec.Cmd.Wait.
package expect
