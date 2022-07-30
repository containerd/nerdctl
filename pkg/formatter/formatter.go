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

package formatter

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/runtime/restart"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/docker/go-units"
	"github.com/sirupsen/logrus"
)

func ContainerStatus(ctx context.Context, c containerd.Container) string {
	// Just in case, there is something wrong in server.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	titleCaser := cases.Title(language.English)

	task, err := c.Task(ctx, nil)
	if err != nil {
		// NOTE: NotFound doesn't mean that container hasn't started.
		// In docker/CRI-containerd plugin, the task will be deleted
		// when it exits. So, the status will be "created" for this
		// case.
		if errdefs.IsNotFound(err) {
			return titleCaser.String(string(containerd.Created))
		}
		return titleCaser.String(string(containerd.Unknown))
	}

	status, err := task.Status(ctx)
	if err != nil {
		return titleCaser.String(string(containerd.Unknown))
	}
	labels, err := c.Labels(ctx)
	if err != nil {
		return titleCaser.String(string(containerd.Unknown))
	}

	switch s := status.Status; s {
	case containerd.Stopped:
		if labels[restart.StatusLabel] == string(containerd.Running) && restart.Reconcile(status, labels) {
			return fmt.Sprintf("Restarting (%v) %s", status.ExitStatus, TimeSinceInHuman(status.ExitTime))
		}
		return fmt.Sprintf("Exited (%v) %s", status.ExitStatus, TimeSinceInHuman(status.ExitTime))
	case containerd.Running:
		return "Up" // TODO: print "status.UpTime" (inexistent yet)
	default:
		return titleCaser.String(string(s))
	}
}

func InspectContainerCommand(spec *oci.Spec, trunc bool) string {
	if spec == nil || spec.Process == nil {
		return ""
	}

	command := spec.Process.CommandLine + strings.Join(spec.Process.Args, " ")
	if trunc {
		command = Ellipsis(command, 20)
	}
	return strconv.Quote(command)
}

func InspectContainerCommandTrunc(spec *oci.Spec) string {
	return InspectContainerCommand(spec, true)
}

func Ellipsis(str string, maxDisplayWidth int) string {
	if maxDisplayWidth <= 0 {
		return ""
	}

	lenStr := len(str)
	if maxDisplayWidth == 1 {
		if lenStr <= maxDisplayWidth {
			return str
		}
		return string(str[0])
	}

	if lenStr <= maxDisplayWidth {
		return str
	}
	return str[:maxDisplayWidth-1] + "…"
}

func FormatPorts(labelMap map[string]string) string {
	portsJSON := labelMap[labels.Ports]
	if portsJSON == "" {
		return ""
	}
	var ports []gocni.PortMapping
	if err := json.Unmarshal([]byte(portsJSON), &ports); err != nil {
		logrus.WithError(err).Errorf("failed to parse label %q=%q", labels.Ports, portsJSON)
		return ""
	}
	if len(ports) == 0 {
		return ""
	}
	strs := make([]string, len(ports))
	for i, p := range ports {
		strs[i] = fmt.Sprintf("%s:%d->%d/%s", p.HostIP, p.HostPort, p.ContainerPort, p.Protocol)
	}
	return strings.Join(strs, ", ")
}

func TimeSinceInHuman(since time.Time) string {
	return fmt.Sprintf("%s ago", units.HumanDuration(time.Since(since)))
}

func FormatLabels(labelMap map[string]string) string {
	strs := make([]string, len(labelMap))
	idx := 0
	for i := range labelMap {
		strs[idx] = fmt.Sprintf("%s=%s", i, labelMap[i])
		idx++
	}
	return strings.Join(strs, ",")
}
