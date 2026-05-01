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
