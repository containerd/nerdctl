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

package statsutil

import (
	"time"

	v1 "github.com/containerd/cgroups/stats/v1"
	v2 "github.com/containerd/cgroups/v2/stats"
	"github.com/vishvananda/netlink"
)

func SetCgroupStatsFields(previousCgroupCPU, previousCgroupSystem uint64, data *v1.Metrics, links []netlink.Link) (StatsEntry, error) {

	cpuPercent := calculateCgroupCPUPercent(previousCgroupCPU, previousCgroupSystem, data)
	blkRead, blkWrite := calculateCgroupBlockIO(data)
	mem := calculateCgroupMemUsage(data)
	memLimit := float64(data.Memory.Usage.Limit)
	memPercent := calculateMemPercent(memLimit, mem)
	pidsStatsCurrent := data.Pids.Current
	netRx, netTx := calculateCgroupNetwork(links)

	return StatsEntry{
		CPUPercentage:    cpuPercent,
		Memory:           mem,
		MemoryPercentage: memPercent,
		MemoryLimit:      memLimit,
		NetworkRx:        netRx,
		NetworkTx:        netTx,
		BlockRead:        float64(blkRead),
		BlockWrite:       float64(blkWrite),
		PidsCurrent:      pidsStatsCurrent,
	}, nil

}

func SetCgroup2StatsFields(previousCgroup2CPU, previousCgroup2System uint64, metrics *v2.Metrics, links []netlink.Link) (StatsEntry, error) {

	cpuPercent := calculateCgroup2CPUPercent(previousCgroup2CPU, previousCgroup2System, metrics)
	blkRead, blkWrite := calculateCgroup2IO(metrics)
	mem := calculateCgroup2MemUsage(metrics)
	memLimit := float64(metrics.Memory.UsageLimit)
	memPercent := calculateMemPercent(memLimit, mem)
	pidsStatsCurrent := metrics.Pids.Current
	netRx, netTx := calculateCgroupNetwork(links)

	return StatsEntry{
		CPUPercentage:    cpuPercent,
		Memory:           mem,
		MemoryPercentage: memPercent,
		MemoryLimit:      memLimit,
		NetworkRx:        netRx,
		NetworkTx:        netTx,
		BlockRead:        float64(blkRead),
		BlockWrite:       float64(blkWrite),
		PidsCurrent:      pidsStatsCurrent,
	}, nil

}

func calculateCgroupCPUPercent(previousCPU, previousSystem uint64, metrics *v1.Metrics) float64 {
	var (
		cpuPercent = 0.0
		// calculate the change for the cpu usage of the container in between readings
		cpuDelta = float64(metrics.CPU.Usage.Total) - float64(previousCPU)
		// calculate the change for the entire system between readings
		systemDelta = float64(metrics.CPU.Usage.Kernel) - float64(previousSystem)
		onlineCPUs  = float64(len(metrics.CPU.Usage.PerCPU))
	)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}
	return cpuPercent
}

//PercpuUsage is not supported in CgroupV2
func calculateCgroup2CPUPercent(previousCPU, previousSystem uint64, metrics *v2.Metrics) float64 {
	var (
		cpuPercent = 0.0
		// calculate the change for the cpu usage of the container in between readings
		cpuDelta = float64(metrics.CPU.UsageUsec*1000) - float64(previousCPU)
		// calculate the change for the entire system between readings
		systemDelta = float64(metrics.CPU.SystemUsec*1000) - float64(previousSystem)
	)

	u, _ := time.ParseDuration("500ms")
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta + systemDelta) / float64(u.Nanoseconds()) * 100.0
	}
	return cpuPercent
}

func calculateCgroupMemUsage(metrics *v1.Metrics) float64 {
	if v := metrics.Memory.TotalInactiveFile; v < metrics.Memory.Usage.Usage {
		return float64(metrics.Memory.Usage.Usage - v)
	}
	return float64(metrics.Memory.Usage.Usage)
}

func calculateCgroup2MemUsage(metrics *v2.Metrics) float64 {
	if v := metrics.Memory.InactiveFile; v < metrics.Memory.Usage {
		return float64(metrics.Memory.Usage - v)
	}
	return float64(metrics.Memory.Usage)
}

func calculateCgroupBlockIO(metrics *v1.Metrics) (uint64, uint64) {
	var blkRead, blkWrite uint64
	for _, bioEntry := range metrics.Blkio.IoServiceBytesRecursive {
		if len(bioEntry.Op) == 0 {
			continue
		}
		switch bioEntry.Op[0] {
		case 'r', 'R':
			blkRead = blkRead + bioEntry.Value
		case 'w', 'W':
			blkWrite = blkWrite + bioEntry.Value
		}
	}
	return blkRead, blkWrite
}

func calculateCgroup2IO(metrics *v2.Metrics) (uint64, uint64) {
	var ioRead, ioWrite uint64

	for _, iOEntry := range metrics.Io.Usage {
		if iOEntry.Rios == 0 && iOEntry.Wios == 0 {
			continue
		}

		if iOEntry.Rios != 0 {
			ioRead = ioRead + iOEntry.Rbytes
		}

		if iOEntry.Wios != 0 {
			ioWrite = ioWrite + iOEntry.Wbytes
		}
	}

	return ioRead, ioWrite
}

func calculateCgroupNetwork(links []netlink.Link) (float64, float64) {
	var rx, tx float64

	for _, l := range links {
		stats := l.Attrs().Statistics
		if stats != nil {
			rx += float64(stats.RxBytes)
			tx += float64(stats.TxBytes)
		}
	}
	return rx, tx
}

func calculateMemPercent(limit float64, usedNo float64) float64 {
	// Limit will never be 0 unless the container is not running and we haven't
	// got any data from cgroup
	if limit != 0 {
		return usedNo / limit * 100.0
	}
	return 0
}
