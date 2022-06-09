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
	"errors"
	"fmt"
	"net"
	"strings"

	v1 "github.com/containerd/cgroups/stats/v1"
	v2 "github.com/containerd/cgroups/v2/stats"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/statsutil"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

func renderStatsEntry(previousStats map[string]uint64, anydata interface{}, pid int, interfaces []native.NetInterface) (statsutil.StatsEntry, error) {

	var (
		data       *v1.Metrics
		data2      *v2.Metrics
		statsEntry statsutil.StatsEntry
	)

	switch v := anydata.(type) {
	case *v1.Metrics:
		data = v
	case *v2.Metrics:
		data2 = v
	default:
		return statsutil.StatsEntry{}, errors.New("cannot convert metric data to cgroups.Metrics.")
	}

	var (
		err      error
		nlinks   []netlink.Link
		nlink    netlink.Link
		nlHandle *netlink.Handle
		ns       netns.NsHandle
	)

	ns, err = netns.GetFromPid(pid)
	if err != nil {
		return statsutil.StatsEntry{}, fmt.Errorf("failed to retrieve the statistics in netns %s: %v", ns, err)
	}

	nlHandle, err = netlink.NewHandleAt(ns)
	if err != nil {
		return statsutil.StatsEntry{}, fmt.Errorf("failed to retrieve the statistics in netns %s: %v", ns, err)
	}

	for _, v := range interfaces {
		nlink, err = nlHandle.LinkByIndex(v.Index)
		if err != nil {
			return statsutil.StatsEntry{}, fmt.Errorf("failed to retrieve the statistics for %s in netns %s: %v", v.Name, ns, err)
		}
		//exclude inactive interface
		if nlink.Attrs().Flags&net.FlagUp != 0 {

			//exclude loopback interface
			if nlink.Attrs().Flags&net.FlagLoopback != 0 || strings.HasPrefix(nlink.Attrs().Name, "lo") {
				continue
			}
			nlinks = append(nlinks, nlink)
		}
	}

	if data != nil {
		statsEntry, err = statsutil.SetCgroupStatsFields(previousStats["CgroupCPU"], previousStats["CgroupSystem"], data, nlinks)
		previousStats["CgroupCPU"] = data.CPU.Usage.Total
		previousStats["CgroupSystem"] = data.CPU.Usage.Kernel
		if err != nil {
			return statsutil.StatsEntry{}, err
		}
	} else if data2 != nil {
		statsEntry, err = statsutil.SetCgroup2StatsFields(previousStats["Cgroup2CPU"], previousStats["Cgroup2System"], data2, nlinks)
		previousStats["Cgroup2CPU"] = data2.CPU.UsageUsec * 1000
		previousStats["Cgroup2System"] = data2.CPU.SystemUsec * 1000
		if err != nil {
			return statsutil.StatsEntry{}, err
		}
	}

	return statsEntry, nil
}
