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

// Healthcheck represents the health check configuration
type Healthcheck struct {
	// Test is the test to perform to check that the container is healthy
	Test []string `json:"Test,omitempty"`

	// Interval is the time to wait between checks
	Interval time.Duration `json:"Interval,omitempty"`

	// Timeout is the time to wait before considering the check to have hung
	Timeout time.Duration `json:"Timeout,omitempty"`

	// Retries is the number of consecutive failures needed to consider a container as unhealthy
	Retries int `json:"Retries,omitempty"`

	// StartPeriod is the period for the container to initialize before the health check starts
	StartPeriod time.Duration `json:"StartPeriod,omitempty"`

	// StartInterval is the time between health checks during the start period
	StartInterval time.Duration `json:"StartInterval,omitempty"`
}

// ToJSONString serializes a Healthcheck struct to a JSON string
func ToJSONString(hc *Healthcheck) (string, error) {
	b, err := json.Marshal(hc)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Parse deserializes a JSON string into a Healthcheck struct
func Parse(s string) (*Healthcheck, error) {
	var hc Healthcheck
	if err := json.Unmarshal([]byte(s), &hc); err != nil {
		return nil, err
	}
	return &hc, nil
}
