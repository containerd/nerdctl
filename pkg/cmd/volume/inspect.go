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
	"io"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/formatter"
)

func Inspect(options *types.VolumeInspectCommandOptions, stdout io.Writer) error {
	volStore, err := Store(options.GOptions.Namespace, options.GOptions.DataRoot, options.GOptions.Address)
	if err != nil {
		return err
	}
	result := make([]interface{}, len(options.Volumes))

	for i, name := range options.Volumes {
		var vol, err = volStore.Get(name, options.Size)
		if err != nil {
			return err
		}
		result[i] = vol
	}
	return formatter.FormatSlice(options.Format, stdout, result)
}
