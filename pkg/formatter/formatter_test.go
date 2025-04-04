package formatter

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestTimeSinceInHuman(t *testing.T) {
	now := time.Now()

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
			result := TimeSinceInHuman(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
