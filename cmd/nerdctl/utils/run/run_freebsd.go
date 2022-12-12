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

package run

import (
	"context"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/spf13/cobra"
)

func SetPlatformOptions(ctx context.Context, opts []oci.SpecOpts, cmd *cobra.Command, client *containerd.Client, id string, internalLabels common.InternalLabels) ([]oci.SpecOpts, common.InternalLabels, error) {
	return opts, internalLabels, nil
}

func WithoutRunMount() func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
	// not valid on freebsd
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error { return nil }
}
