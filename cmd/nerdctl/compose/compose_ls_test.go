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

package compose

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"gotest.tools/v3/assert"
)

func TestProjectListSummary(t *testing.T) {
	containers := []projectContainer{
		{project: "web", state: "running", configFiles: "/app/base.yaml,/app/dev.yaml"},
		{project: "web", state: "running", configFiles: "/app/base.yaml,/app/dev.yaml"},
		{project: "web", state: "exited", configFiles: "/app/base.yaml"},
		{project: "db", state: "paused"},
		{project: "other", state: "created"},
		{state: "running"},
	}
	assert.DeepEqual(t, summarizeProjects(containers, projectListOptions{}), []projectSummary{
		{Name: "web", Status: "running(2)", ConfigFiles: "/app/base.yaml,/app/dev.yaml"},
	})
	assert.DeepEqual(t, summarizeProjects(containers, projectListOptions{all: true}), []projectSummary{
		{Name: "db", Status: "paused(1)", ConfigFiles: "N/A"},
		{Name: "other", Status: "created(1)", ConfigFiles: "N/A"},
		{Name: "web", Status: "exited(1), running(2)", ConfigFiles: "/app/base.yaml,/app/dev.yaml"},
	})
	assert.DeepEqual(t, summarizeProjects(nil, projectListOptions{}), []projectSummary{})
	// The ordering of containerd's list response must not affect the output.
	for left, right := 0, len(containers)-1; left < right; left, right = left+1, right-1 {
		containers[left], containers[right] = containers[right], containers[left]
	}
	projects := summarizeProjects(containers, projectListOptions{all: true})
	assert.Equal(t, "db", projects[0].Name)
	assert.Equal(t, "exited(1), running(2)", projects[2].Status)
	assert.Equal(t, "/app/base.yaml,/app/dev.yaml", projects[2].ConfigFiles)
}

func TestProjectListOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--format", "yaml"},
		{"--filter", "name"},
		{"--filter", "name="},
		{"--filter", "status=running"},
		{"--filter", "name=["},
	} {
		cmd := lsCommand()
		assert.NilError(t, cmd.ParseFlags(args))
		_, err := readProjectListOptions(cmd)
		assert.Assert(t, err != nil, "expected validation error for %v", args)
	}
	cmd := lsCommand()
	assert.NilError(t, cmd.ParseFlags([]string{"-a", "-q", "--format=json", "--filter=name=^web$", "--filter=name=^db$"}))
	opts, err := readProjectListOptions(cmd)
	assert.NilError(t, err)
	assert.Assert(t, opts.all && opts.quiet)
	assert.Equal(t, "json", opts.format)
	assert.Assert(t, projectNameMatches("web", opts.filters))
	assert.Assert(t, projectNameMatches("db", opts.filters))
	assert.Assert(t, !projectNameMatches("web-other", opts.filters))
	assert.NilError(t, cmd.Args(cmd, nil))
	assert.Assert(t, cmd.Args(cmd, []string{"unexpected"}) != nil)
}

func TestProjectListFilters(t *testing.T) {
	projects := summarizeProjects([]projectContainer{
		{project: "prod-web", state: "running"},
		{project: "dev-web", state: "running"},
		{project: "prod-db", state: "exited"},
	}, projectListOptions{all: true, filters: []*regexp.Regexp{regexp.MustCompile("^prod-")}})
	assert.DeepEqual(t, projects, []projectSummary{
		{Name: "prod-db", Status: "exited(1)", ConfigFiles: "N/A"},
		{Name: "prod-web", Status: "running(1)", ConfigFiles: "N/A"},
	})
}

func TestProjectListOutput(t *testing.T) {
	projects := []projectSummary{{Name: "web", Status: "running(2)", ConfigFiles: "/a.yaml"}}
	var output bytes.Buffer
	assert.NilError(t, writeProjectList(&output, projects, projectListOptions{quiet: true, format: "json"}))
	assert.Equal(t, "web\n", output.String())
	output.Reset()
	assert.NilError(t, writeProjectList(&output, projects, projectListOptions{format: "json"}))
	var decoded []projectSummary
	assert.NilError(t, json.Unmarshal(output.Bytes(), &decoded))
	assert.DeepEqual(t, projects, decoded)
	output.Reset()
	assert.NilError(t, writeProjectList(&output, []projectSummary{}, projectListOptions{format: "json"}))
	assert.Equal(t, "[]\n", output.String())
	output.Reset()
	assert.NilError(t, writeProjectList(&output, projects, projectListOptions{format: "table"}))
	assert.Assert(t, bytes.Contains(output.Bytes(), []byte("CONFIG FILES")))
	assert.Assert(t, bytes.Contains(output.Bytes(), []byte("running(2)")))
	assert.Assert(t, bytes.Contains(output.Bytes(), []byte("/a.yaml")))
}

type projectListBrokenWriter struct{ err error }

func (w projectListBrokenWriter) Write([]byte) (int, error) { return 0, w.err }

func TestProjectListWriteFailure(t *testing.T) {
	want := errors.New("broken output")
	for _, opts := range []projectListOptions{{quiet: true}, {format: "json"}, {format: "table"}} {
		err := writeProjectList(projectListBrokenWriter{want}, []projectSummary{{Name: "web"}}, opts)
		assert.Assert(t, errors.Is(err, want))
	}
}
