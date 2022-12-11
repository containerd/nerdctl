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

package action

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/run"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/netutil/nettype"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

func StartContainer(ctx context.Context, container containerd.Container, flagA bool, client *containerd.Client) error {
	lab, err := container.Labels(ctx)
	if err != nil {
		return err
	}

	if err := reconfigNetContainer(ctx, container, client, lab); err != nil {
		return err
	}

	if err := reconfigPIDContainer(ctx, container, client, lab); err != nil {
		return err
	}

	taskCIO := cio.NullIO

	// Choosing the user selected option over the labels
	if flagA {
		taskCIO = cio.NewCreator(cio.WithStreams(os.Stdin, os.Stdout, os.Stderr))
	}
	if logURIStr := lab[labels.LogURI]; logURIStr != "" {
		logURI, err := url.Parse(logURIStr)
		if err != nil {
			return err
		}
		if flagA {
			logrus.Debug("attaching output instead of using the log-uri")
		} else {
			taskCIO = cio.LogURI(logURI)
		}
	}

	cStatus := formatter.ContainerStatus(ctx, container)
	if cStatus == "Up" {
		logrus.Warnf("container %s is already running", container.ID())
		return nil
	}
	if err := run.UpdateContainerStoppedLabel(ctx, container, false); err != nil {
		return err
	}
	if oldTask, err := container.Task(ctx, nil); err == nil {
		if _, err := oldTask.Delete(ctx); err != nil {
			logrus.WithError(err).Debug("failed to delete old task")
		}
	}
	task, err := container.NewTask(ctx, taskCIO)
	if err != nil {
		return err
	}

	var statusC <-chan containerd.ExitStatus
	if flagA {
		statusC, err = task.Wait(ctx)
		if err != nil {
			return err
		}
	}

	if err := task.Start(ctx); err != nil {
		return err
	}

	if !flagA {
		return nil
	}

	sigc := commands.ForwardAllSignals(ctx, task)
	defer commands.StopCatch(sigc)
	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	if code != 0 {
		return common.ExitCodeError{
			Code: int(code),
		}
	}
	return nil
}

func reconfigNetContainer(ctx context.Context, c containerd.Container, client *containerd.Client, lab map[string]string) error {
	networksJSON, ok := lab[labels.Networks]
	if !ok {
		return nil
	}
	var networks []string
	if err := json.Unmarshal([]byte(networksJSON), &networks); err != nil {
		return err
	}
	netType, err := nettype.Detect(networks)
	if err != nil {
		return err
	}
	if netType == nettype.Container {
		network := strings.Split(networks[0], ":")
		if len(network) != 2 {
			return fmt.Errorf("invalid network: %s, should be \"container:<id|name>\"", networks[0])
		}
		targetCon, err := client.LoadContainer(ctx, network[1])
		if err != nil {
			return err
		}
		netNSPath, err := run.GetContainerNetNSPath(ctx, targetCon)
		if err != nil {
			return err
		}
		spec, err := c.Spec(ctx)
		if err != nil {
			return err
		}
		err = c.Update(ctx, containerd.UpdateContainerOpts(
			containerd.WithSpec(spec, oci.WithLinuxNamespace(
				specs.LinuxNamespace{
					Type: specs.NetworkNamespace,
					Path: netNSPath,
				}))))
		if err != nil {
			return err
		}
	}

	return nil
}

func reconfigPIDContainer(ctx context.Context, c containerd.Container, client *containerd.Client, lab map[string]string) error {
	targetContainerID, ok := lab[labels.PIDContainer]
	if !ok {
		return nil
	}

	if runtime.GOOS != "linux" {
		return errors.New("--pid only supported on linux")
	}

	targetCon, err := client.LoadContainer(ctx, targetContainerID)
	if err != nil {
		return err
	}

	opts, err := run.GenerateSharingPIDOpts(ctx, targetCon)
	if err != nil {
		return err
	}

	spec, err := c.Spec(ctx)
	if err != nil {
		return err
	}

	err = c.Update(ctx, containerd.UpdateContainerOpts(
		containerd.WithSpec(spec, oci.Compose(opts...)),
	))
	if err != nil {
		return err
	}

	return nil
}
