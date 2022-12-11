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

package network

import (
	"fmt"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/strutil"

	"github.com/spf13/cobra"
)

func NewNetworkCreateCommand() *cobra.Command {
	var networkCreateCommand = &cobra.Command{
		Use:           "create [flags] NETWORK",
		Short:         "Create a network",
		Long:          `NOTE: To isolate CNI bridge, CNI plugin "firewall" (>= v1.1.0) is needed.`,
		Args:          utils.IsExactArgs(1),
		RunE:          networkCreateAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	networkCreateCommand.Flags().StringP("driver", "d", DefaultNetworkDriver, "Driver to manage the Network")
	networkCreateCommand.RegisterFlagCompletionFunc("driver", shellCompleteNetworkDrivers)
	networkCreateCommand.Flags().StringArrayP("opt", "o", nil, "Set driver specific options")
	networkCreateCommand.Flags().String("ipam-driver", "default", "IP Address Management Driver")
	networkCreateCommand.RegisterFlagCompletionFunc("ipam-driver", shellCompleteIPAMDrivers)
	networkCreateCommand.Flags().StringArray("ipam-opt", nil, "Set IPAM driver specific options")
	networkCreateCommand.Flags().String("subnet", "", `Subnet in CIDR format that represents a network segment, e.g. "10.5.0.0/16"`)
	networkCreateCommand.Flags().String("gateway", "", `Gateway for the master subnet`)
	networkCreateCommand.Flags().String("ip-range", "", `Allocate container ip from a sub-range`)
	networkCreateCommand.Flags().StringArray("label", nil, "Set metadata for a network")
	return networkCreateCommand
}

func networkCreateAction(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := identifiers.Validate(name); err != nil {
		return fmt.Errorf("malformed name %s: %w", name, err)
	}
	cniPath, err := cmd.Flags().GetString("cni-path")
	if err != nil {
		return err
	}
	cniNetconfpath, err := cmd.Flags().GetString("cni-netconfpath")
	if err != nil {
		return err
	}
	driver, err := cmd.Flags().GetString("driver")
	if err != nil {
		return err
	}
	opts, err := cmd.Flags().GetStringArray("opt")
	if err != nil {
		return err
	}
	ipamDriver, err := cmd.Flags().GetString("ipam-driver")
	if err != nil {
		return err
	}
	ipamOpts, err := cmd.Flags().GetStringArray("ipam-opt")
	if err != nil {
		return err
	}
	subnetStr, err := cmd.Flags().GetString("subnet")
	if err != nil {
		return err
	}
	gatewayStr, err := cmd.Flags().GetString("gateway")
	if err != nil {
		return err
	}
	ipRangeStr, err := cmd.Flags().GetString("ip-range")
	if err != nil {
		return err
	}
	labels, err := cmd.Flags().GetStringArray("label")
	if err != nil {
		return err
	}
	labels = strutil.DedupeStrSlice(labels)

	if subnetStr == "" {
		if gatewayStr != "" || ipRangeStr != "" {
			return fmt.Errorf("cannot set gateway or ip-range without subnet, specify --subnet manually")
		}
	}

	e, err := netutil.NewCNIEnv(cniPath, cniNetconfpath)
	if err != nil {
		return err
	}
	createOpts := netutil.CreateOptions{
		Name:        name,
		Driver:      driver,
		Options:     strutil.ConvertKVStringsToMap(opts),
		IPAMDriver:  ipamDriver,
		IPAMOptions: strutil.ConvertKVStringsToMap(ipamOpts),
		Subnet:      subnetStr,
		Gateway:     gatewayStr,
		IPRange:     ipRangeStr,
		Labels:      labels,
	}
	net, err := e.CreateNetwork(createOpts)
	if err != nil {
		if errdefs.IsAlreadyExists(err) {
			return fmt.Errorf("network with name %s already exists", name)
		}
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", *net.NerdctlID)
	return err
}
