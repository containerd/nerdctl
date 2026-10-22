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

func TestStartProbeDelay(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	hc := &Healthcheck{
		Interval:      60 * time.Second,
		StartPeriod:   30 * time.Second,
		StartInterval: 5 * time.Second,
	}

	tests := []struct {
		description   string
		now           time.Time
		inStartPeriod bool
		hc            *Healthcheck
		wantDelay     time.Duration
		wantAgain     bool
	}{
		{
			description:   "first probe in the start period",
			now:           created.Add(1 * time.Second),
			inStartPeriod: true,
			hc:            hc,
			wantDelay:     5 * time.Second,
			wantAgain:     true,
		},
		{
			description:   "more than start interval left",
			now:           created.Add(24 * time.Second),
			inStartPeriod: true,
			hc:            hc,
			wantDelay:     5 * time.Second,
			wantAgain:     true,
		},
		{
			description:   "less than start interval left",
			now:           created.Add(27 * time.Second),
			inStartPeriod: true,
			hc:            hc,
			wantDelay:     3 * time.Second,
			wantAgain:     true,
		},
		{
			description:   "start period over",
			now:           created.Add(30 * time.Second),
			inStartPeriod: true,
			hc:            hc,
		},
		{
			description:   "start period long over",
			now:           created.Add(90 * time.Second),
			inStartPeriod: true,
			hc:            hc,
		},
		{
			description:   "already healthy",
			now:           created.Add(10 * time.Second),
			inStartPeriod: false,
			hc:            hc,
		},
		{
			description:   "no start period",
			now:           created.Add(1 * time.Second),
			inStartPeriod: true,
			hc: &Healthcheck{
				Interval:      60 * time.Second,
				StartInterval: 5 * time.Second,
			},
		},
		{
			description:   "start interval equal to interval",
			now:           created.Add(1 * time.Second),
			inStartPeriod: true,
			hc: &Healthcheck{
				Interval:      5 * time.Second,
				StartPeriod:   30 * time.Second,
				StartInterval: 5 * time.Second,
			},
		},
		{
			description:   "start interval longer than interval",
			now:           created.Add(1 * time.Second),
			inStartPeriod: true,
			hc: &Healthcheck{
				Interval:      5 * time.Second,
				StartPeriod:   30 * time.Second,
				StartInterval: 10 * time.Second,
			},
		},
	}

	for _, tc := range tests {
		delay, again := startProbeDelay(tc.now, created, tc.inStartPeriod, tc.hc)
		assert.Equal(t, again, tc.wantAgain, tc.description)
		assert.Equal(t, delay, tc.wantDelay, tc.description)
	}
}
