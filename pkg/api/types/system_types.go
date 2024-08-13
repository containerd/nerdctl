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

package types

import "io"

// SystemInfoOptions specifies options for `nerdctl (system) info`.
type SystemInfoOptions struct {
	Stdout io.Writer
	Stderr io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Information mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output
	Mode string
	// Format the output using the given Go template, e.g, '{{json .}}
	Format string
}

// SystemEventsOptions specifies options for `nerdctl (system) events`.
type SystemEventsOptions struct {
	Stdout io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Format the output using the given Go template, e.g, '{{json .}}
	Format string
	// Filter events based on given conditions
	Filters []string
}

// SystemPruneOptions specifies options for `nerdctl system prune`.
type SystemPruneOptions struct {
	Stdout io.Writer
	Stderr io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// All remove all unused images, not just dangling ones
	All bool
	// Volumes decide whether prune volumes or not
	Volumes bool
	// BuildKitHost the address of BuildKit host
	BuildKitHost string
	// NetworkDriversToKeep the network drivers which need to keep
	NetworkDriversToKeep []string
}
