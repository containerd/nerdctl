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

package logout

import (
	"fmt"

	"github.com/containerd/nerdctl/pkg/imgutil/dockerconfigresolver"
	dockercliconfig "github.com/docker/cli/cli/config"
	"github.com/spf13/cobra"
)

func NewLogoutCommand() *cobra.Command {
	var logoutCommand = &cobra.Command{
		Use:               "logout [flags] [SERVER]",
		Args:              cobra.MaximumNArgs(1),
		Short:             "Log out from a container registry",
		RunE:              logoutAction,
		ValidArgsFunction: logoutShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return logoutCommand
}

// code inspired from XXX
func logoutAction(cmd *cobra.Command, args []string) error {
	serverAddress := dockerconfigresolver.IndexServer
	isDefaultRegistry := true
	if len(args) >= 1 {
		serverAddress = args[0]
		isDefaultRegistry = false
	}

	var (
		regsToLogout    = []string{serverAddress}
		hostnameAddress = serverAddress
	)

	if !isDefaultRegistry {
		hostnameAddress = dockerconfigresolver.ConvertToHostname(serverAddress)
		// the tries below are kept for backward compatibility where a user could have
		// saved the registry in one of the following format.
		regsToLogout = append(regsToLogout, hostnameAddress, "http://"+hostnameAddress, "https://"+hostnameAddress)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removing login credentials for %s\n", hostnameAddress)

	dockerConfigFile, err := dockercliconfig.Load("")
	if err != nil {
		return err
	}
	errs := make(map[string]error)
	for _, r := range regsToLogout {
		if err := dockerConfigFile.GetCredentialsStore(r).Erase(r); err != nil {
			errs[r] = err
		}
	}

	// if at least one removal succeeded, report success. Otherwise report errors
	if len(errs) == len(regsToLogout) {
		fmt.Fprintln(cmd.ErrOrStderr(), "WARNING: could not erase credentials:")
		for k, v := range errs {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", k, v)
		}
	}

	return nil
}

func logoutShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	dockerConfigFile, err := dockercliconfig.Load("")
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	candidates := []string{}
	for key := range dockerConfigFile.AuthConfigs {
		candidates = append(candidates, key)
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}
