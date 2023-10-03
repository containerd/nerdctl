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
	"errors"
	"io"
	"strings"

	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/login"

	"github.com/spf13/cobra"
)

type loginOptions struct {
	serverAddress string
	username      string
	password      string
	passwordStdin bool
}

var options = new(loginOptions)

func newLoginCommand() *cobra.Command {
	var loginCommand = &cobra.Command{
		Use:           "login [flags] [SERVER]",
		Args:          cobra.MaximumNArgs(1),
		Short:         "Log in to a container registry",
		RunE:          loginAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	loginCommand.Flags().StringVarP(&options.username, "username", "u", "", "Username")
	loginCommand.Flags().StringVarP(&options.password, "password", "p", "", "Password")
	loginCommand.Flags().BoolVar(&options.passwordStdin, "password-stdin", false, "Take the password from stdin")
	return loginCommand
}

func loginAction(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		options.serverAddress = args[0]
	}
	if err := verifyLoginOptions(cmd, options); err != nil {
		return err
	}

	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}

	return login.Login(cmd.Context(), types.LoginCommandOptions{
		GOptions:      globalOptions,
		ServerAddress: options.serverAddress,
		Username:      options.username,
		Password:      options.password,
	}, cmd.OutOrStdout())
}

// copied from github.com/docker/cli/cli/command/registry/login.go (v20.10.3)
func verifyLoginOptions(cmd *cobra.Command, options *loginOptions) error {
	if options.password != "" {
		log.L.Warn("WARNING! Using --password via the CLI is insecure. Use --password-stdin.")
		if options.passwordStdin {
			return errors.New("--password and --password-stdin are mutually exclusive")
		}
	}

	if options.passwordStdin {
		if options.username == "" {
			return errors.New("must provide --username with --password-stdin")
		}

		contents, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return err
		}

		options.password = strings.TrimSuffix(string(contents), "\n")
		options.password = strings.TrimSuffix(options.password, "\r")
	}
	return nil
}
