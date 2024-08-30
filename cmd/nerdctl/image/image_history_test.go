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

package image

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

type historyObj struct {
	Snapshot     string
	CreatedAt    string
	CreatedSince string
	CreatedBy    string
	Size         string
	Comment      string
}

func imageHistoryJSONHelper(base *testutil.Base, reference string, noTrunc bool, quiet bool, human bool) []historyObj {
	cmd := []string{"image", "history"}
	if noTrunc {
		cmd = append(cmd, "--no-trunc")
	}
	if quiet {
		cmd = append(cmd, "--quiet")
	}
	cmd = append(cmd, fmt.Sprintf("--human=%t", human))
	cmd = append(cmd, "--format", "json")
	cmd = append(cmd, reference)

	cmdResult := base.Cmd(cmd...).Run()
	assert.Equal(base.T, cmdResult.ExitCode, 0, cmdResult.Stdout())

	fmt.Println(cmdResult.Stderr())

	dec := json.NewDecoder(strings.NewReader(cmdResult.Stdout()))
	object := []historyObj{}
	for {
		var v historyObj
		if err := dec.Decode(&v); err == io.EOF {
			break
		} else if err != nil {
			base.T.Fatal(err)
		}
		object = append(object, v)
	}

	return object
}

func imageHistoryRawHelper(base *testutil.Base, reference string, noTrunc bool, quiet bool, human bool) string {
	cmd := []string{"image", "history"}
	if noTrunc {
		cmd = append(cmd, "--no-trunc")
	}
	if quiet {
		cmd = append(cmd, "--quiet")
	}
	cmd = append(cmd, fmt.Sprintf("--human=%t", human))
	cmd = append(cmd, reference)

	cmdResult := base.Cmd(cmd...).Run()
	assert.Equal(base.T, cmdResult.ExitCode, 0, cmdResult.Stdout())

	return cmdResult.Stdout()
}

func TestImageHistory(t *testing.T) {
	// Here are the current issues with regard to docker true compatibility:
	// - we have a different definition of what a layer id is (snapshot vs. id)
	//     this will require indepth convergence when moby will handle multi-platform images
	// - our definition of size is different
	//     this requires some investigation to figure out why it differs
	//     possibly one is unpacked on the filessystem while the other is the tar file size?
	// - we do not truncate ids when --quiet has been provided
	//     this is a conscious decision here - truncating with --quiet does not make much sense
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)

	// XXX the results here are obviously platform dependent - and it seems like windows cannot pull a linux image?
	// Disabling for now
	if runtime.GOOS == "windows" {
		t.Skip("Windows is not supported for this test right now")
	}

	// XXX Currently, history does not work on non-native platform, so, we cannot test reliably on other platforms
	if runtime.GOARCH != "arm64" {
		t.Skip("Windows is not supported for this test right now")
	}

	base.Cmd("pull", "--platform", "linux/arm64", testutil.CommonImage).AssertOK()

	localTimeL1, _ := time.Parse(time.RFC3339, "2021-03-31T10:21:23-07:00")
	localTimeL2, _ := time.Parse(time.RFC3339, "2021-03-31T10:21:21-07:00")

	// Human, no quiet, truncate
	history := imageHistoryJSONHelper(base, testutil.CommonImage, false, false, true)
	compTime1, _ := time.Parse(time.RFC3339, history[0].CreatedAt)
	compTime2, _ := time.Parse(time.RFC3339, history[1].CreatedAt)

	// Two layers
	assert.Equal(base.T, len(history), 2)
	// First layer is a comment - zero size, no snap,
	assert.Equal(base.T, history[0].Size, "0B")
	assert.Equal(base.T, history[0].CreatedSince, "3 years ago")
	assert.Equal(base.T, history[0].Snapshot, "<missing>")
	assert.Equal(base.T, history[0].Comment, "")

	assert.Equal(base.T, compTime1.UTC().String(), localTimeL1.UTC().String())
	assert.Equal(base.T, history[0].CreatedBy, "/bin/sh -c #(nop)  CMD [\"/bin/sh\"]")

	assert.Equal(base.T, compTime2.UTC().String(), localTimeL2.UTC().String())
	assert.Equal(base.T, history[1].CreatedBy, "/bin/sh -c #(nop) ADD file:3b16ffee2b26d8af5…")

	assert.Equal(base.T, history[1].Size, "5.947MB")
	assert.Equal(base.T, history[1].CreatedSince, "3 years ago")
	assert.Equal(base.T, history[1].Snapshot, "sha256:56bf55b8eed1f0b4794a30386e4d1d3da949c…")
	assert.Equal(base.T, history[1].Comment, "")

	// No human - dates and sizes and not prettyfied
	history = imageHistoryJSONHelper(base, testutil.CommonImage, false, false, false)

	assert.Equal(base.T, history[0].Size, "0")
	assert.Equal(base.T, history[0].CreatedSince, history[0].CreatedAt)

	assert.Equal(base.T, history[1].Size, "5947392")
	assert.Equal(base.T, history[1].CreatedSince, history[1].CreatedAt)

	// No trunc - do not truncate sha or cmd
	history = imageHistoryJSONHelper(base, testutil.CommonImage, true, false, true)
	assert.Equal(base.T, history[1].Snapshot, "sha256:56bf55b8eed1f0b4794a30386e4d1d3da949c25bcb5155e898097cd75dc77c2a")
	assert.Equal(base.T, history[1].CreatedBy, "/bin/sh -c #(nop) ADD file:3b16ffee2b26d8af5db152fcc582aaccd9e1ec9e3343874e9969a205550fe07d in / ")

	// Quiet has no effect with format, so, go no-json, no-trunc
	rawHistory := imageHistoryRawHelper(base, testutil.CommonImage, true, true, true)
	assert.Equal(base.T, rawHistory, "<missing>\nsha256:56bf55b8eed1f0b4794a30386e4d1d3da949c25bcb5155e898097cd75dc77c2a\n")

	// With quiet, trunc has no effect
	rawHistory = imageHistoryRawHelper(base, testutil.CommonImage, false, true, true)
	assert.Equal(base.T, rawHistory, "<missing>\nsha256:56bf55b8eed1f0b4794a30386e4d1d3da949c25bcb5155e898097cd75dc77c2a\n")
}
