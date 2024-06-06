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
	"errors"
	"os"

	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/dockerutil"
	"github.com/containerd/nerdctl/v2/pkg/nerderr"
)

var PushTracker = docker.NewInMemoryTracker()

type opts struct {
	plainHTTP       bool
	skipVerifyCerts bool
	hostsDirs       []string
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
		log.L.Debug("no hosts dir was specified")
	}
	for _, v := range orig {
		if _, err := os.Stat(v); err == nil {
			log.L.Debugf("Found hosts dir %q", v)
			ss = append(ss, v)
		} else {
			if errors.Is(err, os.ErrNotExist) {
				log.L.WithError(err).Debugf("Ignoring hosts dir %q", v)
			} else {
				log.L.WithError(err).Warnf("Ignoring hosts dir %q", v)
			}
		}
	}
	return func(o *opts) {
		o.hostsDirs = ss
	}
}

type ResolverOptions struct {
	Insecure         bool
	ExplicitInsecure bool
	HostsDirs        []string
}

func New(ctx context.Context, refHostname string, options *ResolverOptions) (remotes.Resolver, error) {
	// Get a credentialStore (does not error on ENOENT).
	// If it errors, it is a hard filesystem error or a JSON parsing error for an existing credentials file,
	// and login in that context does not make sense as we will not be able to save anything, so, just stop here.
	credentialsStore, err := dockerutil.New("")
	if err != nil {
		return nil, errors.Join(nerderr.ErrSystemIsBroken, err)
	}

	// Get a resolver with requested options
	resolver, err := dockerutil.NewResolver(refHostname, credentialsStore, &dockerutil.ResolveOptions{
		Insecure:         options.Insecure,
		ExplicitInsecure: options.ExplicitInsecure,
		HostsDirs:        options.HostsDirs,
	})

	resolverOpts := docker.ResolverOptions{
		Tracker: PushTracker,
		Hosts: func(string) ([]docker.RegistryHost, error) {
			return append(resolver.GetHosts(), resolver.GetServer()), nil
		},
	}

	return docker.NewResolver(resolverOpts), nil
}
