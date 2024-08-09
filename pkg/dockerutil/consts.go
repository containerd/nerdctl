package dockerutil

import "errors"

type scheme string

const (
	standardHTTPSPort        = "443"
	schemeHTTP        scheme = "http"
	schemeHTTPs       scheme = "https"
	// schemeNerdctlExperimental is currently provisional, to unlock namespace based host authentication
	// This may change or break without notice, and you should have no expectations that credentials saved like that
	// will be supported in the future
	schemeNerdctlExperimental scheme = "nerdctl-experimental"
	// See https://github.com/moby/moby/blob/v27.1.1/registry/config.go#L42-L48
	// especially Sebastiaan comments on future domain consolidation
	dockerIndexServer = "https://index.docker.io/v1/"
	// The query parameter that containerd will slap on namespaced hosts
	namespaceQueryParameter = "ns"
)

// Errors returned by `Parse`
var (
	ErrUnparsableURL     = errors.New("unparsable URL")
	ErrUnsupportedScheme = errors.New("unsupported scheme")
)

// Errors returned by the credentials store
var (
	ErrUnableToInstantiate = errors.New("unable to instantiate docker credentials store")
	ErrUnableToErase       = errors.New("unable to erase credentials")
	ErrUnableToStore       = errors.New("unable to store credentials")
	ErrUnableToRetrieve    = errors.New("unable to retrieve credentials")
)

// Errors returned by the Resolver
var (
	ErrNoHostsForNamespace    = errors.New("no hosts found for registry namespace")
	ErrNoSuchHostForNamespace = errors.New("no such host for registry namespace")
)
