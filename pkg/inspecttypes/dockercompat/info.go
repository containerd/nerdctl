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
   Portions from https://github.com/moby/moby/blob/v20.10.8/api/types/types.go
   Portions from https://github.com/docker/cli/blob/v20.10.8/cli/command/system/version.go
   Copyright (C) Docker/Moby authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/moby/moby/blob/v20.10.8/NOTICE , https://github.com/docker/cli/blob/v20.10.8/NOTICE
*/

package dockercompat

// Info mimics a `docker info` object.
// From https://github.com/moby/moby/blob/v20.10.8/api/types/types.go#L146-L216
type Info struct {
	ID          string
	Driver      string
	Plugins     PluginsInfo
	MemoryLimit bool
	SwapLimit   bool
	// KernelMemory is omitted because it is deprecated in the Moby
	CPUCfsPeriod      bool `json:"CpuCfsPeriod"`
	CPUCfsQuota       bool `json:"CpuCfsQuota"`
	CPUShares         bool
	CPUSet            bool
	PidsLimit         bool
	IPv4Forwarding    bool
	BridgeNfIptables  bool
	BridgeNfIP6tables bool `json:"BridgeNfIp6tables"`
	// Nfd is omitted because it does not make sense for nerdctl
	OomKillDisable bool
	// NGoroutines is omitted because it does not make sense for nerdctl
	SystemTime    string
	LoggingDriver string
	CgroupDriver  string
	CgroupVersion string `json:",omitempty"`
	// NEventsListener is omitted because it does not make sense for nerdctl
	KernelVersion   string
	OperatingSystem string
	OSType          string
	Architecture    string // e.g., "x86_64", not "amd64" (Corresponds to Docker)
	NCPU            int
	MemTotal        int64
	Name            string
	ServerVersion   string
	SecurityOptions []string

	Warnings []string
}

type PluginsInfo struct {
	Log     []string
	Storage []string // nerdctl extension
}

// VersionInfo mimics a `docker version` object.
// From https://github.com/docker/cli/blob/v20.10.8/cli/command/system/version.go#L68-L72
type VersionInfo struct {
	Client ClientVersion
	Server *ServerVersion
}

// ClientVersion is from https://github.com/docker/cli/blob/v20.10.8/cli/command/system/version.go#L74-L87
type ClientVersion struct {
	Version    string
	GitCommit  string
	GoVersion  string
	Os         string             // GOOS
	Arch       string             // GOARCH
	Components []ComponentVersion // nerdctl extension
}

// ComponentVersion describes the version information for a specific component.
// From https://github.com/moby/moby/blob/v20.10.8/api/types/types.go#L112-L117
type ComponentVersion struct {
	Name    string
	Version string
	Details map[string]string `json:",omitempty"`
}

// ServerVersion is from https://github.com/moby/moby/blob/v20.10.8/api/types/types.go#L119-L137
type ServerVersion struct {
	Components []ComponentVersion
	// Deprecated fields are not added here
}
