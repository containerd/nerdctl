/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"github.com/containerd/cgroups"
	"github.com/containerd/containerd/oci"
	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

func defaultCgroupnsMode() string {
	if cgroups.Mode() == cgroups.Unified {
		return "private"
	}
	return "host"
}

func generateCgroupOpts(clicontext *cli.Context) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts

	// cpus: from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/run/run_unix.go#L187-L193
	if cpus := clicontext.Float64("cpus"); cpus > 0.0 {
		var (
			period = uint64(100000)
			quota  = int64(cpus * 100000.0)
		)
		opts = append(opts, oci.WithCPUCFS(quota, period))
	}

	if memStr := clicontext.String("memory"); memStr != "" {
		mem64, err := units.RAMInBytes(memStr)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse memory bytes %q", memStr)
		}
		opts = append(opts, oci.WithMemoryLimit(uint64(mem64)))
	}

	if pidsLimit := clicontext.Int("pids-limit"); pidsLimit > 0 {
		opts = append(opts, oci.WithPidsLimit(int64(pidsLimit)))
	}

	switch cgroupns := clicontext.String("cgroupns"); cgroupns {
	case "private":
		ns := specs.LinuxNamespace{
			Type: specs.CgroupNamespace,
		}
		opts = append(opts, oci.WithLinuxNamespace(ns))
	case "host":
		opts = append(opts, oci.WithHostNamespace(specs.CgroupNamespace))
	default:
		return nil, errors.Errorf("unknown cgroupns mode %q", cgroupns)
	}
	return opts, nil
}
