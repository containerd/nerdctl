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

	"github.com/containerd/cgroups"
	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestRunCgroupV2(t *testing.T) {
	if cgroups.Mode() != cgroups.Unified {
		t.Skip("test requires cgroup v2")
	}
	base := testutil.NewBase(t)
	const expected = `42000 100000
44040192
42
77
0-1
`
	//In CgroupV2 CPUWeight replace CPUShares => weight := 1 + ((shares-2)*9999)/262142
	base.Cmd("run", "--rm", "--cpus", "0.42", "--memory", "42m", "--pids-limit", "42", "--cpu-shares", "2000", "--cpuset-cpus", "0-1", testutil.AlpineImage,
		"sh", "-ec", "cd /sys/fs/cgroup && cat cpu.max memory.max pids.max cpu.weight cpuset.cpus").AssertOutContains(expected)
}
