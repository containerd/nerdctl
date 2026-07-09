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
	"time"
)

// IPFSRegistryServeOptions specifies options for `nerdctl ipfs registry serve`.
type IPFSRegistryServeOptions struct {
	// ListenRegistry address to listen
	ListenRegistry string
	// IPFSAddress multiaddr of IPFS API (default is pulled from $IPFS_PATH/api file. If $IPFS_PATH env var is not present, it defaults to ~/.ipfs)
	IPFSAddress string
	// ReadRetryNum times to retry query on IPFS. Zero or lower means no retry.
	ReadRetryNum int
	// ReadTimeout timeout duration of a read request to IPFS. Zero means no timeout.
	ReadTimeout time.Duration
}
