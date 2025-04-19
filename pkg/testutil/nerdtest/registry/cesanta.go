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
	"io"
	"net"
	"strconv"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/utils/testca"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/platform"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
)

type CesantaConfigServer struct {
	Addr        string `yaml:"addr,omitempty"`
	Certificate string
	Key         string
}

type CesantaConfigToken struct {
	Issuer      string `yaml:"issuer,omitempty"`
	Certificate string `yaml:"certificate,omitempty"`
	Key         string `yaml:"key,omitempty"`
	Expiration  int    `yaml:"expiration,omitempty"`
}

type CesantaConfigUser struct {
	Password string `yaml:"password,omitempty"`
}

type CesantaMatchConditions struct {
	Account string `yaml:"account,omitempty"`
}

type CesantaConfigACLEntry struct {
	Match   CesantaMatchConditions `yaml:"match"`
	Actions []string               `yaml:"actions,flow"`
}

type CesantaConfigACL []CesantaConfigACLEntry

type CesantaConfig struct {
	Server CesantaConfigServer          `yaml:"server"`
	Token  CesantaConfigToken           `yaml:"token"`
	Users  map[string]CesantaConfigUser `yaml:"users,omitempty"`
	ACL    CesantaConfigACL             `yaml:"acl,omitempty"`
}

func NewCesantaAuthServer(data test.Data, helpers test.Helpers, ca *testca.Cert, port int, user, pass string, tls bool) *TokenAuthServer {
	// listen on 0.0.0.0 to enable 127.0.0.1
	listenIP := net.ParseIP("0.0.0.0")
	hostIP, err := nettestutil.NonLoopbackIPv4()
	assert.NilError(helpers.T(), err, fmt.Errorf("failed finding ipv4 non loopback interface: %w", err))
	bpass, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	assert.NilError(helpers.T(), err, fmt.Errorf("failed bcrypt encrypting password: %w", err))
	// Prepare configuration file for authentication server
	// Details: https://github.com/cesanta/docker_auth/blob/1.7.1/examples/simple.yml
	cc := &CesantaConfig{
		Server: CesantaConfigServer{
			Addr: ":5100",
		},
		Token: CesantaConfigToken{
			Issuer:     "Cesanta auth server",
			Expiration: 900,
		},
		Users: map[string]CesantaConfigUser{
			user: {
				Password: string(bpass),
			},
		},
		ACL: CesantaConfigACL{
			{
				Match: CesantaMatchConditions{
					Account: user,
				},
				Actions: []string{"*"},
			},
		},
	}

	scheme := "http"
	if tls {
		scheme = "https"
		cc.Server.Certificate = "/auth/domain.crt"
		cc.Server.Key = "/auth/domain.key"
	} else {
		cc.Token.Certificate = "/auth/domain.crt"
		cc.Token.Key = "/auth/domain.key"
	}

	configFileName := data.Temp().SaveToWriter(func(file io.Writer) error {
		return yaml.NewEncoder(file).Encode(cc)
	}, "authconfig")

	cert := ca.GenerateServerX509(data, helpers, hostIP.String())
	// FIXME: this will fail in many circumstances. Review strategy on how to acquire a free port.
	// We probably have better code for that already somewhere.
	port, err = portlock.Acquire(port)
	assert.NilError(helpers.T(), err, fmt.Errorf("failed acquiring port: %w", err))
	containerName := data.Identifier(fmt.Sprintf("cesanta-auth-server-%d-%t", port, tls))
	// Cleanup possible leftovers first
	helpers.Ensure("rm", "-f", containerName)

	cleanup := func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("rm", "-f", containerName)
		_ = portlock.Release(port)
		//if errPortRelease != nil {
		//	helpers.T().Error(errPortRelease.Error())
		//}
	}

	setup := func(data test.Data, helpers test.Helpers) {
		helpers.Ensure(
			"run",
			"--pull=never",
			"-d",
			"-p", fmt.Sprintf("%s:%d:5100", listenIP, port),
			"--name", containerName,
			"-v", cert.CertPath+":/auth/domain.crt",
			"-v", cert.KeyPath+":/auth/domain.key",
			"-v", configFileName+":/config/auth_config.yml",
			platform.DockerAuthImage,
			"/config/auth_config.yml",
		)
		ensureServerStarted(helpers, containerName, scheme, hostIP, port)
	}

	return &TokenAuthServer{
		Scheme:   scheme,
		IP:       hostIP,
		Port:     port,
		CertPath: cert.CertPath,
		Auth: &TokenAuth{
			Address:  scheme + "://" + net.JoinHostPort(hostIP.String(), strconv.Itoa(port)),
			CertPath: cert.CertPath,
		},
		Setup:   setup,
		Cleanup: cleanup,
	}
}
