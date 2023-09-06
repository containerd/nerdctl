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

package namespace

import (
	"context"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	"github.com/containerd/nerdctl/pkg/api/types"
)

func Remove(ctx context.Context, client *containerd.Client, deletedNamespaces []string, options types.NamespaceRemoveOptions) error {
	var exitErr error
	opts, err := namespaceDeleteOpts(options.CGroup)
	if err != nil {
		return err
	}
	namespaces := client.NamespaceService()
	for _, target := range deletedNamespaces {
		if err := namespaces.Delete(ctx, target, opts...); err != nil {
			if !errdefs.IsNotFound(err) {
				if exitErr == nil {
					exitErr = fmt.Errorf("unable to delete %s", target)
				}
				log.G(ctx).WithError(err).Errorf("unable to delete %v", target)
				continue
			}
		}
		_, err := fmt.Fprintln(options.Stdout, target)
		return err
	}
	return exitErr
}
