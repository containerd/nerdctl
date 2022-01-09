package main

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/pkg/errors"
	"gotest.tools/v3/assert"
)

func TestRestart(t *testing.T) {
	const (
		hostPort          = 8080
		testContainerName = "nerdctl-test-stop-start-nginx"
		sleepTime         = 3 * time.Second
	)
	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainerName).Run()

	base.Cmd("run", "-d",
		"--restart=no",
		"--name", testContainerName,
		"-p", fmt.Sprintf("127.0.0.1:%d:80", hostPort),
		testutil.NginxAlpineImage).AssertOK()

	check := func(httpGetRetry int) error {
		resp, err := httpGet(fmt.Sprintf("http://127.0.0.1:%d", hostPort), httpGetRetry)
		if err != nil {
			return err
		}
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if !strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet) {
			return errors.Errorf("expected contain %q, got %q",
				testutil.NginxAlpineIndexHTMLSnippet, string(respBody))
		}
		return nil
	}

	assert.NilError(t, check(30))

	base.Cmd("restart", testContainerName).AssertOK()
	base.Cmd("exec", testContainerName, "ps").AssertOK()
	time.Sleep(sleepTime)
	if check(3) != nil {
		t.Fatal("restart faild,expected to get an error")
	}
	assert.NilError(t, check(30))
}
