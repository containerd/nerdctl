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
	"net/http"

	"github.com/containerd/nerdctl/pkg/ipfs"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const defaultIPFSRegistry = "localhost:5050"

func newIPFSRegistryServeCommand() *cobra.Command {
	var ipfsRegistryServeCommand = &cobra.Command{
		Use:           "serve",
		Short:         "serve read-only registry backed by IPFS on localhost. Use \"nerdctl ipfs registry up\".",
		RunE:          ipfsRegistryServeAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	ipfsRegistryServeCommand.PersistentFlags().String("listen-registry", defaultIPFSRegistry, "address to listen")
	ipfsRegistryServeCommand.PersistentFlags().String("ipfs-address", "", "multiaddr of IPFS API (default is pulled from $IPFS_PATH/api file. If $IPFS_PATH env var is not present, it defaults to ~/.ipfs)")

	return ipfsRegistryServeCommand
}

func ipfsRegistryServeAction(cmd *cobra.Command, args []string) error {
	ipfsAddressStr, err := cmd.Flags().GetString("ipfs-address")
	if err != nil {
		return err
	}
	listenAddress, err := cmd.Flags().GetString("listen-registry")
	if err != nil {
		return err
	}
	var ipfsClient *httpapi.HttpApi
	if ipfsAddressStr != "" {
		a, err := multiaddr.NewMultiaddr(ipfsAddressStr)
		if err != nil {
			return err
		}
		ipfsClient, err = httpapi.NewApi(a)
		if err != nil {
			return err
		}
	} else {
		ipfsClient, err = httpapi.NewLocalApi()
		if err != nil {
			return err
		}
	}
	logrus.Infof("serving on %v", listenAddress)
	http.Handle("/", ipfs.NewRegistry(ipfsClient))
	return http.ListenAndServe(listenAddress, nil)
}
