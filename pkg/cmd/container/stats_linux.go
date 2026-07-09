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

package container

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	v1 "github.com/containerd/cgroups/v3/cgroup1/stats"
	v2 "github.com/containerd/cgroups/v3/cgroup2/stats"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/statsutil"
)

const (
	// The value comes from `C.sysconf(C._SC_CLK_TCK)`, and
	// on Linux it's a constant which is safe to be hard coded,
	// so we can avoid using cgo here. For details, see:
	// https://github.com/containerd/cgroups/pull/12
	clockTicksPerSecond  = 100
	nanoSecondsPerSecond = 1e9
)

//nolint:nakedret
func setContainerStatsAndRenderStatsEntry(previousStats *statsutil.ContainerStats, firstSet bool, anydata interface{}, pid int, interfaces []native.NetInterface, systemInfo statsutil.SystemInfo) (statsEntry statsutil.StatsEntry, err error) {

	var (
		data  *v1.Metrics
		data2 *v2.Metrics
	)

	switch v := anydata.(type) {
	case *v1.Metrics:
		data = v
	case *v2.Metrics:
		data2 = v
	default:
		err = errors.New("cannot convert metric data to cgroups.Metrics")
		return
	}

	var nlinks []netlink.Link

	if !firstSet {
		var (
			nlink    netlink.Link
			nlHandle *netlink.Handle
			ns       netns.NsHandle
		)

		ns, err = netns.GetFromPid(pid)
		if err != nil {
			err = fmt.Errorf("failed to retrieve the statistics in netns %s: %v", ns, err)
			return
		}
		defer func() {
			err = ns.Close()
		}()

		nlHandle, err = netlink.NewHandleAt(ns)
		if err != nil {
			err = fmt.Errorf("failed to retrieve the statistics in netns %s: %v", ns, err)
			return
		}
		defer nlHandle.Close()

		for _, v := range interfaces {
			nlink, err = nlHandle.LinkByIndex(v.Index)
			if err != nil {
				err = fmt.Errorf("failed to retrieve the statistics for %s in netns %s: %v", v.Name, ns, err)
				return
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
	}

	if data != nil {
		if !firstSet {
			statsEntry, err = statsutil.SetCgroupStatsFields(previousStats, data, nlinks, systemInfo)
		}
		previousStats.CgroupCPU = data.CPU.Usage.Total
		previousStats.CgroupSystem = systemInfo.SystemUsage
		if err != nil {
			return
		}
	} else if data2 != nil {
		if !firstSet {
			statsEntry, err = statsutil.SetCgroup2StatsFields(previousStats, data2, nlinks)
		}
		previousStats.Cgroup2CPU = data2.CPU.UsageUsec * 1000
		previousStats.Cgroup2System = data2.CPU.SystemUsec * 1000
		if err != nil {
			return
		}
	}
	previousStats.Time = time.Now()

	return
}

// getSystemCPUUsage reads the system's CPU usage from /proc/stat and returns
// the total CPU usage in nanoseconds and the number of CPUs.
func getSystemCPUUsage() (cpuUsage uint64, cpuNum uint32, _ error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	return readSystemCPUUsage(f)
}

// readSystemCPUUsage parses CPU usage information from a reader providing
// /proc/stat format data. It returns the total CPU usage in nanoseconds
// and the number of CPUs. More:
// https://github.com/moby/moby/blob/26db31fdab628a2345ed8f179e575099384166a9/daemon/stats_unix.go#L327-L368
func readSystemCPUUsage(r io.Reader) (cpuUsage uint64, cpuNum uint32, _ error) {
	rdr := bufio.NewReaderSize(r, 1024)

	for {
		data, isPartial, err := rdr.ReadLine()

		if err != nil {
			return 0, 0, fmt.Errorf("error scanning /proc/stat file: %w", err)
		}
		// Assume all cpu* records are at the start of the file, like glibc:
		// https://github.com/bminor/glibc/blob/5d00c201b9a2da768a79ea8d5311f257871c0b43/sysdeps/unix/sysv/linux/getsysstats.c#L108-L135
		if isPartial || len(data) < 4 {
			break
		}
		line := string(data)
		if line[:3] != "cpu" {
			break
		}
		if line[3] == ' ' {
			parts := strings.Fields(line)
			if len(parts) < 8 {
				return 0, 0, fmt.Errorf("invalid number of cpu fields")
			}
			var totalClockTicks uint64
			for _, i := range parts[1:8] {
				v, err := strconv.ParseUint(i, 10, 64)
				if err != nil {
					return 0, 0, fmt.Errorf("unable to convert value %s to int: %w", i, err)
				}
				totalClockTicks += v
			}
			cpuUsage = (totalClockTicks * nanoSecondsPerSecond) / clockTicksPerSecond
		}
		if '0' <= line[3] && line[3] <= '9' {
			cpuNum++
		}
	}
	return cpuUsage, cpuNum, nil
}
