//go:build unix

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

package buildkitutil

import (
	"fmt"
	"path/filepath"
)

func getBuildkitHostCandidates(namespace string) ([]string, error) {
	if namespace == "" {
		return []string{}, fmt.Errorf("namespace must be specified")
	}
	// Try candidate locations of the current containerd namespace.
	run := getRuntimeVariableDataDir()
	var candidates []string
	if namespace != "default" {
		candidates = append(candidates, "unix://"+filepath.Join(run, fmt.Sprintf("buildkit-%s/buildkitd.sock", namespace)))
	}
	candidates = append(candidates, "unix://"+filepath.Join(run, "buildkit-default/buildkitd.sock"), "unix://"+filepath.Join(run, "buildkit/buildkitd.sock"))

	return candidates, nil
}
