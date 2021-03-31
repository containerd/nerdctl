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
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"syscall"

	"github.com/containerd/nerdctl/pkg/version"
	dockercliconfig "github.com/docker/cli/cli/config"
	clitypes "github.com/docker/cli/cli/config/types"
	dockercliconfigtypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	registrytypes "github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/registry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh/terminal"
)

type loginOptions struct {
	serverAddress string
	username      string
	password      string
	passwordStdin bool
}

var options = new(loginOptions)

var loginCommand = &cli.Command{
	Name:      "login",
	Usage:     "Log in to a Docker registry",
	ArgsUsage: "[flags] [SERVER]",
	// customized function from runLogin function in github.com/docker/cli/cli/command/registry/login.go (v20.10.3)
	Action: func(clicontext *cli.Context) error {
		options.serverAddress = clicontext.Args().First()

		if err := verifyloginOptions(clicontext, options); err != nil {
			return err
		}

		var serverAddress string

		if options.serverAddress == "" {
			serverAddress = registry.IndexServer
		} else {
			serverAddress = options.serverAddress
		}

		var response registrytypes.AuthenticateOKBody
		_, ctx, _, _ := newClient(clicontext)
		isDefaultRegistry := serverAddress == registry.IndexServer
		authConfig, err := GetDefaultAuthConfig(clicontext, options.username == "" && options.password == "", serverAddress, isDefaultRegistry)
		if &authConfig == nil {
			authConfig = &types.AuthConfig{}
		}
		if err == nil && authConfig.Username != "" && authConfig.Password != "" {
			//login With StoreCreds
			response, err = loginClientSide(ctx, *authConfig)
		}

		if err != nil || authConfig.Username == "" || authConfig.Password == "" {
			err = ConfigureAuthentification(clicontext, authConfig, options)
			if err != nil {
				return err
			}

			response, err = loginClientSide(ctx, *authConfig)
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
			return errors.Errorf("error saving credentials: %v", err)
		}

		if response.Status != "" {
			fmt.Fprintln(clicontext.App.Writer, response.Status)
		}

		return nil
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "username",
			Aliases:     []string{"u"},
			Usage:       "Username",
			Destination: &options.username,
		},
		&cli.StringFlag{
			Name:        "password",
			Aliases:     []string{"p"},
			Usage:       "Password",
			Destination: &options.password,
		},
		&cli.BoolFlag{
			Name:        "password-stdin",
			Usage:       "Take the password from stdin",
			Destination: &options.passwordStdin,
		},
	},
}

//copied from github.com/docker/cli/cli/command/registry/login.go (v20.10.3)
func verifyloginOptions(clicontext *cli.Context, options *loginOptions) error {
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

		contents, err := ioutil.ReadAll(clicontext.App.Reader)
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
func GetDefaultAuthConfig(clicontext *cli.Context, checkCredStore bool, serverAddress string, isDefaultRegistry bool) (*types.AuthConfig, error) {
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
func loginClientSide(ctx context.Context, auth types.AuthConfig) (registrytypes.AuthenticateOKBody, error) {
	svc, err := registry.NewService(registry.ServiceOptions{})
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

func ConfigureAuthentification(clicontext *cli.Context, authConfig *types.AuthConfig, options *loginOptions) error {
	authConfig.Username = strings.TrimSpace(authConfig.Username)
	if options.username = strings.TrimSpace(options.username); options.username == "" {
		options.username = authConfig.Username
	}

	if options.username == "" {
		return errors.Errorf("error: Username is Required")
	}

	if options.password == "" {

		fmt.Print("Enter Password: ")
		var fd int
		if terminal.IsTerminal(syscall.Stdin) {
			fd = syscall.Stdin
		} else {
			tty, err := os.Open("/dev/tty")
			if err != nil {
				return errors.Wrap(err, "error allocating terminal")
			}
			defer tty.Close()
			fd = int(tty.Fd())
		}
		bytePassword, err := terminal.ReadPassword(fd)
		if err != nil {
			return errors.Wrap(err, "error reading password")
		}
		options.password = string(bytePassword)
	}

	if options.password == "" {
		return errors.Errorf("password is Required")
	}

	authConfig.Username = options.username
	authConfig.Password = options.password

	return nil
}
