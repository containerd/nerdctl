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

package hostsstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
)

// newUpdater creates an updater for hostsD (/var/lib/nerdctl/<ADDRHASH>/etchosts)
func newUpdater(id, hostsD string) *updater {
	u := &updater{
		id:            id,
		hostsD:        hostsD,
		metaByIPStr:   make(map[string]*Meta),
		nwNameByIPStr: make(map[string]string),
		metaByDir:     make(map[string]*Meta),
	}
	return u
}

// updater is the struct for updater.update()
type updater struct {
	id            string
	hostsD        string            // "/var/lib/nerdctl/<ADDRHASH>/etchosts"
	metaByIPStr   map[string]*Meta  // key: IP string
	nwNameByIPStr map[string]string // key: IP string, value: key of Meta.Networks
	metaByDir     map[string]*Meta  // key: "/var/lib/nerdctl/<ADDRHASH>/etchosts/<NS>/<ID>"
}

// update updates the hostsD tree.
// Must be called with a locker for the hostsD directory.
func (u *updater) update() error {
	// phase1: read meta.json
	if err := u.phase1(); err != nil {
		return err
	}
	// phase2: write hosts
	return u.phase2()
}

// phase1: read meta.json
func (u *updater) phase1() error {
	readMetaWF := func(path string, _ os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filepath.Base(path) != metaJSON {
			return nil
		}
		metaB, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var meta Meta
		if err := json.Unmarshal(metaB, &meta); err != nil {
			return err
		}
		u.metaByDir[filepath.Dir(path)] = &meta
		for nwName, cniRes := range meta.Networks {
			for _, ipCfg := range cniRes.IPs {
				if ip := ipCfg.Address.IP; ip != nil {
					if ip.IsLoopback() || ip.IsUnspecified() {
						continue
					}
					ipStr := ip.String()
					u.metaByIPStr[ipStr] = &meta
					u.nwNameByIPStr[ipStr] = nwName
				}
			}
		}
		return nil
	}
	return filepath.Walk(u.hostsD, readMetaWF)
}

// phase2: write hosts
func (u *updater) phase2() error {
	writeHostsWF := func(path string, _ os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filepath.Base(path) != "hosts" {
			return nil
		}
		dir := filepath.Dir(path)
		myMeta, ok := u.metaByDir[dir]
		if !ok {
			log.L.WithError(errdefs.ErrNotFound).Debugf("hostsstore metadata %q not found in %q?", metaJSON, dir)
			return nil
		}
		myNetworks := make(map[string]struct{})
		for nwName := range myMeta.Networks {
			myNetworks[nwName] = struct{}{}
		}

		// parse the hosts file, keep the original host record
		// retain custom /etc/hosts entries outside <nerdctl> </nerdctl> region
		r, err := os.Open(path)
		if err != nil {
			return err
		}
		defer r.Close()
		var buf bytes.Buffer
		if r != nil {
			if err := parseHostsButSkipMarkedRegion(&buf, r); err != nil {
				log.L.WithError(err).Warn("failed to read hosts file")
			}
		}

		buf.WriteString(fmt.Sprintf("# %s\n", MarkerBegin))
		buf.WriteString("127.0.0.1	localhost localhost.localdomain\n")
		buf.WriteString("::1		localhost localhost.localdomain\n")

		// keep extra hosts first
		for host, ip := range myMeta.ExtraHosts {
			buf.WriteString(fmt.Sprintf("%-15s %s\n", ip, host))
		}

		for ip, nwName := range u.nwNameByIPStr {
			meta := u.metaByIPStr[ip]
			if line := createLine(nwName, meta, myNetworks); len(line) != 0 {
				buf.WriteString(fmt.Sprintf("%-15s %s\n", ip, strings.Join(line, " ")))
			}
		}

		buf.WriteString(fmt.Sprintf("# %s\n", MarkerEnd))
		err = os.WriteFile(path, buf.Bytes(), 0644)
		if err != nil {
			return err
		}
		return nil
	}
	return filepath.Walk(u.hostsD, writeHostsWF)
}

// createLine returns a line string slice.
// line is like "foo foo.nw0 bar bar.nw0\n"
// for `nerdctl --name=foo --hostname=bar --network=n0`.
//
// May return an empty string slice
func createLine(thatNetwork string, meta *Meta, myNetworks map[string]struct{}) []string {
	line := []string{}
	if _, ok := myNetworks[thatNetwork]; !ok {
		// Do not add lines for other networks
		return line
	}
	baseHostnames := []string{meta.Hostname}
	if meta.Name != "" {
		baseHostnames = append(baseHostnames, meta.Name)
	}

	for _, baseHostname := range baseHostnames {
		line = append(line, baseHostname)
		if thatNetwork != netutil.DefaultNetworkName {
			// Do not add a entry like "foo.bridge"
			line = append(line, baseHostname+"."+thatNetwork)
		}
	}
	return line
}
