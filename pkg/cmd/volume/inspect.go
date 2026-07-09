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
	"context"
	"errors"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
)

func Inspect(ctx context.Context, volumes []string, options types.VolumeInspectOptions) error {
	volStore, err := Store(options.GOptions.Namespace, options.GOptions.DataRoot, options.GOptions.Address)
	if err != nil {
		return err
	}
	result := []interface{}{}

	warns := []error{}
	for _, name := range volumes {
		var vol, err = volStore.Get(name, options.Size)
		if err != nil {
			warns = append(warns, err)
			continue
		}
		result = append(result, vol)
	}
	err = formatter.FormatSlice(options.Format, options.Stdout, result)
	if err != nil {
		return err
	}
	for _, warn := range warns {
		log.G(ctx).Warn(warn)
	}

	if len(warns) != 0 {
		return errors.New("some volumes could not be inspected")
	}
	return nil
}
