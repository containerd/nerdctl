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
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestLogs(t *testing.T) {
	base := testutil.NewBase(t)
	const containerName = "nerdctl-test-logs"
	defer base.Cmd("rm", containerName).Run()
	base.Cmd("run", "-d", "--name", containerName, testutil.AlpineImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()
	time.Sleep(3 * time.Second)
	base.Cmd("logs", "-f", containerName).AssertOut("bar")
	// Run logs twice, make sure that the logs are not removed
	base.Cmd("logs", "-f", containerName).AssertOut("foo")
	base.Cmd("rm", "-f", containerName).AssertOK()
}

func TestLogsWithFailingContainer(t *testing.T) {
	base := testutil.NewBase(t)
	const containerName = "nerdctl-test-logs"
	defer base.Cmd("rm", containerName).Run()
	base.Cmd("run", "-d", "--name", containerName, testutil.AlpineImage,
		"sh", "-euxc", "echo foo; echo bar; exit 42; echo baz").AssertOK()
	time.Sleep(3 * time.Second)
	// AssertOut also asserts that the exit code of the logs command == 0,
	// even when the container is failing
	base.Cmd("logs", "-f", containerName).AssertOut("bar")
	base.Cmd("logs", "-f", containerName).AssertNoOut("baz")
	base.Cmd("rm", "-f", containerName).AssertOK()
}
