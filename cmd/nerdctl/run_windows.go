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
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/docker/go-units"
	"github.com/spf13/cobra"
)

const (
	// HostProcessInheritUser inherits permissions of containerd process
	hostProcessInheritUser = "microsoft.com/hostprocess-inherit-user"

	// HostProcessContainer will launch a host process container
	hostProcessContainer = "microsoft.com/hostprocess-container"
)

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
	_ types.GlobalCommandOptions,
	id string,
	internalLabels *internalLabels,
) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts
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

	// TODO implement hyper-v isolation
	isolation, err := cmd.Flags().GetString("isolation")
	if err != nil {
		return nil, err
	}
	switch isolation {
	case "host":
		hpAnnotations := map[string]string{
			hostProcessContainer: "true",
		}

		// If user is set we will attempt to start container with that user (must be present on the host)
		// Otherwise we will inherit permissions from the user that the containerd process is running as
		user, err := cmd.Flags().GetString("user")
		if err != nil {
			return nil, err
		}
		if user == "" {
			hpAnnotations[hostProcessInheritUser] = "true"
		}

		opts = append(opts, oci.WithAnnotations(hpAnnotations))
	case "process":
		// override the default isolation mode in the case where
		// the containerd default_runtime is set to hyper-v
		opts = append(opts, WithWindowsProcessIsolated())
	case "default":
		// no op
		// use containerd's default runtime option `default_runtime` set in the config.toml
	default:
		return nil, fmt.Errorf("unknown isolation value %q. valid values are 'host', 'process' or 'default'", isolation)
	}

	opts = append(opts,
		oci.WithWindowNetworksAllowUnqualifiedDNSQuery(),
		oci.WithWindowsIgnoreFlushesDuringBoot())

	return opts, nil
}

func WithWindowsProcessIsolated() oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Windows == nil {
			s.Windows = &specs.Windows{}
		}
		if s.Windows.HyperV != nil {
			s.Windows.HyperV = nil
		}
		return nil
	}
}
