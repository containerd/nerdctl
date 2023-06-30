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

import (
	"io"

	"github.com/containerd/nerdctl/pkg/imgutil/jobs"
)

// ImageListOptions specifies options for `nerdctl image list`.
type ImageListOptions struct {
	Stdout io.Writer
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

// ImageConvertOptions specifies options for `nerdctl image convert`.
type ImageConvertOptions struct {
	Stdout   io.Writer
	GOptions GlobalCommandOptions

	// #region generic flags
	// Uncompress convert tar.gz layers to uncompressed tar layers
	Uncompress bool
	// Oci convert Docker media types to OCI media types
	Oci bool
	// #endregion

	// #region platform flags
	// Platforms convert content for a specific platform
	Platforms []string
	// AllPlatforms convert content for all platforms
	AllPlatforms bool
	// #endregion

	// Format the output using the given Go template, e.g, 'json'
	Format string

	// #region estargz flags
	// Estargz convert legacy tar(.gz) layers to eStargz for lazy pulling. Should be used in conjunction with '--oci'
	Estargz bool
	// EstargzRecordIn read 'ctr-remote optimize --record-out=<FILE>' record file (EXPERIMENTAL)
	EstargzRecordIn string
	// EstargzCompressionLevel eStargz compression level
	EstargzCompressionLevel int
	// EstargzChunkSize eStargz chunk size
	EstargzChunkSize int
	// EstargzMinChunkSize the minimal number of bytes of data must be written in one gzip stream. (requires stargz-snapshotter >= v0.13.0)
	EstargzMinChunkSize int
	// EstargzExternalToc separate TOC JSON into another image (called "TOC image"). The name of TOC image is the original + "-esgztoc" suffix. Both eStargz and the TOC image should be pushed to the same registry. (requires stargz-snapshotter >= v0.13.0) (EXPERIMENTAL)
	EstargzExternalToc bool
	// EstargzKeepDiffID convert to esgz without changing diffID (cannot be used in conjunction with '--estargz-record-in'. must be specified with '--estargz-external-toc')
	EstargzKeepDiffID bool
	// #endregion

	// #region zstd:chunked flags
	// ZstdChunked convert legacy tar(.gz) layers to zstd:chunked for lazy pulling. Should be used in conjunction with '--oci'
	ZstdChunked bool
	// ZstdChunkedCompressionLevel zstd compression level
	ZstdChunkedCompressionLevel int
	// ZstdChunkedChunkSize zstd chunk size
	ZstdChunkedChunkSize int
	// ZstdChunkedRecordIn read 'ctr-remote optimize --record-out=<FILE>' record file (EXPERIMENTAL)
	ZstdChunkedRecordIn string
	// #endregion

	// #region nydus flags
	// Nydus convert legacy tar(.gz) layers to nydus for lazy pulling. Should be used in conjunction with '--oci'
	Nydus bool
	// NydusBuilderPath the nydus-image binary path, if unset, search in PATH environment
	NydusBuilderPath string
	// NydusWorkDir work directory path for image conversion, default is the nerdctl data root directory
	NydusWorkDir string
	// NydusPrefetchPatterns the file path pattern list want to prefetch
	NydusPrefetchPatterns string
	// NydusCompressor nydus blob compression algorithm, possible values: `none`, `lz4_block`, `zstd`, default is `lz4_block`
	NydusCompressor string
	// #endregion

	// #region overlaybd flags
	// Overlaybd convert tar.gz layers to overlaybd layers
	Overlaybd bool
	// OverlayFsType filesystem type for overlaybd
	OverlayFsType string
	// OverlaydbDBStr database config string for overlaybd
	OverlaydbDBStr string
	// #endregion

}

// ImageCryptOptions specifies options for `nerdctl image encrypt` and `nerdctl image decrypt`.
type ImageCryptOptions struct {
	Stdout   io.Writer
	GOptions GlobalCommandOptions
	// Platforms convert content for a specific platform
	Platforms []string
	// AllPlatforms convert content for all platforms
	AllPlatforms bool
	// GpgHomeDir the GPG homedir to use; by default gpg uses ~/.gnupg"
	GpgHomeDir string
	// GpgVersion the GPG version ("v1" or "v2"), default will make an educated guess
	GpgVersion string
	// Keys a secret key's filename and an optional password separated by colon;
	Keys []string
	// DecRecipients recipient of the image; used only for PKCS7 and must be an x509 certificate
	DecRecipients []string
	// Recipients of the image is the person who can decrypt it in the form specified above (i.e. jwe:/path/to/pubkey)
	Recipients []string
}

// ImageInspectOptions specifies options for `nerdctl image inspect`.
type ImageInspectOptions struct {
	Stdout   io.Writer
	GOptions GlobalCommandOptions
	// Mode Inspect mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output
	Mode string
	// Format the output using the given Go template, e.g, 'json'
	Format string
	// Platform inspect content for a specific platform
	Platform string
}

// ImagePushOptions specifies options for `nerdctl (image) push`.
type ImagePushOptions struct {
	Stdout      io.Writer
	GOptions    GlobalCommandOptions
	SignOptions ImageSignOptions
	// Platforms convert content for a specific platform
	Platforms []string
	// AllPlatforms convert content for all platforms
	AllPlatforms bool

	// Estargz convert image to sStargz
	Estargz bool
	// IpfsEnsureImage ensure image is pushed to IPFS
	IpfsEnsureImage bool
	// IpfsAddress multiaddr of IPFS API (default uses $IPFS_PATH env variable if defined or local directory ~/.ipfs)
	IpfsAddress string
	// Suppress verbose output
	Quiet bool
	// If non-nil, the Push job will send upload statuses to the handler instead of Stdout
	ProgressHandler jobs.StatusHandler
	// AllowNondistributableArtifacts allow pushing non-distributable artifacts
	AllowNondistributableArtifacts bool
}

// ImagePullOptions specifies options for `nerdctl (image) pull`.
type ImagePullOptions struct {
	Stdout        io.Writer
	GOptions      GlobalCommandOptions
	VerifyOptions ImageVerifyOptions
	// Unpack the image for the current single platform (auto/true/false)
	Unpack string
	// Pull content for a specific platform
	Platform []string
	// Pull content for all platforms
	AllPlatforms bool
	// Suppress verbose output
	Quiet bool
	// If non-nil, the Pull job will send download statuses to the handler instead of Stdout
	ProgressHandler jobs.StatusHandler
	// multiaddr of IPFS API (default uses $IPFS_PATH env variable if defined or local directory ~/.ipfs)
	IPFSAddress string
}

// ImageTagOptions specifies options for `nerdctl (image) tag`.
type ImageTagOptions struct {
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Source is the image to be referenced.
	Source string
	// Target is the image to be created.
	Target string
}

// ImageRemoveOptions specifies options for `nerdctl rmi` and `nerdctl image rm`.
type ImageRemoveOptions struct {
	Stdout io.Writer
	// GOptions is the global options
	GOptions GlobalCommandOptions
	// Force removal of the image
	Force bool
	// Async asynchronous mode or not
	Async bool
}

// ImagePruneOptions specifies options for `nerdctl image prune` and `nerdctl image rm`.
type ImagePruneOptions struct {
	Stdout io.Writer
	// GOptions is the global options.
	GOptions GlobalCommandOptions
	// All Remove all unused images, not just dangling ones.
	All bool
	// Force will not prompt for confirmation.
	Force bool
}

// ImageSaveOptions specifies options for `nerdctl (image) save`.
type ImageSaveOptions struct {
	Stdout   io.Writer
	GOptions GlobalCommandOptions
	// Export content for all platforms
	AllPlatforms bool
	// Export content for a specific platform
	Platform []string
}

// ImageSignOptions contains options for signing an image. It contains options from
// all providers. The `provider` field determines which provider is used.
type ImageSignOptions struct {
	// Provider used to sign the image (none|cosign|notation)
	Provider string
	// CosignKey Path to the private key file, KMS URI or Kubernetes Secret for --sign=cosign
	CosignKey string
	// NotationKeyName Signing key name for a key previously added to notation's key list for --sign=notation
	NotationKeyName string
}

// ImageVerifyOptions contains options for verifying an image. It contains options from
// all providers. The `provider` field determines which provider is used.
type ImageVerifyOptions struct {
	// Provider used to verify the image (none|cosign|notation)
	Provider string
	// CosignKey Path to the public key file, KMS URI or Kubernetes Secret for --verify=cosign
	CosignKey string
	// CosignCertificateIdentity The identity expected in a valid Fulcio certificate for --verify=cosign. Valid values include email address, DNS names, IP addresses, and URIs. Either --cosign-certificate-identity or --cosign-certificate-identity-regexp must be set for keyless flows
	CosignCertificateIdentity string
	// CosignCertificateIdentityRegexp A regular expression alternative to --cosign-certificate-identity for --verify=cosign. Accepts the Go regular expression syntax described at https://golang.org/s/re2syntax. Either --cosign-certificate-identity or --cosign-certificate-identity-regexp must be set for keyless flows
	CosignCertificateIdentityRegexp string
	// CosignCertificateOidcIssuer The OIDC issuer expected in a valid Fulcio certificate for --verify=cosign, e.g. https://token.actions.githubusercontent.com or https://oauth2.sigstore.dev/auth. Either --cosign-certificate-oidc-issuer or --cosign-certificate-oidc-issuer-regexp must be set for keyless flows
	CosignCertificateOidcIssuer string
	// CosignCertificateOidcIssuerRegexp A regular expression alternative to --certificate-oidc-issuer for --verify=cosign. Accepts the Go regular expression syntax described at https://golang.org/s/re2syntax. Either --cosign-certificate-oidc-issuer or --cosign-certificate-oidc-issuer-regexp must be set for keyless flows
	CosignCertificateOidcIssuerRegexp string
}
