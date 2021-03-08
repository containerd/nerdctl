/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"crypto/tls"
	"net"
	"net/http"
	"net/url"

	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	dockercliconfig "github.com/docker/cli/cli/config"
	dockercliconfigtypes "github.com/docker/cli/cli/config/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type opts struct {
	plainHTTP       bool
	skipVerifyCerts bool
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

// New instantiates a resolver using $DOCKER_CONFIG/config.json .
//
// $DOCKER_CONFIG defaults to "~/.docker".
//
// refHostname is like "docker.io".
func New(refHostname string, optFuncs ...Opt) (remotes.Resolver, error) {
	var o opts
	for _, of := range optFuncs {
		of(&o)
	}
	var authzOpts []docker.AuthorizerOpt
	if authCreds, err := NewAuthCreds(refHostname); err != nil {
		return nil, err
	} else {
		authzOpts = append(authzOpts, docker.WithAuthCreds(authCreds))
	}
	authz := docker.NewDockerAuthorizer(authzOpts...)
	plainHTTPFunc := docker.MatchLocalhost
	if o.plainHTTP {
		plainHTTPFunc = docker.MatchAllHosts
	}
	regOpts := []docker.RegistryOpt{
		docker.WithAuthorizer(authz),
		docker.WithPlainHTTP(plainHTTPFunc),
	}
	if o.skipVerifyCerts {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
		client := &http.Client{
			Transport: tr,
		}
		regOpts = append(regOpts, docker.WithClient(client))
	}
	resovlerOpts := docker.ResolverOptions{
		Hosts: docker.ConfigureDefaultRegistries(regOpts...),
	}
	resolver := docker.NewResolver(resovlerOpts)
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
		// GetAuthConfig takes the hostname part as the argument: "index.docker.io"
		authConfigHostnames = append([]string{"index.docker.io"}, refHostname)
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
			// - authConfigHostname:       "index.docker.io"
			// - ac.ServerAddress:         "https://index.docker.io/v1/".
			if !isAuthConfigEmpty(ac) {
				if ac.ServerAddress == "" {
					// Can this happen?
					logrus.Warnf("failed to get ac.ServerAddress for authConfigHostname=%q (refHostname=%q)",
						authConfigHostname, refHostname)
				} else {
					acsaURL, err := url.Parse(ac.ServerAddress)
					if err != nil {
						return nil, errors.Wrapf(err, "failed to parse ac.ServerAddress %q", ac.ServerAddress)
					}
					acsaHostname := acsaURL.Hostname()
					if acsaPort := acsaURL.Port(); acsaPort != "" {
						acsaHostname = net.JoinHostPort(acsaHostname, acsaPort)
					}
					if acsaHostname != authConfigHostname {
						return nil, errors.Errorf("expected the hostname part of ac.ServerAddress (%q) to be authConfigHostname=%q, got %q",
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
						return "", "", errors.Errorf("expected credFuncExpectedHostname=%q (refHostname=%q), got credFuncArg=%q",
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
