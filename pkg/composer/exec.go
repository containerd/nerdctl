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
	"sort"
	"strconv"
	"strings"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

// ExecOptions stores options passed from users as flags and args.
type ExecOptions struct {
	ServiceName string
	Index       int
	// params to be passed to `nerdctl exec`
	Detach      bool
	Interactive bool
	Tty         bool
	Privileged  bool
	User        string
	WorkDir     string
	Env         []string
	Args        []string
}

// Exec executes a given command on a running container specified by
// `ServiceName` (and `Index` if it has multiple instances).
func (c *Composer) Exec(ctx context.Context, eo ExecOptions) error {
	// Exec does not need to lock and should allow concurrency.
	if err := Unlock(); err != nil {
		return err
	}

	container, err := c.serviceContainer(ctx, eo.ServiceName, eo.Index)
	if err != nil {
		return err
	}
	return c.exec(ctx, container, eo)
}

// serviceContainer returns the container of the given service at the given
// index, starting at 1, for services that have multiple instances.
func (c *Composer) serviceContainer(ctx context.Context, serviceName string, index int) (containerd.Container, error) {
	containers, err := c.Containers(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("fail to get containers for service %s: %w", serviceName, err)
	}
	if len(containers) == 0 {
		return nil, fmt.Errorf("no running containers from service %s", serviceName)
	}
	if index > len(containers) {
		return nil, fmt.Errorf("index (%d) out of range: only %d running instances from service %s",
			index, len(containers), serviceName)
	}
	if len(containers) == 1 {
		return containers[0], nil
	}
	// The order of the containers is not consistently ascending
	// we need to re-sort them.
	sort.SliceStable(containers, func(i, j int) bool {
		infoI, _ := containers[i].Info(ctx, containerd.WithoutRefreshedMetadata)
		infoJ, _ := containers[j].Info(ctx, containerd.WithoutRefreshedMetadata)
		segsI := strings.Split(infoI.Labels[labels.Name], serviceparser.Separator)
		segsJ := strings.Split(infoJ.Labels[labels.Name], serviceparser.Separator)
		indexI, _ := strconv.Atoi(segsI[len(segsI)-1])
		indexJ, _ := strconv.Atoi(segsJ[len(segsJ)-1])
		return indexI < indexJ
	})
	return containers[index-1], nil
}

// exec constructs/executes the `nerdctl exec` command to be executed on the given container.
func (c *Composer) exec(ctx context.Context, container containerd.Container, eo ExecOptions) error {
	args := []string{
		"exec",
		fmt.Sprintf("--detach=%t", eo.Detach),
		fmt.Sprintf("--interactive=%t", eo.Interactive),
		fmt.Sprintf("--tty=%t", eo.Tty),
		fmt.Sprintf("--privileged=%t", eo.Privileged),
	}
	if eo.User != "" {
		args = append(args, "--user", eo.User)
	}
	if eo.WorkDir != "" {
		args = append(args, "--workdir", eo.WorkDir)
	}
	for _, e := range eo.Env {
		args = append(args, "--env", e)
	}
	args = append(args, container.ID())
	args = append(args, eo.Args...)
	cmd := c.createNerdctlCmd(ctx, args...)

	if eo.Interactive {
		cmd.Stdin = os.Stdin
	}
	if !eo.Detach {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if c.DebugPrintFull {
		log.G(ctx).Debugf("Executing %v", cmd.Args)
	}
	return cmd.Run()
}
