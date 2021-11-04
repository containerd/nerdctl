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
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/containerd/nerdctl/pkg/version"
	dockercliconfig "github.com/docker/cli/cli/config"
	clitypes "github.com/docker/cli/cli/config/types"
	dockercliconfigtypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	registrytypes "github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/registry"

	"github.com/sirupsen/logrus"
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
		Short:         "Log in to a Docker registry",
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
	if err := verifyloginOptions(cmd, options); err != nil {
		return err
	}

	var serverAddress string

	if options.serverAddress == "" {
		serverAddress = registry.IndexServer
	} else {
		serverAddress = options.serverAddress
	}

	var response registrytypes.AuthenticateOKBody
	ctx := cmd.Context()
	isDefaultRegistry := serverAddress == registry.IndexServer
	authConfig, err := GetDefaultAuthConfig(options.username == "" && options.password == "", serverAddress, isDefaultRegistry)
	if &authConfig == nil {
		authConfig = &types.AuthConfig{}
	}
	if err == nil && authConfig.Username != "" && authConfig.Password != "" {
		//login With StoreCreds
		response, err = loginClientSide(ctx, cmd, *authConfig)
	}

	if err != nil || authConfig.Username == "" || authConfig.Password == "" {
		err = ConfigureAuthentification(authConfig, options)
		if err != nil {
			return err
		}

		response, err = loginClientSide(ctx, cmd, *authConfig)
		if err != nil {
			return err
		}

	}

	if response.IdentityToken != "" {
		authConfig.Password = ""
		authConfig.IdentityToken = response.IdentityToken
	}

	dockerConfigFile, err := dockercliconfig.Load("")
	if err != nil {
		return err
	}

	if err := dockerConfigFile.GetCredentialsStore(serverAddress).Store(clitypes.AuthConfig(*(authConfig))); err != nil {
		return fmt.Errorf("error saving credentials: %w", err)
	}

	if response.Status != "" {
		fmt.Fprintln(cmd.OutOrStdout(), response.Status)
	}

	return nil
}

//copied from github.com/docker/cli/cli/command/registry/login.go (v20.10.3)
func verifyloginOptions(cmd *cobra.Command, options *loginOptions) error {
	if options.password != "" {
		logrus.Warn("WARNING! Using --password via the CLI is insecure. Use --password-stdin.")
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

// Code from github.com/cli/cli/command/registry.go (v20.10.3)
// GetDefaultAuthConfig gets the default auth config given a serverAddress
// If credentials for given serverAddress exists in the credential store, the configuration will be populated with values in it
func GetDefaultAuthConfig(checkCredStore bool, serverAddress string, isDefaultRegistry bool) (*types.AuthConfig, error) {
	var authconfig = dockercliconfigtypes.AuthConfig{}
	if checkCredStore {
		dockerConfigFile, err := dockercliconfig.Load("")
		if err != nil {
			return nil, err
		}
		authconfig, err = dockerConfigFile.GetAuthConfig(serverAddress)
		if err != nil {
			return nil, err
		}
	}
	authconfig.ServerAddress = serverAddress
	authconfig.IdentityToken = ""
	res := types.AuthConfig(authconfig)
	return &res, nil
}

// Code from github.com/cli/cli/command/registry/login.go
func loginClientSide(ctx context.Context, cmd *cobra.Command, auth types.AuthConfig) (registrytypes.AuthenticateOKBody, error) {

	var insecureRegistries []string
	insecureRegistry, err := cmd.Flags().GetBool("insecure-registry")
	if err != nil {
		return registrytypes.AuthenticateOKBody{}, err
	}
	if insecureRegistry {
		insecureRegistries = append(insecureRegistries, auth.ServerAddress)
	}
	svc, err := registry.NewService(registry.ServiceOptions{
		InsecureRegistries: insecureRegistries,
	})

	if err != nil {
		return registrytypes.AuthenticateOKBody{}, err
	}

	userAgent := fmt.Sprintf("Docker-Client/nerdctl-%s (%s)", version.Version, runtime.GOOS)

	status, token, err := svc.Auth(ctx, &auth, userAgent)

	return registrytypes.AuthenticateOKBody{
		Status:        status,
		IdentityToken: token,
	}, err
}

func ConfigureAuthentification(authConfig *types.AuthConfig, options *loginOptions) error {
	authConfig.Username = strings.TrimSpace(authConfig.Username)
	if options.username = strings.TrimSpace(options.username); options.username == "" {
		options.username = authConfig.Username
	}

	if options.username == "" {

		fmt.Print("Enter Username: ")
		_, err := fmt.Scanf("%s", &options.username)
		if err != nil {
			return fmt.Errorf("error: Username is Required")
		}
	}

	if options.password == "" {

		fmt.Print("Enter Password: ")
		pwd, err := readPassword()
		fmt.Println()
		if err != nil {
			return err
		}
		options.password = pwd
	}

	if options.password == "" {
		return fmt.Errorf("password is Required")
	}

	authConfig.Username = options.username
	authConfig.Password = options.password

	return nil
}
