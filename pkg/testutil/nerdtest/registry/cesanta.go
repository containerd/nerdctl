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
	"os"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/ca"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/platform"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
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

func (cc *CesantaConfig) Save(path string) error {
	var err error
	var r *os.File
	if r, err = os.Create(path); err == nil {
		defer r.Close()
		err = yaml.NewEncoder(r).Encode(cc)
	}
	return err
}

func ensureContainerStarted(helpers test.Helpers, con string) {
	const maxRetry = 5
	const sleep = time.Second
	success := false
	for i := 0; i < maxRetry && !success; i++ {
		time.Sleep(sleep)
		count := i
		cmd := helpers.Command("container", "inspect", con)
		cmd.Run(&test.Expected{
			Output: func(stdout string, info string, t *testing.T) {
				var dc []dockercompat.Container
				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err, "Unable to unmarshal output\n"+info)
				assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
				if dc[0].State.Running {
					success = true
					return
				}
				if count == maxRetry-1 {
					// FIXME: there is currently no simple way to capture stderr
					// Sometimes, it is convenient for debugging, like here
					// Here we cheat with unbuffer which will bundle stderr and stdout together
					// This is just bad
					t.Error(helpers.Err("logs", con))
					t.Fatalf("container %s still not running after %d retries", con, count)
				}
			},
		})
	}
}

func NewCesantaAuthServer(data test.Data, helpers test.Helpers, ca *ca.CA, port int, user, pass string, tls bool) *TokenAuthServer {
	// listen on 0.0.0.0 to enable 127.0.0.1
	listenIP := net.ParseIP("0.0.0.0")
	hostIP, err := nettestutil.NonLoopbackIPv4()
	assert.NilError(helpers.T(), err, fmt.Errorf("failed finding ipv4 non loopback interface: %w", err))
	bpass, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	assert.NilError(helpers.T(), err, fmt.Errorf("failed bcrypt encrypting password: %w", err))
	// Prepare configuration file for authentication server
	// Details: https://github.com/cesanta/docker_auth/blob/1.7.1/examples/simple.yml
	configFile, err := os.CreateTemp(data.TempDir(), "authconfig")
	assert.NilError(helpers.T(), err, fmt.Errorf("failed creating temporary directory for config file: %w", err))
	configFileName := configFile.Name()

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

	err = cc.Save(configFileName)
	assert.NilError(helpers.T(), err, fmt.Errorf("failed writing configuration: %w", err))

	cert := ca.NewCert(hostIP.String())
	// FIXME: this will fail in many circumstances. Review strategy on how to acquire a free port.
	// We probably have better code for that already somewhere.
	port, err = portlock.Acquire(port)
	assert.NilError(helpers.T(), err, fmt.Errorf("failed acquiring port: %w", err))
	containerName := data.Identifier(fmt.Sprintf("cesanta-auth-server-%d-%t", port, tls))
	// Cleanup possible leftovers first
	helpers.Ensure("rm", "-f", containerName)

	cleanup := func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("rm", "-f", containerName)
		errPortRelease := portlock.Release(port)
		errCertClose := cert.Close()
		errConfigClose := configFile.Close()
		errConfigRemove := os.Remove(configFileName)
		if errPortRelease != nil {
			helpers.T().Error(errPortRelease.Error())
		}
		if errCertClose != nil {
			helpers.T().Error(errCertClose.Error())
		}
		if errConfigClose != nil {
			helpers.T().Error(errConfigClose.Error())
		}
		if errConfigRemove != nil {
			helpers.T().Error(errConfigRemove.Error())
		}
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
		ensureContainerStarted(helpers, containerName)
		_, err = nettestutil.HTTPGet(fmt.Sprintf("%s://%s/auth",
			scheme,
			net.JoinHostPort(hostIP.String(), strconv.Itoa(port)),
		),
			10,
			true)
		assert.NilError(helpers.T(), err, fmt.Errorf("failed starting auth container in a timely manner: %w", err))

	}

	return &TokenAuthServer{
		IP:       hostIP,
		Port:     port,
		Scheme:   scheme,
		CertPath: cert.CertPath,
		Auth: &TokenAuth{
			Address:  scheme + "://" + net.JoinHostPort(hostIP.String(), strconv.Itoa(port)),
			CertPath: cert.CertPath,
		},
		Setup:   setup,
		Cleanup: cleanup,
		Logs: func(data test.Data, helpers test.Helpers) {
			helpers.T().Error(helpers.Err("logs", containerName))
		},
	}
}
