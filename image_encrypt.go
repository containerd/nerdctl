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

package main

import (
	"github.com/spf13/cobra"
)

const imageEncryptHelp = `Encrypt image layers.

Use '--recipient' to specify the recipients.
The following protocol prefixes are supported:
- pgp:<email-address>
- jwe:<public-key-file-path>
- pkcs7:<x509-file-path>

Use '--platform' to define the platforms to encrypt. Defaults to the host platform.
When '--all-platforms' is given all images in a manifest list must be available.
Unspecified platforms are omitted from the output image.

Example:
  openssl genrsa -out mykey.pem
  openssl rsa -in mykey.pem -pubout -out mypubkey.pem
  nerdctl image encrypt --recipient=jwe:mypubkey.pem --platform=linux/amd64,linux/arm64 foo example.com/foo:encrypted
  nerdctl push example.com/foo:encrypted

To run the encrypted image, put the private key file (mykey.pem) to /etc/containerd/ocicrypt/keys (rootful) or ~/.config/containerd/ocicrypt/keys (rootless).
containerd before v1.4 requires extra configuration steps, see https://github.com/containerd/nerdctl/blob/master/docs/ocicrypt.md

CAUTION: This command only encrypts image layers, but does NOT encrypt container configuration such as 'Env' and 'Cmd'.
To see non-encrypted information, run 'nerdctl image inspect --mode=native --platform=PLATFORM example.com/foo:encrypted' .
`

func newImageEncryptCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "encrypt [flags] <source_ref> <target_ref>...",
		Short:             "encrypt image layers",
		Long:              imageEncryptHelp,
		Args:              cobra.MinimumNArgs(2),
		RunE:              getImgcryptAction(true),
		ValidArgsFunction: imgcryptShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	registerImgcryptFlags(cmd, true)
	return cmd
}
