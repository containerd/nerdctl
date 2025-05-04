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

package registry

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
)

// Auth describes a struct able to serialize authenticator information into arguments to be fed to a registry container run
type Auth interface {
	Params(data test.Data) []string
}

type NoAuth struct {
}

func (na *NoAuth) Params(data test.Data) []string {
	return []string{}
}

type TokenAuth struct {
	Address  string
	CertPath string
}

// FIXME: this is specific to Docker Registry
// Like need something else for Harbor and Gitlab

func (ta *TokenAuth) Params(data test.Data) []string {
	return []string{
		"--env", "REGISTRY_AUTH=token",
		"--env", "REGISTRY_AUTH_TOKEN_REALM=" + ta.Address + "/auth",
		"--env", "REGISTRY_AUTH_TOKEN_SERVICE=Docker registry",
		"--env", "REGISTRY_AUTH_TOKEN_ISSUER=Cesanta auth server",
		"--env", "REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE=/auth/domain.crt",
		"-v", ta.CertPath + ":/auth/domain.crt",
	}
}

type BasicAuth struct {
	Realm    string
	HtFile   string
	Username string
	Password string
}

func (ba *BasicAuth) Params(data test.Data) []string {
	if ba.Realm == "" {
		ba.Realm = "Basic Realm"
	}
	if ba.HtFile == "" && ba.Username != "" && ba.Password != "" {
		pass := ba.Password
		encryptedPass, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
		ba.HtFile = data.Temp().Save(fmt.Sprintf(`%s:%s`, ba.Username, string(encryptedPass[:])), "htpasswd")
	}
	ret := []string{
		"--env", "REGISTRY_AUTH=htpasswd",
		"--env", "REGISTRY_AUTH_HTPASSWD_REALM=" + ba.Realm,
		"--env", "REGISTRY_AUTH_HTPASSWD_PATH=/htpasswd",
	}
	if ba.HtFile != "" {
		ret = append(ret, "-v", ba.HtFile+":/htpasswd")
	}
	return ret
}

type TokenAuthServer struct {
	Scheme   string
	IP       net.IP
	Port     int
	CertPath string
	Cleanup  func(data test.Data, helpers test.Helpers)
	Setup    func(data test.Data, helpers test.Helpers)
	Logs     func(data test.Data, helpers test.Helpers)
	Auth     Auth
}

type Server struct {
	Scheme   string
	IP       net.IP
	Port     int
	Cleanup  func(data test.Data, helpers test.Helpers)
	Setup    func(data test.Data, helpers test.Helpers)
	HostsDir string // contains "<HostIP>:<ListenPort>/hosts.toml"
}

const (
	maxRetry = 20
	sleep    = time.Second
)

// Note this mostly duplicates EnsureContainerStarted
func ensureServerStarted(helpers test.Helpers, containerName string, scheme string, ip net.IP, port int) {
	helpers.T().Helper()

	// First ensure the container has been started
	started := false
	for i := 0; i < maxRetry && !started; i++ {
		helpers.Command("container", "inspect", containerName).
			Run(&test.Expected{
				ExitCode: expect.ExitCodeNoCheck,
				Output: func(stdout string, t tig.T) {
					// Note: we can't use JSON comparator because it would hard fail if there is no content
					var dc []dockercompat.Container
					err := json.Unmarshal([]byte(stdout), &dc)
					if err != nil || len(dc) == 0 {
						return
					}
					assert.Equal(t, len(dc), 1, "Unexpectedly got multiple results\n")
					started = dc[0].State.Running
				},
			})
		time.Sleep(sleep)
	}

	// Now, verify we can talk to it
	var err error
	if started {
		_, err = nettestutil.HTTPGet(fmt.Sprintf("%s://%s/auth",
			scheme,
			net.JoinHostPort(ip.String(), strconv.Itoa(port)),
		),
			10,
			true)
	}

	if !started || err != nil {
		ins := helpers.Capture("container", "inspect", containerName)
		ps := helpers.Capture("ps", "-a")
		stdout := helpers.Capture("logs", containerName)
		stderr := helpers.Err("logs", containerName)

		helpers.T().Log(ins)
		helpers.T().Log(ps)
		helpers.T().Log(stdout)
		helpers.T().Log(stderr)
		helpers.T().Log(fmt.Sprintf("container %s still not running after %d retries", containerName, maxRetry))
		helpers.T().FailNow()
	}
}
