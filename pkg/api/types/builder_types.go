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

// BuilderBuildOptions specifies options for `nerdctl (image/builder) build`.
type BuilderBuildOptions struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// BuildKitHost is the buildkit host
	BuildKitHost string
	// Tag is the tag of the image
	Tag []string
	// File Name of the Dockerfile
	File string
	// Target is the target of the build
	Target string
	// BuildArgs is the build-time variables
	BuildArgs []string
	// NoCache disables cache
	NoCache bool
	// Output is the output destination
	Output string
	// Progress Set type of progress output (auto, plain, tty). Use plain to show container output
	Progress string
	// Secret file to expose to the build: id=mysecret,src=/local/secret
	Secret []string
	// Allow extra privileged entitlement, e.g. network.host, security.insecure
	Allow []string
	// Attestation parameters (format: "type=sbom,generator=image")"
	Attest []string
	// SSH agent socket or keys to expose to the build (format: default|<id>[=<socket>|<key>[,<key>]])
	SSH []string
	// Quiet suppress the build output and print image ID on success
	Quiet bool
	// CacheFrom external cache sources (eg. user/app:cache, type=local,src=path/to/dir)
	CacheFrom []string
	// CacheTo cache export destinations (eg. user/app:cache, type=local,dest=path/to/dir)
	CacheTo []string
	// Rm remove intermediate containers after a successful build
	Rm bool
	// Platform set target platform for build (e.g., "amd64", "arm64")
	Platform []string
	// IidFile write the image ID to the file
	IidFile string
	// Label is the metadata for an image
	Label []string
	// BuildContext is the build context
	BuildContext string
	// NetworkMode mode for the build context
	NetworkMode string
}

// BuilderPruneOptions specifies options for `nerdctl builder prune`.
type BuilderPruneOptions struct {
	Stderr io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// BuildKitHost is the buildkit host
	BuildKitHost string
	// All will remove all unused images and all build cache, not just dangling ones
	All bool
}
