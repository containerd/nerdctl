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
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestStart(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run", "--name", containerName, testutil.CommonImage).AssertOK()
	base.Cmd("start", containerName).AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, containerName) {
			t.Log(stdout)
			return errors.New("got bad container name output on start")
		}
		return nil
	})
}

func TestStartAttach(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("start attach test is not yet implemented on Windows")
	}
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run", "--name", containerName, testutil.CommonImage, "sh", "-euxc", "echo foo").AssertOK()
	base.Cmd("start", "-a", containerName).AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "foo") {
			return errors.New("got bad attached output on start")
		}
		return nil
	})
}
