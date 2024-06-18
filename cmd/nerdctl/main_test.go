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
	"os"
	"path/filepath"
	"testing"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestMain(m *testing.M) {
	testutil.M(m)
}

// TestUnknownCommand tests https://github.com/containerd/nerdctl/issues/487
func TestUnknownCommand(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("non-existent-command").AssertFail()
	base.Cmd("non-existent-command", "info").AssertFail()
	base.Cmd("system", "non-existent-command").AssertFail()
	base.Cmd("system", "non-existent-command", "info").AssertFail()
	base.Cmd("system").AssertOK() // show help without error
	base.Cmd("system", "info").AssertOutContains("Kernel Version:")
	base.Cmd("info").AssertOutContains("Kernel Version:")
}

// TestNerdctlConfig validates the configuration precedence [CLI, Env, TOML, Default].
func TestNerdctlConfig(t *testing.T) {
	testutil.DockerIncompatible(t)
	t.Parallel()
	tomlPath := filepath.Join(t.TempDir(), "nerdctl.toml")
	err := os.WriteFile(tomlPath, []byte(`
snapshotter = "dummy-snapshotter-via-toml"
`), 0400)
	assert.NilError(t, err)
	base := testutil.NewBase(t)

	// [Default]
	base.Cmd("info", "-f", "{{.Driver}}").AssertOutExactly(containerd.DefaultSnapshotter + "\n")

	// [TOML, Default]
	base.Env = append(base.Env, "NERDCTL_TOML="+tomlPath)
	base.Cmd("info", "-f", "{{.Driver}}").AssertOutExactly("dummy-snapshotter-via-toml\n")

	// [CLI, TOML, Default]
	base.Cmd("info", "-f", "{{.Driver}}", "--snapshotter=dummy-snapshotter-via-cli").AssertOutExactly("dummy-snapshotter-via-cli\n")

	// [Env, TOML, Default]
	base.Env = append(base.Env, "CONTAINERD_SNAPSHOTTER=dummy-snapshotter-via-env")
	base.Cmd("info", "-f", "{{.Driver}}").AssertOutExactly("dummy-snapshotter-via-env\n")

	// [CLI, Env, TOML, Default]
	base.Cmd("info", "-f", "{{.Driver}}", "--snapshotter=dummy-snapshotter-via-cli").AssertOutExactly("dummy-snapshotter-via-cli\n")
}

func TestNerdctlConfigBad(t *testing.T) {
	testutil.DockerIncompatible(t)
	t.Parallel()
	tomlPath := filepath.Join(t.TempDir(), "config.toml")
	err := os.WriteFile(tomlPath, []byte(`
# containerd config, not nerdctl config
version = 2
`), 0400)
	assert.NilError(t, err)
	base := testutil.NewBase(t)
	base.Env = append(base.Env, "NERDCTL_TOML="+tomlPath)
	base.Cmd("info").AssertFail()
}
