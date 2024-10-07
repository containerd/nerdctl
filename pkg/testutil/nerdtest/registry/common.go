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
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"

	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
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
		tmpDir, _ := os.MkdirTemp(data.TempDir(), "htpasswd")
		ba.HtFile = filepath.Join(tmpDir, "htpasswd")
		_ = os.WriteFile(ba.HtFile, []byte(fmt.Sprintf(`%s:%s`, ba.Username, string(encryptedPass[:]))), 0600)
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
	Logs     func(data test.Data, helpers test.Helpers)
	HostsDir string // contains "<HostIP>:<ListenPort>/hosts.toml"
}
