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
//
// SPDX-License-Identifier: Apache-2.0

package expect

import (
	"context"
	"io"
	"sync"
	"testing"

	"gotest.tools/v3/assert"
)

func TestReaderLease(t *testing.T) {
	in, out := io.Pipe()
	defer out.Close()
	defer in.Close()

	rm := NewReaderLease(in)

	tests := []struct {
		title    string
		expected string
	}{
		{
			"Read cancels with deadline",
			"apple",
		},
		{
			"Second read has no bytes stolen",
			"banana",
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			tin, tout := io.Pipe()

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				io.Copy(tout, rm.NewReader(ctx))
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := out.Write([]byte(test.expected))
				assert.NilError(t, err)
			}()

			for i := 0; i < len(test.expected); i++ {
				p := make([]byte, 1)
				n, err := tin.Read(p)
				assert.NilError(t, err)
				assert.Equal(t, 1, n)
				assert.Equal(t, test.expected[i], p[0])
			}

			cancel()
			wg.Wait()
		})
	}
}
