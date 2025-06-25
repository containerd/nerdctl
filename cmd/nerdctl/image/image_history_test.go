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

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

type historyObj struct {
	Snapshot     string
	CreatedAt    string
	CreatedSince string
	CreatedBy    string
	Size         string
	Comment      string
}

const createdAt1 = "2021-03-31T10:21:21-07:00"
const createdAt2 = "2021-03-31T10:21:23-07:00"

// Expected content of the common image on arm64
var (
	createdAtTime, _ = time.Parse(time.RFC3339, createdAt2)
	expectedHistory  = []historyObj{
		{
			CreatedBy:    "/bin/sh -c #(nop)  CMD [\"/bin/sh\"]",
			Size:         "0B",
			CreatedAt:    createdAt2,
			Snapshot:     "<missing>",
			Comment:      "",
			CreatedSince: formatter.TimeSinceInHuman(createdAtTime),
		},
		{
			CreatedBy:    "/bin/sh -c #(nop) ADD file:3b16ffee2b26d8af5…",
			Size:         "5.947MB",
			CreatedAt:    createdAt1,
			Snapshot:     "sha256:56bf55b8eed1f0b4794a30386e4d1d3da949c…",
			Comment:      "",
			CreatedSince: formatter.TimeSinceInHuman(createdAtTime),
		},
	}
	expectedHistoryNoTrunc = []historyObj{
		{
			Snapshot: "<missing>",
			Size:     "0",
		},
		{
			Snapshot:  "sha256:56bf55b8eed1f0b4794a30386e4d1d3da949c25bcb5155e898097cd75dc77c2a",
			CreatedBy: "/bin/sh -c #(nop) ADD file:3b16ffee2b26d8af5db152fcc582aaccd9e1ec9e3343874e9969a205550fe07d in / ",
			Size:      "5947392",
		},
	}
)

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
		Require: require.All(
			require.Not(nerdtest.Docker),
			// XXX the results here are obviously platform dependent - and it seems like windows cannot pull a linux image?
			require.Not(require.Windows),
			// XXX Currently, history does not work on non-native platform, so, we cannot test reliably on other platforms
			require.Arm64,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			// XXX: despite efforts to isolate this test, it keeps on having side effects linked to
			// https://github.com/containerd/nerdctl/issues/3512
			// Isolating it into a completely different root is the last ditched attempt at avoiding the issue
			helpers.Write(nerdtest.DataRoot, test.ConfigValue(data.Temp().Path()))
			helpers.Ensure("pull", "--quiet", "--platform", "linux/arm64", testutil.CommonImage)
		},
		SubTests: []*test.Case{
			{
				Description: "trunc, no quiet, human",
				Command:     test.Command("image", "history", "--human=true", "--format=json", testutil.CommonImage),
				Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
					history, err := decode(stdout)
					assert.NilError(t, err, "decode should not fail")
					assert.Equal(t, len(history), 2, "history should be 2 in length")

					h0Time, _ := time.Parse(time.RFC3339, history[0].CreatedAt)
					h1Time, _ := time.Parse(time.RFC3339, history[1].CreatedAt)
					comp0Time, _ := time.Parse(time.RFC3339, expectedHistory[0].CreatedAt)
					comp1Time, _ := time.Parse(time.RFC3339, expectedHistory[1].CreatedAt)

					assert.Equal(t, h0Time.UTC().String(), comp0Time.UTC().String())
					assert.Equal(t, history[0].CreatedBy, expectedHistory[0].CreatedBy)
					assert.Equal(t, history[0].Size, expectedHistory[0].Size)
					assert.Equal(t, history[0].CreatedSince, expectedHistory[0].CreatedSince)
					assert.Equal(t, history[0].Snapshot, expectedHistory[0].Snapshot)
					assert.Equal(t, history[0].Comment, expectedHistory[0].Comment)

					assert.Equal(t, h1Time.UTC().String(), comp1Time.UTC().String())
					assert.Equal(t, history[1].CreatedBy, expectedHistory[1].CreatedBy)
					assert.Equal(t, history[1].Size, expectedHistory[1].Size)
					assert.Equal(t, history[1].CreatedSince, expectedHistory[1].CreatedSince)
					assert.Equal(t, history[1].Snapshot, expectedHistory[1].Snapshot)
					assert.Equal(t, history[1].Comment, expectedHistory[1].Comment)
				}),
			},
			{
				Description: "no human - dates and sizes are not prettyfied",
				Command:     test.Command("image", "history", "--human=false", "--format=json", testutil.CommonImage),
				Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
					history, err := decode(stdout)
					assert.NilError(t, err, "decode should not fail")
					assert.Equal(t, history[0].Size, expectedHistoryNoTrunc[0].Size)
					assert.Equal(t, history[0].CreatedSince, history[0].CreatedAt)
					assert.Equal(t, history[1].Size, expectedHistoryNoTrunc[1].Size)
					assert.Equal(t, history[1].CreatedSince, history[1].CreatedAt)
				}),
			},
			{
				Description: "no trunc - do not truncate sha or cmd",
				Command:     test.Command("image", "history", "--human=false", "--no-trunc", "--format=json", testutil.CommonImage),
				Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
					history, err := decode(stdout)
					assert.NilError(t, err, "decode should not fail")
					assert.Equal(t, history[1].Snapshot, expectedHistoryNoTrunc[1].Snapshot)
					assert.Equal(t, history[1].CreatedBy, expectedHistoryNoTrunc[1].CreatedBy)
				}),
			},
			{
				Description: "Quiet has no effect with format, so, go no-json, no-trunc",
				Command:     test.Command("image", "history", "--human=false", "--no-trunc", "--quiet", testutil.CommonImage),
				Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
					assert.Equal(t, stdout, expectedHistoryNoTrunc[0].Snapshot+"\n"+expectedHistoryNoTrunc[1].Snapshot+"\n")
				}),
			},
			{
				Description: "With quiet, trunc has no effect",
				Command:     test.Command("image", "history", "--human=false", "--no-trunc", "--quiet", testutil.CommonImage),
				Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
					assert.Equal(t, stdout, expectedHistoryNoTrunc[0].Snapshot+"\n"+expectedHistoryNoTrunc[1].Snapshot+"\n")
				}),
			},
		},
	}

	testCase.Run(t)
}
