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

package testregistry

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testca"

	"golang.org/x/crypto/bcrypt"
	"gotest.tools/v3/assert"
)

type TestRegistry struct {
	IP         net.IP
	ListenIP   net.IP
	ListenPort int
	HostsDir   string // contains "<HostIP>:<ListenPort>/hosts.toml"
	Cleanup    func()
	Logs       func()
}

func NewPlainHTTP(base *testutil.Base, port int) *TestRegistry {
	hostIP, err := nettestutil.NonLoopbackIPv4()
	assert.NilError(base.T, err)
	// listen on 0.0.0.0 to enable 127.0.0.1
	listenIP := net.ParseIP("0.0.0.0")
	listenPort := port
	base.T.Logf("hostIP=%q, listenIP=%q, listenPort=%d", hostIP, listenIP, listenPort)

	registryContainerName := "reg-" + testutil.Identifier(base.T)
	cmd := base.Cmd("run",
		"-d",
		"-p", fmt.Sprintf("%s:%d:5000", listenIP, listenPort),
		"--name", registryContainerName,
		testutil.RegistryImage)
	cmd.AssertOK()
	if _, err = nettestutil.HTTPGet(fmt.Sprintf("http://%s:%d/v2", hostIP.String(), listenPort), 30, false); err != nil {
		base.Cmd("rm", "-f", registryContainerName).Run()
		base.T.Fatal(err)
	}
	return &TestRegistry{
		IP:         hostIP,
		ListenIP:   listenIP,
		ListenPort: listenPort,
		Cleanup:    func() { base.Cmd("rm", "-f", registryContainerName).AssertOK() },
	}
}

func NewAuthWithHTTP(base *testutil.Base, user, pass string, listenPort int, authPort int) *TestRegistry {
	name := testutil.Identifier(base.T)
	hostIP, err := nettestutil.NonLoopbackIPv4()
	assert.NilError(base.T, err)
	// listen on 0.0.0.0 to enable 127.0.0.1
	listenIP := net.ParseIP("0.0.0.0")
	base.T.Logf("hostIP=%q, listenIP=%q, listenPort=%d, authPort=%d", hostIP, listenIP, listenPort, authPort)

	ca := testca.New(base.T)
	registryCert := ca.NewCert(hostIP.String())
	authCert := ca.NewCert(hostIP.String())

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
	authContainerName := fmt.Sprintf("auth-%s-%d", name, authPort)
	cmd := base.Cmd("run",
		"-d",
		"-p", fmt.Sprintf("%s:%d:5100", listenIP, authPort),
		"--name", authContainerName,
		"-v", authCert.CertPath+":/auth/domain.crt",
		"-v", authCert.KeyPath+":/auth/domain.key",
		"-v", authConfigFileName+":/config/auth_config.yml",
		testutil.DockerAuthImage,
		"/config/auth_config.yml")
	cmd.AssertOK()

	// Run docker_auth-enabled registry
	// Details: https://github.com/cesanta/docker_auth/blob/1.7.1/examples/simple.yml
	registryContainerName := fmt.Sprintf("%s-%s-%d", "reg", name, listenPort)
	cmd = base.Cmd("run",
		"-d",
		"-p", fmt.Sprintf("%s:%d:5000", listenIP, listenPort),
		"--name", registryContainerName,
		"--env", "REGISTRY_AUTH=token",
		"--env", "REGISTRY_AUTH_TOKEN_REALM="+fmt.Sprintf("https://%s:%d/auth", hostIP.String(), authPort),
		"--env", "REGISTRY_AUTH_TOKEN_SERVICE=Docker registry",
		"--env", "REGISTRY_AUTH_TOKEN_ISSUER=Acme auth server",
		"--env", "REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE=/auth/domain.crt",
		// rootcertbundle is not CA cert: https://github.com/distribution/distribution/issues/1143
		"-v", authCert.CertPath+":/auth/domain.crt",
		testutil.RegistryImage)
	cmd.AssertOK()
	joined := net.JoinHostPort(hostIP.String(), strconv.Itoa(listenPort))
	if _, err = nettestutil.HTTPGet(fmt.Sprintf("http://%s/v2", joined), 30, true); err != nil {
		base.Cmd("rm", "-f", registryContainerName).Run()
		base.T.Fatal(err)
	}
	hostsDir, err := os.MkdirTemp(base.T.TempDir(), "certs.d")
	assert.NilError(base.T, err)
	hostsSubDir := filepath.Join(hostsDir, joined)
	err = os.MkdirAll(hostsSubDir, 0700)
	assert.NilError(base.T, err)
	hostsTOMLPath := filepath.Join(hostsSubDir, "hosts.toml")
	// See https://github.com/containerd/containerd/blob/main/docs/hosts.md
	hostsTOML := fmt.Sprintf(`
server = "https://%s"
[host."https://%s"]
  ca = %q
		`, joined, joined, ca.CertPath)
	base.T.Logf("Writing %q: %q", hostsTOMLPath, hostsTOML)
	err = os.WriteFile(hostsTOMLPath, []byte(hostsTOML), 0700)
	assert.NilError(base.T, err)
	return &TestRegistry{
		IP:         hostIP,
		ListenIP:   listenIP,
		ListenPort: listenPort,
		HostsDir:   hostsDir,
		Cleanup: func() {
			base.Cmd("rm", "-f", registryContainerName).AssertOK()
			base.Cmd("rm", "-f", authContainerName).AssertOK()
			assert.NilError(base.T, registryCert.Close())
			assert.NilError(base.T, authCert.Close())
			assert.NilError(base.T, authConfigFile.Close())
			os.Remove(authConfigFileName)
		},
		Logs: func() {
			base.T.Logf("%s: %q", registryContainerName, base.Cmd("logs", registryContainerName).Run().String())
			base.T.Logf("%s: %q", authContainerName, base.Cmd("logs", authContainerName).Run().String())
		},
	}
}

func NewHTTPS(base *testutil.Base, user, pass string) *TestRegistry {
	name := testutil.Identifier(base.T)
	hostIP, err := nettestutil.NonLoopbackIPv4()
	assert.NilError(base.T, err)
	// listen on 0.0.0.0 to enable 127.0.0.1
	listenIP := net.ParseIP("0.0.0.0")
	const listenPort = 5000 // TODO: choose random empty port
	const authPort = 5100   // TODO: choose random empty port
	base.T.Logf("hostIP=%q, listenIP=%q, listenPort=%d, authPort=%d", hostIP, listenIP, listenPort, authPort)

	ca := testca.New(base.T)
	registryCert := ca.NewCert(hostIP.String())
	authCert := ca.NewCert(hostIP.String())

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
		"-v", authCert.CertPath+":/auth/domain.crt",
		"-v", authCert.KeyPath+":/auth/domain.key",
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
		// rootcertbundle is not CA cert: https://github.com/distribution/distribution/issues/1143
		"-v", authCert.CertPath+":/auth/domain.crt",
		"-v", registryCert.CertPath+":/registry/domain.crt",
		"-v", registryCert.KeyPath+":/registry/domain.key",
		testutil.RegistryImage)
	cmd.AssertOK()
	joined := net.JoinHostPort(hostIP.String(), strconv.Itoa(listenPort))
	if _, err = nettestutil.HTTPGet(fmt.Sprintf("https://%s/v2", joined), 30, true); err != nil {
		base.Cmd("rm", "-f", registryContainerName).Run()
		base.T.Fatal(err)
	}
	hostsDir, err := os.MkdirTemp(base.T.TempDir(), "certs.d")
	assert.NilError(base.T, err)
	hostsSubDir := filepath.Join(hostsDir, joined)
	err = os.MkdirAll(hostsSubDir, 0700)
	assert.NilError(base.T, err)
	hostsTOMLPath := filepath.Join(hostsSubDir, "hosts.toml")
	// See https://github.com/containerd/containerd/blob/main/docs/hosts.md
	hostsTOML := fmt.Sprintf(`
server = "https://%s"
[host."https://%s"]
  ca = %q
		`, joined, joined, ca.CertPath)
	base.T.Logf("Writing %q: %q", hostsTOMLPath, hostsTOML)
	err = os.WriteFile(hostsTOMLPath, []byte(hostsTOML), 0700)
	assert.NilError(base.T, err)
	return &TestRegistry{
		IP:         hostIP,
		ListenIP:   listenIP,
		ListenPort: listenPort,
		HostsDir:   hostsDir,
		Cleanup: func() {
			base.Cmd("rm", "-f", registryContainerName).AssertOK()
			base.Cmd("rm", "-f", authContainerName).AssertOK()
			assert.NilError(base.T, registryCert.Close())
			assert.NilError(base.T, authCert.Close())
			assert.NilError(base.T, authConfigFile.Close())
			os.Remove(authConfigFileName)
		},
		Logs: func() {
			base.T.Logf("%s: %q", registryContainerName, base.Cmd("logs", registryContainerName).Run().String())
			base.T.Logf("%s: %q", authContainerName, base.Cmd("logs", authContainerName).Run().String())
		},
	}
}
