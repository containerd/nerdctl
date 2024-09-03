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

package container

import (
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

// https://github.com/containerd/nerdctl/issues/2598
func TestContainerListWithFormatLabel(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	cID := tID
	labelK := "label-key-" + tID
	labelV := "label-value-" + tID

	base.Cmd("run", "-d",
		"--name", cID,
		"--label", labelK+"="+labelV,
		testutil.CommonImage, "sleep", "infinity").AssertOK()
	defer base.Cmd("rm", "-f", cID).AssertOK()
	base.Cmd("ps", "-a",
		"--filter", "label="+labelK,
		"--format", fmt.Sprintf("{{.Label %q}}", labelK)).AssertOutExactly(labelV + "\n")
}

func TestContainerListWithJsonFormatLabel(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	cID := tID
	labelK := "label-key-" + tID
	labelV := "label-value-" + tID

	base.Cmd("run", "-d",
		"--name", cID,
		"--label", labelK+"="+labelV,
		testutil.CommonImage, "sleep", "infinity").AssertOK()
	defer base.Cmd("rm", "-f", cID).AssertOK()
	base.Cmd("ps", "-a",
		"--filter", "label="+labelK,
		"--format", "json").AssertOutContains(fmt.Sprintf("%s=%s", labelK, labelV))
}
