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
	"io"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/testutil"

	"gotest.tools/v3/assert"
)

func TestRunRestart(t *testing.T) {
	const (
		hostPort          = 8080
		testContainerName = "nerdctl-test-restart-nginx"
	)
	if testing.Short() {
		t.Skipf("test is long")
	}
	base := testutil.NewBase(t)
	if !base.DaemonIsKillable {
		t.Skip("daemon is not killable (hint: set \"-test.kill-daemon\")")
	}
	t.Log("NOTE: this test may take a while")

	defer base.Cmd("rm", "-f", testContainerName).Run()

	base.Cmd("run", "-d",
		"--restart=always",
		"--name", testContainerName,
		"-p", fmt.Sprintf("127.0.0.1:%d:80", hostPort),
		testutil.NginxAlpineImage).AssertOK()

	check := func(httpGetRetry int) error {
		resp, err := httpGet(fmt.Sprintf("http://127.0.0.1:%d", hostPort), httpGetRetry)
		if err != nil {
			return err
		}
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if !strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet) {
			return fmt.Errorf("expected contain %q, got %q",
				testutil.NginxAlpineIndexHTMLSnippet, string(respBody))
		}
		return nil
	}
	assert.NilError(t, check(30))

	base.KillDaemon()
	base.EnsureDaemonActive()

	const (
		maxRetry = 30
		sleep    = 3 * time.Second
	)
	for i := 0; i < maxRetry; i++ {
		t.Logf("(retry %d) ps -a: %q", i, base.Cmd("ps", "-a").Run().Combined())
		err := check(1)
		if err == nil {
			t.Logf("test is passing, after %d retries", i)
			return
		}
		time.Sleep(sleep)
	}
	base.DumpDaemonLogs(10)
	t.Fatalf("the container does not seem to be restarted")
}
