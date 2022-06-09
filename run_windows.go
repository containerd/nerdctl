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
	"fmt"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/docker/go-units"
	"github.com/spf13/cobra"
)

func WithoutRunMount() func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
	// not valid on windows
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error { return nil }
}

func capShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	candidates := []string{}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func runShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func setPlatformOptions(opts []oci.SpecOpts, cmd *cobra.Command, id string) ([]oci.SpecOpts, error) {
	cpus, err := cmd.Flags().GetFloat64("cpus")
	if err != nil {
		return nil, err
	}
	if cpus > 0.0 {
		opts = append(opts, oci.WithWindowsCPUCount(uint64(cpus)))
	}

	memStr, err := cmd.Flags().GetString("memory")
	if err != nil {
		return nil, err
	}
	if memStr != "" {
		mem64, err := units.RAMInBytes(memStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse memory bytes %q: %w", memStr, err)
		}
		opts = append(opts, oci.WithMemoryLimit(uint64(mem64)))
	}

	opts = append(opts,
		oci.WithWindowNetworksAllowUnqualifiedDNSQuery(),
		oci.WithWindowsIgnoreFlushesDuringBoot())

	return opts, nil
}
