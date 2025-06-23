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
	"encoding/json"
	"fmt"
	"os"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	nerdctl "github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/containerd/nerdctl/v2/pkg/config"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/logging"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

func main() {
	// Implement logging
	if len(os.Args) == 3 && os.Args[1] == logging.MagicArgv1 {
		err := logging.Main(os.Args[2])
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	// Get options
	globalOpt := types.GlobalCommandOptions(*config.New())

	// Rootless
	_ = rootlessutil.ParentMain(globalOpt.HostGatewayIP)

	// Printout options for debug
	f, _ := json.MarshalIndent(globalOpt, "", "  ")
	fmt.Printf("%s\n", f)

	// Create container options
	createOpt := types.ContainerCreateOptions{
		GOptions: globalOpt,
		// TODO: this example should implement oci-hook as well instead of relying on nerdctl
		NerdctlCmd:  "/usr/local/bin/nerdctl",
		Name:        "my-container",
		Label:       []string{},
		Cgroupns:    "private",
		InRun:       true,
		Rm:          false,
		Pull:        "missing",
		LogDriver:   "json-file",
		StopSignal:  "SIGTERM",
		Restart:     "unless-stopped",
		Interactive: true,
	}

	// Create client
	client, ctx, cancel, err := clientutil.NewClient(context.Background(), globalOpt.Namespace, globalOpt.Address)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer cancel()

	// Create network manager
	networkManager, err := containerutil.NewNetworkingOptionsManager(createOpt.GOptions, types.NetworkOptions{
		NetworkSlice: []string{"bridge"},
	}, client)

	if err != nil {
		fmt.Println(err)
		return
	}

	// Create container
	container, _, err := nerdctl.Create(ctx, client, []string{"debian"}, networkManager, createOpt)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Start container
	err = nerdctl.Start(ctx, client, []string{"my-container"}, types.ContainerStartOptions{
		Attach: true,
		Stdout: os.Stdout,
	})

	if err != nil {
		fmt.Println(err)
		return
	}

	cc, _ := json.MarshalIndent(container, "", "  ")
	fmt.Println(string(cc))
}
