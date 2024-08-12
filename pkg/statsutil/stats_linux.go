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

	"github.com/vishvananda/netlink"

	v1 "github.com/containerd/cgroups/v3/cgroup1/stats"
	v2 "github.com/containerd/cgroups/v3/cgroup2/stats"
)

func SetCgroupStatsFields(previousStats *ContainerStats, data *v1.Metrics, links []netlink.Link) (StatsEntry, error) {

	cpuPercent := calculateCgroupCPUPercent(previousStats, data)
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

func SetCgroup2StatsFields(previousStats *ContainerStats, metrics *v2.Metrics, links []netlink.Link) (StatsEntry, error) {

	cpuPercent := calculateCgroup2CPUPercent(previousStats, metrics)
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

func calculateCgroupCPUPercent(previousStats *ContainerStats, metrics *v1.Metrics) float64 {
	var (
		cpuPercent = 0.0
		// calculate the change for the cpu usage of the container in between readings
		cpuDelta = float64(metrics.CPU.Usage.Total) - float64(previousStats.CgroupCPU)
		// calculate the change for the entire system between readings
		systemDelta = float64(metrics.CPU.Usage.Kernel) - float64(previousStats.CgroupSystem)
		onlineCPUs  = float64(len(metrics.CPU.Usage.PerCPU))
	)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}
	return cpuPercent
}

// PercpuUsage is not supported in CgroupV2
func calculateCgroup2CPUPercent(previousStats *ContainerStats, metrics *v2.Metrics) float64 {
	var (
		cpuPercent = 0.0
		// calculate the change for the cpu usage of the container in between readings
		cpuDelta = float64(metrics.CPU.UsageUsec*1000) - float64(previousStats.Cgroup2CPU)
		// calculate the change for the entire system between readings
		_ = float64(metrics.CPU.SystemUsec*1000) - float64(previousStats.Cgroup2System)
		// time duration
		timeDelta = time.Since(previousStats.Time)
	)
	if cpuDelta > 0.0 {
		cpuPercent = cpuDelta / float64(timeDelta.Nanoseconds()) * 100.0
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
