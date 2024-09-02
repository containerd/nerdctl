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

package internal

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/ocihook"
)

func newInternalOCIHookCommandCommand() *cobra.Command {
	var internalOCIHookCommand = &cobra.Command{
		Use:           "oci-hook",
		Short:         "OCI hook",
		RunE:          internalOCIHookAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return internalOCIHookCommand
}

func internalOCIHookAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	event := ""
	if len(args) > 0 {
		event = args[0]
	}
	if event == "" {
		return errors.New("event type needs to be passed")
	}
	dataStore, err := clientutil.DataStore(globalOptions.DataRoot, globalOptions.Address)
	if err != nil {
		return err
	}
	cniPath := globalOptions.CNIPath
	cniNetconfpath := globalOptions.CNINetConfPath
	return ocihook.Run(os.Stdin, os.Stderr, event,
		dataStore,
		cniPath,
		cniNetconfpath,
	)
}
