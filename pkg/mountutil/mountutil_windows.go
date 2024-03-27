//go:build windows

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

/*
   Portions from https://github.com/moby/moby/blob/f5c7673ff8fcbd359f75fb644b1365ca9d20f176/volume/mounts/windows_parser.go#L26
   Copyright (C) Docker/Moby authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/moby/moby/blob/master/NOTICE
*/

package mountutil

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/mountutil/volumestore"
)

const (
	// Defaults to an empty string
	// https://github.com/microsoft/hcsshim/blob/5c75f29c1f5cb4d3498d66228637d07477bcb6a1/internal/hcsoci/resources_wcow.go#L140
	DefaultMountType = ""

	// DefaultPropagationMode is the default propagation of mounts
	// where user doesn't specify mount propagation explicitly.
	// See also: https://github.com/moby/moby/blob/v20.10.7/volume/mounts/windows_parser.go#L440-L442
	DefaultPropagationMode = ""
)

func UnprivilegedMountFlags(path string) ([]string, error) {
	m := []string{}
	return m, nil
}

// parseVolumeOptions parses specified optsRaw with using information of
// the volume type and the src directory when necessary.
func parseVolumeOptions(vType, src, optsRaw string) ([]string, []oci.SpecOpts, error) {
	var writeModeRawOpts []string
	for _, opt := range strings.Split(optsRaw, ",") {
		switch opt {
		case "rw":
			writeModeRawOpts = append(writeModeRawOpts, opt)
		case "ro":
			writeModeRawOpts = append(writeModeRawOpts, opt)
		case "":
			// NOP
		default:
			log.L.Warnf("unsupported volume option %q", opt)
		}
	}
	var opts []string
	if len(writeModeRawOpts) > 1 {
		return nil, nil, fmt.Errorf("duplicated read/write volume option: %+v", writeModeRawOpts)
	} else if len(writeModeRawOpts) > 0 && writeModeRawOpts[0] == "ro" {
		opts = append(opts, "ro")
	} // No need to return option when "rw"
	return opts, nil, nil
}

func ProcessFlagTmpfs(s string) (*Processed, error) {
	return nil, errdefs.ErrNotImplemented
}

func ProcessFlagMount(s string, volStore volumestore.VolumeStore) (*Processed, error) {
	return nil, errdefs.ErrNotImplemented
}

func handleVolumeToMount(source string, dst string, volStore volumestore.VolumeStore, createDir bool) (volumeSpec, error) {
	// Validate source and destination types
	if _, err := (validateNamedPipeSpec(source, dst)); err != nil {
		return volumeSpec{}, err
	}

	switch {
	// Handle named volumes
	case isNamedVolume(source):
		return handleNamedVolumes(source, volStore)

	// Handle named pipes
	case isNamedPipe(source):
		return handleNpipeToMount(source)

	// Handle bind volumes (file paths)
	default:
		return handleBindMounts(source, createDir)
	}
}

func handleNpipeToMount(source string) (volumeSpec, error) {
	res := volumeSpec{
		Type:   Npipe,
		Source: source,
	}
	return res, nil
}

func splitVolumeSpec(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimLeft(raw, ":")
	if raw == "" {
		return nil, fmt.Errorf("invalid empty volume specification")
	}

	const (
		// Root drive or relative paths starting with .
		rxHostDir = `(?:[a-zA-Z]:|\.)[\/\\]`

		// https://learn.microsoft.com/en-us/dotnet/standard/io/file-path-formats
		// Windows UNC paths and DOS device paths (and namde pipes)
		rxUNC  = `(?:\\{2}[a-zA-Z0-9_\-\.\?]+\\{1}[^\\*?"|\r\n]+)\\`
		rxName = `[^\/\\:*?"<>|\r\n]+`

		rxSource      = `((?P<source>((` + rxHostDir + `|` + rxUNC + `)` + `(` + rxName + `[\/\\]?)+` + `|` + rxName + `)):)?`
		rxDestination = `(?P<destination>(` + rxHostDir + `|` + rxUNC + `)` + `(` + rxName + `[\/\\]?)+` + `|` + rxName + `)`
		rxMode        = `(?::(?P<mode>(?i)\w+(,\w+)?))`

		rxWindows = `^` + rxSource + rxDestination + `(?:` + rxMode + `)?$`
	)

	compiledRegex, err := regexp.Compile(rxWindows)
	if err != nil {
		return nil, fmt.Errorf("error compiling regex: %s", err)
	}
	return splitRawSpec(raw, compiledRegex)
}

func isNamedPipe(s string) bool {
	pattern := `^\\{2}.\\pipe\\[^\/\\:*?"<>|\r\n]+$`
	matches, err := regexp.MatchString(pattern, s)
	if err != nil {
		log.L.Errorf("Invalid pattern %s", pattern)
	}

	return matches
}

func cleanMount(p string) string {
	if isNamedPipe(p) {
		return p
	}
	return filepath.Clean(p)
}

func isValidPath(s string) (bool, error) {
	if isNamedPipe(s) || filepath.IsAbs(s) {
		return true, nil
	}

	return false, fmt.Errorf("expected an absolute path or a named pipe, got %q", s)
}

/*
For docker compatibility on Windows platforms:
Docker only allows for absolute paths as anonymous volumes.
Docker does not allows anonymous named volumes or anonymous named piped
to be mounted into a container.
*/
func validateAnonymousVolumeDestination(s string) (bool, error) {
	if isNamedPipe(s) || isNamedVolume(s) {
		return false, fmt.Errorf("invalid volume specification: %q. only directories can be mapped as anonymous volumes", s)
	}

	if filepath.IsAbs(s) {
		return true, nil
	}

	return false, fmt.Errorf("expected an absolute path, got %q", s)
}

func splitRawSpec(raw string, splitRegexp *regexp.Regexp) ([]string, error) {
	match := splitRegexp.FindStringSubmatch(raw)
	if len(match) == 0 {
		return nil, fmt.Errorf("invalid volume specification: '%s'", raw)
	}

	var split []string
	matchgroups := make(map[string]string)
	// Pull out the sub expressions from the named capture groups
	for i, name := range splitRegexp.SubexpNames() {
		matchgroups[name] = match[i]
	}
	if source, exists := matchgroups["source"]; exists {
		if source == "." {
			return nil, fmt.Errorf("invalid volume specification: %q", raw)
		}

		if source != "" {
			split = append(split, source)
		}
	}

	mode, modExists := matchgroups["mode"]

	if destination, exists := matchgroups["destination"]; exists {
		if destination == "." {
			return nil, fmt.Errorf("invalid volume specification: %q", raw)
		}

		// If mode exists and destination is empty, set destination to an empty string
		// source::ro
		if destination != "" || modExists && mode != "" {
			split = append(split, destination)
		}
	}

	if mode, exists := matchgroups["mode"]; exists {
		if mode != "" {
			split = append(split, mode)
		}
	}
	return split, nil
}

// Function to parse the source type
func parseSourceType(source string) string {
	switch {
	case isNamedVolume(source):
		return Volume
	case isNamedPipe(source):
		return Npipe
	// Add more cases for different source types as needed
	default:
		return Bind
	}
}

func validateNamedPipeSpec(source string, dst string) (bool, error) {
	// Validate source and destination types
	sourceType := parseSourceType(source)
	destType := parseSourceType(dst)

	if (destType == Npipe && sourceType != Npipe) || (sourceType == Npipe && destType != Npipe) {
		return false, fmt.Errorf("invalid volume specification. named pipes can only be mapped to named pipes")
	}
	return true, nil
}
