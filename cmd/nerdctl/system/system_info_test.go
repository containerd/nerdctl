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

package system

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/infoutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func testInfoJSON(stdout string) error {
	var info dockercompat.Info
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		return err
	}
	unameM := infoutil.UnameM()
	if info.Architecture != unameM {
		return fmt.Errorf("expected info.Architecture to be %q, got %q", unameM, info.Architecture)
	}
	return nil
}

func TestInfo(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("info", "--format", "{{json .}}").AssertOutWithFunc(testInfoJSON)
}

func TestInfoConvenienceForm(t *testing.T) {
	testutil.DockerIncompatible(t) // until https://github.com/docker/cli/pull/3355 gets merged
	base := testutil.NewBase(t)
	base.Cmd("info", "--format", "json").AssertOutWithFunc(testInfoJSON)
}

func TestInfoWithNamespace(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	base.Args = nil // unset "--namespace=nerdctl-test"

	base.Cmd("info").AssertOutContains("Namespace:	default")

	base.Env = append(base.Env, "CONTAINERD_NAMESPACE=test")
	base.Cmd("info").AssertOutContains("Namespace:	test")
}
