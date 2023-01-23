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
	"github.com/containerd/nerdctl/pkg/cmd/ipfs"
	"github.com/spf13/cobra"
)

const (
	defaultIPFSRegistry            = "localhost:5050"
	defaultIPFSReadRetryNum        = 0
	defaultIPFSReadTimeoutDuration = 0
)

func newIPFSRegistryServeCommand() *cobra.Command {
	var ipfsRegistryServeCommand = &cobra.Command{
		Use:           "serve",
		Short:         "serve read-only registry backed by IPFS on localhost.",
		RunE:          ipfsRegistryServeAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	AddStringFlag(ipfsRegistryServeCommand, "listen-registry", nil, defaultIPFSRegistry, "IPFS_REGISTRY_SERVE_LISTEN_REGISTRY", "address to listen")
	AddStringFlag(ipfsRegistryServeCommand, "ipfs-address", nil, "", "IPFS_REGISTRY_SERVE_IPFS_ADDRESS", "multiaddr of IPFS API (default is pulled from $IPFS_PATH/api file. If $IPFS_PATH env var is not present, it defaults to ~/.ipfs)")
	AddIntFlag(ipfsRegistryServeCommand, "read-retry-num", nil, defaultIPFSReadRetryNum, "IPFS_REGISTRY_SERVE_READ_RETRY_NUM", "times to retry query on IPFS. Zero or lower means no retry.")
	AddDurationFlag(ipfsRegistryServeCommand, "read-timeout", nil, defaultIPFSReadTimeoutDuration, "IPFS_REGISTRY_SERVE_READ_TIMEOUT", "timeout duration of a read request to IPFS. Zero means no timeout.")

	return ipfsRegistryServeCommand
}

func processIPFSRegistryServeOptions(cmd *cobra.Command) (opts types.IPFSRegistryServeOptions, err error) {
	ipfsAddressStr, err := cmd.Flags().GetString("ipfs-address")
	if err != nil {
		return types.IPFSRegistryServeOptions{}, err
	}
	listenAddress, err := cmd.Flags().GetString("listen-registry")
	if err != nil {
		return types.IPFSRegistryServeOptions{}, err
	}
	readTimeout, err := cmd.Flags().GetDuration("read-timeout")
	if err != nil {
		return types.IPFSRegistryServeOptions{}, err
	}
	readRetryNum, err := cmd.Flags().GetInt("read-retry-num")
	if err != nil {
		return types.IPFSRegistryServeOptions{}, err
	}
	return types.IPFSRegistryServeOptions{
		ListenRegistry: listenAddress,
		IPFSAddress:    ipfsAddressStr,
		ReadTimeout:    readTimeout,
		ReadRetryNum:   readRetryNum,
	}, nil
}

func ipfsRegistryServeAction(cmd *cobra.Command, args []string) error {
	options, err := processIPFSRegistryServeOptions(cmd)
	if err != nil {
		return err
	}
	return ipfs.RegistryServe(options)
}
