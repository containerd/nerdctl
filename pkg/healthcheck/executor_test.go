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
			description: "first probe",
			now:         created,
			lastProbeAt: time.Time{},
			hc:          hc,
			want:        true,
		},
		{
			description:   "start period: start interval elapsed",
			now:           created.Add(10 * time.Second),
			lastProbeAt:   created.Add(5 * time.Second),
			inStartPeriod: true,
			hc:            hc,
			want:          true,
		},
		{
			description:   "start period: before start interval",
			now:           created.Add(8 * time.Second),
			lastProbeAt:   created.Add(5 * time.Second),
			inStartPeriod: true,
			hc:            hc,
			want:          false,
		},
		{
			description:   "after start period: before interval",
			now:           created.Add(35 * time.Second),
			lastProbeAt:   created.Add(31 * time.Second),
			inStartPeriod: false,
			hc:            hc,
			want:          false,
		},
		{
			description:   "start period ended early by a healthy probe",
			now:           created.Add(12 * time.Second),
			lastProbeAt:   created.Add(10 * time.Second),
			inStartPeriod: false,
			hc:            hc,
			want:          false,
		},
		{
			description:   "after start period: interval elapsed",
			now:           created.Add(95 * time.Second),
			lastProbeAt:   created.Add(35 * time.Second),
			inStartPeriod: false,
			hc:            hc,
			want:          true,
		},
		{
			description:   "start period: less than start interval left",
			now:           created.Add(29 * time.Second),
			lastProbeAt:   created.Add(27 * time.Second),
			inStartPeriod: true,
			hc:            hc,
			want:          false,
		},
		{
			description:   "start period: time left elapsed",
			now:           created.Add(30 * time.Second),
			lastProbeAt:   created.Add(27 * time.Second),
			inStartPeriod: true,
			hc:            hc,
			want:          true,
		},
		{
			description:   "zero start period uses interval",
			now:           created.Add(2 * time.Second),
			lastProbeAt:   created.Add(1 * time.Second),
			inStartPeriod: true,
			hc: &Healthcheck{
				Interval:      60 * time.Second,
				StartPeriod:   0,
				StartInterval: 5 * time.Second,
			},
			want: false,
		},
	}

	for _, tc := range tests {
		got := shouldRunProbeAt(tc.now, created, tc.lastProbeAt, tc.inStartPeriod, tc.hc)
		assert.Equal(t, got, tc.want, tc.description)
	}
}
