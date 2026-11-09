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
)

func TestRemoveAll(t *testing.T) {
	walk := func(_ context.Context, req string) (int, error) {
		switch req {
		case "missing":
			return 0, nil
		case "broken":
			return -1, errors.New("failed to delete task")
		default:
			return 1, nil
		}
	}

	testCases := []struct {
		name    string
		reqs    []string
		force   bool
		wantErr string
	}{
		{name: "removed", reqs: []string{"ok"}},
		{name: "missing without force", reqs: []string{"missing"}, wantErr: "1 errors:\nno such container: missing"},
		{name: "missing with force", reqs: []string{"missing"}, force: true},
		{name: "failure without force", reqs: []string{"broken"}, wantErr: "1 errors:\nfailed to delete task"},
		{name: "failure with force", reqs: []string{"broken"}, force: true, wantErr: "1 errors:\nfailed to delete task"},
		{name: "mixed with force", reqs: []string{"ok", "missing", "broken"}, force: true, wantErr: "1 errors:\nfailed to delete task"},
		{name: "mixed without force", reqs: []string{"ok", "missing", "broken"}, wantErr: "2 errors:\nno such container: missing\nfailed to delete task"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := removeAll(context.Background(), tc.reqs, walk, tc.force)
			if tc.wantErr == "" {
				assert.NilError(t, err)
				return
			}
			assert.Error(t, err, tc.wantErr)
		})
	}
}
