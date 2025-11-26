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

package containerdutil

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver/v3"

	containerd "github.com/containerd/containerd/v2/client"
)

func ServerSemVer(ctx context.Context, client *containerd.Client) (*semver.Version, error) {
	v, err := client.Version(ctx)
	if err != nil {
		return nil, err
	}
	sv, err := semver.NewVersion(v.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the containerd version %q: %w", v.Version, err)
	}
	return sv, nil
}

// SupportsFullTransferService checks if the containerd version fully supports the Transfer service.
// While containerd 1.7 has Transfer service, full support is only available in 2.0+.
// The following features are missing in containerd 1.7:
//   - Non-distributable artifacts support
//   - Registry configuration options: WithHostDir(), WithDefaultScheme() etc.
func SupportsFullTransferService(ctx context.Context, client *containerd.Client) bool {
	sv, err := ServerSemVer(ctx, client)
	if err != nil {
		// If we can't determine version, assume it's an older version for safety
		return false
	}
	v20, _ := semver.NewVersion("2.0.0")
	return !sv.LessThan(v20)
}
