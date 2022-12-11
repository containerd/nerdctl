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

package image

import (
	"github.com/spf13/cobra"
)

const imageDecryptHelp = `Decrypt an image locally.

Use '--key' to specify the private keys.
Private keys in PEM format may be encrypted and the password may be passed
along in any of the following formats:
- <filename>:<password>
- <filename>:pass=<password>
- <filename>:fd=<file descriptor> (not available for rootless mode)
- <filename>:filename=<password file>

Use '--platform' to define the platforms to decrypt. Defaults to the host platform.
When '--all-platforms' is given all images in a manifest list must be available.
Unspecified platforms are omitted from the output image.

Example (encrypt):
  openssl genrsa -out mykey.pem
  openssl rsa -in mykey.pem -pubout -out mypubkey.pem
  nerdctl image encrypt --recipient=jwe:mypubkey.pem --platform=linux/amd64,linux/arm64 foo example.com/foo:encrypted
  nerdctl push example.com/foo:encrypted

Example (decrypt):
  nerdctl pull --unpack=false example.com/foo:encrypted
  nerdctl image decrypt --key=mykey.pem example.com/foo:encrypted foo:decrypted
`

func NewImageDecryptCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "decrypt [flags] <source_ref> <target_ref>...",
		Short:             "decrypt an image",
		Long:              imageDecryptHelp,
		Args:              cobra.MinimumNArgs(2),
		RunE:              getImgcryptAction(false),
		ValidArgsFunction: imgcryptShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	registerImgcryptFlags(cmd, false)
	return cmd
}
