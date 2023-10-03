/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package consoleutil

import (
	"errors"
	"fmt"
	"io"

	"github.com/containerd/log"
	"github.com/moby/term"
)

const DefaultDetachKeys = "ctrl-p,ctrl-q"

type detachableStdin struct {
	stdin  io.Reader
	closer func()
}

// NewDetachableStdin returns an io.Reader that
// uses a TTY proxy reader to read from stdin and detect when the specified detach keys are read,
// in which case closer will be called.
func NewDetachableStdin(stdin io.Reader, keys string, closer func()) (io.Reader, error) {
	if len(keys) == 0 {
		keys = DefaultDetachKeys
	}
	b, err := term.ToBytes(keys)
	if err != nil {
		return nil, fmt.Errorf("failed to convert the detach keys to bytes: %w", err)
	}
	return &detachableStdin{
		stdin:  term.NewEscapeProxy(stdin, b),
		closer: closer,
	}, nil
}

func (ds *detachableStdin) Read(p []byte) (int, error) {
	n, err := ds.stdin.Read(p)
	var eerr term.EscapeError
	if errors.As(err, &eerr) {
		log.L.Info("read detach keys")
		if ds.closer != nil {
			ds.closer()
		}
		return n, io.EOF
	}
	return n, err
}
