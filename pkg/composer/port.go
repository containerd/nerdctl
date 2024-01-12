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
	"io"

	"github.com/containerd/nerdctl/v2/pkg/containerutil"
)

// PortOptions has args for getting the public port of a given private port/protocol
// in a service container.
type PortOptions struct {
	ServiceName string
	Index       int
	Port        int
	Protocol    string
}

// Port gets the corresponding public port of a given private port/protocol
// on a service container.
func (c *Composer) Port(ctx context.Context, writer io.Writer, po PortOptions) error {
	containers, err := c.Containers(ctx, po.ServiceName)
	if err != nil {
		return fmt.Errorf("fail to get containers for service %s: %s", po.ServiceName, err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("no running containers from service %s", po.ServiceName)
	}
	if po.Index > len(containers) {
		return fmt.Errorf("index (%d) out of range: only %d running instances from service %s",
			po.Index, len(containers), po.ServiceName)
	}
	container := containers[po.Index-1]

	return containerutil.PrintHostPort(ctx, writer, container, po.Port, po.Protocol)
}
