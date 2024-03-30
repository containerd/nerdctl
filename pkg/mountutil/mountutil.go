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

package mountutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/pkg/userns"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/idgen"
	"github.com/containerd/nerdctl/v2/pkg/mountutil/volumestore"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const (
	Bind          = "bind"
	Volume        = "volume"
	Tmpfs         = "tmpfs"
	Npipe         = "npipe"
	pathSeparator = string(os.PathSeparator)
)

type Processed struct {
	Type            string
	Mount           specs.Mount
	Name            string // name
	AnonymousVolume string // anonymous volume name
	Mode            string
	Opts            []oci.SpecOpts
}

type volumeSpec struct {
	Type            string
	Name            string
	Source          string
	AnonymousVolume string
}

func ProcessFlagV(s string, volStore volumestore.VolumeStore, createDir bool) (*Processed, error) {
	var (
		res      *Processed
		volSpec  volumeSpec
		src, dst string
		options  []string
	)

	split, err := splitVolumeSpec(s)
	if err != nil {
		return nil, fmt.Errorf("failed to split volume mount specification: %v", err)
	}

	switch len(split) {
	case 1:
		// validate destination
		dst = split[0]
		if _, err := validateAnonymousVolumeDestination(dst); err != nil {
			return nil, err
		}

		// create anonymous volume
		volSpec, err = handleAnonymousVolumes(dst, volStore)
		if err != nil {
			return nil, err
		}

		src = volSpec.Source
		res = &Processed{
			Type:            volSpec.Type,
			AnonymousVolume: volSpec.AnonymousVolume,
		}
	case 2, 3:
		// Vaildate destination
		dst = split[1]
		dst = strings.TrimLeft(dst, ":")
		if _, err := isValidPath(dst); err != nil {
			return nil, err
		}

		// Get volume spec
		src = split[0]
		volSpec, err = handleVolumeToMount(src, dst, volStore, createDir)
		if err != nil {
			return nil, err
		}

		src = volSpec.Source
		res = &Processed{
			Type:            volSpec.Type,
			Name:            volSpec.Name,
			AnonymousVolume: volSpec.AnonymousVolume,
		}

		// Parse volume options
		if len(split) == 3 {
			res.Mode = split[2]

			rawOpts := res.Mode

			options, res.Opts, err = getVolumeOptions(src, res.Type, rawOpts)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("failed to parse %q", s)
	}

	fstype := DefaultMountType
	if runtime.GOOS != "freebsd" {
		found := false
		for _, opt := range options {
			switch opt {
			case "rbind", "bind":
				fstype = "bind"
				found = true
			}
			if found {
				break
			}
		}
		if !found {
			options = append(options, "rbind")
		}
	}
	res.Mount = specs.Mount{
		Type:        fstype,
		Source:      cleanMount(src),
		Destination: cleanMount(dst),
		Options:     options,
	}
	if userns.RunningInUserNS() {
		unpriv, err := UnprivilegedMountFlags(src)
		if err != nil {
			return nil, fmt.Errorf("failed to get unprivileged mount flags for %q: %w", src, err)
		}
		res.Mount.Options = strutil.DedupeStrSlice(append(res.Mount.Options, unpriv...))
	}

	log.L.Debugf("processed mount: %+v\n", res)
	return res, nil
}

func handleBindMounts(source string, createDir bool) (volumeSpec, error) {
	var res volumeSpec
	res.Type = Bind
	res.Source = source

	// Handle relative paths
	if !filepath.IsAbs(source) {
		log.L.Warnf("expected an absolute path, got a relative path %q (allowed for nerdctl, but disallowed for Docker, so unrecommended)", source)
		absPath, err := filepath.Abs(source)
		if err != nil {
			return res, fmt.Errorf("failed to get the absolute path of %q: %w", source, err)
		}
		res.Source = absPath
	}

	// Create dir if it does not exist
	if err := createDirOnHost(source, createDir); err != nil {
		return res, err
	}

	return res, nil
}

func handleAnonymousVolumes(s string, volStore volumestore.VolumeStore) (volumeSpec, error) {
	var res volumeSpec
	res.AnonymousVolume = idgen.GenerateID()

	log.L.Debugf("creating anonymous volume %q, for %q", res.AnonymousVolume, s)
	anonVol, err := volStore.Create(res.AnonymousVolume, []string{})
	if err != nil {
		return res, fmt.Errorf("failed to create an anonymous volume %q: %w", res.AnonymousVolume, err)
	}

	res.Type = Volume
	res.Source = anonVol.Mountpoint
	return res, nil
}

func handleNamedVolumes(source string, volStore volumestore.VolumeStore) (volumeSpec, error) {
	var res volumeSpec
	res.Name = source
	vol, err := volStore.Get(res.Name, false)
	if err != nil {
		if errors.Is(err, errdefs.ErrNotFound) {
			vol, err = volStore.Create(res.Name, nil)
			if err != nil {
				return res, fmt.Errorf("failed to create volume %q: %w", res.Name, err)
			}
		} else {
			return res, fmt.Errorf("failed to get volume %q: %w", res.Name, err)
		}
	}
	// src is now an absolute path
	res.Type = Volume
	res.Source = vol.Mountpoint

	return res, nil
}

func getVolumeOptions(src string, vType string, rawOpts string) ([]string, []oci.SpecOpts, error) {
	// always call parseVolumeOptions for bind mount to allow the parser to add some default options
	var err error
	var specOpts []oci.SpecOpts
	options, specOpts, err := parseVolumeOptions(vType, src, rawOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse volume options (%q, %q, %q): %w", vType, src, rawOpts, err)
	}

	specOpts = append(specOpts, specOpts...)
	return options, specOpts, nil
}

func createDirOnHost(src string, createDir bool) error {
	_, err := os.Stat(src)
	if err == nil {
		return nil
	}

	if !createDir {

		/**
		* In pkg\mountutil\mountutil_linux.go:432, we disallow creating directories on host if not found
		* The user gets an error if the directory does not exist:
		*	  error mounting "/foo" to rootfs at "/foo": stat /foo: no such file or directory: unknown.
		* We log this error to give the user a hint that they may need to create the directory on the host.
		* https://docs.docker.com/storage/bind-mounts/
		 */
		if os.IsNotExist(err) {
			log.L.Warnf("mount source %q does not exist. Please make sure to create the directory on the host.", src)
			return nil
		}
		return fmt.Errorf("failed to stat %q: %w", src, err)
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat %q: %w", src, err)
	}
	if err := os.MkdirAll(src, 0o755); err != nil {
		return fmt.Errorf("failed to mkdir %q: %w", src, err)
	}
	return nil
}

func isNamedVolume(s string) bool {
	err := identifiers.Validate(s)

	// If the volume name is invalid, we assume it is a path
	return err == nil
}
