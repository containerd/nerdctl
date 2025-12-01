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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-units"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/runtime/restart"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/containerd/go-cni"
)

func ContainerStatus(ctx context.Context, c containerd.Container) string {
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

func InspectContainerCommand(spec *oci.Spec, trunc, quote bool) string {
	if spec == nil || spec.Process == nil {
		return ""
	}

	command := spec.Process.CommandLine + strings.Join(spec.Process.Args, " ")
	if trunc {
		command = Ellipsis(command, 20)
	}
	if quote {
		command = strconv.Quote(command)
	}
	return command
}

func InspectContainerCommandTrunc(spec *oci.Spec) string {
	return InspectContainerCommand(spec, true, true)
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

func formatRange(startHost, endHost, startContainer, endContainer int32) string {
	if startHost == endHost && startContainer == endContainer {
		return fmt.Sprintf("%d->%d", startHost, startContainer)
	}
	return fmt.Sprintf("%d-%d->%d-%d", startHost, endHost, startContainer, endContainer)
}

func FormatPorts(ports []cni.PortMapping) string {
	if len(ports) == 0 {
		return ""
	}

	type key struct {
		HostIP   string
		Protocol string
	}
	grouped := make(map[key][]cni.PortMapping)

	for _, p := range ports {
		k := key{HostIP: p.HostIP, Protocol: p.Protocol}
		grouped[k] = append(grouped[k], p)
	}

	var displayPorts []string
	for k, pms := range grouped {
		sort.Slice(pms, func(i, j int) bool {
			return pms[i].HostPort < pms[j].HostPort
		})

		var i int
		var ranges []string
		for i = 0; i < len(pms); {
			start, end := pms[i], pms[i]
			for i+1 < len(pms) &&
				pms[i+1].HostPort == end.HostPort+1 &&
				pms[i+1].ContainerPort == end.ContainerPort+1 {
				i++
				end = pms[i]
			}

			ranges = append(
				ranges,
				formatRange(start.HostPort, end.HostPort, start.ContainerPort, end.ContainerPort),
			)
			i++
		}
		displayPorts = append(
			displayPorts,
			fmt.Sprintf("%s:%s/%s", k.HostIP, strings.Join(ranges, ", "), k.Protocol),
		)
	}

	sort.Strings(displayPorts)

	return strings.Join(displayPorts, ", ")
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

// ToJSON return a string with the JSON representation of the interface{}
// https://github.com/docker/compose/blob/v2/cmd/formatter/json.go#L31C4-L39
func ToJSON(i interface{}, prefix string, indentation string) (string, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent(prefix, indentation)
	err := encoder.Encode(i)
	return buffer.String(), err
}
