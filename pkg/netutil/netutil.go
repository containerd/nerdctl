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

package netutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containernetworking/cni/libcni"

	"github.com/sirupsen/logrus"
)

type NetworkConfigList struct {
	*libcni.NetworkConfigList
	NerdctlID     *int
	NerdctlLabels *map[string]string
	File          string
}

type CNIEnv struct {
	Path        string
	NetconfPath string
}

func DefaultConfigList(e *CNIEnv) (*NetworkConfigList, error) {
	return GenerateConfigList(e, nil, DefaultID, DefaultNetworkName, DefaultCIDR)
}

type ConfigListTemplateOpts struct {
	ID           int
	Name         string // e.g. "nerdctl"
	Labels       string // e.g. `{"version":"1.1.0"}`
	Subnet       string // e.g. "10.4.0.0/24"
	Gateway      string // e.g. "10.4.0.1"
	ExtraPlugins string // e.g. `,{"type":"isolation"}`
}

// GenerateConfigList creates NetworkConfigList.
// GenerateConfigList does not fill "File" field.
//
// TODO: enable CNI isolation plugin
func GenerateConfigList(e *CNIEnv, labels []string, id int, name, cidr string) (*NetworkConfigList, error) {
	if e == nil || id < 0 || name == "" || cidr == "" {
		return nil, errdefs.ErrInvalidArgument
	}
	for _, f := range basicPlugins {
		p := filepath.Join(e.Path, f)
		if _, err := exec.LookPath(p); err != nil {
			return nil, fmt.Errorf("needs CNI plugin %q to be installed in CNI_PATH (%q), see https://github.com/containernetworking/plugins/releases: %w", f, e.Path, err)
		}
	}
	var extraPlugins string
	if _, err := exec.LookPath(filepath.Join(e.Path, "isolation")); err == nil {
		logrus.Debug("found CNI isolation plugin")
		extraPlugins = ",\n    {\n      \"type\":\"isolation\"\n    }"
	} else if name != DefaultNetworkName {
		// the warning is suppressed for DefaultNetworkName
		logrus.Warnf("To isolate bridge networks, CNI plugin \"isolation\" needs to be installed in CNI_PATH (%q), see https://github.com/AkihiroSuda/cni-isolation",
			e.Path)
	}

	subnetIP, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CIDR %q", cidr)
	}
	if !subnet.IP.Equal(subnetIP) {
		return nil, fmt.Errorf("unexpected CIDR %q, maybe you meant %q?", cidr, subnet.String())
	}
	gateway := make(net.IP, len(subnet.IP))
	copy(gateway, subnet.IP)
	gateway[3] += 1

	labelsMap := strutil.ConvertKVStringsToMap(labels)
	labelsJson, err := json.Marshal(labelsMap)
	if err != nil {
		return nil, err
	}

	opts := &ConfigListTemplateOpts{
		ID:           id,
		Name:         name,
		Labels:       string(labelsJson),
		Subnet:       subnet.String(),
		Gateway:      gateway.String(),
		ExtraPlugins: extraPlugins,
	}

	tmpl, err := template.New("").Parse(ConfigListTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, opts); err != nil {
		return nil, err
	}

	l, err := libcni.ConfListFromBytes(buf.Bytes())
	if err != nil {
		return nil, err
	}
	return &NetworkConfigList{
		NetworkConfigList: l,
		NerdctlID:         &id,
		NerdctlLabels:     &labelsMap,
		File:              "",
	}, nil
}

// ConfigLists loads config from dir if dir exists.
// The result also contains DefaultConfigList
func ConfigLists(e *CNIEnv) ([]*NetworkConfigList, error) {
	def, err := DefaultConfigList(e)
	if err != nil {
		return nil, err
	}
	l := []*NetworkConfigList{def}
	if _, err := os.Stat(e.NetconfPath); err != nil {
		if os.IsNotExist(err) {
			return l, nil
		}
		return nil, err
	}
	fileNames, err := libcni.ConfFiles(e.NetconfPath, []string{".conf", ".conflist", ".json"})
	if err != nil {
		return nil, err
	}
	sort.Strings(fileNames)
	for _, fileName := range fileNames {
		var lcl *libcni.NetworkConfigList
		if strings.HasSuffix(fileName, ".conflist") {
			lcl, err = libcni.ConfListFromFile(fileName)
			if err != nil {
				return nil, err
			}
		} else {
			lc, err := libcni.ConfFromFile(fileName)
			if err != nil {
				return nil, err
			}
			lcl, err = libcni.ConfListFromConf(lc)
			if err != nil {
				return nil, err
			}
		}
		l = append(l, &NetworkConfigList{
			NetworkConfigList: lcl,
			NerdctlID:         NerdctlID(lcl.Bytes),
			NerdctlLabels:     NerdctlLabels(lcl.Bytes),
			File:              fileName,
		})
	}
	return l, nil
}

// AcquireNextID suggests the next ID.
func AcquireNextID(l []*NetworkConfigList) (int, error) {
	maxID := DefaultID
	for _, f := range l {
		if f.NerdctlID != nil && *f.NerdctlID > maxID {
			maxID = *f.NerdctlID
		}
	}
	nextID := maxID + 1
	return nextID, nil
}

func NerdctlID(b []byte) *int {
	type nerdctlConfigList struct {
		NerdctlID *int `json:"nerdctlID,omitempty"`
	}
	var ncl nerdctlConfigList
	if err := json.Unmarshal(b, &ncl); err != nil {
		// The network is managed outside nerdctl
		return nil
	}
	return ncl.NerdctlID
}

func NerdctlLabels(b []byte) *map[string]string {
	type nerdctlConfigList struct {
		NerdctlLabels *map[string]string `json:"nerdctlLabels,omitempty"`
	}
	var ncl nerdctlConfigList
	if err := json.Unmarshal(b, &ncl); err != nil {
		return nil
	}
	return ncl.NerdctlLabels
}
