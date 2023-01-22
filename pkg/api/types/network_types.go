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

import "github.com/containerd/nerdctl/pkg/netutil"

// NetworkCreateCommandOptions specifies options for `nerdctl network create`.
type NetworkCreateCommandOptions struct {
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// CreateOptions is the option for creating network
	CreateOptions netutil.CreateOptions
}

// NetworkInspectCommandOptions specifies options for `nerdctl network inspect`.
type NetworkInspectCommandOptions struct {
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Inspect mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output
	Mode string
	// Format the output using the given Go template, e.g, '{{json .}}'
	Format string
	// Networks are the networks to be inspected
	Networks []string
}

// NetworkListCommandOptions specifies options for `nerdctl network ls`.
type NetworkListCommandOptions struct {
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Quiet only show numeric IDs
	Quiet bool
	// Format the output using the given Go template, e.g, '{{json .}}', 'wide'
	Format string
}

// NetworkPruneCommandOptions specifies options for `nerdctl network prune`.
type NetworkPruneCommandOptions struct {
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Network drivers to keep while pruning
	NetworkDriversToKeep []string
}

// NetworkRemoveCommandOptions specifies options for `nerdctl network rm`.
type NetworkRemoveCommandOptions struct {
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Networks are the networks to be removed
	Networks []string
}
