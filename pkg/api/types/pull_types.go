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

// PullCommandOptions specifies options for `nerdctl (image) pull`.
type PullCommandOptions struct {
	GOptions GlobalCommandOptions
	// Unpack the image for the current single platform (auto/true/false)
	Unpack string
	// Pull content for a specific platform
	Platform []string
	// Pull content for all platforms
	AllPlatforms bool
	// Verify the image (none|cosign)
	Verify string
	// Path to the public key file, KMS, URI or Kubernetes Secret for --verify=cosign
	CosignKey string
	// Suppress verbose output
	Quiet bool
	// multiaddr of IPFS API (default uses $IPFS_PATH env variable if defined or local directory ~/.ipfs)
	IPFSAddress string
}
