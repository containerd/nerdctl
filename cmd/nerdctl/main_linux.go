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
	"fmt"

	ncdefaults "github.com/containerd/nerdctl/pkg/defaults"

	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/urfave/cli/v2"
)

func appNeedsRootlessParentMain(clicontext *cli.Context) bool {
	if !rootlessutil.IsRootlessParent() {
		return false
	}
	// TODO: allow `nerdctl <SUBCOMMAND> --help` without nsentering into RootlessKit
	switch clicontext.Args().First() {
	case "", "completion", "login", "logout":
		return false
	}
	return true
}

func appBashComplete(clicontext *cli.Context) {
	w := clicontext.App.Writer
	coco := parseCompletionContext(clicontext)
	switch coco.flagName {
	case "n", "namespace":
		bashCompleteNamespaceNames(clicontext)
		return
	case "snapshotter", "storage-driver":
		bashCompleteSnapshotterNames(clicontext)
		return
	case "cgroup-manager":
		fmt.Fprintln(w, "cgroupfs")
		if ncdefaults.IsSystemdAvailable() {
			fmt.Fprintln(w, "systemd")
		}
		if rootlessutil.IsRootless() {
			fmt.Fprintln(w, "none")
		}
		return
	}
	cli.DefaultAppComplete(clicontext)
	for _, subcomm := range clicontext.App.Commands {
		fmt.Fprintln(clicontext.App.Writer, subcomm.Name)
	}
}
