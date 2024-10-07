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
	"errors"
	"testing"

	"github.com/containerd/containerd/v2/defaults"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestMain(m *testing.M) {
	testutil.M(m)
}

// TestUnknownCommand tests https://github.com/containerd/nerdctl/issues/487
func TestUnknownCommand(t *testing.T) {
	testCase := nerdtest.Setup()

	var unknownSubCommand = errors.New("unknown subcommand")

	testCase.SubTests = []*test.Case{
		{
			Description: "non-existent-command",
			Command:     test.Command("non-existent-command"),
			Expected:    test.Expects(1, []error{unknownSubCommand}, nil),
		},
		{
			Description: "non-existent-command info",
			Command:     test.Command("non-existent-command", "info"),
			Expected:    test.Expects(1, []error{unknownSubCommand}, nil),
		},
		{
			Description: "system non-existent-command",
			Command:     test.Command("system", "non-existent-command"),
			Expected:    test.Expects(1, []error{unknownSubCommand}, nil),
		},
		{
			Description: "system non-existent-command info",
			Command:     test.Command("system", "non-existent-command", "info"),
			Expected:    test.Expects(1, []error{unknownSubCommand}, nil),
		},
		{
			Description: "system",
			Command:     test.Command("system"),
			Expected:    test.Expects(0, nil, nil),
		},
		{
			Description: "system info",
			Command:     test.Command("system", "info"),
			Expected:    test.Expects(0, nil, test.Contains("Kernel Version:")),
		},
		{
			Description: "info",
			Command:     test.Command("info"),
			Expected:    test.Expects(0, nil, test.Contains("Kernel Version:")),
		},
	}

	testCase.Run(t)
}

// TestNerdctlConfig validates the configuration precedence [CLI, Env, TOML, Default] and broken config rejection
func TestNerdctlConfig(t *testing.T) {
	testCase := nerdtest.Setup()

	// Docker does not support nerdctl.toml obviously
	testCase.Require = test.Not(nerdtest.Docker)

	testCase.SubTests = []*test.Case{
		{
			Description: "Default",
			Command:     test.Command("info", "-f", "{{.Driver}}"),
			Expected:    test.Expects(0, nil, test.Equals(defaults.DefaultSnapshotter+"\n")),
		},
		{
			Description: "TOML > Default",
			Command:     test.Command("info", "-f", "{{.Driver}}"),
			Expected:    test.Expects(0, nil, test.Equals("dummy-snapshotter-via-toml\n")),
			Config:      test.WithConfig(nerdtest.NerdctlToml, `snapshotter = "dummy-snapshotter-via-toml"`),
		},
		{
			Description: "Cli > TOML > Default",
			Command:     test.Command("info", "-f", "{{.Driver}}", "--snapshotter=dummy-snapshotter-via-cli"),
			Expected:    test.Expects(0, nil, test.Equals("dummy-snapshotter-via-cli\n")),
			Config:      test.WithConfig(nerdtest.NerdctlToml, `snapshotter = "dummy-snapshotter-via-toml"`),
		},
		{
			Description: "Env > TOML > Default",
			Command:     test.Command("info", "-f", "{{.Driver}}"),
			Env:         map[string]string{"CONTAINERD_SNAPSHOTTER": "dummy-snapshotter-via-env"},
			Expected:    test.Expects(0, nil, test.Equals("dummy-snapshotter-via-env\n")),
			Config:      test.WithConfig(nerdtest.NerdctlToml, `snapshotter = "dummy-snapshotter-via-toml"`),
		},
		{
			Description: "Cli > Env > TOML > Default",
			Command:     test.Command("info", "-f", "{{.Driver}}", "--snapshotter=dummy-snapshotter-via-cli"),
			Env:         map[string]string{"CONTAINERD_SNAPSHOTTER": "dummy-snapshotter-via-env"},
			Expected:    test.Expects(0, nil, test.Equals("dummy-snapshotter-via-cli\n")),
			Config:      test.WithConfig(nerdtest.NerdctlToml, `snapshotter = "dummy-snapshotter-via-toml"`),
		},
		{
			Description: "Broken config",
			Command:     test.Command("info"),
			Expected:    test.Expects(1, []error{errors.New("failed to load nerdctl config")}, nil),
			Config: test.WithConfig(nerdtest.NerdctlToml, `# containerd config, not nerdctl config
version = 2`),
		},
	}

	testCase.Run(t)
}
