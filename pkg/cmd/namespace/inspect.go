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

	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
)

func Inspect(ctx context.Context, inspectedNamespaces []string, options types.NamespaceInspectOptions) error {
	client, ctx, cancel, err := clientutil.NewClient(ctx, options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	result := make([]interface{}, len(inspectedNamespaces))
	for index, ns := range inspectedNamespaces {
		ctx = namespaces.WithNamespace(ctx, ns)
		labels, err := client.NamespaceService().Labels(ctx, ns)
		if err != nil {
			return err
		}
		nsInspect := native.Namespace{
			Name:   ns,
			Labels: &labels,
		}
		result[index] = nsInspect
	}
	return formatter.FormatSlice(options.Format, options.Stdout, result)
}
