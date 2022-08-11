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

	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	dockerconfig "github.com/containerd/containerd/remotes/docker/config"
	dockercliconfig "github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/credentials"
	dockercliconfigtypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/registry"
	"github.com/sirupsen/logrus"
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
	var ss []string
	if len(orig) == 0 {
		logrus.Debug("no hosts dir was specified")
	}
	for _, v := range orig {
		if _, err := os.Stat(v); err == nil {
			logrus.Debugf("Found hosts dir %q", v)
			ss = append(ss, v)
		} else {
			if errors.Is(err, os.ErrNotExist) {
				logrus.WithError(err).Debugf("Ignoring hosts dir %q", v)
			} else {
				logrus.WithError(err).Warnf("Ignoring hosts dir %q", v)
			}
		}
	}
	return func(o *opts) {
		o.hostsDirs = ss
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

	ho.HostDir = func(s string) (string, error) {
		for _, hostsDir := range o.hostsDirs {
			found, err := dockerconfig.HostDirFromRoot(hostsDir)(s)
			if (err != nil && !errdefs.IsNotFound(err)) || (found != "") {
				return found, err
			}
		}
		return "", nil
	}

	if o.authCreds != nil {
		ho.Credentials = o.authCreds
	} else {
		if authCreds, err := NewAuthCreds(refHostname); err != nil {
			return nil, err
		} else {
			ho.Credentials = authCreds
		}
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
	// Load does not raise an error on ENOENT
	dockerConfigFile, err := dockercliconfig.Load("")
	if err != nil {
		return nil, err
	}

	// DefaultHost converts "docker.io" to "registry-1.docker.io",
	// which is wanted  by credFunc .
	credFuncExpectedHostname, err := docker.DefaultHost(refHostname)
	if err != nil {
		return nil, err
	}

	var credFunc AuthCreds

	authConfigHostnames := []string{refHostname}
	if refHostname == "docker.io" || refHostname == "registry-1.docker.io" {
		// "docker.io" appears as ""https://index.docker.io/v1/" in ~/.docker/config.json .
		// Unlike other registries, we have to pass the full URL to GetAuthConfig.
		authConfigHostnames = append([]string{registry.IndexServer}, refHostname)
	}

	for _, authConfigHostname := range authConfigHostnames {
		// GetAuthConfig does not raise an error on ENOENT
		ac, err := dockerConfigFile.GetAuthConfig(authConfigHostname)
		if err != nil {
			logrus.WithError(err).Warnf("cannot get auth config for authConfigHostname=%q (refHostname=%q)",
				authConfigHostname, refHostname)
		} else {
			// When refHostname is "docker.io":
			// - credFuncExpectedHostname: "registry-1.docker.io"
			// - credFuncArg:              "registry-1.docker.io"
			// - authConfigHostname:       "https://index.docker.io/v1/" (registry.IndexServer)
			// - ac.ServerAddress:         "https://index.docker.io/v1/".
			if !isAuthConfigEmpty(ac) {
				if authConfigHostname == registry.IndexServer {
					if ac.ServerAddress != "" && ac.ServerAddress != registry.IndexServer {
						return nil, fmt.Errorf("expected ac.ServerAddress (%q) to be %q", ac.ServerAddress, registry.IndexServer)
					}
				} else if ac.ServerAddress == "" {
					// This can happen with Amazon ECR: https://github.com/containerd/nerdctl/issues/733
					logrus.Debugf("failed to get ac.ServerAddress for authConfigHostname=%q (refHostname=%q)",
						authConfigHostname, refHostname)
				} else {
					acsaHostname := credentials.ConvertToHostname(ac.ServerAddress)
					if acsaHostname != authConfigHostname {
						return nil, fmt.Errorf("expected the hostname part of ac.ServerAddress (%q) to be authConfigHostname=%q, got %q",
							ac.ServerAddress, authConfigHostname, acsaHostname)
					}
				}

				if ac.RegistryToken != "" {
					// Even containerd/CRI does not support RegistryToken as of v1.4.3,
					// so, nobody is actually using RegistryToken?
					logrus.Warnf("ac.RegistryToken (for %q) is not supported yet (FIXME)", authConfigHostname)
				}

				credFunc = func(credFuncArg string) (string, string, error) {
					// credFuncArg should be like "registry-1.docker.io"
					if credFuncArg != credFuncExpectedHostname {
						return "", "", fmt.Errorf("expected credFuncExpectedHostname=%q (refHostname=%q), got credFuncArg=%q",
							credFuncExpectedHostname, refHostname, credFuncArg)
					}
					if ac.IdentityToken != "" {
						return "", ac.IdentityToken, nil
					}
					return ac.Username, ac.Password, nil
				}
				break
			}
		}
	}
	// credsFunc can be nil here
	return credFunc, nil
}

func isAuthConfigEmpty(ac dockercliconfigtypes.AuthConfig) bool {
	if ac.IdentityToken != "" || ac.Username != "" || ac.Password != "" || ac.RegistryToken != "" {
		return false
	}
	return true
}
