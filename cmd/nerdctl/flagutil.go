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
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/spf13/cobra"
)

func processGlobalFlag(cmd *cobra.Command) (*types.GlobalCommandOptions, error) {
	options := &types.GlobalCommandOptions{}
	debug, err := cmd.Flags().GetBool("debug")
	if err != nil {
		return nil, err
	}
	options.Debug = debug
	debugFull, err := cmd.Flags().GetBool("debug-full")
	if err != nil {
		return nil, err
	}
	options.DebugFull = debugFull
	address, err := cmd.Flags().GetString("address")
	if err != nil {
		return nil, err
	}
	options.Address = address
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return nil, err
	}
	options.Namespace = namespace
	snapshotter, err := cmd.Flags().GetString("snapshotter")
	if err != nil {
		return nil, err
	}
	options.Snapshotter = snapshotter
	cniPath, err := cmd.Flags().GetString("cni-path")
	if err != nil {
		return nil, err
	}
	options.CNIPath = cniPath
	cniConfigPath, err := cmd.Flags().GetString("cni-netconfpath")
	if err != nil {
		return nil, err
	}
	options.CNINetConfPath = cniConfigPath
	dataRoot, err := cmd.Flags().GetString("data-root")
	if err != nil {
		return nil, err
	}
	options.DataRoot = dataRoot
	cgroupManager, err := cmd.Flags().GetString("cgroup-manager")
	if err != nil {
		return nil, err
	}
	options.CgroupManager = cgroupManager
	insecureRegistry, err := cmd.Flags().GetBool("insecure-registry")
	if err != nil {
		return nil, err
	}
	options.InsecureRegistry = insecureRegistry
	hostsDir, err := cmd.Flags().GetStringSlice("hosts-dir")
	if err != nil {
		return nil, err
	}
	options.HostsDir = hostsDir
	experimental, err := cmd.Flags().GetBool("experimental")
	if err != nil {
		return nil, err
	}
	options.Experimental = experimental
	return options, nil
}
