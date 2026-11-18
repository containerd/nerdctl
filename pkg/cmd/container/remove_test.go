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
	"context"
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/errdefs"
)

func TestWalkAllForce(t *testing.T) {
	t.Parallel()

	walkResults := map[string]struct {
		count int
		err   error
	}{
		"found":   {count: 1},
		"missing": {count: 0},
		"broken":  {count: -1, err: errors.New("failed to delete task: no such directory")},
		"other":   {count: -1, err: errors.New("cannot kill")},
		"gone":    {count: -1, err: errdefs.ErrNotFound},
	}
	var walked []string
	walk := func(_ context.Context, req string) (int, error) {
		walked = append(walked, req)
		res := walkResults[req]
		return res.count, res.err
	}

	tests := []struct {
		name        string
		reqs        []string
		expectedErr string
		expectedRun []string
	}{
		{
			name:        "removed containers succeed",
			reqs:        []string{"found"},
			expectedRun: []string{"found"},
		},
		{
			name:        "a container that does not exist is not an error",
			reqs:        []string{"missing"},
			expectedRun: []string{"missing"},
		},
		{
			name:        "a container that vanishes during removal is not an error",
			reqs:        []string{"gone"},
			expectedRun: []string{"gone"},
		},
		{
			name:        "a container that cannot be removed fails",
			reqs:        []string{"broken"},
			expectedErr: "1 errors:\nfailed to delete task: no such directory",
			expectedRun: []string{"broken"},
		},
		{
			name:        "it keeps going after a failure and reports every one",
			reqs:        []string{"broken", "found", "missing", "other"},
			expectedErr: "2 errors:\nfailed to delete task: no such directory\ncannot kill",
			expectedRun: []string{"broken", "found", "missing", "other"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			walked = nil
			err := walkAllForce(context.Background(), tc.reqs, walk)
			if tc.expectedErr == "" {
				assert.NilError(t, err)
			} else {
				assert.Error(t, err, tc.expectedErr)
			}
			assert.DeepEqual(t, walked, tc.expectedRun)
		})
	}
}
