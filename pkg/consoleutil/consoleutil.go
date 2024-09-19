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
	"context"
	"os"

	"github.com/containerd/console"
)

// Current is from https://github.com/containerd/console/blob/v1.0.4/console.go#L68-L81
// adapted so that it does not panic
func Current() (c console.Console, err error) {
	for _, s := range []*os.File{os.Stderr, os.Stdout, os.Stdin} {
		if c, err = console.ConsoleFromFile(s); err == nil {
			return c, nil
		}
	}
	return nil, console.ErrNotAConsole
}

// resizer is from https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/tasks/tasks.go#L25-L27
type resizer interface {
	Resize(ctx context.Context, w, h uint32) error
}
