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
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	dockerconfig "github.com/containerd/containerd/v2/core/remotes/docker/config"
	"github.com/containerd/containerd/v2/core/transfer/registry"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
)

var PushTracker = docker.NewInMemoryTracker()

// Global semaphores per registry host to enforce true concurrency limits
var (
	semaphoresMutex sync.RWMutex
	semaphores      = make(map[string]chan struct{})
)

// getSemaphore returns or creates a semaphore for a given host with the specified limit
func getSemaphore(host string, limit int) chan struct{} {
	semaphoresMutex.Lock()
	defer semaphoresMutex.Unlock()

	key := fmt.Sprintf("%s:%d", host, limit)
	if sem, exists := semaphores[key]; exists {
		return sem
	}

	// Create a new semaphore with the specified limit
	sem := make(chan struct{}, limit)
	semaphores[key] = sem
	log.L.Debugf("Created semaphore for %s with limit %d", host, limit)
	return sem
}

// semaphoreTransport wraps an http.RoundTripper to enforce true concurrency limits using semaphores
type semaphoreTransport struct {
	transport http.RoundTripper
	limit     int
}

// RoundTrip implements http.RoundTripper with semaphore-based concurrency limiting
func (st *semaphoreTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if st.limit <= 0 {
		// No limit, use underlying transport directly
		return st.transport.RoundTrip(req)
	}

	host := req.URL.Host
	sem := getSemaphore(host, st.limit)

	// Acquire semaphore (blocks if limit reached)
	log.L.Debugf("Acquiring semaphore for %s (limit %d)", host, st.limit)
	sem <- struct{}{}
	defer func() {
		<-sem // Release semaphore
		log.L.Debugf("Released semaphore for %s", host)
	}()

	log.L.Debugf("Acquired semaphore for %s, making request", host)
	return st.transport.RoundTrip(req)
}

// retryTransport wraps an http.RoundTripper to add retry logic for 503 errors
type retryTransport struct {
	transport    http.RoundTripper
	maxRetries   int
	initialDelay time.Duration
}

// classifies whether the error should trigger a retry and retruns true or false
// depending on the result
func RoundTripErrorClassifier(resp *http.Response, err error, rt *retryTransport, attempt int) bool {
	if resp != nil && resp.StatusCode == http.StatusServiceUnavailable {
		log.L.Infof("retryTransport.RoundTrip: Retrying due to 503 Service Unavailable error (attempt %d/%d)", attempt+1, rt.maxRetries)
		return true
	} else if err != nil {
		// Check for specific network errors that warrant a retry
		if errors.Is(err, io.EOF) {
			log.L.Debugf("retryTransport.RoundTrip: Retrying due to io.EOF error (attempt %d/%d)", attempt+1, rt.maxRetries)
			return true
		} else if strings.Contains(err.Error(), "connection reset by peer") {
			log.L.Debugf("retryTransport.RoundTrip: Retrying due to 'connection reset by peer' error (attempt %d/%d)", attempt+1, rt.maxRetries)
			return true
		} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.L.Debugf("retryTransport.RoundTrip: Retrying due to timeout network error: %v (attempt %d/%d)", netErr, attempt+1, rt.maxRetries)
			return true
		} else if errors.Is(err, context.DeadlineExceeded) {
			log.L.Debugf("retryTransport.RoundTrip: Retrying due to context deadline exceeded error (attempt %d/%d)", attempt+1, rt.maxRetries)
			return true
		} else if errors.Is(err, context.Canceled) {
			log.L.Debugf("retryTransport.RoundTrip: Retrying due to context canceled error (attempt %d/%d)", attempt+1, rt.maxRetries)
			return true
		}
		log.L.Debugf("retryTransport.RoundTrip: Not retrying for non-retryable error: %T %v", err, err)
	}
	return false
}

// RoundTrip implements http.RoundTripper with retry logic for 503 Service Unavailable errors
func (rt *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	log.L.Infof("retryTransport.RoundTrip: Starting request to %s (maxRetries=%d)", req.URL.Host, rt.maxRetries)

	for attempt := 0; attempt <= rt.maxRetries; attempt++ {
		// Clone the request for potential retries
		reqClone := req.Clone(req.Context())

		resp, err := rt.transport.RoundTrip(reqClone)

		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		log.L.Infof("retryTransport.RoundTrip: attempt %d, err=%v, status=%d", attempt, err, statusCode)

		// Retry logic: retry on 503, EOF, connection reset, or temporary network errors.
		// These errors are often transient and can be resolved by a retry.
		log.L.Infof("retryTransport.RoundTrip: Evaluating retry conditions - resp=%v, statusCode=%d, StatusServiceUnavailable=%d", resp != nil, statusCode, http.StatusServiceUnavailable)
		shouldRetry := RoundTripErrorClassifier(resp, err, rt, attempt)
		log.L.Infof("retryTransport.RoundTrip: shouldRetry=%v for attempt %d", shouldRetry, attempt)
		if shouldRetry {
			// We have a condition that warrants a retry.
			if attempt == rt.maxRetries {
				log.L.Debugf("Max retries (%d) exceeded for request to %s", rt.maxRetries, req.URL.Host)
				return resp, err // Return the last response and error
			}

			// Close the response body before retrying.
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}

			// Calculate exponential backoff delay: initialDelay * 2^attempt
			delay := time.Duration(float64(rt.initialDelay) * math.Pow(2, float64(attempt)))
			log.L.Debugf("Request to %s failed, retrying in %v (attempt %d/%d)",
				req.URL.Host, delay, attempt+1, rt.maxRetries)

			// Wait before retrying.
			time.Sleep(delay)
			continue // Continue to the next retry attempt
		}

		// If we are here, it means we are not retrying.
		log.L.Infof("retryTransport.RoundTrip: Not retrying, returning response (status=%d, err=%v)", statusCode, err)
		return resp, err
	}

	// This should never be reached, but return error just in case
	return nil, fmt.Errorf("unexpected retry logic error")
}

type opts struct {
	plainHTTP         bool
	skipVerifyCerts   bool
	hostsDirs         []string
	AuthCreds         AuthCreds
	maxConnsPerHost   int
	maxIdleConns      int
	requestTimeout    time.Duration
	maxRetries        int
	retryInitialDelay time.Duration
	tracker           docker.StatusTrackLocker
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
		o.AuthCreds = ac
	}
}

// WithMaxConnsPerHost sets the maximum number of connections per host
func WithMaxConnsPerHost(n int) Opt {
	return func(o *opts) {
		o.maxConnsPerHost = n
	}
}

// WithMaxIdleConns sets the maximum number of idle connections
func WithMaxIdleConns(n int) Opt {
	return func(o *opts) {
		o.maxIdleConns = n
	}
}

// WithRequestTimeout sets the request timeout
func WithRequestTimeout(d time.Duration) Opt {
	return func(o *opts) {
		o.requestTimeout = d
	}
}

// WithMaxRetries sets the maximum number of retry attempts for 503 errors
func WithMaxRetries(n int) Opt {
	return func(o *opts) {
		o.maxRetries = n
	}
}

// WithRetryInitialDelay sets the initial delay before first retry
func WithRetryInitialDelay(d time.Duration) Opt {
	return func(o *opts) {
		o.retryInitialDelay = d
	}
}

// WithTracker sets a custom status tracker
func WithTracker(tracker docker.StatusTrackLocker) Opt {
	return func(o *opts) {
		o.tracker = tracker
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

	if o.AuthCreds != nil {
		ho.Credentials = o.AuthCreds
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
type customResolver struct {
	remotes.Resolver
	client *http.Client
}

func (r *customResolver) Client(ctx context.Context, host string) (*http.Client, error) {
	return r.client, nil
}

func New(ctx context.Context, refHostname string, optFuncs ...Opt) (remotes.Resolver, error) {
	ho, err := NewHostOptions(ctx, refHostname, optFuncs...)
	if err != nil {
		return nil, err
	}

	// Configure HTTP client with connection limits to prevent registry overload
	var o opts
	for _, of := range optFuncs {
		of(&o)
	}

	// Use custom tracker if provided, otherwise use global PushTracker
	tracker := PushTracker
	if o.tracker != nil {
		tracker = o.tracker
	}

	// Build the custom transport chain first
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Apply connection limits
	if o.maxConnsPerHost > 0 {
		transport.MaxConnsPerHost = o.maxConnsPerHost
	}
	if o.maxIdleConns > 0 {
		transport.MaxIdleConns = o.maxIdleConns
	}

	finalTransport := http.RoundTripper(transport)

	// Wrap transport with semaphore-based concurrency limiting first
	if o.maxConnsPerHost > 0 {
		finalTransport = &semaphoreTransport{
			transport: finalTransport,
			limit:     o.maxConnsPerHost,
		}
		log.L.Debugf("Enabled semaphore-based concurrency limiting: limit=%d", o.maxConnsPerHost)
	}

	// Then, wrap with retry logic if retries are configured
	if o.maxRetries > 0 {
		retryDelay := o.retryInitialDelay
		if retryDelay == 0 {
			retryDelay = 1000 * time.Millisecond // Default to 1 second
		}
		finalTransport = &retryTransport{
			transport:    finalTransport,
			maxRetries:   o.maxRetries,
			initialDelay: retryDelay,
		}
		log.L.Infof("Enabled retry logic: maxRetries=%d, initialDelay=%v for %s", o.maxRetries, retryDelay, refHostname)
	}

	client := &http.Client{
		Transport: finalTransport,
	}

	if o.requestTimeout > 0 {
		client.Timeout = o.requestTimeout
	}

	// Set up the host options with our custom client via UpdateClient
	ho.UpdateClient = func(defaultClient *http.Client) error {
		// Replace the default client's transport with our custom retry transport
		defaultClient.Transport = finalTransport
		if o.requestTimeout > 0 {
			defaultClient.Timeout = o.requestTimeout
		}
		return nil
	}

	resolverOpts := docker.ResolverOptions{
		Tracker: tracker,
		Hosts:   dockerconfig.ConfigureHosts(ctx, *ho),
	}

	resolver := docker.NewResolver(resolverOpts)
	return &customResolver{
		Resolver: resolver,
		client:   client,
	}, nil
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
