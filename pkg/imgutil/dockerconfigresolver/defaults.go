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

import "errors"

type scheme string

const (
	standardHTTPSPort        = "443"
	schemeHTTP        scheme = "http"
	schemeHTTPS       scheme = "https"
	// schemeNerdctlExperimental is currently provisional, to unlock namespace based host authentication
	// This may change or break without notice, and you should have no expectations that credentials saved like that
	// will be supported in the future
	schemeNerdctlExperimental scheme = "nerdctl-experimental"
	// See https://github.com/moby/moby/blob/v27.1.1/registry/config.go#L42-L48
	//nolint:misspell
	// especially Sebastiaan comments on future domain consolidation
	dockerIndexServer = "https://index.docker.io/v1/"
	// The query parameter that containerd will slap on namespaced hosts
	namespaceQueryParameter = "ns"
)

// Errors returned by the credentials store
var (
	ErrUnableToInstantiate = errors.New("unable to instantiate docker credentials store")
	ErrUnableToErase       = errors.New("unable to erase credentials")
	ErrUnableToStore       = errors.New("unable to store credentials")
	ErrUnableToRetrieve    = errors.New("unable to retrieve credentials")
)

// Errors returned by `Parse`
var (
	ErrUnparsableURL     = errors.New("unparsable registry URL")
	ErrUnsupportedScheme = errors.New("unsupported scheme in registry URL")
)
