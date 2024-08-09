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

package ipcutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

type IPCMode string

type IPC struct {
	Mode IPCMode `json:"mode,omitempty"`
	// VictimContainer is only used when mode is container
	VictimContainerID *string `json:"victimContainerId,omitempty"`

	// HostShmPath is only used when mode is shareable
	HostShmPath *string `json:"hostShmPath,omitempty"`

	// ShmSize is only used when mode is private or shareable
	// Devshm size in bytes
	ShmSize string `json:"shmSize,omitempty"`
}

const (
	Private   IPCMode = "private"
	Host      IPCMode = "host"
	Shareable IPCMode = "shareable"
	Container IPCMode = "container"
)

// DetectFlags detects IPC mode from the given ipc string and shmSize string.
// If ipc is empty, it returns IPC{Mode: Private}.
func DetectFlags(ctx context.Context, client *containerd.Client, stateDir string, ipc string, shmSize string) (IPC, error) {
	var res IPC
	res.ShmSize = shmSize
	switch ipc {
	case "", "private":
		res.Mode = Private
	case "host":
		res.Mode = Host
	case "shareable":
		res.Mode = Shareable
		shmPath := filepath.Join(stateDir, "shm")
		res.HostShmPath = &shmPath
	default: // container:<id|name>
		res.Mode = Container
		parsed := strings.Split(ipc, ":")
		if len(parsed) < 2 || parsed[0] != "container" {
			return res, fmt.Errorf("invalid ipc namespace. Set --ipc=[host|container:<name|id>")
		}

		containerName := parsed[1]
		walker := &containerwalker.ContainerWalker{
			Client: client,
			OnFound: func(ctx context.Context, found containerwalker.Found) error {
				if found.MatchCount > 1 {
					return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
				}
				victimContainerID := found.Container.ID()
				res.VictimContainerID = &victimContainerID

				return nil
			},
		}
		matchedCount, err := walker.Walk(ctx, containerName)
		if err != nil {
			return res, err
		}
		if matchedCount < 1 {
			return res, fmt.Errorf("no such container: %s", containerName)
		}
	}

	return res, nil
}

// EncodeIPCLabel encodes IPC spec into a label.
func EncodeIPCLabel(ipc IPC) (string, error) {
	if ipc.Mode == "" {
		return "", nil
	}
	b, err := json.Marshal(ipc)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeIPCLabel decodes IPC spec from a label.
// For backward compatibility, if ipcLabel is empty, it returns IPC{Mode: Private}.
func DecodeIPCLabel(ipcLabel string) (IPC, error) {
	if ipcLabel == "" {
		return IPC{
			Mode: Private,
		}, nil
	}

	var ipc IPC
	if err := json.Unmarshal([]byte(ipcLabel), &ipc); err != nil {
		return IPC{}, err
	}
	return ipc, nil
}

// GenerateIPCOpts generates IPC spec opts from the given IPC.
func GenerateIPCOpts(ctx context.Context, ipc IPC, client *containerd.Client) ([]oci.SpecOpts, error) {
	opts := make([]oci.SpecOpts, 0)

	switch ipc.Mode {
	case Private:
		// If nothing is specified, or if private, default to normal behavior
		if len(ipc.ShmSize) > 0 {
			shmBytes, err := units.RAMInBytes(ipc.ShmSize)
			if err != nil {
				return nil, err
			}
			opts = append(opts, oci.WithDevShmSize(shmBytes/1024))
		}
	case Host:
		opts = append(opts, withBindMountHostIPC)
		if runtime.GOOS != "windows" {
			opts = append(opts, oci.WithHostNamespace(specs.IPCNamespace))
		}
	case Shareable:
		if ipc.HostShmPath == nil {
			return nil, errors.New("ipc mode is shareable, but host shm path is nil")
		}
		err := makeShareableDevshm(*ipc.HostShmPath, ipc.ShmSize)
		if err != nil {
			return nil, err
		}
		opts = append(opts, withBindMountHostOtherSourceIPC(*ipc.HostShmPath))
	case Container:
		if ipc.VictimContainerID == nil {
			return nil, errors.New("ipc mode is container, but victim container id is nil")
		}
		targetCon, err := client.LoadContainer(ctx, *ipc.VictimContainerID)
		if err != nil {
			return nil, err
		}

		task, err := targetCon.Task(ctx, nil)
		if err != nil {
			return nil, err
		}

		status, err := task.Status(ctx)
		if err != nil {
			return nil, err
		}

		if status.Status != containerd.Running {
			return nil, fmt.Errorf("shared container is not running")
		}

		targetConLabels, err := targetCon.Labels(ctx)
		if err != nil {
			return nil, err
		}

		targetConIPC, err := DecodeIPCLabel(targetConLabels[labels.IPC])
		if err != nil {
			return nil, err
		}

		if targetConIPC.Mode == Host {
			opts = append(opts, oci.WithHostNamespace(specs.IPCNamespace))
			opts = append(opts, withBindMountHostIPC)
			return opts, nil
		} else if targetConIPC.Mode != Shareable {
			return nil, errors.New("victim container's ipc mode is not shareable")
		}

		if targetConIPC.HostShmPath == nil {
			return nil, errors.New("victim container's host shm path is nil")
		}

		opts = append(opts, withBindMountHostOtherSourceIPC(*targetConIPC.HostShmPath))
	}

	return opts, nil
}

// WithBindMountHostOtherSourceIPC replaces /dev/shm mount with rbind by the given path on host
func withBindMountHostOtherSourceIPC(source string) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		for i, m := range s.Mounts {
			p := path.Clean(m.Destination)
			if p == "/dev/shm" {
				s.Mounts[i] = specs.Mount{
					Type:        "bind",
					Destination: p,
					Source:      source,
					Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
				}
			}
		}
		return nil
	}
}

// WithBindMountHostIPC replaces /dev/shm and /dev/mqueue mount with rbind.
// Required for --ipc=host on rootless.
func withBindMountHostIPC(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
	for i, m := range s.Mounts {
		switch p := path.Clean(m.Destination); p {
		case "/dev/shm", "/dev/mqueue":
			s.Mounts[i] = specs.Mount{
				Destination: p,
				Type:        "bind",
				Source:      p,
				Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
			}
		}
	}
	return nil
}

func CleanUp(ipc IPC) error {
	return cleanUpPlatformSpecificIPC(ipc)
}
