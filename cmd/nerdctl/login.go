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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/remotes/docker"
	dockerconfig "github.com/containerd/containerd/remotes/docker/config"
	ncTypes "github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/errutil"
	"github.com/containerd/nerdctl/pkg/imgutil/dockerconfigresolver"
	dockercliconfig "github.com/docker/cli/cli/config"
	dockercliconfigtypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/term"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type loginOptions struct {
	serverAddress string
	username      string
	password      string
	passwordStdin bool
}

const unencryptedPasswordWarning = `WARNING: Your password will be stored unencrypted in %s.
Configure a credential helper to remove this warning. See
https://docs.docker.com/engine/reference/commandline/login/#credentials-store
`

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

type isFileStore interface {
	IsFileStore() bool
	GetFilename() string
}

func loginAction(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		options.serverAddress = args[0]
	}
	if err := verifyloginOptions(cmd, options); err != nil {
		return err
	}

	var serverAddress string
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	if options.serverAddress == "" {
		serverAddress = dockerconfigresolver.IndexServer
	} else {
		serverAddress = options.serverAddress
	}

	var responseIdentityToken string
	ctx := cmd.Context()
	isDefaultRegistry := serverAddress == dockerconfigresolver.IndexServer

	authConfig, err := GetDefaultAuthConfig(options.username == "" && options.password == "", serverAddress, isDefaultRegistry)
	if authConfig == nil {
		authConfig = &types.AuthConfig{ServerAddress: serverAddress}
	}
	if err == nil && authConfig.Username != "" && authConfig.Password != "" {
		//login With StoreCreds
		responseIdentityToken, err = loginClientSide(ctx, cmd, globalOptions, *authConfig)
	}

	if err != nil || authConfig.Username == "" || authConfig.Password == "" {
		err = ConfigureAuthentication(authConfig, options)
		if err != nil {
			return err
		}

		responseIdentityToken, err = loginClientSide(ctx, cmd, globalOptions, *authConfig)
		if err != nil {
			return err
		}

	}

	if responseIdentityToken != "" {
		authConfig.Password = ""
		authConfig.IdentityToken = responseIdentityToken
	}

	dockerConfigFile, err := dockercliconfig.Load("")
	if err != nil {
		return err
	}

	creds := dockerConfigFile.GetCredentialsStore(serverAddress)

	store, isFile := creds.(isFileStore)
	// Display a warning if we're storing the users password (not a token) and credentials store type is file.
	if isFile && authConfig.Password != "" {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), fmt.Sprintf(unencryptedPasswordWarning, store.GetFilename()))
		if err != nil {
			return err
		}
	}

	if err := creds.Store(dockercliconfigtypes.AuthConfig(*(authConfig))); err != nil {
		return fmt.Errorf("error saving credentials: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Login Succeeded")

	return nil
}

// copied from github.com/docker/cli/cli/command/registry/login.go (v20.10.3)
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

// Code from github.com/docker/cli/cli/command (v20.10.3)
// GetDefaultAuthConfig gets the default auth config given a serverAddress
// If credentials for given serverAddress exists in the credential store, the configuration will be populated with values in it
func GetDefaultAuthConfig(checkCredStore bool, serverAddress string, isDefaultRegistry bool) (*types.AuthConfig, error) {
	if !isDefaultRegistry {
		var err error
		serverAddress, err = convertToHostname(serverAddress)
		if err != nil {
			return nil, err
		}
	}
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

func loginClientSide(ctx context.Context, cmd *cobra.Command, globalOptions ncTypes.GlobalCommandOptions, auth types.AuthConfig) (string, error) {
	host, err := convertToHostname(auth.ServerAddress)
	if err != nil {
		return "", err
	}
	var dOpts []dockerconfigresolver.Opt
	if globalOptions.InsecureRegistry {
		logrus.Warnf("skipping verifying HTTPS certs for %q", host)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(globalOptions.HostsDir))

	authCreds := func(acArg string) (string, string, error) {
		if acArg == host {
			if auth.RegistryToken != "" {
				// Even containerd/CRI does not support RegistryToken as of v1.4.3,
				// so, nobody is actually using RegistryToken?
				logrus.Warnf("RegistryToken (for %q) is not supported yet (FIXME)", host)
			}
			return auth.Username, auth.Password, nil
		}
		return "", "", fmt.Errorf("expected acArg to be %q, got %q", host, acArg)
	}

	dOpts = append(dOpts, dockerconfigresolver.WithAuthCreds(authCreds))
	ho, err := dockerconfigresolver.NewHostOptions(ctx, host, dOpts...)
	if err != nil {
		return "", err
	}
	fetchedRefreshTokens := make(map[string]string) // key: req.URL.Host
	// onFetchRefreshToken is called when tryLoginWithRegHost calls rh.Authorizer.Authorize()
	onFetchRefreshToken := func(ctx context.Context, s string, req *http.Request) {
		fetchedRefreshTokens[req.URL.Host] = s
	}
	ho.AuthorizerOpts = append(ho.AuthorizerOpts, docker.WithFetchRefreshToken(onFetchRefreshToken))
	regHosts, err := dockerconfig.ConfigureHosts(ctx, *ho)(host)
	if err != nil {
		return "", err
	}
	logrus.Debugf("len(regHosts)=%d", len(regHosts))
	if len(regHosts) == 0 {
		return "", fmt.Errorf("got empty []docker.RegistryHost for %q", host)
	}
	for i, rh := range regHosts {
		err = tryLoginWithRegHost(ctx, rh)
		if err != nil && globalOptions.InsecureRegistry && (errutil.IsErrHTTPResponseToHTTPSClient(err) || errutil.IsErrConnectionRefused(err)) {
			rh.Scheme = "http"
			err = tryLoginWithRegHost(ctx, rh)
		}
		identityToken := fetchedRefreshTokens[rh.Host] // can be empty
		if err == nil {
			return identityToken, nil
		}
		logrus.WithError(err).WithField("i", i).Error("failed to call tryLoginWithRegHost")
	}
	return "", err
}

func tryLoginWithRegHost(ctx context.Context, rh docker.RegistryHost) error {
	if rh.Authorizer == nil {
		return errors.New("got nil Authorizer")
	}
	if rh.Path == "/v2" {
		// If the path is using /v2 endpoint but lacks trailing slash add it
		// https://docs.docker.com/registry/spec/api/#detail. Acts as a workaround
		// for containerd issue https://github.com/containerd/containerd/blob/2986d5b077feb8252d5d2060277a9c98ff8e009b/remotes/docker/config/hosts.go#L110
		rh.Path = "/v2/"
	}
	u := url.URL{
		Scheme: rh.Scheme,
		Host:   rh.Host,
		Path:   rh.Path,
	}
	var ress []*http.Response
	for i := 0; i < 10; i++ {
		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			return err
		}
		for k, v := range rh.Header.Clone() {
			for _, vv := range v {
				req.Header.Add(k, vv)
			}
		}
		if err := rh.Authorizer.Authorize(ctx, req); err != nil {
			return fmt.Errorf("failed to call rh.Authorizer.Authorize: %w", err)
		}
		res, err := ctxhttp.Do(ctx, rh.Client, req)
		if err != nil {
			return fmt.Errorf("failed to call rh.Client.Do: %w", err)
		}
		ress = append(ress, res)
		if res.StatusCode == 401 {
			if err := rh.Authorizer.AddResponses(ctx, ress); err != nil && !errdefs.IsNotImplemented(err) {
				return fmt.Errorf("failed to call rh.Authorizer.AddResponses: %w", err)
			}
			continue
		}
		if res.StatusCode/100 != 2 {
			return fmt.Errorf("unexpected status code %d", res.StatusCode)
		}

		return nil
	}

	return errors.New("too many 401 (probably)")
}

func ConfigureAuthentication(authConfig *types.AuthConfig, options *loginOptions) error {
	authConfig.Username = strings.TrimSpace(authConfig.Username)
	if options.username = strings.TrimSpace(options.username); options.username == "" {
		options.username = authConfig.Username
	}

	if options.username == "" {
		fmt.Print("Enter Username: ")
		username, err := readUsername()
		if err != nil {
			return err
		}
		options.username = username
	}

	if options.username == "" {
		return fmt.Errorf("error: Username is Required")
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
		return fmt.Errorf("error: Password is Required")
	}

	authConfig.Username = options.username
	authConfig.Password = options.password

	return nil
}

func readUsername() (string, error) {
	var fd *os.File
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fd = os.Stdin
	} else {
		return "", fmt.Errorf("stdin is not a terminal (Hint: use `nerdctl login --username=USERNAME --password-stdin`)")
	}

	reader := bufio.NewReader(fd)
	username, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("error reading username: %w", err)
	}
	username = strings.TrimSpace(username)

	return username, nil
}

func convertToHostname(serverAddress string) (string, error) {
	// Ensure that URL contains scheme for a good parsing process
	if strings.Contains(serverAddress, "://") {
		u, err := url.Parse(serverAddress)
		if err != nil {
			return "", err
		}
		serverAddress = u.Host
	} else {
		u, err := url.Parse("https://" + serverAddress)
		if err != nil {
			return "", err
		}
		serverAddress = u.Host
	}

	return serverAddress, nil
}
