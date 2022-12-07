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
	"fmt"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestComposePauseAndUnpause(t *testing.T) {
	base := testutil.NewBase(t)
	switch base.Info().CgroupDriver {
	case "none", "":
		t.Skip("requires cgroup (for pausing)")
	}

	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    command: "sleep infinity"
  svc1:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	pausedAssertHandler := func(svc string) func(stdout string) error {
		return func(stdout string) error {
			// Docker Compose v1: "Paused", v2: "paused"
			if !strings.Contains(stdout, "Paused") && !strings.Contains(stdout, "paused") {
				return fmt.Errorf("service \"%s\" must have paused", svc)
			}
			return nil
		}
	}
	upAssertHandler := func(svc string) func(stdout string) error {
		return func(stdout string) error {
			// Docker Compose v1: "Up", v2: "running"
			if !strings.Contains(stdout, "Up") && !strings.Contains(stdout, "running") {
				return fmt.Errorf("service \"%s\" must have been still running", svc)
			}
			return nil
		}
	}

	// pause a service should (only) pause its own container
	base.ComposeCmd("-f", comp.YAMLFullPath(), "pause", "svc0").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc0").AssertOutWithFunc(pausedAssertHandler("svc0"))
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc1").AssertOutWithFunc(upAssertHandler("svc1"))

	// unpause should be able to recover the paused service container
	base.ComposeCmd("-f", comp.YAMLFullPath(), "unpause", "svc0").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc0").AssertOutWithFunc(upAssertHandler("svc0"))
}
