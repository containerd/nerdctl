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

package ipfs

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/ipfs"
)

func RegistryServe(options types.IPFSRegistryServeOptions) error {
	var ipfsPath string
	if options.IPFSAddress != "" {
		dir, err := os.MkdirTemp("", "apidirtmp")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		if err := os.WriteFile(filepath.Join(dir, "api"), []byte(options.IPFSAddress), 0600); err != nil {
			return err
		}
		ipfsPath = dir
	}
	h, err := ipfs.NewRegistry(ipfs.RegistryOptions{
		IpfsPath:     ipfsPath,
		ReadRetryNum: options.ReadRetryNum,
		ReadTimeout:  options.ReadTimeout,
	})
	if err != nil {
		return err
	}
	log.L.Infof("serving on %v", options.ListenRegistry)
	http.Handle("/", h)
	return http.ListenAndServe(options.ListenRegistry, nil)
}
