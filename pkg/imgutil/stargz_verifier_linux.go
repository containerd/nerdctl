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

package imgutil

import (
	"context"
	"fmt"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/stargz-snapshotter/service/verifier"

	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

// stargzVerifier normalizes ref (e.g., adds the default tag) because the underlying
// verifier requires a tag or a digest. It is linux-only: stargz-snapshotter's service package depends on
// linux-only containerd packages (e.g., overlayutils).
func stargzVerifier(ctx context.Context, client *containerd.Client, ref string, resolver remotes.Resolver) error {
	parsed, err := referenceutil.Parse(ref)
	if err != nil {
		return fmt.Errorf("failed to parse image reference %q: %w", ref, err)
	}
	return verifier.VerifyImage(ctx, client, parsed.String(), resolver)
}
