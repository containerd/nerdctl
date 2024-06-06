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
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/login"
	"github.com/containerd/nerdctl/v2/pkg/nerderr"
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

func processLoginOptions(cmd *cobra.Command) (*types.LoginCommandOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return nil, err
	}

	username, err := cmd.Flags().GetString("username")
	if err != nil {
		return nil, err
	}
	password, err := cmd.Flags().GetString("password")
	if err != nil {
		return nil, err
	}
	passwordStdin, err := cmd.Flags().GetBool("password-stdin")
	if err != nil {
		return nil, err
	}

	if strings.Contains(username, ":") {
		return nil, errors.New("username cannot contain colons")
	}

	if password != "" {
		log.L.Warn("WARNING! Using --password via the CLI is insecure. Use --password-stdin.")
		if passwordStdin {
			return nil, errors.New("--password and --password-stdin are mutually exclusive")
		}
	}

	if passwordStdin {
		if username == "" {
			return nil, errors.New("must provide --username with --password-stdin")
		}

		contents, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, err
		}

		password = strings.TrimSpace(string(contents))
	}

	return &types.LoginCommandOptions{
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

	stdo := cmd.OutOrStdout()

	// Warnings may be returned with or without an error
	warnings, err := login.Login(cmd.Context(), options, stdo)

	// Note that we are ignoring Fprintln errors here, as we do not want to return BEFORE the main error is handled
	for _, warning := range warnings {
		_, _ = fmt.Fprintln(stdo, warning)
	}

	switch err {
	case nil:
		_, fErr := fmt.Fprintln(cmd.OutOrStdout(), "Login Succeeded")
		return fErr
	case nerderr.ErrSystemIsBroken:
		log.L.Error("Your system is misconfigured or in a broken state. Probably your hosts.toml files have error, or your ~/.docker/config.json file is hosed")
	case nerderr.ErrInvalidArgument:
		log.L.Error("Invalid arguments provided")
	case nerderr.ErrServerIsMisbehaving:
		log.L.Error("The registry server you are trying to log into is possibly misconfigured, or otherwise misbehaving")
	case login.ErrCredentialsCannotBeRead:
		log.L.Error("Nerdctl cannot login without a username and password")
	case login.ErrConnectionFailed:
		log.L.Error("Failed establishing a connection. There was a DNS, TCP, or TLS issue preventing nerdctl from talking to the registry")
	case login.ErrAuthenticationFailure:
		log.L.Error("Authentication failed. Provided credentials were refused by the registry")
	}
	/*
		var dnsErr *net.DNSError
		var sysCallErr *os.SyscallError
		var opErr *net.OpError
		var urlErr *url.Error
		var tlsErr *tls.CertificateVerificationError
		// Providing understandable feedback to user for specific errors
		if errors.Is(err, login.ErrLoginCredentialsCannotBeRead) {
			log.L.Errorf("Unable to read credentials from docker store (~/.docker/config.json or credentials helper). Please manually inspect.")
		} else if errors.Is(err, login.ErrLoginCredentialsCannotBeWritten) {
			log.L.Errorf("Though the login was succesfull, credentials could not be saved to the docker store (~/.docker/config.json or credentials helper). Please manually inspect.")
		} else if errors.Is(err, login.ErrLoginCredentialsRefused) {
			log.L.Errorf("Unable to login with the provided credentials")
		} else if errors.As(err, &dnsErr) {
			if dnsErr.IsNotFound && !dnsErr.IsTemporary {
				// donotresolveeverever.foo
				log.L.Errorf("domain name %q is unknown to your DNS server %q (hint: is the domain name spelled correctly?)", dnsErr.Name, dnsErr.Server)
			} else if dnsErr.Timeout() {
				// resolve using an unreachable DNS server
				log.L.Errorf("unable to get a timely response from your DNS server %q (hint: is your DNS configuration ok?)", dnsErr.Server)
			} else {
				debErr, _ := json.Marshal(dnsErr)
				log.L.Errorf("non-specific DNS resolution error (timeout: %t):\n%s", dnsErr.Timeout(), string(debErr))
			}
		} else if errors.Is(err, http.ErrSchemeMismatch) {
			log.L.Errorf("the server does not speak https")
		} else if errors.As(err, &sysCallErr) {
			if sysCallErr.Syscall == "connect" {
				// Connect error - no way to reach that server, or server dropped us immediately
				log.L.Error("failed connecting to server")
			} else {
				debErr, _ := json.Marshal(sysCallErr)
				log.L.Errorf("non-specific syscall error (timeout: %t):\n%s", sysCallErr.Timeout(), string(debErr))
			}
		} else if errors.As(err, &opErr) {
			// Typically a tcp timeout
			if opErr.Timeout() {
				log.L.Errorf("timeout trying to connect to server)")
			} else {
				debErr, _ := json.Marshal(opErr)
				log.L.Errorf("non-specific tcp error:\n%s", string(debErr))
			}
		} else if errors.As(err, &tlsErr) {
			log.L.Debugf("server certificate verification error")
		} else if errors.As(err, &urlErr) {
			// Typically a TLS handshake timeout
			if urlErr.Timeout() {
				log.L.Debugf("server timeout while awaiting response")
			} else {
				debErr, _ := json.Marshal(urlErr)
				log.L.Errorf("non-specific server error:\n%s", string(debErr))
			}
		} else if errors.Is(err, login.ErrTooManyRedirects) {
			log.L.Errorf("too many redirects sent back by server - it is likely misconfigured")
		} else if errors.Is(err, login.ErrRedirectAuthorizerError) {
			log.L.Errorf("server is redirecting to a different location and credentials are not going to be sent there - " +
				"server is either misconfigured, or there is a security problem")
		} else if errors.Is(err, login.ErrAuthorizerError) {
			log.L.Errorf("credentials cannot be sent to that server - " +
				"server is possibly misconfigured, or there is a security problem")
			//} else {
			// log.L.Error("non-specific error")
		}

	*/

	return err
}
