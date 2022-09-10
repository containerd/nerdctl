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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestLogs(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("`nerdctl logs` is not implemented on Windows (why?)")
	}
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	const expected = `foo
bar`

	defer base.Cmd("rm", containerName).Run()
	base.Cmd("run", "-d", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()

	time.Sleep(3 * time.Second)
	base.Cmd("logs", "-f", containerName).AssertOutContains("bar")
	// Run logs twice, make sure that the logs are not removed
	base.Cmd("logs", "-f", containerName).AssertOutContains("foo")

	//test timestamps flag
	base.Cmd("logs", "-t", containerName).AssertOutContains(time.Now().Format("2006-01-02"))

	//test tail flag
	base.Cmd("logs", "-n", "all", containerName).AssertOutContains(expected)
	base.Cmd("logs", "-n", "1", containerName).AssertOutWithFunc(func(stdout string) error {
		if !(stdout == "bar\n" || stdout == "") {
			return fmt.Errorf("expected %q or %q, got %q", "bar", "", stdout)
		}
		return nil
	})

	//test since / until flag
	base.Cmd("logs", "--since", "1s", containerName).AssertNoOut(expected)
	base.Cmd("logs", "--since", "5s", containerName).AssertOutContains(expected)
	base.Cmd("logs", "--until", "5s", containerName).AssertNoOut(expected)
	base.Cmd("logs", "--until", "1s", containerName).AssertOutContains(expected)

	base.Cmd("rm", "-f", containerName).AssertOK()
}

func TestLogsWithInheritedFlags(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("`nerdctl logs` is not implemented on Windows (why?)")
	}
	base := testutil.NewBase(t)
	for k, v := range base.Args {
		if strings.HasPrefix(v, "--namespace=") {
			base.Args[k] = "-n=" + testutil.Namespace
		}
	}
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", containerName).Run()
	base.Cmd("run", "-d", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()

	// test rootCmd alias `-n` already used in logs subcommand
	base.Cmd("logs", "-n", "1", containerName).AssertOutWithFunc(func(stdout string) error {
		if !(stdout == "bar\n" || stdout == "") {
			return fmt.Errorf("expected %q or %q, got %q", "bar", "", stdout)
		}
		return nil
	})
}

func TestLogsOfJournaldDriver(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("`nerdctl logs` is not implemented on Windows (why?)")
	}
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", containerName).Run()
	base.Cmd("run", "-d", "--network", "none", "--log-driver", "journald", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()

	time.Sleep(3 * time.Second)
	base.Cmd("logs", containerName).AssertOutContains("bar")
	// Run logs twice, make sure that the logs are not removed
	base.Cmd("logs", containerName).AssertOutContains("foo")

	base.Cmd("logs", "--since", "5s", containerName).AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "bar") {
			return fmt.Errorf("expected bar, got %s", stdout)
		}
		if !strings.Contains(stdout, "foo") {
			return fmt.Errorf("expected foo, got %s", stdout)
		}
		return nil
	})

	base.Cmd("rm", "-f", containerName).AssertOK()
}

func TestLogsWithFailingContainer(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("`nerdctl logs` is not implemented on Windows (why?)")
	}
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", containerName).Run()
	base.Cmd("run", "-d", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar; exit 42; echo baz").AssertOK()
	time.Sleep(3 * time.Second)
	// AssertOutContains also asserts that the exit code of the logs command == 0,
	// even when the container is failing
	base.Cmd("logs", "-f", containerName).AssertOutContains("bar")
	base.Cmd("logs", "-f", containerName).AssertNoOut("baz")
	base.Cmd("rm", "-f", containerName).AssertOK()
}
