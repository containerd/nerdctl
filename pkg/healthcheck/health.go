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
	"encoding/json"
	"time"
)

type HealthStatus = string

// Health states
const (
	NoHealthcheck HealthStatus = "none" // Indicates there is no healthcheck
	Starting      HealthStatus = "starting"
	Healthy       HealthStatus = "healthy"
	Unhealthy     HealthStatus = "unhealthy"
)

// Healthcheck cmd types
const (
	CmdNone  = "NONE"
	Cmd      = "CMD"
	CmdShell = "CMD-SHELL"
	TestNone = ""
)

const (
	DefaultProbeInterval   = 30 * time.Second // Default interval between probe runs. Also applies before the first probe.
	DefaultProbeTimeout    = 30 * time.Second // Max duration a single probe run may take before it's considered failed.
	DefaultStartPeriod     = 0 * time.Second  // Grace period for container startup before health checks count as failures.
	DefaultProbeRetries    = 3                // Number of consecutive failures before marking container as unhealthy.
	MaxLogEntries          = 5                // Maximum number of health check log entries to keep.
	MaxOutputLenForInspect = 4096             // Max output length (in bytes) stored in health check logs during inspect. Longer outputs are truncated.
	MaxOutputLen           = 1 * 1024 * 1024  // Max output size for health check logs: 1MB limit (prevents excessive memory usage)
	HealthLogFilename      = "health.json"    // HealthLogFilename is the name of the file used to persist health check status for a container.
)

// NOTE: Health, HealthcheckResult and Healthcheck types are kept Docker-compatible.
// See: https://github.com/moby/moby/blob/9d1b069a4bfdcee368e67767978eff596b696d4c/api/types/container/health.go
// Health stores information about the container's healthcheck results
type Health struct {
	Status        HealthStatus         // Status is one of [Starting], [Healthy] or [Unhealthy].
	FailingStreak int                  // FailingStreak is the number of consecutive failures
	Log           []*HealthcheckResult // Log contains the last few results (oldest first)
}

// HealthcheckResult stores information about a single run of a healthcheck probe
type HealthcheckResult struct {
	Start    time.Time // Start is the time this check started
	End      time.Time // End is the time this check ended
	ExitCode int       // ExitCode meanings: 0=healthy, 1=unhealthy, 2=reserved (considered unhealthy), else=error running probe
	Output   string    // Output from last check
}

// Healthcheck represents the health check configuration
type Healthcheck struct {
	Test        []string      `json:"Test,omitempty"`        // Test is the check to perform that the container is healthy
	Interval    time.Duration `json:"Interval,omitempty"`    // Interval is the time to wait between checks
	Timeout     time.Duration `json:"Timeout,omitempty"`     // Timeout is the time to wait before considering the check to have hung
	Retries     int           `json:"Retries,omitempty"`     // Retries is the number of consecutive failures needed to consider a container as unhealthy
	StartPeriod time.Duration `json:"StartPeriod,omitempty"` // StartPeriod is the period for the container to initialize before the health check starts
}

// HealthState stores the current health state of a container
type HealthState struct {
	Status        HealthStatus // Status is one of [Starting], [Healthy] or [Unhealthy]
	FailingStreak int          // FailingStreak is the number of consecutive failures
	InStartPeriod bool         // InStartPeriod indicates if we're in the start period workflow
}

// ToJSONString serializes HealthState to a JSON string for label storage
func (hs *HealthState) ToJSONString() (string, error) {
	b, err := json.Marshal(hs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// HealthStateFromJSON deserializes a JSON string into a HealthState
func HealthStateFromJSON(s string) (*HealthState, error) {
	var hs HealthState
	if err := json.Unmarshal([]byte(s), &hs); err != nil {
		return nil, err
	}
	return &hs, nil
}

// ToJSONString serializes a Healthcheck struct to a JSON string
func (hc *Healthcheck) ToJSONString() (string, error) {
	b, err := json.Marshal(hc)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// HealthCheckFromJSON deserializes a JSON string into a Healthcheck struct
func HealthCheckFromJSON(s string) (*Healthcheck, error) {
	var hc Healthcheck
	if err := json.Unmarshal([]byte(s), &hc); err != nil {
		return nil, err
	}
	return &hc, nil
}

// ToJSONString serializes a HealthcheckResult struct to a JSON string
func (r *HealthcheckResult) ToJSONString() (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// HealthcheckResultFromJSON deserializes a JSON string into a HealthcheckResult struct
func HealthcheckResultFromJSON(s string) (*HealthcheckResult, error) {
	var r HealthcheckResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return nil, err
	}
	return &r, nil
}
