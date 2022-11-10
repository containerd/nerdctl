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

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/strutil"

	"github.com/sirupsen/logrus"
)

// StopOptions stores all option input from `nerdctl compose stop`
type StopOptions struct {
	TimeChanged bool
	Timeout     uint
}

// Stop stops containers in `services` without removing them. It calls
// `nerdctl stop CONTAINER_ID` to do the actual job.
func (c *Composer) Stop(ctx context.Context, opt StopOptions, services []string) error {
	serviceNames, err := c.ServiceNames(services...)
	if err != nil {
		return err
	}
	// reverse dependency order
	for _, svc := range strutil.ReverseStrSlice(serviceNames) {
		containers, err := c.Containers(ctx, svc)
		if err != nil {
			return err
		}
		if err := c.stopContainers(ctx, containers, opt); err != nil {
			return err
		}
	}
	return nil
}

func (c *Composer) stopContainers(ctx context.Context, containers []containerd.Container, opt StopOptions) error {
	var timeoutArg string
	if opt.TimeChanged {
		timeoutArg = fmt.Sprintf("--timeout=%d", opt.Timeout)
	}

	for _, container := range containers {
		info, _ := container.Info(ctx, containerd.WithoutRefreshedMetadata)
		logrus.Infof("Stopping container %s", info.Labels[labels.Name])
		args := []string{"stop"}
		if opt.TimeChanged {
			args = append(args, timeoutArg)
		}
		args = append(args, container.ID())
		if err := c.runNerdctlCmd(ctx, args...); err != nil {
			logrus.Warn(err)
		}
	}

	return nil
}
