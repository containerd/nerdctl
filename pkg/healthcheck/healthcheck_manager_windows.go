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
	"context"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/pkg/config"
)

// CreateTimer sets up the transient systemd timer and service for healthchecks.
func CreateTimer(ctx context.Context, container containerd.Container, cfg *config.Config, nerdctlCmd string, nerdctlArgs []string) error {
	return nil
}

// StartTimer starts the healthcheck timer unit.
func StartTimer(ctx context.Context, container containerd.Container, cfg *config.Config) error {
	return nil
}

// RemoveTransientHealthCheckFiles stops and cleans up the transient timer and service.
func RemoveTransientHealthCheckFiles(ctx context.Context, container containerd.Container) error {
	return nil
}

// ForceRemoveTransientHealthCheckFiles forcefully stops and cleans up the transient timer and service
// using just the container ID. This function is non-blocking and uses timeouts to prevent hanging
// on systemd operations. On Windows, this is a no-op since systemd is not available.
func ForceRemoveTransientHealthCheckFiles(ctx context.Context, containerID string) error {
	return nil
}
