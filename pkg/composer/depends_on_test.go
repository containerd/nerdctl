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

package composer

import (
	"context"
	"testing"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/errdefs"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/healthcheck"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

// fakeContainer serves the labels of a container; it has no task.
type fakeContainer struct {
	containerd.Container
	lbs map[string]string
}

func (f fakeContainer) ID() string { return "fake" }

func (f fakeContainer) Labels(context.Context) (map[string]string, error) { return f.lbs, nil }

func (f fakeContainer) Task(context.Context, cio.Attach) (containerd.Task, error) {
	return nil, errdefs.ErrNotFound
}

func TestWaitContainerHealthy(t *testing.T) {
	origInterval := dependencyPollInterval
	dependencyPollInterval = time.Millisecond
	t.Cleanup(func() { dependencyPollInterval = origInterval })

	state := func(status healthcheck.HealthStatus) string {
		s, err := (&healthcheck.HealthState{Status: status}).ToJSONString()
		assert.NilError(t, err)
		return s
	}
	check := `{"Test":["CMD","true"]}`

	for _, tc := range []struct {
		name    string
		lbs     map[string]string
		wantErr string
	}{
		{name: "healthy", lbs: map[string]string{labels.HealthCheck: check, labels.HealthState: state(healthcheck.Healthy)}},
		{name: "unhealthy", lbs: map[string]string{labels.HealthCheck: check, labels.HealthState: state(healthcheck.Unhealthy)}, wantErr: "is unhealthy"},
		{name: "no healthcheck", lbs: map[string]string{}, wantErr: "no healthcheck configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := waitContainerHealthy(ctx, fakeContainer{lbs: tc.lbs})
			if tc.wantErr == "" {
				assert.NilError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}

	t.Run("still starting until the context ends", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		err := waitContainerHealthy(ctx, fakeContainer{lbs: map[string]string{
			labels.HealthCheck: check, labels.HealthState: state(healthcheck.Starting),
		}})
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}
