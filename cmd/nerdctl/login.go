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
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/login"

	"github.com/spf13/cobra"
)

func newLoginCommand() *cobra.Command {
	var loginCommand = &cobra.Command{
		Use:           "login [flags] [SERVER]",
		Args:          cobra.MaximumNArgs(1),
		Short:         "Log in to a container registry",
		RunE:          loginAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	loginCommand.Flags().StringP("username", "u", "", "Username")
	loginCommand.Flags().StringP("password", "p", "", "Password")
	loginCommand.Flags().Bool("password-stdin", false, "Take the password from stdin")
	return loginCommand
}

func processLoginOptions(cmd *cobra.Command) (types.LoginCommandOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.LoginCommandOptions{}, err
	}

	username, err := cmd.Flags().GetString("username")
	if err != nil {
		return types.LoginCommandOptions{}, err
	}
	password, err := cmd.Flags().GetString("password")
	if err != nil {
		return types.LoginCommandOptions{}, err
	}
	passwordStdin, err := cmd.Flags().GetBool("password-stdin")
	if err != nil {
		return types.LoginCommandOptions{}, err
	}

	if strings.Contains(username, ":") {
		return types.LoginCommandOptions{}, errors.New("username cannot contain colons")
	}

	if password != "" {
		log.L.Warn("WARNING! Using --password via the CLI is insecure. Use --password-stdin.")
		if passwordStdin {
			return types.LoginCommandOptions{}, errors.New("--password and --password-stdin are mutually exclusive")
		}
	}

	if passwordStdin {
		if username == "" {
			return types.LoginCommandOptions{}, errors.New("must provide --username with --password-stdin")
		}

		contents, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return types.LoginCommandOptions{}, err
		}

		password = strings.TrimSuffix(string(contents), "\n")
		password = strings.TrimSuffix(password, "\r")
	}
	return types.LoginCommandOptions{
		GOptions: globalOptions,
		Username: username,
		Password: password,
	}, nil
}

func loginAction(cmd *cobra.Command, args []string) error {
	options, err := processLoginOptions(cmd)
	if err != nil {
		return err
	}

	if len(args) == 1 {
		options.ServerAddress = args[0]
	}

	return login.Login(cmd.Context(), options, cmd.OutOrStdout())
}
