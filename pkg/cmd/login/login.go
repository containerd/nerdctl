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

package login

import (
	"context"
	"errors"
	"fmt"
	"github.com/containerd/nerdctl/v2/pkg/version"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/net/context/ctxhttp"

	"github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/containerd/containerd/v2/core/remotes/docker/config"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
)

const (
	redirectLimit              = 10
	maxResponsesRetries        = 5
	unencryptedPasswordWarning = `WARNING: Your password will be stored unencrypted in %s.
Configure a credential helper to remove this warning. See
https://docs.docker.com/engine/reference/commandline/login/#credentials-store
`
)

var (
	defaultNerdUserAgent = fmt.Sprintf("nerdctl/%s (os: %s)", version.GetVersion(), runtime.GOOS)

	// ErrSystemIsBroken should wrap all system-level errors (filesystem unexpected conditions, hosed files, misbehaving subsystems)
	ErrSystemIsBroken = errors.New("system error")
)

func Login(ctx context.Context, options types.LoginCommandOptions, stdout io.Writer) error {
	registryURL, err := dockerconfigresolver.Parse(options.ServerAddress)
	if err != nil {
		return err
	}

	credStore, err := dockerconfigresolver.NewCredentialsStore("")
	if err != nil {
		return err
	}

	var responseIdentityToken string

	credentials, err := credStore.Retrieve(registryURL, options.Username == "" && options.Password == "")
	credentials.IdentityToken = ""

	if err == nil && credentials.Username != "" && credentials.Password != "" {
		responseIdentityToken, err = loginClientSide(ctx, options.GOptions, registryURL, credentials)
	}

	if err != nil || credentials.Username == "" || credentials.Password == "" {
		err = promptUserForAuthentication(credentials, options.Username, options.Password, stdout)
		if err != nil {
			return err
		}

		responseIdentityToken, err = loginClientSide(ctx, options.GOptions, registryURL, credentials)
		if err != nil {
			return err
		}
	}

	if responseIdentityToken != "" {
		credentials.Password = ""
		credentials.IdentityToken = responseIdentityToken
	}

	// Display a warning if we're storing the users password (not a token) and credentials store type is file.
	storageFileLocation := credStore.FileStorageLocation(registryURL)
	if storageFileLocation != "" && credentials.Password != "" {
		_, err = fmt.Fprintln(stdout, fmt.Sprintf(unencryptedPasswordWarning, storageFileLocation))
		if err != nil {
			return err
		}
	}

	err = credStore.Store(registryURL, credentials)
	if err != nil {
		return fmt.Errorf("error saving credentials: %w", err)
	}

	_, err = fmt.Fprintln(stdout, "Login Succeeded")

	return err
}

func loginClientSide(ctx context.Context, globalOptions types.GlobalCommandOptions, registryURL *dockerconfigresolver.RegistryURL, credentials *dockerconfigresolver.Credentials) (string, error) {
	host := registryURL.Host
	var dOpts []dockerconfigresolver.Opt
	if globalOptions.InsecureRegistry {
		log.G(ctx).Warnf("skipping verifying HTTPS certs for %q", host)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(globalOptions.HostsDir))

	authCreds := func(acArg string) (string, string, error) {
		if acArg == host {
			if credentials.RegistryToken != "" {
				// Even containerd/CRI does not support RegistryToken as of v1.4.3,
				// so, nobody is actually using RegistryToken?
				log.G(ctx).Warnf("RegistryToken (for %q) is not supported yet (FIXME)", host)
			}
			return credentials.Username, credentials.Password, nil
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
	regHosts, err := config.ConfigureHosts(ctx, *ho)(host)
	if err != nil {
		return "", err
	}
	log.G(ctx).Debugf("len(regHosts)=%d", len(regHosts))
	if len(regHosts) == 0 {
		return "", fmt.Errorf("got empty []docker.RegistryHost for %q", host)
	}
	for i, rh := range regHosts {
		err = tryLoginWithRegHost(ctx, rh)
		if err != nil && globalOptions.InsecureRegistry && (errors.Is(err, http.ErrSchemeMismatch) || errutil.IsErrConnectionRefused(err)) {
			rh.Scheme = "http"
			err = tryLoginWithRegHost(ctx, rh)
		}
		identityToken := fetchedRefreshTokens[rh.Host] // can be empty
		if err == nil {
			return identityToken, nil
		}
		log.G(ctx).WithError(err).WithField("i", i).Error("failed to call tryLoginWithRegHost")
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

// Login will try to authenticate with the provided LoginCommandOptions, retrieving credentials and hosts.toml configuration
// for the provided registry namespace, possibly prompting the user for credentials.
// It may return the following errors:
// - ErrSystemIsBroken: this should rarely happen, and is a symptom of a borked docker credentials store or broken hosts.toml configuration
// - ErrInvalidArgument: provided namespace cannot be parsed, uses an invalid scheme, or is impossible to login because of hosts.toml configuration
// - ErrCredentialsCannotBeRead: terminal error, or user did not provide credentials when prompted
// - ErrConnectionFailed: dns, tcp or tls class of errors
// - ErrServerIsMisbehaving: any server side error, 50x status code, redirect misconfiguration, etc
// - ErrAuthenticationFailure: wrong credentials
// See details about these errors for more fine-grained wrapped errors
// Additionally, Login will return a slice of strings containing warnings that should be displayed to the user
func Login(ctx context.Context, options *types.LoginCommandOptions, stdout io.Writer) ([]string, error) {
	warnings := []string{}

	// Get a credentialStore (does not error on ENOENT).
	// If it errors, it is a hard filesystem error or a JSON parsing error for an existing credentials file,
	// and login in that context does not make sense as we will not be able to save anything, so, just stop here.
	credentialsStore, err := dockerconfigresolver.NewCredentialsStore("")
	if err != nil {
		return warnings, errors.Join(ErrSystemIsBroken, err)
	}

	// Get a resolver, with the requested options
	resolver, err := dockerutil.NewResolver(options.ServerAddress, credentialsStore, &dockerutil.ResolveOptions{
		Insecure:         options.GOptions.InsecureRegistry,
		ExplicitInsecure: options.GOptions.ExplicitInsecureRegistry,
		HostsDirs:        options.GOptions.HostsDir,
		Username:         options.Username,
		Password:         options.Password,
	})

	// Handle possible errors
	if errors.Is(err, dockerutil.ErrNoHostsForNamespace) {
		return warnings, errors.Join(nerderr.ErrInvalidArgument, err)
	} else if errors.Is(err, dockerutil.ErrNoSuchHostForNamespace) {
		return warnings, errors.Join(nerderr.ErrInvalidArgument, err)
	} else if err != nil {
		return warnings, errors.Join(nerderr.ErrSystemIsBroken, err)
	}

	// Warn the user that schemes are meaningless, especially http://, if they used it
	if strings.HasPrefix(options.ServerAddress, "http://") {
		log.L.Warnf("Login to the server hosted at %q will ignore the provided scheme (http) and will connect using https, "+
			"unless you explicitly request to use it in insecure mode with the --insecure-registry flag", resolver.RegistryNamespace.Host)
	}

	// Get the resolved server and hosts
	registryHosts := resolver.GetHosts()
	registryServer := resolver.GetServer()

	// Ensure we have a port for it
	if _, _, err = net.SplitHostPort(registryServer.Host); err != nil {
		registryServer.Host = net.JoinHostPort(registryServer.Host, "443")
	}

	// If the passed-in ServerAddress is the namespace, and the server does not resolve to that, we are stopping now
	// If it is not the namespace, we already know it exists and is valid, we just don't know which registryHost object it is
	// ... if the server (which is either the explicit `server` section, or the implied host) does
	// NOT match the namespace we are asked to log into, we are stopping here.
	if registryServer.Host != resolver.RegistryNamespace.Host {
		warnings = append(
			warnings,
			fmt.Sprintf("The registry namespace (%q) has a hosts.toml configuration that resolves to a different server host (%q).\n"+
				"We cannot login to that registry namespace directly. If you are expecting the configured endpoints to be authenticated, please login to them individually with:",
				resolver.RegistryNamespace.Host,
				registryServer.Host,
			))
		for _, regHost := range registryHosts {
			warnings = append(
				warnings,
				fmt.Sprintf("  nerdctl login %s%s?ns=%s", regHost.Host, regHost.Path, resolver.RegistryNamespace.Host),
			)
		}
		warnings = append(
			warnings,
			fmt.Sprintf("  nerdctl login %s%s?ns=%s", registryServer.Host, registryServer.Path, resolver.RegistryNamespace.Host),
		)
		return warnings, nerderr.ErrInvalidArgument
	}

	// var responseIdentityHost string
	// var responseIdentityToken string

	// Query the credentialStore, but only force a lookup if both username and password have not been provided explicitly
	// fmt.Println("DUUUF", resolver.RegistryNamespace.CanonicalIdentifier())
	credentials, credStoreErr := credentialsStore.Retrieve(resolver.RegistryNamespace, options.Username == "" && options.Password == "")

	// We should downgrade to http IF
	// we have --insecure-registry (or --insecure-registry=true)
	// OR
	// we are on localhost AND we do NOT have --insecure-registry=false
	insecureLogin := options.GOptions.InsecureRegistry || (resolver.RegistryNamespace.IsLocalhost() && !options.GOptions.ExplicitInsecureRegistry)

	var queryErr error
	// If `Retrieve` did not error and there is a username and password from the store, then try to log in with that
	if credStoreErr == nil && credentials.Username != "" && credentials.Password != "" {
		queryErr = login(ctx, registryServer, insecureLogin)
		// Note: failing to authenticate here with invalid (stored) credentials will NOT delete said saved credentials
	}

	// If the above failed, or if we had an error from `Retrieve`, or we did not have a username and password,
	// ask the user for what's missing and try (again)
	if queryErr != nil || (credStoreErr != nil || credentials.Username == "" || credentials.Password == "") {
		err = promptUserForAuthentication(credentials, options.Username, options.Password, stdout)
		if err != nil {
			return warnings, errors.Join(ErrCredentialsCannotBeRead, err)
		}

		// We have credentials, let's try to login
		err = login(ctx, registryServer, insecureLogin)

		if err != nil {
			if errors.Is(err, ErrServerUnspecified) ||
				errors.Is(err, ErrServerBlacklist) ||
				errors.Is(err, ErrServerUnavailable) ||
				errors.Is(err, ErrServerTimeout) ||
				errors.Is(err, ErrServerTooManyRedirects) ||
				errors.Is(err, ErrServerTooManyRetries) {
				// Wrap all server related issues
				err = errors.Join(nerderr.ErrServerIsMisbehaving, err)
			} else if errors.Is(err, ErrAuthorizerError) ||
				errors.Is(err, ErrAuthorizerRedirectError) ||
				errors.Is(err, ErrCredentialsRefused) ||
				errors.Is(err, ErrUnsupportedAuthenticationMethod) {
				// Wrap all authentication-proper related issues
				err = errors.Join(ErrAuthenticationFailure, err)
			} else {
				log.L.Error("non-specific error condition - please report this as a bug")
				// } else if errors.Is(err, ErrConnectionFailed) {
			}
			return warnings, err
		}
	}

	// If we got an identity token back, this is what we are going to store instead of the password
	responseIdentityToken := resolver.IdentityTokenForHost(resolver.RegistryNamespace.Host)
	if responseIdentityToken != "" {
		credentials.Password = ""
		credentials.IdentityToken = responseIdentityToken
	}

	// Add a warning if we're storing the users password (not a token) and credentials store type is file.
	if filename := credentialsStore.FileStorageLocation(resolver.RegistryNamespace); credentials.Password != "" && filename != "" {
		warnings = append(warnings, fmt.Sprintf(unencryptedPasswordWarning, filename))
	}

	// fmt.Println("AGAIN", resolver.RegistryNamespace.CanonicalIdentifier())
	if err = credentialsStore.Store(resolver.RegistryNamespace, credentials); err != nil {
		return warnings, errors.Join(nerderr.ErrSystemIsBroken, err)
	}

	if len(registryHosts) > 1 {
		warnings = append(warnings, fmt.Sprintf("The registry namespace %q has a hosts.toml configuration that "+
			"resolves to other hosts.\n"+
			"If you are expecting these endpoints to be authenticated as well, please login to them individually with:",
			resolver.RegistryNamespace.Host))
		for _, regHost := range registryHosts {
			warnings = append(
				warnings,
				fmt.Sprintf("  nerdctl login %s%s/?ns=%s", regHost.Host, regHost.Path, resolver.RegistryNamespace.Host),
			)
		}
	}

	return warnings, nil
}

// login will try a registry server and possibly downgrade to http if insecure.
func login(ctx context.Context, registryHost docker.RegistryHost, insecure bool) error {
	err := registryLogin(ctx, registryHost)
	if err != nil {
		// If we have been asked to do insecure, and if the server gave us a http answer, or refused to connect,
		// downgrade to plain http and try again
		// TODO: replace IsErrConnectionRefused with the actual conditions we want to consider - or just retry anyhow?
		if insecure && (errors.As(err, &http.ErrSchemeMismatch) || errutil.IsErrConnectionRefused(err)) {
			registryHost.Scheme = "http"
			err = registryLogin(ctx, registryHost)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

var (
	ErrServerUnspecified    = errors.New("unspecified server error")
	ErrServerBlacklist      = errors.New("server blacklisted us")
	ErrServerUnavailable    = errors.New("server responded but did 500")
	ErrServerTimeout        = errors.New("server response timeout")
	ErrServerTooManyRetries = errors.New("too many retries")

	ErrCredentialsRefused              = errors.New("failed login with provided credentials")
	ErrUnsupportedAuthenticationMethod = errors.New("unsupported authentication")
)

// registryLogin will try to log into the provided registryHost with a maximum of maxResponsesRetries
// It does workaround some registries idiosyncrasies, and return expressive enough errors to provide meaningful
// feedback to the user.
// This method does not try to downgrade protocol, or bypass certificate validation, etc
// (downstream consumer should take care of that)
// In addition to the errors returned from "do" (which are going to be passed through as-is), it may error with:
//   - any of the ErrServer* errors - including ErrServerTooManyRetries in case we hit maxResponsesRetries with no error except 401
//   - ErrCredentialsRefused when authentication has been refused by the server
//   - ErrUnsupportedAuthenticationMethod if the server is requesting an authentication method we do not know about
//     currently supported: basic and token auth
//     currently NOT supported: registry bearer token auth, oauth device flow
func registryLogin(ctx context.Context, registryHost docker.RegistryHost) error {
	var resp *http.Response
	var err error
	responses := []*http.Response{}
	for x := 0; x < maxResponsesRetries; x++ {
		// Do the request - exit on error
		resp, err = do(ctx, registryHost)
		if err != nil {
			return err
		}

		// Make sure the body gets closed when we return, but leave it open for now as the authorizer might want to inspect it
		defer func() {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
		}()

		// Attach the last response for the authorizer to inspect
		responses = append(responses, resp)

		log.L.Debugf("received response with status code %d", resp.StatusCode)

		// Decide if we should try again
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			// Faulty code not providing an authorizer will trip this
			if registryHost.Authorizer == nil {
				panic("unable to login without an authorizer - please report this as a bug")
			}

			// Add the last response
			err = registryHost.Authorizer.AddResponses(ctx, responses)
			// Not implemented is the only AddResponses documented error condition
			if err != nil {
				if errdefs.IsNotImplemented(err) {
					return ErrUnsupportedAuthenticationMethod
				}
				panic("unhandled condition from Authorizer.AddResponses - report this as a bug")
			}

			// Handle bugs and bizarro-registry-behaviors
			if len(responses) >= 2 {
				// Fix https://github.com/containerd/nerdctl/issues/3068
				// This is a (dirty) workaround and should probably be fixed in containerd remote/docker basic auth instead
				// With basic authentication, we should not retry the same thing over and over again...
				last := responses[len(responses)-1]
				prior := responses[len(responses)-2]
				// Note: Get returns case-insensitive
				wwwAuth := last.Header.Get("www-authenticate")
				wwwAuthPrior := prior.Header.Get("www-authenticate")

				if prior.Request.URL == last.Request.URL {
					// If we received the same challenge twice in a "basic" context for the same URL, that's it.
					if strings.HasPrefix(wwwAuth, "basic") && wwwAuth == wwwAuthPrior {
						return ErrCredentialsRefused
					}

					// See https://github.com/containerd/nerdctl/issues/1675
					// Some misbehaving registries may (buggily) switch to a different authentication type on auth failure
					// In that case, just reset the responses and retry from scratch
					// TODO: the ticket issue happens on push, as the scope is not enough. This logic needs to go there as well.
					if wwwAuth[0:6] != wwwAuthPrior[0:6] {
						log.L.Warn("Misbehaving server! We received different authentication types for the same URL. Resetting responses.")
						responses = []*http.Response{}
					}
				}
			}
		case http.StatusRequestTimeout:
			// It is worth assuming this is a fluke - retry if possible
			err = ErrServerTimeout
		case http.StatusServiceUnavailable:
			// This is assumed to be a fluke (docker hub has a lot of these) - retry if possible
			err = ErrServerUnavailable
		case http.StatusTooManyRequests:
			// We got blacklisted. We need to stop now.
			return ErrServerBlacklist
		case http.StatusOK:
			// Authentication successful
			return nil
		default:
			// Non-specific error condition. Drop off.
			return ErrServerUnspecified
		}

		// Retry - make sure we close first
		_ = resp.Body.Close()
	}

	// If we are here and the error is nil, we have exhausted our attempts
	if err == nil {
		err = ErrServerTooManyRetries
	}

	return err
}

var (
	ErrConnectionFailed        = errors.New("http client connection error")
	ErrServerTooManyRedirects  = errors.New("too many redirects: " + strconv.Itoa(redirectLimit))
	ErrAuthorizerError         = errors.New("authorizer fail")
	ErrAuthorizerRedirectError = errors.New("authorizer fail on redirect")
)

// do is a private function performing the actual http requests
// It might error with:
//   - ErrConnectionFailed which will wrap any connection error, like:
//     tcp timeouts, certificate validation errors, DNS resolution errors, etc
//   - ErrServerTooManyRedirects in case there are too many redirects:
//     this is indicative of a server misconfiguration (or malicious)
//   - ErrAuthorizerError and ErrAuthorizerRedirectError errors, wrapping the underlying authorizer error
//     TODO clarify what these are
func do(ctx context.Context, registryHost docker.RegistryHost) (*http.Response, error) {
	if registryHost.Path == "/v2" {
		// Containerd usually return the path without the trailing slash
		// https://github.com/containerd/containerd/blob/2986d5b077feb8252d5d2060277a9c98ff8e009b/remotes/docker/config/hosts.go#L110
		// This may cause issues with certain registries, or a (useless) extra redirect for most
		// See spec for details https://docs.docker.com/registry/spec/api/#detail
		registryHost.Path = "/v2/"
	}

	// Prep the http request (note: the only case where this would error is if ctx is nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryHost.Scheme+"://"+registryHost.Host+registryHost.Path, nil)
	// The only reason for this to fail is ctx == nil, which would happen solely if we had faulty code
	if err != nil {
		panic(fmt.Sprintf("login: http.NewRequestWithContext errored with: %v - please report this as a bug", err))
	}

	req.Header = http.Header{}
	// Set default user-agent - this will get overridden if hosts.toml defines it too
	req.Header.Set("user-agent", defaultNerdUserAgent)
	// Add headers if any are specified in the regHost object (eg: in the hosts.toml file)
	if registryHost.Header != nil {
		req.Header = registryHost.Header.Clone()
	}

	// Attach a redirect handler, to limit the number of redirects, and to be able to reauthorize
	if registryHost.Client.CheckRedirect == nil {
		registryHost.Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			log.L.Debugf("redirecting for the %d-th time to %q", len(via), req.URL)
			if len(via) >= redirectLimit {
				return ErrServerTooManyRedirects
			}
			if registryHost.Authorizer != nil {
				if err = registryHost.Authorizer.Authorize(ctx, req); err != nil {
					log.L.Debugf("authorizer errored on the redirect for url %q", req.URL)
					return errors.Join(ErrAuthorizerRedirectError, err)
				}
			}
			return nil
		}
	}

	// Authorize if we have an authorizer
	if registryHost.Authorizer != nil {
		if err = registryHost.Authorizer.Authorize(ctx, req); err != nil {
			log.L.Debugf("authorizer errored for url %q", req.URL)
			return nil, errors.Join(ErrAuthorizerError, err)
		}
	}

	// Do the request and return
	resp, err := registryHost.Client.Do(req)
	if err != nil {
		log.L.Debugf("http client do errored on url %q", req.URL)
		err = errors.Join(ErrConnectionFailed, err)
	}

	return resp, err
}
