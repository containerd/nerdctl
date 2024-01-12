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

package container

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const (
	// HostProcessInheritUser inherits permissions of containerd process
	hostProcessInheritUser = "microsoft.com/hostprocess-inherit-user"

	// HostProcessContainer will launch a host process container
	hostProcessContainer = "microsoft.com/hostprocess-container"
	uvmMemorySizeInMB    = "io.microsoft.virtualmachine.computetopology.memory.sizeinmb"
	uvmCPUCount          = "io.microsoft.virtualmachine.computetopology.processor.count"
)

func setPlatformOptions(
	ctx context.Context,
	client *containerd.Client,
	id, uts string,
	internalLabels *internalLabels,
	options types.ContainerCreateOptions,
) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts
	if options.CPUs > 0.0 {
		opts = append(opts, oci.WithWindowsCPUCount(uint64(options.CPUs)))
	}

	if options.Memory != "" {
		mem64, err := units.RAMInBytes(options.Memory)
		if err != nil {
			return nil, fmt.Errorf("failed to parse memory bytes %q: %w", options.Memory, err)
		}
		opts = append(opts, oci.WithMemoryLimit(uint64(mem64)))
	}

	switch options.Isolation {
	case "hyperv":
		if options.Memory != "" {
			mem64, err := units.RAMInBytes(options.Memory)
			if err != nil {
				return nil, fmt.Errorf("failed to parse memory bytes %q: %w", options.Memory, err)
			}
			UVMMemmory := map[string]string{
				uvmMemorySizeInMB: fmt.Sprintf("%v", mem64),
			}
			opts = append(opts, oci.WithAnnotations(UVMMemmory))
		}

		if options.CPUs > 0.0 {
			UVMCPU := map[string]string{
				uvmCPUCount: fmt.Sprintf("%v", options.CPUs),
			}
			opts = append(opts, oci.WithAnnotations(UVMCPU))
		}
		opts = append(opts, oci.WithWindowsHyperV)
	case "host":
		hpAnnotations := map[string]string{
			hostProcessContainer: "true",
		}

		// If user is set we will attempt to start container with that user (must be present on the host)
		// Otherwise we will inherit permissions from the user that the containerd process is running as
		if options.User == "" {
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
		return nil, fmt.Errorf("unknown isolation value %q. valid values are 'host', 'process' or 'default'", options.Isolation)
	}

	opts = append(opts,
		oci.WithWindowNetworksAllowUnqualifiedDNSQuery(),
		oci.WithWindowsIgnoreFlushesDuringBoot())

	for _, dev := range options.Device {
		idType, devID, ok := strings.Cut(dev, "://")
		if !ok {
			return nil, errors.New("devices must be in the format IDType://ID")
		}
		if idType == "" {
			return nil, errors.New("devices must have a non-empty IDType")
		}
		opts = append(opts, oci.WithWindowsDevice(idType, devID))
	}

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
