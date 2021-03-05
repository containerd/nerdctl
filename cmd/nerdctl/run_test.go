/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

	"github.com/AkihiroSuda/nerdctl/pkg/testutil"
)

func TestRunWorkdir(t *testing.T) {
	base := testutil.NewBase(t)
	cmd := base.Cmd("run", "--rm", "--workdir=/foo", testutil.AlpineImage, "pwd")
	cmd.AssertOut("/foo")
}
