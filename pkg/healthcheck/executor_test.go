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

package healthcheck

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestShouldRunProbeAt(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	hc := &Healthcheck{
		Interval:      60 * time.Second,
		StartPeriod:   30 * time.Second,
		StartInterval: 5 * time.Second,
	}

	tests := []struct {
		description   string
		now           time.Time
		lastProbeAt   time.Time
		inStartPeriod bool
		hc            *Healthcheck
		want          bool
	}{
		{
			description: "always runs the first ever probe",
			now:         created,
			lastProbeAt: time.Time{},
			hc:          hc,
			want:        true,
		},
		{
			description:   "runs again once start-interval has elapsed, inside the start period",
			now:           created.Add(10 * time.Second),
			lastProbeAt:   created.Add(5 * time.Second),
			inStartPeriod: true,
			hc:            hc,
			want:          true, // 5s elapsed >= 5s start-interval
		},
		{
			description:   "skips a tick that arrives before start-interval has elapsed",
			now:           created.Add(8 * time.Second),
			lastProbeAt:   created.Add(5 * time.Second),
			inStartPeriod: true,
			hc:            hc,
			want:          false, // 3s elapsed < 5s start-interval
		},
		{
			description:   "skips a tick within health-interval once the start period has elapsed",
			now:           created.Add(35 * time.Second),
			lastProbeAt:   created.Add(31 * time.Second),
			inStartPeriod: false,
			hc:            hc,
			want:          false, // 4s elapsed < 60s health-interval, no longer in start period
		},
		{
			description:   "skips a tick within health-interval once InStartPeriod flips off, even if still before start-period elapses",
			now:           created.Add(12 * time.Second),
			lastProbeAt:   created.Add(10 * time.Second),
			inStartPeriod: false, // e.g. an earlier healthy result already ended the start period
			hc:            hc,
			want:          false, // 2s elapsed < 60s health-interval
		},
		{
			description:   "runs again once health-interval has elapsed after the start period",
			now:           created.Add(95 * time.Second),
			lastProbeAt:   created.Add(35 * time.Second),
			inStartPeriod: false,
			hc:            hc,
			want:          true, // 60s elapsed >= 60s health-interval
		},
		{
			description:   "treats a zero start period as never in the start-interval phase",
			now:           created.Add(2 * time.Second),
			lastProbeAt:   created.Add(1 * time.Second),
			inStartPeriod: true,
			hc: &Healthcheck{
				Interval:      60 * time.Second,
				StartPeriod:   0,
				StartInterval: 5 * time.Second,
			},
			want: false, // StartPeriod == 0 means health-interval always applies
		},
	}

	for _, tc := range tests {
		got := shouldRunProbeAt(tc.now, created, tc.lastProbeAt, tc.inStartPeriod, tc.hc)
		assert.Equal(t, got, tc.want, tc.description)
	}
}
