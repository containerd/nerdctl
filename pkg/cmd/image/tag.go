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

package image

import (
	"context"

	containerd "github.com/containerd/containerd/v2/client"
	transferimage "github.com/containerd/containerd/v2/core/transfer/image"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

func Tag(ctx context.Context, client *containerd.Client, options types.ImageTagOptions) error {
	return tagWithTransfer(ctx, client, options)
}

func tagWithTransfer(ctx context.Context, client *containerd.Client, options types.ImageTagOptions) error {
	parsedSource, err := referenceutil.Parse(options.Source)
	if err != nil {
		return err
	}

	parsedTarget, err := referenceutil.Parse(options.Target)
	if err != nil {
		return err
	}

	platMC, err := platformutil.NewMatchComparer(false, nil)
	if err != nil {
		return err
	}
	err = EnsureAllContent(ctx, client, parsedSource.String(), platMC, options.GOptions)
	if err != nil {
		return err
	}

	sourceStore := transferimage.NewStore(parsedSource.String())
	targetStore := transferimage.NewStore(parsedTarget.String())

	return client.Transfer(ctx, sourceStore, targetStore)
}
