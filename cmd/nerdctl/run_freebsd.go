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

package main

import (
	"context"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/spf13/cobra"
)

func WithoutRunMount() func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
	// not valid on freebsd
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error { return nil }
}

func capShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	candidates := []string{}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func runShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func setPlatformOptions(
	ctx context.Context,
	cmd *cobra.Command,
	client *containerd.Client,
	id string,
	internalLabels *internalLabels,
) ([]oci.SpecOpts, error) {
	return []oci.SpecOpts{}, nil
}
