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

package contentutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	digest "github.com/opencontainers/go-digest"
)

// WithBlobLabel set an index digest label to config blobs in the content store
func WithConfigBlobLabel(ctx context.Context, client *containerd.Client, idxDgst digest.Digest, confDgsts []digest.Digest) error {
	cs := client.ContentStore()
	for _, cd := range confDgsts {
		info, err := cs.Info(ctx, cd)
		if err != nil {
			return err
		}
		if info.Labels == nil {
			info.Labels = make(map[string]string)
		}

		kPrefix := "containerd.io/blobref.index."
		var n int
		for k := range info.Labels {
			if strings.HasPrefix(k, kPrefix) {
				n++
			}
		}

		key := fmt.Sprintf("%s%d", kPrefix, n)
		info.Labels[key] = idxDgst.String()

		var fields []string
		fields = append(fields, "labels."+key)

		if _, err := cs.Update(ctx, info, fields...); err != nil {
			return err
		}
	}

	return nil
}
