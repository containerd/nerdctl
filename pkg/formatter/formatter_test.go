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

package formatter

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestTimeSinceInHuman(t *testing.T) {
	now := time.Now()
	t.Parallel()

	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "1 second ago",
			input:    now.Add(-1 * time.Second),
			expected: "1 second ago",
		},
		{
			name:     "59 seconds ago",
			input:    now.Add(-59 * time.Second),
			expected: "59 seconds ago",
		},
		{
			name:     "1 minute ago",
			input:    now.Add(-1 * time.Minute),
			expected: "About a minute ago",
		},
		{
			name:     "1 hour ago",
			input:    now.Add(-1 * time.Hour),
			expected: "About an hour ago",
		},
		{
			name:     "1 day ago",
			input:    now.Add(-24 * time.Hour),
			expected: "24 hours ago",
		},
		{
			name:     "4 days ago",
			input:    now.Add(-4 * 24 * time.Hour),
			expected: "4 days ago",
		},
		{
			name:     "1 year ago",
			input:    now.Add(-365 * 24 * time.Hour),
			expected: "12 months ago",
		},
		{
			name:     "4 years ago",
			input:    now.Add(-4 * 365 * 24 * time.Hour),
			expected: "4 years ago",
		},
		{
			name:     "zero duration",
			input:    now,
			expected: "Less than a second ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := TimeSinceInHuman(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
