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

package manifest

import (
	"context"
	"fmt"
	"strings"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/manifeststore"
	"github.com/containerd/nerdctl/v2/pkg/manifestutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

func Remove(ctx context.Context, ref string, options types.GlobalCommandOptions) error {
	parsedRef, err := referenceutil.Parse(ref)
	if err != nil {
		return fmt.Errorf("failed to parse reference: %w", err)
	}
	manifestStore, err := manifeststore.NewStore(options.DataRoot)
	if err != nil {
		return fmt.Errorf("failed to create manifest store: %w", err)
	}
	_, err = manifestStore.GetList(parsedRef)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return manifestutil.NewNoSuchManifestError(parsedRef.String())
		}
		return err
	}
	err = manifestStore.Remove(parsedRef)
	if err != nil {
		return fmt.Errorf("failed to remove manifest list: %w", err)
	}
	return nil
}
