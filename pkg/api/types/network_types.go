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

type NetworkInspectCommandOptions struct {
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Format the output using the given go template
	Format string
	// Mode Inspect mode
	Mode string
	// Networks are the networks to be inspected
	Networks []string
}

type NetworkListCommandOptions struct {
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Quiet only show numeric IDs
	Quiet bool
	// Format the output using the given Go template, e.g, '{{json .}}', 'wide'
	Format string
}
