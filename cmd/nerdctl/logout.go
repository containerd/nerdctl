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

	dockercliconfig "github.com/docker/cli/cli/config"
	"github.com/docker/docker/registry"
	"github.com/urfave/cli/v2"
)

var logoutCommand = &cli.Command{
	Name:      "logout",
	Usage:     "Log out from a Docker registry",
	ArgsUsage: "[flags] [SERVER]",
	Action:    logoutAction,
}

// code inspired from XXX
func logoutAction(clicontext *cli.Context) error {
	if clicontext.Bool("help") {
		return cli.ShowCommandHelp(clicontext, "login")
	}

	serverAddress := clicontext.Args().First()

	var isDefaultRegistry bool

	if serverAddress == "" {
		serverAddress = registry.IndexServer
		isDefaultRegistry = true
	}

	var (
		regsToLogout    = []string{serverAddress}
		hostnameAddress = serverAddress
	)

	if !isDefaultRegistry {
		hostnameAddress = registry.ConvertToHostname(serverAddress)
		// the tries below are kept for backward compatibility where a user could have
		// saved the registry in one of the following format.
		regsToLogout = append(regsToLogout, hostnameAddress, "http://"+hostnameAddress, "https://"+hostnameAddress)
	}

	fmt.Fprintf(clicontext.App.Writer, "Removing login credentials for %s\n", hostnameAddress)

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
		fmt.Fprintln(clicontext.App.Writer, "WARNING: could not erase credentials:")
		for k, v := range errs {
			fmt.Fprintf(clicontext.App.Writer, "%s: %s\n", k, v)
		}
	}

	return nil

}
