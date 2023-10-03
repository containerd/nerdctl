//go:build freebsd || linux

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
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/spf13/cobra"
)

func shellCompleteNamespaceNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	if rootlessutil.IsRootlessParent() {
		_ = rootlessutil.ParentMain(globalOptions.HostGatewayIP)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer cancel()
	nsService := client.NamespaceService()
	nsList, err := nsService.List(ctx)
	if err != nil {
		log.L.Warn(err)
		return nil, cobra.ShellCompDirectiveError
	}
	var candidates []string
	candidates = append(candidates, nsList...)
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func shellCompleteSnapshotterNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	if rootlessutil.IsRootlessParent() {
		_ = rootlessutil.ParentMain(globalOptions.HostGatewayIP)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer cancel()
	snapshotterPlugins, err := infoutil.GetSnapshotterNames(ctx, client.IntrospectionService())
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var candidates []string
	candidates = append(candidates, snapshotterPlugins...)
	return candidates, cobra.ShellCompDirectiveNoFileComp
}
