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
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

// Opt allows settings Expect options.
type Opt func(*Opts) error

// WithTimeout sets a read timeout for an Expect statement.
func WithTimeout(timeout time.Duration) Opt {
	return func(opts *Opts) error {
		opts.ReadTimeout = &timeout
		return nil
	}
}

// ConsoleCallback is a callback function to execute if a match is found for
// the chained matcher.
type ConsoleCallback func(buf *bytes.Buffer) error

// Then returns an Expect condition to execute a callback if a match is found
// for the chained matcher.
func (eo Opt) Then(f ConsoleCallback) Opt {
	return func(opts *Opts) error {
		var options Opts
		err := eo(&options)
		if err != nil {
			return err
		}

		for _, matcher := range options.Matchers {
			opts.Matchers = append(opts.Matchers, &callbackMatcher{
				f:       f,
				matcher: matcher,
			})
		}
		return nil
	}
}

// Opts provides additional options on Expect.
type Opts struct {
	Matchers    []Matcher
	ReadTimeout *time.Duration
}

// Match sequentially calls Match on all matchers in Opts and returns the
// first matcher if a match exists, otherwise nil.
func (eo Opts) Match(v interface{}) Matcher {
	for _, matcher := range eo.Matchers {
		if matcher.Match(v) {
			return matcher
		}
	}
	return nil
}

// CallbackMatcher is a matcher that provides a Callback function.
type CallbackMatcher interface {
	// Callback executes the matcher's callback with the content buffer at the
	// time of match.
	Callback(buf *bytes.Buffer) error
}

// Matcher provides an interface for finding a match in content read from
// Console's tty.
type Matcher interface {
	// Match returns true iff a match is found.
	Match(v interface{}) bool
	Criteria() interface{}
}

// callbackMatcher fulfills the Matcher and CallbackMatcher interface to match
// using its embedded matcher and provide a callback function.
type callbackMatcher struct {
	f       ConsoleCallback
	matcher Matcher
}

func (cm *callbackMatcher) Match(v interface{}) bool {
	return cm.matcher.Match(v)
}

func (cm *callbackMatcher) Criteria() interface{} {
	return cm.matcher.Criteria()
}

func (cm *callbackMatcher) Callback(buf *bytes.Buffer) error {
	cb, ok := cm.matcher.(CallbackMatcher)
	if ok {
		err := cb.Callback(buf)
		if err != nil {
			return err
		}
	}
	err := cm.f(buf)
	if err != nil {
		return err
	}
	return nil
}

// errorMatcher fulfills the Matcher interface to match a specific error.
type errorMatcher struct {
	err error
}

func (em *errorMatcher) Match(v interface{}) bool {
	err, ok := v.(error)
	if !ok {
		return false
	}
	return errors.Is(err, em.err)
}

func (em *errorMatcher) Criteria() interface{} {
	return em.err
}

// closedErrMatcher fulfills the Matcher interface to match the
// platform-specific errors returned when reading from a pseudo-terminal that
// has been closed.
type closedErrMatcher struct{}

func (cm *closedErrMatcher) Match(v interface{}) bool {
	err, ok := v.(error)
	if !ok {
		return false
	}
	return isClosedErr(err)
}

func (cm *closedErrMatcher) Criteria() interface{} {
	return "pseudo-terminal closed"
}

// stringMatcher fulfills the Matcher interface to match strings against a given
// bytes.Buffer.
type stringMatcher struct {
	str string
}

func (sm *stringMatcher) Match(v interface{}) bool {
	buf, ok := v.(*bytes.Buffer)
	if !ok {
		return false
	}
	if strings.Contains(buf.String(), sm.str) {
		return true
	}
	return false
}

func (sm *stringMatcher) Criteria() interface{} {
	return sm.str
}

// regexpMatcher fulfills the Matcher interface to match Regexp against a given
// bytes.Buffer.
type regexpMatcher struct {
	re *regexp.Regexp
}

func (rm *regexpMatcher) Match(v interface{}) bool {
	buf, ok := v.(*bytes.Buffer)
	if !ok {
		return false
	}
	return rm.re.Match(buf.Bytes())
}

func (rm *regexpMatcher) Criteria() interface{} {
	return rm.re
}

// allMatcher fulfills the Matcher interface to match a group of Opt
// against any value.
type allMatcher struct {
	options Opts
}

func (am *allMatcher) Match(v interface{}) bool {
	var matchers []Matcher
	for _, matcher := range am.options.Matchers {
		if matcher.Match(v) {
			continue
		}
		matchers = append(matchers, matcher)
	}

	am.options.Matchers = matchers
	return len(matchers) == 0
}

func (am *allMatcher) Criteria() interface{} {
	criteria := make([]interface{}, 0, len(am.options.Matchers))
	for _, matcher := range am.options.Matchers {
		criteria = append(criteria, matcher.Criteria())
	}
	return criteria
}

// All adds an Expect condition to exit if the content read from Console's tty
// matches all of the provided Opt, in any order.
func All(expectOpts ...Opt) Opt {
	return func(opts *Opts) error {
		var options Opts
		for _, opt := range expectOpts {
			if err := opt(&options); err != nil {
				return err
			}
		}

		opts.Matchers = append(opts.Matchers, &allMatcher{
			options: options,
		})
		return nil
	}
}

// String adds an Expect condition to exit if the content read from Console's
// tty contains any of the given strings.
func String(strs ...string) Opt {
	return func(opts *Opts) error {
		for _, str := range strs {
			opts.Matchers = append(opts.Matchers, &stringMatcher{
				str: str,
			})
		}
		return nil
	}
}

// Regexp adds an Expect condition to exit if the content read from Console's
// tty matches the given Regexp.
func Regexp(res ...*regexp.Regexp) Opt {
	return func(opts *Opts) error {
		for _, re := range res {
			opts.Matchers = append(opts.Matchers, &regexpMatcher{
				re: re,
			})
		}
		return nil
	}
}

// RegexpPattern adds an Expect condition to exit if the content read from
// Console's tty matches the given Regexp patterns. Expect returns an error if
// the patterns were unsuccessful in compiling the Regexp.
func RegexpPattern(ps ...string) Opt {
	return func(opts *Opts) error {
		var res []*regexp.Regexp
		for _, p := range ps {
			re, err := regexp.Compile(p)
			if err != nil {
				return err
			}
			res = append(res, re)
		}
		return Regexp(res...)(opts)
	}
}

// Error adds an Expect condition to exit if reading from Console's tty returns
// one of the provided errors.
func Error(errs ...error) Opt {
	return func(opts *Opts) error {
		for _, err := range errs {
			opts.Matchers = append(opts.Matchers, &errorMatcher{
				err: err,
			})
		}
		return nil
	}
}

// EOF adds an Expect condition to exit if io.EOF is returned from reading
// Console's tty.
func EOF(opts *Opts) error {
	return Error(io.EOF)(opts)
}

// PTSClosed adds an Expect condition to exit if the pseudo-terminal was
// closed while reading. The errors that signal this differ by platform: on
// Linux reading from the ptm after the pts is closed returns EIO (see
// https://github.com/kr/pty/issues/21#issuecomment-129381749), on Windows a
// closed ConPTY surfaces as a broken pipe, and on all platforms a closed
// Console returns a closed-file error.
func PTSClosed(opts *Opts) error {
	opts.Matchers = append(opts.Matchers, &closedErrMatcher{})
	return nil
}
