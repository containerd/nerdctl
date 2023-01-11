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

type ImageListCommandOptions struct {
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Quiet only show numeric IDs
	Quiet bool
	// NoTrunc don't truncate output
	NoTrunc bool
	// Format the output using the given Go template, e.g, '{{json .}}', 'wide'
	Format string
	// Filter output based on conditions provided, for the --filter argument
	Filters []string
	// NameAndRefFilter filters images by name and reference
	NameAndRefFilter []string
	// Digests show digests (compatible with Docker, unlike ID)
	Digests bool
	// Names show image names
	Names bool
	// All (unimplemented yet, always true)
	All bool
}
