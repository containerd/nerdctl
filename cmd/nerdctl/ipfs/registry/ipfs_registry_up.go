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

package registry

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/hashicorp/go-multierror"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const ipfsRegistryContainerName = "ipfs-registry"

func NewUpCommand() *cobra.Command {
	var ipfsRegistryUpCommand = &cobra.Command{
		Use:           "up",
		Short:         "Start registry as a background container \"ipfs-registry\", backed by the current user's IPFS API",
		RunE:          ipfsRegistryUpAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	ipfsRegistryUpCommand.PersistentFlags().String("listen-registry", defaultIPFSRegistry, "address to listen")
	ipfsRegistryUpCommand.PersistentFlags().Int("read-retry-num", defaultIPFSReadRetryNum, "times to retry query on IPFS. Zero or lower means no retry.")
	ipfsRegistryUpCommand.PersistentFlags().Duration("read-timeout", defaultIPFSReadTimeoutDuration, "timeout duration of a read request to IPFS. Zero means no timeout.")

	return ipfsRegistryUpCommand
}

func ipfsRegistryUpAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := ncclient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			return nil
		},
	}
	nc, err := walker.Walk(ctx, ipfsRegistryContainerName)
	if err != nil {
		return err
	}
	if nc > 0 {
		logrus.Infof("IPFS registry %q is already running. Using the existing one. Restart manually if needed.", ipfsRegistryContainerName)
		return nil
	}
	if err := runRegistryAsContainer(cmd); err != nil {
		logrus.Errorf("Failed to serve registry. Use \"nerdctl ipfs registry serve\" command instead")
		return err
	}
	return nil
}

// runRegistryAsContainer runs "nerdctl ipfs registry serve" as a container with --net=host.
// This function bind mounts nerdctl binary to a directory and runs that directory as the rootfs.
func runRegistryAsContainer(cmd *cobra.Command) error {
	listenAddress, err := cmd.Flags().GetString("listen-registry")
	if err != nil {
		return err
	}
	readTimeout, err := cmd.Flags().GetDuration("read-timeout")
	if err != nil {
		return err
	}
	readRetryNum, err := cmd.Flags().GetInt("read-retry-num")
	if err != nil {
		return err
	}
	dataStore, err := ncclient.GetDataStore(cmd)
	if err != nil {
		return err
	}
	nerdctlCmd, nerdctlArgs := utils.GlobalFlags(cmd)
	registryRoot := filepath.Join(dataStore, "ipfs-registry", "rootfs")
	if err := os.RemoveAll(registryRoot); err != nil {
		return err
	}
	if err := os.MkdirAll(registryRoot, 0700); err != nil {
		return err
	}
	// Get IPFS API address in the same convention with IPFS client library does.
	// https://github.com/ipfs/go-ipfs-http-client/blob/3af36210f80fb86aae50da582b494ceddd64c3de/api.go#L42-L54
	baseDir := os.Getenv(httpapi.EnvDir)
	if baseDir == "" {
		baseDir = httpapi.DefaultPathRoot
	}
	ipfsAPIAddr, err := httpapi.ApiAddr(baseDir)
	if err != nil {
		return err
	}
	if out, err := exec.Command(nerdctlCmd, append(nerdctlArgs,
		"run", "-d", "--name", ipfsRegistryContainerName, "--net=host", "--entrypoint", "/mnt/nerdctl",
		"--read-only", "-v", nerdctlCmd+":/mnt/nerdctl:ro", "--rootfs", registryRoot,
		"ipfs", "registry", "serve", "--ipfs-address", ipfsAPIAddr.String(), "--listen-registry", listenAddress,
		"--read-retry-num", fmt.Sprintf("%d", readRetryNum), "--read-timeout", readTimeout.String(),
	)...).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to execute registry: %v: %v", string(out), err)
	}
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	logrus.Infof("Waiting for the registry being ready...")
	var allErr error
	for i := 0; i < 3; i++ {
		_, err := client.Get("http://" + listenAddress + "/v2/")
		if err == nil {
			logrus.Infof("Registry is up-and-running")
			return nil
		}
		allErr = multierror.Append(allErr, err)
		time.Sleep(time.Second)
	}
	return fmt.Errorf("started registry but failed to connect: %v", allErr)
}
