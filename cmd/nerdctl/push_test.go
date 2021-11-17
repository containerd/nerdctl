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

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/testutil"

	"golang.org/x/crypto/bcrypt"
	"gotest.tools/v3/assert"
)

func getNonLoopbackIPv4() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		ipv4 := ip.To4()
		if ipv4 == nil {
			continue
		}
		if ipv4.IsLoopback() {
			continue
		}
		return ipv4, nil
	}
	return nil, fmt.Errorf("non-loopback IPv4 address not found, attempted=%+v: %w", addrs, errdefs.ErrNotFound)
}

type testRegistry struct {
	ip         net.IP
	listenIP   net.IP
	listenPort int
	cleanup    func()
}

func newTestRegistry(base *testutil.Base, name string) *testRegistry {
	hostIP, err := getNonLoopbackIPv4()
	assert.NilError(base.T, err)
	// listen on 0.0.0.0 to enable 127.0.0.1
	listenIP := net.ParseIP("0.0.0.0")
	const listenPort = 5000 // TODO: choose random empty port
	base.T.Logf("hostIP=%q, listenIP=%q, listenPort=%d", hostIP, listenIP, listenPort)

	registryContainerName := "reg-" + name
	cmd := base.Cmd("run",
		"-d",
		"-p", fmt.Sprintf("%s:%d:5000", listenIP, listenPort),
		"--name", registryContainerName,
		testutil.RegistryImage)
	cmd.AssertOK()
	if _, err = httpGet(fmt.Sprintf("http://%s:%d/v2", hostIP.String(), listenPort), 30); err != nil {
		base.Cmd("rm", "-f", registryContainerName).Run()
		base.T.Fatal(err)
	}
	return &testRegistry{
		ip:         hostIP,
		listenIP:   listenIP,
		listenPort: listenPort,
		cleanup:    func() { base.Cmd("rm", "-f", registryContainerName).AssertOK() },
	}
}

func newTestInsecureRegistry(base *testutil.Base, name, user, pass string) *testRegistry {
	hostIP, err := getNonLoopbackIPv4()
	assert.NilError(base.T, err)
	// listen on 0.0.0.0 to enable 127.0.0.1
	listenIP := net.ParseIP("0.0.0.0")
	const listenPort = 5000 // TODO: choose random empty port
	const authPort = 5100   // TODO: choose random empty port
	base.T.Logf("hostIP=%q, listenIP=%q, listenPort=%d, authPort=%d", hostIP, listenIP, listenPort, authPort)

	registryCert, registryKey, registryClose := generateTestCert(base, hostIP.String())
	authCert, authKey, authClose := generateTestCert(base, hostIP.String())

	// Prepare configuration file for authentication server
	// Details: https://github.com/cesanta/docker_auth/blob/1.7.1/examples/simple.yml
	authConfigFile, err := os.CreateTemp("", "authconfig")
	assert.NilError(base.T, err)
	bpass, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	assert.NilError(base.T, err)
	authConfigFileName := authConfigFile.Name()
	_, err = authConfigFile.Write([]byte(fmt.Sprintf(`
server:
  addr: ":5100"
  certificate: "/auth/domain.crt"
  key: "/auth/domain.key"
token:
  issuer: "Acme auth server"
  expiration: 900
users:
  "%s":
    password: "%s"
acl:
  - match: {account: "%s"}
    actions: ["*"]
`, user, string(bpass), user)))
	assert.NilError(base.T, err)

	// Run authentication server
	authContainerName := "auth-" + name
	cmd := base.Cmd("run",
		"-d",
		"-p", fmt.Sprintf("%s:%d:5100", listenIP, authPort),
		"--name", authContainerName,
		"-v", authCert+":/auth/domain.crt",
		"-v", authKey+":/auth/domain.key",
		"-v", authConfigFileName+":/config/auth_config.yml",
		testutil.DockerAuthImage,
		"/config/auth_config.yml")
	cmd.AssertOK()

	// Run docker_auth-enabled registry
	// Details: https://github.com/cesanta/docker_auth/blob/1.7.1/examples/simple.yml
	registryContainerName := "reg-" + name
	cmd = base.Cmd("run",
		"-d",
		"-p", fmt.Sprintf("%s:%d:5000", listenIP, listenPort),
		"--name", registryContainerName,
		"--env", "REGISTRY_AUTH=token",
		"--env", "REGISTRY_AUTH_TOKEN_REALM="+fmt.Sprintf("https://%s:%d/auth", hostIP.String(), authPort),
		"--env", "REGISTRY_AUTH_TOKEN_SERVICE=Docker registry",
		"--env", "REGISTRY_AUTH_TOKEN_ISSUER=Acme auth server",
		"--env", "REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE=/auth/domain.crt",
		"--env", "REGISTRY_HTTP_TLS_CERTIFICATE=/registry/domain.crt",
		"--env", "REGISTRY_HTTP_TLS_KEY=/registry/domain.key",
		"-v", authCert+":/auth/domain.crt",
		"-v", registryCert+":/registry/domain.crt",
		"-v", registryKey+":/registry/domain.key",
		testutil.RegistryImage)
	cmd.AssertOK()
	if _, err = httpInsecureGet(fmt.Sprintf("https://%s:%d/v2", hostIP.String(), listenPort), 30); err != nil {
		base.Cmd("rm", "-f", registryContainerName).Run()
		base.T.Fatal(err)
	}
	return &testRegistry{
		ip:         hostIP,
		listenIP:   listenIP,
		listenPort: listenPort,
		cleanup: func() {
			base.Cmd("rm", "-f", registryContainerName).AssertOK()
			base.Cmd("rm", "-f", authContainerName).AssertOK()
			assert.NilError(base.T, registryClose())
			assert.NilError(base.T, authClose())
			assert.NilError(base.T, authConfigFile.Close())
			os.Remove(authConfigFileName)
		},
	}
}

func TestPushPlainHTTPFails(t *testing.T) {
	base := testutil.NewBase(t)
	reg := newTestRegistry(base, "test-push-plain-http-fails")
	defer reg.cleanup()

	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/test-push-plain-http-fails:%s",
		reg.ip.String(), reg.listenPort, strings.Split(testutil.AlpineImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.AlpineImage, testImageRef).AssertOK()

	res := base.Cmd("push", testImageRef).Run()
	resCombined := res.Combined()
	t.Logf("result: exitCode=%d, out=%q", res.ExitCode, res.Combined())
	assert.Assert(t, res.ExitCode != 0)
	assert.Assert(t, strings.Contains(resCombined, "server gave HTTP response to HTTPS client"))
}

func TestPushPlainHTTPLocalhost(t *testing.T) {
	base := testutil.NewBase(t)
	reg := newTestRegistry(base, "test-push-plain-localhost")
	defer reg.cleanup()
	localhostIP := "127.0.0.1"
	t.Logf("localhost IP=%q", localhostIP)

	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/test-push-plain-http-insecure:%s",
		localhostIP, reg.listenPort, strings.Split(testutil.AlpineImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.AlpineImage, testImageRef).AssertOK()

	base.Cmd("push", testImageRef).AssertOK()
}

func TestPushPlainHTTPInsecure(t *testing.T) {
	// Skip docker, because "dockerd --insecure-registries" requires restarting the daemon
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := newTestRegistry(base, "test-push-plain-http-insecure")
	defer reg.cleanup()

	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/test-push-plain-http-insecure:%s",
		reg.ip.String(), reg.listenPort, strings.Split(testutil.AlpineImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.AlpineImage, testImageRef).AssertOK()

	base.Cmd("--insecure-registry", "push", testImageRef).AssertOK()
}

func TestPushInsecureWithLogin(t *testing.T) {
	// Skip docker, because "dockerd --insecure-registries" requires restarting the daemon
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	reg := newTestInsecureRegistry(base, "test-push-insecure-tls", "admin", "badmin")
	defer reg.cleanup()

	base.Cmd("--insecure-registry", "login", "-u", "admin", "-p", "badmin",
		fmt.Sprintf("%s:%d", reg.ip.String(), reg.listenPort)).AssertOK()
	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	testImageRef := fmt.Sprintf("%s:%d/test-push-insecure-tls:%s",
		reg.ip.String(), reg.listenPort, strings.Split(testutil.AlpineImage, ":")[1])
	t.Logf("testImageRef=%q", testImageRef)
	base.Cmd("tag", testutil.AlpineImage, testImageRef).AssertOK()

	base.Cmd("--insecure-registry", "push", testImageRef).AssertOK()
}

func generateTestCert(base *testutil.Base, host string) (crtPath, keyPath string, closeFn func() error) {
	certF, err := os.CreateTemp("", "certtemp")
	assert.NilError(base.T, err)
	keyF, err := os.CreateTemp("", "keytemp")
	assert.NilError(base.T, err)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 60)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	assert.NilError(base.T, err)
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{Organization: []string{"test"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		DNSNames:     []string{host},
	}
	privatekey, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NilError(base.T, err)
	publickey := &privatekey.PublicKey
	cert, err := x509.CreateCertificate(rand.Reader, &template, &template, publickey, privatekey)
	assert.NilError(base.T, err)
	privBytes, err := x509.MarshalPKCS8PrivateKey(privatekey)
	assert.NilError(base.T, err)

	assert.NilError(base.T, pem.Encode(certF, &pem.Block{Type: "CERTIFICATE", Bytes: cert}))
	assert.NilError(base.T, pem.Encode(keyF, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	return certF.Name(), keyF.Name(), func() error {
		var errors []error
		certFName := certF.Name()
		keyFName := keyF.Name()
		for _, f := range []func() error{
			certF.Close,
			keyF.Close,
			func() error { return os.Remove(certFName) },
			func() error { return os.Remove(keyFName) },
		} {
			if err := f(); err != nil {
				errors = append(errors, err)
			}
		}
		if len(errors) > 0 {
			return fmt.Errorf("failed to close tmpfile: %v", errors)
		}
		return nil
	}
}

func httpInsecureGet(urlStr string, attempts int) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)
	if attempts < 1 {
		return nil, errdefs.ErrInvalidArgument
	}
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	for i := 0; i < attempts; i++ {
		resp, err = client.Get(urlStr)
		if err == nil {
			return resp, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("error after %d attempts: %w", attempts, err)
}
