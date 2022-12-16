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

func TestComposeStart(t *testing.T) {
	base := testutil.NewBase(t)
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

	upAssertHandler := func(svc string) func(stdout string) error {
		return func(stdout string) error {
			// Docker Compose v1: "Up", v2: "running"
			if !strings.Contains(stdout, "Up") && !strings.Contains(stdout, "running") {
				return fmt.Errorf("service \"%s\" must have been still running", svc)
			}
			return nil
		}
	}

	// calling `compose start` after all services up has no effect.
	base.ComposeCmd("-f", comp.YAMLFullPath(), "start").AssertOK()

	// `compose start`` can start a stopped/killed service container
	base.ComposeCmd("-f", comp.YAMLFullPath(), "stop", "--timeout", "1", "svc0").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "kill", "svc1").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "start").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc0").AssertOutWithFunc(upAssertHandler("svc0"))
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc1").AssertOutWithFunc(upAssertHandler("svc1"))
}

func TestComposeStartFailWhenServicePause(t *testing.T) {
	// Incompatible with docker compose v1. Currently CI is using compose v1.
	// Starting a paused container triggers an error in v2 but is ignored in v1.
	testutil.DockerIncompatible(t)

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
`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// `compose start` cannot start a paused service container
	base.ComposeCmd("-f", comp.YAMLFullPath(), "pause", "svc0").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "start").AssertFail()
}
