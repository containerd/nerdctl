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

package common

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/mountutil"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/docker/cli/opts"
	"github.com/spf13/cobra"
)

const (
	Category       = "category"
	Management     = "management"
	TiniInitBinary = "tini"
)

type ReadCounter struct {
	io.Reader
	N int
}

func (r *ReadCounter) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.N += n
	}
	return n, err
}

type SnapshotKey string

type DockerArchiveManifestJSONEntry struct {
	Config   string
	RepoTags []string
	Layers   []string
}

type DockerArchiveManifestJSON []DockerArchiveManifestJSONEntry

// recursive function to calculate total usage of key's parent
func (key SnapshotKey) Add(ctx context.Context, s snapshots.Snapshotter, usage *snapshots.Usage) error {
	if key == "" {
		return nil
	}
	u, err := s.Usage(ctx, string(key))
	if err != nil {
		return err
	}

	usage.Add(u)

	info, err := s.Stat(ctx, string(key))
	if err != nil {
		return err
	}

	key = SnapshotKey(info.Parent)
	return key.Add(ctx, s, usage)
}

// removing a non-stoped/non-created container without force, will cause a error
type StatusError struct {
	error
}

func NewStatusError(err error) StatusError {
	return StatusError{err}
}

func (e StatusError) Error() string {
	return e.error.Error()
}

type ExitCoder interface {
	error
	ExitCode() int
}

type ExitCodeError struct {
	error
	Code int
}

func (e ExitCodeError) ExitCode() int {
	return e.Code
}

func HandleExitCoder(err error) {
	if err == nil {
		return
	}

	if exitErr, ok := err.(ExitCoder); ok {
		os.Exit(exitErr.ExitCode())
	}
}

type InternalLabels struct {
	// labels from cmd options
	Namespace  string
	Platform   string
	ExtraHosts []string
	PidFile    string
	// labels from cmd options or automatically set
	Name     string
	Hostname string
	// automatically generated
	StateDir string
	// network
	Networks   []string
	IPAddress  string
	Ports      []cni.PortMapping
	MacAddress string
	// volumn
	MountPoints []*mountutil.Processed
	AnonVolumes []string
	// pid Namespace
	PidContainer string
	// log
	LogURI string
}

func WithInternalLabels(internalLabels InternalLabels) (containerd.NewContainerOpts, error) {
	m := make(map[string]string)
	m[labels.Namespace] = internalLabels.Namespace
	if internalLabels.Name != "" {
		m[labels.Name] = internalLabels.Name
	}
	m[labels.Hostname] = internalLabels.Hostname
	extraHostsJSON, err := json.Marshal(internalLabels.ExtraHosts)
	if err != nil {
		return nil, err
	}
	m[labels.ExtraHosts] = string(extraHostsJSON)
	m[labels.StateDir] = internalLabels.StateDir
	networksJSON, err := json.Marshal(internalLabels.Networks)
	if err != nil {
		return nil, err
	}
	m[labels.Networks] = string(networksJSON)
	if len(internalLabels.Ports) > 0 {
		portsJSON, err := json.Marshal(internalLabels.Ports)
		if err != nil {
			return nil, err
		}
		m[labels.Ports] = string(portsJSON)
	}
	if internalLabels.LogURI != "" {
		m[labels.LogURI] = internalLabels.LogURI
	}
	if len(internalLabels.AnonVolumes) > 0 {
		anonVolumeJSON, err := json.Marshal(internalLabels.AnonVolumes)
		if err != nil {
			return nil, err
		}
		m[labels.AnonymousVolumes] = string(anonVolumeJSON)
	}

	if internalLabels.PidFile != "" {
		m[labels.PIDFile] = internalLabels.PidFile
	}

	if internalLabels.IPAddress != "" {
		m[labels.IPAddress] = internalLabels.IPAddress
	}

	m[labels.Platform], err = platformutil.NormalizeString(internalLabels.Platform)
	if err != nil {
		return nil, err
	}

	if len(internalLabels.MountPoints) > 0 {
		mounts := DockercompatMounts(internalLabels.MountPoints)
		mountPointsJSON, err := json.Marshal(mounts)
		if err != nil {
			return nil, err
		}
		m[labels.Mounts] = string(mountPointsJSON)
	}

	if internalLabels.MacAddress != "" {
		m[labels.MACAddress] = internalLabels.MacAddress
	}

	if internalLabels.PidContainer != "" {
		m[labels.PIDContainer] = internalLabels.PidContainer
	}

	return containerd.WithAdditionalContainerLabels(m), nil
}

func DockercompatMounts(mountPoints []*mountutil.Processed) []dockercompat.MountPoint {
	reuslt := make([]dockercompat.MountPoint, len(mountPoints))
	for i := range mountPoints {
		mp := mountPoints[i]
		reuslt[i] = dockercompat.MountPoint{
			Type:        mp.Type,
			Name:        mp.Name,
			Source:      mp.Mount.Source,
			Destination: mp.Mount.Destination,
			Driver:      "",
			Mode:        mp.Mode,
		}

		// it's a anonymous volume
		if mp.AnonymousVolume != "" {
			reuslt[i].Name = mp.AnonymousVolume
		}

		// volume only support local driver
		if mp.Type == "volume" {
			reuslt[i].Driver = "local"
		}
	}
	return reuslt
}

func ReadKVStringsMapfFromLabel(cmd *cobra.Command) (map[string]string, error) {
	labelsMap, err := cmd.Flags().GetStringArray("label")
	if err != nil {
		return nil, err
	}
	labelsMap = strutil.DedupeStrSlice(labelsMap)
	labelsFilePath, err := cmd.Flags().GetStringSlice("label-file")
	if err != nil {
		return nil, err
	}
	labelsFilePath = strutil.DedupeStrSlice(labelsFilePath)
	labels, err := opts.ReadKVStrings(labelsFilePath, labelsMap)
	if err != nil {
		return nil, err
	}

	return strutil.ConvertKVStringsToMap(labels), nil
}

func ContainerStateDirPath(cmd *cobra.Command, dataStore, id string) (string, error) {
	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return "", err
	}
	if ns == "" {
		return "", errors.New("namespace is required")
	}
	if strings.Contains(ns, "/") {
		return "", errors.New("namespace with '/' is unsupported")
	}
	return filepath.Join(dataStore, "containers", ns, id), nil
}
