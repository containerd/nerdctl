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
	"testing"

	"gotest.tools/v3/assert"
)

func TestFoldContainerFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("supported filters are accepted", func(t *testing.T) {
		t.Parallel()
		for _, f := range []string{
			"id=abc",
			"name=foo",
			"label=env",
			"label=env=prod",
			"label=com.example.payload=a=b",
			"status=running",
			"exited=0",
		} {
			_, err := foldContainerFilters(ctx, nil, []string{f})
			assert.NilError(t, err, "filter %q should be accepted", f)
		}
	})

	t.Run("unknown filters are rejected", func(t *testing.T) {
		t.Parallel()
		// The keys below share a prefix with a supported filter but are not a
		// supported filter themselves. Docker rejects each of them with
		// "invalid filter '<key>'"; nerdctl used to silently route them to the
		// prefix's handler (e.g. "labels=env" behaved like "label=env").
		for _, f := range []string{
			"labels=env",
			"name2=foo",
			"statuss=running",
			"ids=abc",
			"volumes=v",
			"networkfoo=n",
			"totallybogus=x",
		} {
			_, err := foldContainerFilters(ctx, nil, []string{f})
			assert.ErrorContains(t, err, "invalid filter", "filter %q should be rejected", f)
		}
	})

	t.Run("supported filter without a value is a format error", func(t *testing.T) {
		t.Parallel()
		_, err := foldContainerFilters(ctx, nil, []string{"label"})
		assert.ErrorContains(t, err, "bad format of filter")
	})
}
