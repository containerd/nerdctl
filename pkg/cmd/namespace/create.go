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

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

func Create(ctx context.Context, client *containerd.Client, namespace string, options types.NamespaceCreateOptions) error {
	labelsArg := objectWithLabelArgs(options.Labels)
	namespaces := client.NamespaceService()
	return namespaces.Create(ctx, namespace, labelsArg)
}
