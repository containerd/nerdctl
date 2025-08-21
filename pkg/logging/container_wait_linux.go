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

package logging

import (
	"context"
	"net"
	"strings"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v3"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/runtime/v2"
	"github.com/containerd/containerd/v2/core/runtime/v2/logging"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/shim"
	"github.com/containerd/ttrpc"
)

// Connect shim directly to avoid to connect containerd.
func waitContainerExited(ctx context.Context, address string, config *logging.Config, _ containerd.Task) (<-chan containerd.ExitStatus, error) {
	ctx = namespaces.WithNamespace(ctx, config.Namespace)
	shimCli, err := connectToShim(ctx, strings.TrimPrefix(address, "unix://"), 3, config.ID)
	if err != nil {
		return nil, err
	}
	c := make(chan containerd.ExitStatus, 1)
	go func() {
		defer close(c)
		response, err := shimCli.Wait(ctx, &task.WaitRequest{
			ID: config.ID,
		})

		if err != nil {
			c <- *containerd.NewExitStatus(containerd.UnknownExitStatus, time.Time{}, err)
			return
		}
		c <- *containerd.NewExitStatus(response.ExitStatus, response.ExitedAt.AsTime(), nil)
	}()
	return c, nil
}

func connectToShim(ctx context.Context, ctrdEndpoint string, version int, id string) (v2.TaskServiceClient, error) {
	addr, err := shim.SocketAddress(ctx, ctrdEndpoint, id, false)
	if err != nil {
		return nil, err
	}
	addr = strings.TrimPrefix(addr, "unix://")
	conn, err := net.Dial("unix", addr)
	if err != nil {
		return nil, err
	}

	client := ttrpc.NewClient(conn)
	cli, err := v2.NewTaskClient(client, version)
	if err != nil {
		return nil, err
	}
	return cli, nil
}
