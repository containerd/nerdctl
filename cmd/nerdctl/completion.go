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

package main

import (
	"context"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
	"github.com/spf13/cobra"
)

func shellCompleteImageNames(cmd *cobra.Command) ([]string, cobra.ShellCompDirective) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
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

func shellCompleteContainerNames(cmd *cobra.Command, filterFunc func(containerd.ProcessStatus) bool) ([]string, cobra.ShellCompDirective) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
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

// shellCompleteNetworkNames includes {"bridge","host","none"}
func shellCompleteNetworkNames(cmd *cobra.Command, exclude []string) ([]string, cobra.ShellCompDirective) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	excludeMap := make(map[string]struct{}, len(exclude))
	for _, ex := range exclude {
		excludeMap[ex] = struct{}{}
	}

	e, err := netutil.NewCNIEnv(globalOptions.CNIPath, globalOptions.CNINetConfPath)
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

func shellCompleteVolumeNames(cmd *cobra.Command) ([]string, cobra.ShellCompDirective) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	vols, err := getVolumes(cmd, globalOptions)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	candidates := []string{}
	for _, v := range vols {
		candidates = append(candidates, v.Name)
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func shellCompletePlatforms(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
