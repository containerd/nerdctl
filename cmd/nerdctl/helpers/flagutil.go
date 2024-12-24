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

package helpers

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

func ProcessImageVerifyOptions(cmd *cobra.Command) (opt types.ImageVerifyOptions, err error) {
	if opt.Provider, err = cmd.Flags().GetString("verify"); err != nil {
		return
	}
	if opt.CosignKey, err = cmd.Flags().GetString("cosign-key"); err != nil {
		return
	}
	if opt.CosignCertificateIdentity, err = cmd.Flags().GetString("cosign-certificate-identity"); err != nil {
		return
	}
	if opt.CosignCertificateIdentityRegexp, err = cmd.Flags().GetString("cosign-certificate-identity-regexp"); err != nil {
		return
	}
	if opt.CosignCertificateOidcIssuer, err = cmd.Flags().GetString("cosign-certificate-oidc-issuer"); err != nil {
		return
	}
	if opt.CosignCertificateOidcIssuerRegexp, err = cmd.Flags().GetString("cosign-certificate-oidc-issuer-regexp"); err != nil {
		return
	}
	return
}

func ProcessRootCmdFlags(cmd *cobra.Command) (types.GlobalCommandOptions, error) {
	debug, err := cmd.Flags().GetBool("debug")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	debugFull, err := cmd.Flags().GetBool("debug-full")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	address, err := cmd.Flags().GetString("address")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	snapshotter, err := cmd.Flags().GetString("snapshotter")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	cniPath, err := cmd.Flags().GetString("cni-path")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	cniConfigPath, err := cmd.Flags().GetString("cni-netconfpath")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	dataRoot, err := cmd.Flags().GetString("data-root")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	cgroupManager, err := cmd.Flags().GetString("cgroup-manager")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	insecureRegistry, err := cmd.Flags().GetBool("insecure-registry")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	hostsDir, err := cmd.Flags().GetStringSlice("hosts-dir")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	experimental, err := cmd.Flags().GetBool("experimental")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	hostGatewayIP, err := cmd.Flags().GetString("host-gateway-ip")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	bridgeIP, err := cmd.Flags().GetString("bridge-ip")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	kubeHideDupe, err := cmd.Flags().GetBool("kube-hide-dupe")
	if err != nil {
		return types.GlobalCommandOptions{}, err
	}
	return types.GlobalCommandOptions{
		Debug:            debug,
		DebugFull:        debugFull,
		Address:          address,
		Namespace:        namespace,
		Snapshotter:      snapshotter,
		CNIPath:          cniPath,
		CNINetConfPath:   cniConfigPath,
		DataRoot:         dataRoot,
		CgroupManager:    cgroupManager,
		InsecureRegistry: insecureRegistry,
		HostsDir:         hostsDir,
		Experimental:     experimental,
		HostGatewayIP:    hostGatewayIP,
		BridgeIP:         bridgeIP,
		KubeHideDupe:     kubeHideDupe,
	}, nil
}

func CheckExperimental(feature string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		globalOptions, err := ProcessRootCmdFlags(cmd)
		if err != nil {
			return err
		}
		if !globalOptions.Experimental {
			return fmt.Errorf("%s is experimental feature, you should enable experimental config", feature)
		}
		return nil
	}
}
