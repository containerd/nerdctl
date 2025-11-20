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

package dockerconfigresolver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	dockerconfig "github.com/containerd/containerd/v2/core/remotes/docker/config"
	"github.com/containerd/containerd/v2/core/transfer/registry"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/pelletier/go-toml/v2"
)

var PushTracker = docker.NewInMemoryTracker()

type opts struct {
	plainHTTP       bool
	skipVerifyCerts bool
	hostsDirs       []string
	authCreds       AuthCreds
}

// Opt for New
type Opt func(*opts)

// WithPlainHTTP enables insecure plain HTTP
func WithPlainHTTP(b bool) Opt {
	return func(o *opts) {
		o.plainHTTP = b
	}
}

// WithSkipVerifyCerts skips verifying TLS certs
func WithSkipVerifyCerts(b bool) Opt {
	return func(o *opts) {
		o.skipVerifyCerts = b
	}
}

// WithHostsDirs specifies directories like /etc/containerd/certs.d and /etc/docker/certs.d
func WithHostsDirs(orig []string) Opt {
	validDirs := validateDirectories(orig)
	return func(o *opts) {
		o.hostsDirs = validDirs
	}
}

func WithAuthCreds(ac AuthCreds) Opt {
	return func(o *opts) {
		o.authCreds = ac
	}
}

// NewHostOptions instantiates a HostOptions struct using $DOCKER_CONFIG/config.json .
//
// $DOCKER_CONFIG defaults to "~/.docker".
//
// refHostname is like "docker.io".
func NewHostOptions(ctx context.Context, refHostname string, optFuncs ...Opt) (*dockerconfig.HostOptions, error) {
	var o opts
	for _, of := range optFuncs {
		of(&o)
	}
	var ho dockerconfig.HostOptions

	ho.HostDir = func(hostURL string) (string, error) {
		regURL, err := Parse(hostURL)
		// Docker inconsistencies handling: `index.docker.io` actually expects `docker.io` for hosts.toml on the filesystem
		// See https://github.com/containerd/nerdctl/issues/3697
		// FIXME: we need to reevaluate this comparing with what docker does. What should happen for FQ images with alternate docker domains? (eg: registry-1.docker.io)
		if regURL.Hostname() == "index.docker.io" {
			regURL.Host = "docker.io"
		}

		if err != nil {
			return "", err
		}
		dir, err := hostDirsFromRoot(regURL, o.hostsDirs)
		if err != nil {
			if errors.Is(err, errdefs.ErrNotFound) {
				err = nil
			}
			return "", err
		}
		return dir, nil
	}

	if o.authCreds != nil {
		ho.Credentials = o.authCreds
	} else {
		authCreds, err := NewAuthCreds(refHostname)
		if err != nil {
			return nil, err
		}
		ho.Credentials = authCreds

	}

	if o.skipVerifyCerts {
		ho.DefaultTLS = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	if o.plainHTTP {
		ho.DefaultScheme = "http"
	} else {
		if isLocalHost, err := docker.MatchLocalhost(refHostname); err != nil {
			return nil, err
		} else if isLocalHost {
			ho.DefaultScheme = "http"
		}
	}
	if ho.DefaultScheme == "http" {
		// https://github.com/containerd/containerd/issues/9208
		ho.DefaultTLS = nil
	}
	return &ho, nil
}

// New instantiates a resolver using $DOCKER_CONFIG/config.json .
//
// $DOCKER_CONFIG defaults to "~/.docker".
//
// refHostname is like "docker.io".
func New(ctx context.Context, refHostname string, optFuncs ...Opt) (remotes.Resolver, error) {
	ho, err := NewHostOptions(ctx, refHostname, optFuncs...)
	if err != nil {
		return nil, err
	}

	resolverOpts := docker.ResolverOptions{
		Tracker: PushTracker,
		Hosts:   dockerconfig.ConfigureHosts(ctx, *ho),
	}

	resolver := docker.NewResolver(resolverOpts)
	return resolver, nil
}

// AuthCreds is for docker.WithAuthCreds
type AuthCreds func(string) (string, string, error)

// NewAuthCreds returns AuthCreds that uses $DOCKER_CONFIG/config.json .
// AuthCreds can be nil.
func NewAuthCreds(refHostname string) (AuthCreds, error) {
	// Note: does not raise an error on ENOENT
	credStore, err := NewCredentialsStore("")
	if err != nil {
		return nil, err
	}

	credFunc := func(host string) (string, string, error) {
		rHost, err := Parse(host)
		if err != nil {
			return "", "", err
		}

		ac, err := credStore.Retrieve(rHost, true)
		if err != nil {
			return "", "", err
		}

		if ac.IdentityToken != "" {
			return "", ac.IdentityToken, nil
		}

		if ac.RegistryToken != "" {
			// Even containerd/CRI does not support RegistryToken as of v1.4.3,
			// so, nobody is actually using RegistryToken?
			log.L.Warnf("ac.RegistryToken (for %q) is not supported yet (FIXME)", rHost.Host)
		}

		return ac.Username, ac.Password, nil
	}

	return credFunc, nil
}

func NewCredentialHelper(refHostname string) (registry.CredentialHelper, error) {
	authCreds, err := NewAuthCreds(refHostname)
	if err != nil {
		return nil, err
	}
	return &credentialHelper{authCreds: authCreds}, nil
}

type credentialHelper struct {
	authCreds AuthCreds
}

func (ch *credentialHelper) GetCredentials(ctx context.Context, ref, host string) (registry.Credentials, error) {
	username, secret, err := ch.authCreds(host)
	if err != nil {
		return registry.Credentials{}, err
	}
	return registry.Credentials{
		Host:     host,
		Username: username,
		Secret:   secret,
	}, nil
}

type hostFileConfig struct {
	SkipVerify *bool `toml:"skip_verify,omitempty"`
}

// CreateTmpHostsConfig creates a temporary hosts directory with hosts.toml configured for skip_verify
// Returns the temporary directory path or empty string if creation failed
func CreateTmpHostsConfig(hostname string, skipVerify bool) (string, error) {
	if !skipVerify {
		return "", nil
	}

	tempDir, err := os.MkdirTemp("", "nerdctl-hosts-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	hostDir := filepath.Join(tempDir, hostname)
	if err := os.MkdirAll(hostDir, 0755); err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to create host directory: %w", err)
	}

	config := hostFileConfig{}
	if skipVerify {
		skip := true
		config.SkipVerify = &skip
	}

	data, err := toml.Marshal(config)
	if err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to marshal hosts config: %w", err)
	}

	hostsTomlPath := filepath.Join(hostDir, "hosts.toml")
	if err := os.WriteFile(hostsTomlPath, data, 0644); err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to write hosts.toml: %w", err)
	}

	return tempDir, nil
}
