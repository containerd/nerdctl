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

func TestRunPortMappingWithEmptyIP(t *testing.T) {
	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", "testPortMappingWithEmptyIP").Run()
	const expected = `80/tcp -> 0.0.0.0:80
80/tcp -> :::80`
	base.Cmd("run", "-d", "--name", "testPortMappingWithEmptyIP", "-p", "80:80", testutil.NginxAlpineImage).Run()
	base.Cmd("port", "testPortMappingWithEmptyIP").AssertOut(expected)
}

func TestRunPortMappingWithIPv6(t *testing.T) {
	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", "testPortMappingWithIPv6").Run()
	base.Cmd("run", "-d", "--name", "testPortMappingWithIPv6", "-p", "[::]:80:80", testutil.NginxAlpineImage).Run()
	const expected = `80/tcp -> :::80`
	base.Cmd("port", "testPortMappingWithIPv6").AssertOut(expected)
}
