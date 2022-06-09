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

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestRunSysctl(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "--sysctl", "net.ipv4.ip_forward=1", testutil.AlpineImage, "cat", "/proc/sys/net/ipv4/ip_forward").AssertOutExactly("1\n")
}
