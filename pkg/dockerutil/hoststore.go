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

package dockerutil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/containerd/remotes/docker/config"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/nerderr"
)

// validateDirectories inspect a slice of strings and returns the ones that are valid readable directories
func validateDirectories(orig []string) []string {
	ss := []string{}
	for _, v := range orig {
		fi, err := os.Stat(v)
		if err != nil || !fi.IsDir() {
			if !errors.Is(err, os.ErrNotExist) {
				log.L.WithError(err).Warnf("Ignoring hosts location %q", v)
			}
			continue
		}
		ss = append(ss, v)
	}
	return ss
}

type ResolveOptions struct {
	Insecure         bool
	ExplicitInsecure bool
	HostsDirs        []string
	Username         string
	Password         string
}

type ResolverNG struct {
	// A RegistryNamespace should be obtained from Parse(registryString)
	RegistryNamespace *RegistryURL

	// server will yield the resolved server location for that namespace (implied or explicit)
	server docker.RegistryHost
	// hosts will yield any other resolved hosts for that namespace, excluding the server
	hosts []docker.RegistryHost
	// options for the resolver
	options *ResolveOptions
	// A ref to the credentialsStore that we will query for the authorizer
	credentialsStore *CredentialsStore

	// XXX reconsider
	refreshTokens map[string]string
}

func (rng *ResolverNG) GetServer() docker.RegistryHost {
	return rng.server
}

func (rng *ResolverNG) GetHosts() []docker.RegistryHost {
	return rng.hosts
}

// IdentityTokenForHost will return a potential refresh token retrieved during authentication
func (res *ResolverNG) IdentityTokenForHost(host string) string {
	return res.refreshTokens[host]
}

// IsInsecure
//func (rng *ResolverNG) IsInsecure() bool {
//	return rng.insecure || (rng.RegistryNamespace.IsLocalhost() && !rng.explicitInsecure)
//}

/*
func (rng *ResolverNG) GetResolver(ctx context.Context, tracker docker.StatusTracker, name string) remotes.Resolver {
	ro := docker.ResolverOptions{
		Tracker: tracker,
		Hosts: func(string) ([]docker.RegistryHost, error) {
			return
		},
	}
	return docker.NewResolver(ro)
}*/

func NewResolver(serverAddress string, credStore *CredentialsStore, options *ResolveOptions) (*ResolverNG, error) {
	ctx := context.Background()

	// If we cannot even parse the address, bail out
	registryURL, err := Parse(serverAddress)
	if err != nil {
		return nil, errors.Join(nerderr.ErrInvalidArgument, err)
	}

	ns := registryURL.Namespace
	if ns == nil {
		ns = registryURL
	}

	// Create a resolver with the options, for that registry namespace, and the passed credentialStore
	resolver := &ResolverNG{
		credentialsStore:  credStore,
		RegistryNamespace: ns,
		options:           options,
	}

	// Build docker host options to be used
	hostOptions := &config.HostOptions{
		// Always start with https
		// Note that doing WILL bypass some of the localhost/default port logic in containerd
		// and will make it so that we ALWAYS try https first for every host, before considering falling back to http
		// This is desirable, as containerd will otherwise prevent tls communication with localhost
		DefaultScheme: string(schemeHTTPS),
		// Credentials retrieval function
		Credentials: func(host string) (string, string, error) {
			// If we were passed an explicit username/password, we should use that
			// (if it matches the host the user expects)
			if resolver.options.Username != "" && resolver.options.Password != "" {
				if host != registryURL.Host {
					return "", "", errors.New("wrong host thing")
				}
				return resolver.options.Username, resolver.options.Password, nil
			}
			// Otherwise, retrieve from the store with that url
			servURL, err := Parse(host)
			if err != nil {
				return "", "", err
			}
			credentials, credErr := credStore.Retrieve(servURL, true)
			if credErr != nil {
				return "", "", credErr
			}

			if credentials.IdentityToken != "" {
				return "", credentials.IdentityToken, nil
			}
			if credentials.RegistryToken != "" {
				// Even containerd/CRI does not support RegistryToken as of v1.4.3,
				// so, nobody is actually using RegistryToken?
				log.L.Warnf("RegistryToken (for %q) is not supported yet (FIXME)", host)
				return "", "", errors.New("unsuported authentication method")
			}

			return credentials.Username, credentials.Password, nil
		},
		// HostDir resolution function will retrieve a host.toml file for the namespace host
		HostDir: func(host string) (string, error) {
			servURL := ns
			hostsDirs := validateDirectories(resolver.options.HostsDirs)

			// Go through the configured system location to consider for hosts.toml files
			for _, hostsDir := range hostsDirs {
				found, err := config.HostDirFromRoot(hostsDir)(servURL.Host)
				if (err != nil && !errdefs.IsNotFound(err)) || (found != "") {
					return found, err
				}
				// If not found, and the port is standard, try again without the port
				if servURL.Port() == standardHTTPSPort {
					found, err = config.HostDirFromRoot(hostsDir)(servURL.Hostname())
					if (err != nil && !errors.Is(err, errdefs.ErrNotFound)) || (found != "") {
						return found, err
					}
				}
			}
			return "", nil
		},
	}

	resolver.refreshTokens = make(map[string]string)
	// Additional authorizer opt to capture the refresh token
	onFetchRefreshToken := func(ctx context.Context, s string, req *http.Request) {
		fmt.Println("Got refresh token", s)
		// XXX add NS query / path?
		resolver.refreshTokens[req.URL.Host] = s
	}
	hostOptions.AuthorizerOpts = append(hostOptions.AuthorizerOpts, docker.WithFetchRefreshToken(onFetchRefreshToken))

	// Finally, get the list of configured hosts for that namespace
	regHosts, err := config.ConfigureHosts(ctx, *hostOptions)(resolver.RegistryNamespace.Host)

	// If there is none (eg: an existing empty hosts.toml file, which is a legit use case preventing any interaction
	// for that registry namespace), return an error
	if err == nil && len(regHosts) == 0 {
		err = ErrNoHostsForNamespace
	}

	found := false
	for _, host := range regHosts {
		log.L.Debugf("inspecting: %q (against: %q - namespace: %q)", host.Host, registryURL.Host, resolver.RegistryNamespace.Host)
		// Ensure we disable TLS verification if host is on localhost and no --insecure-registry=false has been passed
		test, _, err := net.SplitHostPort(host.Host)
		if err != nil {
			test = host.Host
		}
		if resolver.options.Insecure ||
			((test == "localhost" || net.ParseIP(test).IsLoopback()) && !resolver.options.ExplicitInsecure) {
			host.Client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
		}
		// If we are on the namespace, or if the target matches the hosts here, mark it found
		if resolver.RegistryNamespace.Host == registryURL.Host || registryURL.Host == host.Host {
			found = true
		}
	}

	if !found {
		err = ErrNoSuchHostForNamespace
	}

	// Split out the server and hosts, and retain them
	if len(regHosts) > 0 {
		resolver.server = regHosts[len(regHosts)-1]
		resolver.hosts = regHosts[0 : len(regHosts)-1]
	}

	return resolver, err
}

/*

UX

nerdctl login --namespace upregistry.com server.com


*/

/*
// A Resolver will provide a list of hosts configured with the appropriate headers and authorizers, from a RegistryURL and Credentials
type Resolver struct {
	// Insecure should reflect the value of the --insecure flag
	Insecure bool
	// ExplicitInsecure should be set to true if the --insecure flag was provided explicitly regardless of value
	ExplicitInsecure bool
	// A RegistryURL should be obtained from Parse(registryString)
	RegistryURL *RegistryURL
	// Credentials should be obtained from credentialsStore.Retrieve(RegistryURL)
	Credentials *Credentials
	// The directories to look into for hosts.toml configuration
	HostsDirs []string

	refreshTokens map[string]string
}

// IdentityTokenForHost will return a potential refresh token retrieved during authentication
func (res *Resolver) IdentityTokenForHost(host string) string {
	return res.refreshTokens[host]
}

// IsInsecure
func (res *Resolver) IsInsecure() bool {
	return res.Insecure || (res.RegistryURL.IsLocalhost() && !res.ExplicitInsecure)
}

func (res *Resolver) GetHosts(ctx context.Context) ([]docker.RegistryHost, error) {
	hostsDirs := validateDirectories(res.HostsDirs)

	// Prepare host options
	hostOptions := &config.HostOptions{
		// Always start with https
		// Note that using an explicit scheme here will bypass some of the localhost/default port logic in containerd
		// and will make it so that we ALWAYS try https first for every host, before considering falling back to http
		DefaultScheme: string(schemeHTTPS),
		// Credentials retrieval function
		Credentials: func(host string) (string, string, error) {
			// XXX remove
			fmt.Printf("Host being passed: %q %q\n", host, res.Credentials.ServerAddress)

			if res.Credentials.IdentityToken != "" {
				return "", res.Credentials.IdentityToken, nil
			}
			if res.Credentials.RegistryToken != "" {
				// Even containerd/CRI does not support RegistryToken as of v1.4.3,
				// so, nobody is actually using RegistryToken?
				log.G(ctx).Warnf("RegistryToken (for %q) is not supported yet (FIXME)", host)
			}

			return res.Credentials.Username, res.Credentials.Password, nil
		},
		// HostDir resolution function will retrieve a host.toml file for the given host
		HostDir: func(s string) (string, error) {
			for _, hostsDir := range hostsDirs {
				found, err := config.HostDirFromRoot(hostsDir)(s)
				if (err != nil && !errdefs.IsNotFound(err)) || (found != "") {
					return found, err
				}
				servURL, _ := Parse(s)
				// If not found, and the port is standard, try again without the port
				if servURL.Port() == standardHTTPSPort {
					found, err = config.HostDirFromRoot(hostsDir)(servURL.Hostname())
					if (err != nil && !errdefs.IsNotFound(err)) || (found != "") {
						return found, err
					}
				}
			}
			return "", nil
		},
	}

	// Set to insecure if asked by the user, or if it is localhost and the user did NOT set the flag explicitly to false
	if res.IsInsecure() {
		log.G(ctx).Warnf("WARNING! When using `insecure`, nerdctl will skip any verification of HTTPS certificates, and will potentially switch to plain HTTP. " +
			"This can be trivially exploited and carries significant security risks.")
		hostOptions.DefaultTLS = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	res.refreshTokens = make(map[string]string)
	// Additional authorizer opt to capture the refresh token
	onFetchRefreshToken := func(ctx context.Context, s string, req *http.Request) {
		fmt.Println("Got refresh token", s)
		res.refreshTokens[req.URL.Host] = s
	}
	hostOptions.AuthorizerOpts = append(hostOptions.AuthorizerOpts, docker.WithFetchRefreshToken(onFetchRefreshToken))

	regHosts, err := config.ConfigureHosts(ctx, *hostOptions)(res.RegistryURL.Host)
	if err == nil && len(regHosts) == 0 {
		err = ErrNoHostsForNamespace
	}

	return regHosts, err
}


*/
