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
	"errors"
	"fmt"
	"github.com/containerd/log"
	"net"
	"net/url"
	"strings"
)

// Parse will return a normalized Docker Registry url from the provided string address (with or without scheme and port)
func Parse(address string) (*RegistryURL, error) {
	log.L.Debugf("parsing docker registry URL %q", address)
	var err error
	// No address or address as docker.io? Default to standardized index
	if address == "" || address == "docker.io" {
		log.L.Debugf("normalized to %q", dockerIndexServer)
		address = dockerIndexServer
	}
	// If it has no scheme, slap one just so we can parse
	if !strings.Contains(address, "://") {
		address = fmt.Sprintf("%s://%s", schemeHTTPS, address)
	}
	// Parse it
	u, err := url.Parse(address)
	if err != nil {
		log.L.Debug("unparsable - giving up")
		return nil, errors.Join(ErrUnparsableURL, err)
	}
	sch := scheme(u.Scheme)
	// Scheme is entirely disregarded anyhow, so, just drop it all and set to https
	if sch == schemeHTTP {
		log.L.Debug("changing http to https")
		u.Scheme = string(schemeHTTPS)
	} else if sch != schemeHTTPS && sch != schemeNerdctlExperimental {
		log.L.Debugf("unrecognized scheme %q", sch)
		// Docker is wildly buggy when it comes to non-http schemes. Being more defensive.
		return nil, ErrUnsupportedScheme
	}
	// If it has no port, add the standard port explicitly
	if u.Port() == "" {
		log.L.Debug("adding standard port")
		u.Host = u.Hostname() + ":" + standardHTTPSPort
	}
	reg := &RegistryURL{URL: *u}
	queryParams := u.Query()
	nsQuery := queryParams.Get(namespaceQueryParameter)
	if nsQuery != "" {
		log.L.Debugf("this is a namespaced url, parsing namespace %q", nsQuery)
		reg.Namespace, err = Parse(nsQuery)
		if err != nil {
			return nil, err
		}
	}
	return reg, nil
}

// RegistryURL is a struct that represents a registry namespace or host, meant specifically to deal with
// credentials storage and retrieval inside Docker config file.
type RegistryURL struct {
	url.URL
	Namespace *RegistryURL
}

// CanonicalIdentifier returns the identifier expected to be used to save credentials to docker auth config
func (rn *RegistryURL) CanonicalIdentifier() string {
	log.L.Debugf("retrieving canonical identifier for %q", rn.URL.String())
	// If it is the docker index over https, port 443, on the /v1/ path, we use the docker fully qualified identifier
	if rn.Scheme == string(schemeHTTPS) && rn.Hostname() == "index.docker.io" && rn.Path == "/v1/" && rn.Port() == standardHTTPSPort ||
		rn.URL.String() == dockerIndexServer {
		log.L.Debugf("assimilated to docker %q", dockerIndexServer)
		return dockerIndexServer
	}
	// Otherwise, for anything else, we use the hostname+port part
	identifier := rn.Host
	// If this is a namespaced entry, wrap it, and slap the path as well, as hosts can be non-compliant
	if rn.Namespace != nil {
		log.L.Debug("namespaced identifier")
		identifier = fmt.Sprintf("%s://%s/host/%s%s", schemeNerdctlExperimental, rn.Namespace.CanonicalIdentifier(), identifier, rn.Path)
	}
	log.L.Debugf("final value: %q", identifier)
	return identifier
}

// AllIdentifiers returns a list of identifiers that may have been used to save credentials,
// accounting for legacy formats including scheme, with and without ports
func (rn *RegistryURL) AllIdentifiers() []string {
	canonicalID := rn.CanonicalIdentifier()
	fullList := []string{
		// This is rn.Host, and always have a port (see parsing)
		canonicalID,
	}
	// If the canonical identifier points to Docker Hub, or is one of our experimental ids, there is no alternative / legacy id
	if canonicalID == dockerIndexServer || rn.Namespace != nil {
		return fullList
	}

	// Docker behavior: if the domain was index.docker.io over 443, we are allowed to additionally read the canonical
	// docker credentials
	if rn.Hostname() == "index.docker.io" && rn.Port() == standardHTTPSPort {
		fullList = append(fullList, dockerIndexServer)
	}

	// Add legacy variants
	fullList = append(fullList,
		fmt.Sprintf("%s://%s", schemeHTTPS, rn.Host),
		fmt.Sprintf("%s://%s", schemeHTTP, rn.Host),
	)

	// Note that docker does not try to be smart wrt explicit port vs. implied port
	// If standard port, allow retrieving credentials from the variant without a port as well
	if rn.Port() == standardHTTPSPort {
		fullList = append(
			fullList,
			rn.Hostname(),
			fmt.Sprintf("%s://%s", schemeHTTPS, rn.Hostname()),
			fmt.Sprintf("%s://%s", schemeHTTP, rn.Hostname()),
		)
	}

	return fullList
}

func (rn *RegistryURL) IsLocalhost() bool {
	// Containerd exposes both a IsLocalhost and a MatchLocalhost method
	// There does not seem to be a clear reason for the duplication, nor the differences in implementation.
	// Either way, they both reparse the host with net.SplitHostPort, which is unnecessary
	return rn.Hostname() == "localhost" || net.ParseIP(rn.Hostname()).IsLoopback()
}
