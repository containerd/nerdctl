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

package containerinspector

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/containerd/nerdctl/pkg/inspecttypes/native"

	"github.com/containernetworking/plugins/pkg/ns"
)

func inspectNetNS(ctx context.Context, pid int) (*native.NetNS, error) {
	nsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
	res := &native.NetNS{}
	fn := func(_ ns.NetNS) error {
		intf, err := net.Interfaces()
		if err != nil {
			return err
		}
		res.Interfaces = make([]native.NetInterface, len(intf))
		for i, f := range intf {
			x := native.NetInterface{
				Interface: f,
			}
			if f.HardwareAddr != nil {
				x.HardwareAddr = f.HardwareAddr.String()
			}
			if x.Interface.Flags.String() != "0" {
				x.Flags = strings.Split(x.Interface.Flags.String(), "|")
			}
			if addrs, err := x.Interface.Addrs(); err == nil {
				x.Addrs = make([]string, len(addrs))
				for j, a := range addrs {
					x.Addrs[j] = a.String()
				}
			}
			res.Interfaces[i] = x
		}
		res.PrimaryInterface = determinePrimaryInterface(res.Interfaces)
		return nil
	}
	if err := ns.WithNetNSPath(nsPath, fn); err != nil {
		return nil, err
	}
	return res, nil
}

// determinePrimaryInterface returns a net.Interface.Index value, not a slice index.
// Zero means no primary interface was detected.
func determinePrimaryInterface(interfaces []native.NetInterface) int {
	for _, f := range interfaces {
		if f.Interface.Flags&net.FlagLoopback == 0 && f.Interface.Flags&net.FlagUp != 0 && !strings.HasPrefix(f.Name, "lo") {
			return f.Index
		}
	}
	return 0
}
