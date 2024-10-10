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

package hoststoml

import (
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pelletier/go-toml/v2"
)

type hostsTomlHost struct {
	CA         string     `toml:"ca,omitempty"`
	SkipVerify bool       `toml:"skip_verify,omitempty"`
	Client     [][]string `toml:"client,omitempty"`
}

// See https://github.com/containerd/containerd/blob/main/docs/hosts.md
type HostsToml struct {
	CA         string                    `toml:"ca,omitempty"`
	SkipVerify bool                      `toml:"skip_verify,omitempty"`
	Client     [][]string                `toml:"client,omitempty"`
	Headers    map[string]string         `toml:"header,omitempty"`
	Server     string                    `toml:"server,omitempty"`
	Endpoints  map[string]*hostsTomlHost `toml:"host,omitempty"`
}

func (ht *HostsToml) Save(dir string, hostIP string, port int) error {
	var err error
	var r *os.File

	hostSubDir := hostIP
	if port != 0 {
		hostSubDir = net.JoinHostPort(hostIP, strconv.Itoa(port))
	}

	hostsSubDir := filepath.Join(dir, hostSubDir)
	err = os.MkdirAll(hostsSubDir, 0700)
	if err != nil {
		return err
	}

	if r, err = os.Create(filepath.Join(dir, hostSubDir, "hosts.toml")); err == nil {
		defer r.Close()
		enc := toml.NewEncoder(r)
		enc.SetIndentTables(true)
		err = enc.Encode(ht)
	}

	return err
}
