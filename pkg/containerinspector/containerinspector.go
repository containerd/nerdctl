/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

	"github.com/AkihiroSuda/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/containerd"
	"github.com/containerd/typeurl"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
)

func Inspect(ctx context.Context, container containerd.Container) (*native.Container, error) {
	info, err := container.Info(ctx)
	if err != nil {
		return nil, err
	}
	n := &native.Container{
		Container: info,
	}
	id := container.ID()

	n.Spec, err = typeurl.UnmarshalAny(info.Spec)
	if err != nil {
		logrus.WithError(err).WithField("id", id).Warnf("failed to inspect Spec")
		return n, nil
	}
	task, err := container.Task(ctx, nil)
	if err != nil {
		logrus.WithError(err).WithField("id", id).Warnf("failed to inspect Task")
		return n, nil
	}
	n.Process = &native.Process{
		Pid: int(task.Pid()),
	}
	st, err := task.Status(ctx)
	if err != nil {
		logrus.WithError(err).WithField("id", id).Warnf("failed to inspect Status")
		return n, nil
	}
	n.Process.Status = st
	netNSPath := fmt.Sprintf("/proc/%d/ns/net", n.Process.Pid)
	netNS, err := inspectNetNS(ctx, netNSPath)
	if err != nil {
		logrus.WithError(err).WithField("id", id).Warnf("failed to inspect NetNS")
		return n, nil
	}
	n.Process.NetNS = netNS
	return n, nil
}

func inspectNetNS(ctx context.Context, nsPath string) (*native.NetNS, error) {
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
// Zero means no priary interface was detected.
func determinePrimaryInterface(interfaces []native.NetInterface) int {
	for _, f := range interfaces {
		if f.Interface.Flags&net.FlagLoopback == 0 && f.Interface.Flags&net.FlagUp != 0 && !strings.HasPrefix(f.Name, "lo") {
			return f.Index
		}
	}
	return 0
}
