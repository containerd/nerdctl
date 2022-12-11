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

package integration

import (
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestComposePort(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    command: "sleep infinity"
    ports:
    - "12345:10000"
    - "12346:10001/udp"
`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// `port` should work for given port and protocol
	base.ComposeCmd("-f", comp.YAMLFullPath(), "port", "svc0", "10000").AssertOutExactly("0.0.0.0:12345\n")
	base.ComposeCmd("-f", comp.YAMLFullPath(), "port", "--protocol", "udp", "svc0", "10001").AssertOutExactly("0.0.0.0:12346\n")
}

func TestComposePortFailure(t *testing.T) {
	// when no port mapping is found, docker compose v1 prints `\n` while v2 prints `:0\n`
	// both v1 and v2 have exit code 0 (succeess)
	// nerdctl compose will fail with error (no public port found).
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    command: "sleep infinity"
    ports:
    - "12345:10000"
    - "12346:10001/udp"
`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// `port` should fail if given port and protocol don't exist
	base.ComposeCmd("-f", comp.YAMLFullPath(), "port", "svc0", "9999").AssertFail()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "port", "--protocol", "udp", "svc0", "10000").AssertFail()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "port", "--protocol", "tcp", "svc0", "10001").AssertFail()
}
