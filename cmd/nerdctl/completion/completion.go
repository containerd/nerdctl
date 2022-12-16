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

package completion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/volume"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/spf13/cobra"
)

func ShellCompleteImageNames(cmd *cobra.Command) ([]string, cobra.ShellCompDirective) {
	client, ctx, cancel, err := client.New(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer cancel()

	imageList, err := client.ImageService().List(ctx, "")

	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	candidates := []string{}
	for _, img := range imageList {
		candidates = append(candidates, img.Name)
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func ShellCompleteContainerNames(cmd *cobra.Command, filterFunc func(containerd.ProcessStatus) bool) ([]string, cobra.ShellCompDirective) {
	client, ctx, cancel, err := client.New(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer cancel()
	containers, err := client.Containers(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	getStatus := func(c containerd.Container) containerd.ProcessStatus {
		ctx2, cancel2 := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel2()
		task, err := c.Task(ctx2, nil)
		if err != nil {
			return containerd.Unknown
		}
		st, err := task.Status(ctx2)
		if err != nil {
			return containerd.Unknown
		}
		return st.Status
	}
	candidates := []string{}
	for _, c := range containers {
		if filterFunc != nil {
			if !filterFunc(getStatus(c)) {
				continue
			}
		}
		lab, err := c.Labels(ctx)
		if err != nil {
			continue
		}
		name := lab[labels.Name]
		if name != "" {
			candidates = append(candidates, name)
			continue
		}
		candidates = append(candidates, c.ID())
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

// ShellCompleteNetworkNames includes {"bridge","host","none"}
func ShellCompleteNetworkNames(cmd *cobra.Command, exclude []string) ([]string, cobra.ShellCompDirective) {
	excludeMap := make(map[string]struct{}, len(exclude))
	for _, ex := range exclude {
		excludeMap[ex] = struct{}{}
	}

	cniPath, err := cmd.Flags().GetString("cni-path")
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	cniNetconfpath, err := cmd.Flags().GetString("cni-netconfpath")
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	e, err := netutil.NewCNIEnv(cniPath, cniNetconfpath)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	candidates := []string{}
	netConfigs, err := e.NetworkMap()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	for netName := range netConfigs {
		if _, ok := excludeMap[netName]; !ok {
			candidates = append(candidates, netName)
		}
	}
	for _, s := range []string{"host", "none"} {
		if _, ok := excludeMap[s]; !ok {
			candidates = append(candidates, s)
		}
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func ShellCompleteVolumeNames(cmd *cobra.Command) ([]string, cobra.ShellCompDirective) {
	vols, err := volume.GetVolumes(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	candidates := []string{}
	for _, v := range vols {
		candidates = append(candidates, v.Name)
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func ShellCompletePlatforms(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	candidates := []string{
		"amd64",
		"arm64",
		"riscv64",
		"ppc64le",
		"s390x",
		"386",
		"arm",          // alias of "linux/arm/v7"
		"linux/arm/v6", // "arm/v6" is invalid (interpreted as OS="arm", Arch="v7")
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func TopShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	return ShellCompleteContainerNames(cmd, statusFilterFn)
}

func PauseShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	return ShellCompleteContainerNames(cmd, statusFilterFn)
}

func UnpauseShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show paused container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Paused
	}
	return ShellCompleteContainerNames(cmd, statusFilterFn)
}

func LogsShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names (TODO: only show Containers with logs)
	return ShellCompleteContainerNames(cmd, nil)
}

func StartShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show non-running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st != containerd.Running && st != containerd.Unknown
	}
	return ShellCompleteContainerNames(cmd, statusFilterFn)
}

func StatsShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	return ShellCompleteContainerNames(cmd, statusFilterFn)
}

func StopShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show non-stopped container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st != containerd.Stopped && st != containerd.Created && st != containerd.Unknown
	}
	return ShellCompleteContainerNames(cmd, statusFilterFn)
}

// UnknownSubcommandAction is needed to let `nerdctl system non-existent-command` fail
// https://github.com/containerd/nerdctl/issues/487
//
// Ideally this should be implemented in Cobra itself.
func UnknownSubcommandAction(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	// The output mimics https://github.com/spf13/cobra/blob/v1.2.1/command.go#L647-L662
	msg := fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.Name())
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?\n"
		for _, s := range suggestions {
			msg += fmt.Sprintf("\t%v\n", s)
		}
	}
	return errors.New(msg)
}
