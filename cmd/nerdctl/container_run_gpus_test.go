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

	"github.com/containerd/nerdctl/pkg/cmd/container"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func TestParseGpusOptAll(t *testing.T) {
	t.Parallel()
	for _, testcase := range []string{
		"all",
		"-1",
		"count=all",
		"count=-1",
	} {
		req, err := container.ParseGPUOptCSV(testcase)
		assert.NilError(t, err)
		assert.Equal(t, req.Count, -1)
		assert.Equal(t, len(req.DeviceIDs), 0)
		assert.Equal(t, len(req.Capabilities), 0)
	}
}

func TestParseGpusOpts(t *testing.T) {
	t.Parallel()
	for _, testcase := range []string{
		"driver=nvidia,\"capabilities=compute,utility\"",
		"1,driver=nvidia,\"capabilities=compute,utility\"",
		"count=1,driver=nvidia,\"capabilities=compute,utility\"",
		"driver=nvidia,\"capabilities=compute,utility\",count=1",
		"\"capabilities=compute,utility\",count=1",
	} {
		req, err := container.ParseGPUOptCSV(testcase)
		assert.NilError(t, err)
		assert.Equal(t, req.Count, 1)
		assert.Equal(t, len(req.DeviceIDs), 0)
		assert.Check(t, is.DeepEqual(req.Capabilities, []string{"compute", "utility"}))
	}
}
