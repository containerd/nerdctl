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
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestMain(m *testing.M) {
	testutil.M(m)
}

// TestUnknownCommand tests https://github.com/containerd/nerdctl/issues/487
func TestUnknownCommand(t *testing.T) {
	testCase := nerdtest.Setup()

	var cmd = errors.New("unknown subcommand")

	testCase.SubTests = []*test.Case{
		{
			Description: "non-existent-command",
			Command:     test.Command("non-existent-command"),
			Expected:    test.Expects(1, []error{cmd}, nil),
		},
		{
			Description: "non-existent-command info",
			Command:     test.Command("non-existent-command", "info"),
			Expected:    test.Expects(1, []error{cmd}, nil),
		},
		{
			Description: "system non-existent-command",
			Command:     test.Command("system", "non-existent-command"),
			Expected:    test.Expects(1, []error{cmd}, nil),
		},
		{
			Description: "system non-existent-command info",
			Command:     test.Command("system", "non-existent-command", "info"),
			Expected:    test.Expects(1, []error{cmd}, nil),
		},
		{
			Description: "system",
			Command:     test.Command("system"),
			Expected:    test.Expects(0, nil, nil),
		},
		{
			Description: "system info",
			Command:     test.Command("system", "info"),
			Expected:    test.Expects(0, nil, expect.Contains("Kernel Version:")),
		},
		{
			Description: "info",
			Command:     test.Command("info"),
			Expected:    test.Expects(0, nil, expect.Contains("Kernel Version:")),
		},
	}

	testCase.Run(t)
}

// TestNerdctlConfig validates the configuration precedence [CLI, Env, TOML, Default] and broken config rejection
func TestNerdctlConfig(t *testing.T) {
	testCase := nerdtest.Setup()

	// Docker does not support nerdctl.toml obviously
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.SubTests = []*test.Case{
		{
			Description: "Default",
			Command:     test.Command("info", "-f", "{{.Driver}}"),
			Expected:    test.Expects(0, nil, expect.Equals(""+"\n")),
		},
		{
			Description: "TOML > Default",
			Command:     test.Command("info", "-f", "{{.Driver}}"),
			Expected:    test.Expects(0, nil, expect.Equals("dummy-snapshotter-via-toml\n")),
			Config:      test.WithConfig(nerdtest.NerdctlToml, `snapshotter = "dummy-snapshotter-via-toml"`),
		},
		{
			Description: "Cli > TOML > Default",
			Command:     test.Command("info", "-f", "{{.Driver}}", "--snapshotter=dummy-snapshotter-via-cli"),
			Expected:    test.Expects(0, nil, expect.Equals("dummy-snapshotter-via-cli\n")),
			Config:      test.WithConfig(nerdtest.NerdctlToml, `snapshotter = "dummy-snapshotter-via-toml"`),
		},
		{
			Description: "Env > TOML > Default",
			Command:     test.Command("info", "-f", "{{.Driver}}"),
			Env:         map[string]string{"CONTAINERD_SNAPSHOTTER": "dummy-snapshotter-via-env"},
			Expected:    test.Expects(0, nil, expect.Equals("dummy-snapshotter-via-env\n")),
			Config:      test.WithConfig(nerdtest.NerdctlToml, `snapshotter = "dummy-snapshotter-via-toml"`),
		},
		{
			Description: "Cli > Env > TOML > Default",
			Command:     test.Command("info", "-f", "{{.Driver}}", "--snapshotter=dummy-snapshotter-via-cli"),
			Env:         map[string]string{"CONTAINERD_SNAPSHOTTER": "dummy-snapshotter-via-env"},
			Expected:    test.Expects(0, nil, expect.Equals("dummy-snapshotter-via-cli\n")),
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

func TestRootHelpHidesAliasImplementationFlags(t *testing.T) {
	app, err := newApp()
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	app.SetOut(&stdout)
	app.SetErr(&stdout)
	app.SetArgs([]string{"--help"})

	if err := app.Execute(); err != nil {
		t.Fatal(err)
	}

	out := stdout.String()
	for _, unexpected := range []string{
		"-a, --a",
		"-H, --H",
		"-n, --n",
	} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("help output unexpectedly contains %q\n%s", unexpected, out)
		}
	}
	for _, expected := range []string{
		"--address string           containerd address, optionally with \"unix://\" prefix [$CONTAINERD_ADDRESS] (aliases: -a, -H, --host)",
		"--namespace string         containerd namespace, such as \"moby\" for Docker, \"k8s.io\" for Kubernetes [$CONTAINERD_NAMESPACE] (aliases: -n)",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("help output missing %q\n%s", expected, out)
		}
	}
}

func TestRootHiddenAliasesStillParse(t *testing.T) {
	app, err := newApp()
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	app.SetOut(&stdout)
	app.SetErr(&stdout)
	app.SetArgs([]string{
		"-a", "unix:///tmp/a.sock",
		"-H", "unix:///tmp/h.sock",
		"--host", "unix:///tmp/host.sock",
		"-n", "testns",
		"--storage-driver", "native",
		"--help",
	})

	if err := app.Execute(); err != nil {
		t.Fatal(err)
	}

	if got := app.Flag("address").Value.String(); got != "unix:///tmp/host.sock" {
		t.Fatalf("address flag = %q, want %q", got, "unix:///tmp/host.sock")
	}
	if got := app.Flag("namespace").Value.String(); got != "testns" {
		t.Fatalf("namespace flag = %q, want %q", got, "testns")
	}
	if got := app.Flag("snapshotter").Value.String(); got != "native" {
		t.Fatalf("snapshotter flag = %q, want %q", got, "native")
	}
}
