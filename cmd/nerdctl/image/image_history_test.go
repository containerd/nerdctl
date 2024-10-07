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
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

type historyObj struct {
	Snapshot     string
	CreatedAt    string
	CreatedSince string
	CreatedBy    string
	Size         string
	Comment      string
}

func decode(stdout string) ([]historyObj, error) {
	dec := json.NewDecoder(strings.NewReader(stdout))
	object := []historyObj{}
	for {
		var v historyObj
		if err := dec.Decode(&v); err == io.EOF {
			break
		} else if err != nil {
			return nil, errors.New("failed to decode history object")
		}
		object = append(object, v)
	}

	return object, nil
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

	nerdtest.Setup()

	testCase := &test.Case{
		Require: test.Require(
			test.Not(nerdtest.Docker),
			// XXX the results here are obviously platform dependent - and it seems like windows cannot pull a linux image?
			test.Not(test.Windows),
			// XXX Currently, history does not work on non-native platform, so, we cannot test reliably on other platforms
			test.Arm64,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			// XXX: despite efforts to isolate this test, it keeps on having side effects linked to
			// https://github.com/containerd/nerdctl/issues/3512
			// Isolating it into a completely different root is the last ditched attempt at avoiding the issue
			helpers.Write(nerdtest.DataRoot, test.ConfigValue(data.TempDir()))
			helpers.Ensure("pull", "--platform", "linux/arm64", testutil.CommonImage)
		},
		SubTests: []*test.Case{
			{
				Description: "trunc, no quiet, human",
				Command:     test.Command("image", "history", "--human=true", "--format=json", testutil.CommonImage),
				Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
					history, err := decode(stdout)
					assert.NilError(t, err, info)
					assert.Equal(t, len(history), 2, info)
					assert.Equal(t, history[0].Size, "0B", info)
					// FIXME: how is this going to age?
					assert.Equal(t, history[0].CreatedSince, "3 years ago", info)
					assert.Equal(t, history[0].Snapshot, "<missing>", info)
					assert.Equal(t, history[0].Comment, "", info)

					localTimeL1, _ := time.Parse(time.RFC3339, "2021-03-31T10:21:23-07:00")
					localTimeL2, _ := time.Parse(time.RFC3339, "2021-03-31T10:21:21-07:00")
					compTime1, _ := time.Parse(time.RFC3339, history[0].CreatedAt)
					compTime2, _ := time.Parse(time.RFC3339, history[1].CreatedAt)
					assert.Equal(t, compTime1.UTC().String(), localTimeL1.UTC().String(), info)
					assert.Equal(t, history[0].CreatedBy, "/bin/sh -c #(nop)  CMD [\"/bin/sh\"]", info)
					assert.Equal(t, compTime2.UTC().String(), localTimeL2.UTC().String(), info)
					assert.Equal(t, history[1].CreatedBy, "/bin/sh -c #(nop) ADD file:3b16ffee2b26d8af5…", info)

					assert.Equal(t, history[1].Size, "5.947MB", info)
					assert.Equal(t, history[1].CreatedSince, "3 years ago", info)
					assert.Equal(t, history[1].Snapshot, "sha256:56bf55b8eed1f0b4794a30386e4d1d3da949c…", info)
					assert.Equal(t, history[1].Comment, "", info)
				}),
			},
			{
				Description: "no human - dates and sizes and not prettyfied",
				Command:     test.Command("image", "history", "--human=false", "--format=json", testutil.CommonImage),
				Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
					history, err := decode(stdout)
					assert.NilError(t, err, info)
					assert.Equal(t, history[0].Size, "0", info)
					assert.Equal(t, history[0].CreatedSince, history[0].CreatedAt, info)
					assert.Equal(t, history[1].Size, "5947392", info)
					assert.Equal(t, history[1].CreatedSince, history[1].CreatedAt, info)
				}),
			},
			{
				Description: "no trunc - do not truncate sha or cmd",
				Command:     test.Command("image", "history", "--human=false", "--no-trunc", "--format=json", testutil.CommonImage),
				Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
					history, err := decode(stdout)
					assert.NilError(t, err, info)
					assert.Equal(t, history[1].Snapshot, "sha256:56bf55b8eed1f0b4794a30386e4d1d3da949c25bcb5155e898097cd75dc77c2a")
					assert.Equal(t, history[1].CreatedBy, "/bin/sh -c #(nop) ADD file:3b16ffee2b26d8af5db152fcc582aaccd9e1ec9e3343874e9969a205550fe07d in / ")
				}),
			},
			{
				Description: "Quiet has no effect with format, so, go no-json, no-trunc",
				Command:     test.Command("image", "history", "--human=false", "--no-trunc", "--quiet", testutil.CommonImage),
				Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
					assert.Equal(t, stdout, "<missing>\nsha256:56bf55b8eed1f0b4794a30386e4d1d3da949c25bcb5155e898097cd75dc77c2a\n")
				}),
			},
			{
				Description: "With quiet, trunc has no effect",
				Command:     test.Command("image", "history", "--human=false", "--no-trunc", "--quiet", testutil.CommonImage),
				Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
					assert.Equal(t, stdout, "<missing>\nsha256:56bf55b8eed1f0b4794a30386e4d1d3da949c25bcb5155e898097cd75dc77c2a\n")
				}),
			},
		},
	}

	testCase.Run(t)
}
