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

package composer

import (
	"context"
	"fmt"
	"os"

	"github.com/containerd/log"
)

// AttachOptions stores options passed from users as flags and args.
type AttachOptions struct {
	ServiceName string
	Index       int
	// params to be passed to `nerdctl attach`
	DetachKeys string
	NoStdin    bool
}

// Attach attaches the standard input, output and error streams of the
// container specified by `ServiceName` (and `Index` if it has multiple
// instances) to the terminal.
func (c *Composer) Attach(ctx context.Context, ao AttachOptions) error {
	// Attach does not need to lock and should allow concurrency.
	if err := Unlock(); err != nil {
		return err
	}

	container, err := c.serviceContainer(ctx, ao.ServiceName, ao.Index)
	if err != nil {
		return err
	}

	args := []string{
		"attach",
		fmt.Sprintf("--no-stdin=%t", ao.NoStdin),
	}
	if ao.DetachKeys != "" {
		args = append(args, "--detach-keys", ao.DetachKeys)
	}
	args = append(args, container.ID())
	cmd := c.createNerdctlCmd(ctx, args...)
	if !ao.NoStdin {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if c.DebugPrintFull {
		log.G(ctx).Debugf("Executing %v", cmd.Args)
	}
	return cmd.Run()
}
