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

// LoadCommandOptions specifies options for `nerdctl (image) load`.
type LoadCommandOptions struct {
	GOptions GlobalCommandOptions
	// Input read from tar archive file, instead of STDIN
	Input string
	// Platform import content for a specific platform
	Platform []string
	// AllPlatforms import content for all platforms
	AllPlatforms bool
}
