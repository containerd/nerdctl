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

package container

import (
	"context"
	"strings"

	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

func generateUlimitsOpts(ulimits []string) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts
	ulimits = strutil.DedupeStrSlice(ulimits)
	if len(ulimits) > 0 {
		var rlimits []specs.POSIXRlimit
		for _, ulimit := range ulimits {
			l, err := units.ParseUlimit(ulimit)
			if err != nil {
				return nil, err
			}
			rlimits = append(rlimits, specs.POSIXRlimit{
				Type: "RLIMIT_" + strings.ToUpper(l.Name),
				Hard: uint64(l.Hard),
				Soft: uint64(l.Soft),
			})
		}
		opts = append(opts, withRlimits(rlimits))
	}
	return opts, nil
}

func withRlimits(rlimits []specs.POSIXRlimit) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Process.Rlimits = rlimits
		return nil
	}
}
