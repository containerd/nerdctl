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

package nerdtest

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

const (
	// It seems that at this moment, the busybox on windows image we are using has an outdated version of sleep
	// that does not support inf/infinity.
	// This constant is provided as a mean for tests to express the intention of sleep infinity without having to
	// worry about that and get windows compatibility.
	Infinity = "3600"
)

func IsDocker() bool {
	return testutil.GetTarget() == "docker"
}

// InspectContainer is a helper that can be used inside custom commands or Setup
func InspectContainer(helpers test.Helpers, name string) dockercompat.Container {
	helpers.T().Helper()
	var res dockercompat.Container
	cmd := helpers.Command("container", "inspect", name)
	cmd.Run(&test.Expected{
		Output: expect.JSON([]dockercompat.Container{}, func(dc []dockercompat.Container, _ string, t tig.T) {
			assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results")
			res = dc[0]
		}),
	})
	return res
}

func InspectVolume(helpers test.Helpers, name string) native.Volume {
	helpers.T().Helper()
	var res native.Volume
	cmd := helpers.Command("volume", "inspect", name)
	cmd.Run(&test.Expected{
		Output: expect.JSON([]native.Volume{}, func(dc []native.Volume, _ string, t tig.T) {
			assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results")
			res = dc[0]
		}),
	})
	return res
}

func InspectNetwork(helpers test.Helpers, name string) dockercompat.Network {
	helpers.T().Helper()
	var res dockercompat.Network
	cmd := helpers.Command("network", "inspect", name)
	cmd.Run(&test.Expected{
		Output: expect.JSON([]dockercompat.Network{}, func(dc []dockercompat.Network, _ string, t tig.T) {
			assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results")
			res = dc[0]
		}),
	})
	return res
}

func InspectImage(helpers test.Helpers, name string) dockercompat.Image {
	helpers.T().Helper()
	var res dockercompat.Image
	cmd := helpers.Command("image", "inspect", name)
	cmd.Run(&test.Expected{
		Output: expect.JSON([]dockercompat.Image{}, func(dc []dockercompat.Image, _ string, t tig.T) {
			assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results")
			res = dc[0]
		}),
	})
	return res
}

const (
	maxRetry = 20
	sleep    = time.Second
)

func EnsureContainerStarted(helpers test.Helpers, con string) {
	helpers.T().Helper()
	started := false
	for i := 0; i < maxRetry && !started; i++ {
		helpers.Command("container", "inspect", con).
			Run(&test.Expected{
				ExitCode: expect.ExitCodeNoCheck,
				Output: func(stdout string, info string, t *testing.T) {
					var dc []dockercompat.Container
					err := json.Unmarshal([]byte(stdout), &dc)
					if err != nil || len(dc) == 0 {
						return
					}
					assert.Equal(t, len(dc), 1, "Unexpectedly got multiple results\n"+info)
					started = dc[0].State.Running
				},
			})
		time.Sleep(sleep)
	}

	if !started {
		ins := helpers.Capture("container", "inspect", con)
		lgs := helpers.Capture("logs", con)
		ps := helpers.Capture("ps", "-a")
		helpers.T().Log(ins)
		helpers.T().Log(lgs)
		helpers.T().Log(ps)
		helpers.T().Fatalf("container %s still not running after %d retries", con, maxRetry)
	}
}

func GenerateJWEKeyPair(data test.Data, helpers test.Helpers) (string, string) {
	helpers.T().Helper()

	path := "jwe-key-pair"
	data.Temp().Dir(path)

	pass, message := require.Binary("openssl").Check(data, helpers)
	if !pass {
		helpers.T().Skip(message)
	}

	pri := data.Temp().Path(path, "mykey.pem")
	pub := data.Temp().Path(path, "mypubkey.pem")

	// Exec openssl commands to ensure that nerdctl is compatible with the output of openssl commands.
	// Do NOT refactor this function to use "crypto/rsa" stdlib.
	helpers.Custom("openssl", "genrsa", "-out", pri).Run(&test.Expected{})
	helpers.Custom("openssl", "rsa", "-in", pri, "-pubout", "-out", pub).Run(&test.Expected{})

	return pri, pub
}

func GenerateCosignKeyPair(data test.Data, helpers test.Helpers, password string) (pri string, pub string) {
	helpers.T().Helper()

	path := "cosign-key-pair"
	data.Temp().Dir(path)

	pass, message := require.Binary("cosign").Check(data, helpers)
	if !pass {
		helpers.T().Skip(message)
	}

	cmd := helpers.Custom("cosign", "generate-key-pair")
	cmd.WithCwd(data.Temp().Path(path))
	cmd.Setenv("COSIGN_PASSWORD", password)
	cmd.Run(&test.Expected{})

	return data.Temp().Path(path, "cosign.key"), data.Temp().Path(path, "cosign.pub")
}

func FindIPv6(output string) net.IP {
	var ipv6 string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "inet6") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				ipv6 = strings.Split(fields[1], "/")[0]
				break
			}
		}
	}
	return net.ParseIP(ipv6)
}
