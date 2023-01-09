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

package volume

import (
	"fmt"
	"io"

	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/strutil"
)

func Create(options *types.VolumeCreateCommandOptions, stdout io.Writer) error {
	if err := identifiers.Validate(options.Name); err != nil {
		return fmt.Errorf("malformed name %s: %w", options.Name, err)
	}
	volStore, err := Store(options.GOptions.Namespace, options.GOptions.DataRoot, options.GOptions.Address)
	if err != nil {
		return err
	}
	labels := strutil.DedupeStrSlice(options.Labels)
	if _, err := volStore.Create(options.Name, labels); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s\n", options.Name)
	return nil
}
