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
	"encoding/json"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestContainerInspectContainsPortConfig(t *testing.T) {
	//nerdctl do not support yet ipv6
	testutil.DockerIncompatible(t)
	const (
		testContainer0 = "nerdctl-test-inspect-with-port-config"
	)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer0).Run()

	const expected = `{"80/tcp":[{"HostIp":"0.0.0.0","HostPort":"8080"}]}`
	base.Cmd("run", "-d", "--name", testContainer0, "-p", "8080:80", testutil.NginxAlpineImage).AssertOK()
	inspect0 := base.InspectContainer(testContainer0)
	returnedJson, _ := json.Marshal(inspect0.NetworkSettings.Ports)
	assert.Equal(base.T, expected, string(returnedJson))
}
