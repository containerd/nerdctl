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
	"fmt"
	"sync"
	"time"

	units "github.com/docker/go-units"
)

// StatsEntry represents the statistics data collected from a container
type StatsEntry struct {
	Container        string
	Name             string
	ID               string
	CPUPercentage    float64
	Memory           float64
	MemoryLimit      float64
	MemoryPercentage float64
	NetworkRx        float64
	NetworkTx        float64
	BlockRead        float64
	BlockWrite       float64
	PidsCurrent      uint64
	IsInvalid        bool
}

// FormattedStatsEntry represents a formatted StatsEntry
type FormattedStatsEntry struct {
	Name     string
	ID       string
	CPUPerc  string
	MemUsage string
	MemPerc  string
	NetIO    string
	BlockIO  string
	PIDs     string
}

// Stats represents an entity to store containers statistics synchronously
type Stats struct {
	mutex sync.RWMutex
	StatsEntry
	err error
}

// ContainerStats represents the runtime container stats
type ContainerStats struct {
	Time                        time.Time
	CgroupCPU, Cgroup2CPU       uint64
	CgroupSystem, Cgroup2System uint64
}

//NewStats is from https://github.com/docker/cli/blob/3fb4fb83dfb5db0c0753a8316f21aea54dab32c5/cli/command/container/formatter_stats.go#L113-L116
func NewStats(container string) *Stats {
	return &Stats{StatsEntry: StatsEntry{Container: container}}
}

//SetStatistics is from https://github.com/docker/cli/blob/3fb4fb83dfb5db0c0753a8316f21aea54dab32c5/cli/command/container/formatter_stats.go#L87-L93
func (cs *Stats) SetStatistics(s StatsEntry) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	s.Container = cs.Container
	cs.StatsEntry = s
}

//GetStatistics is from https://github.com/docker/cli/blob/3fb4fb83dfb5db0c0753a8316f21aea54dab32c5/cli/command/container/formatter_stats.go#L95-L100
func (cs *Stats) GetStatistics() StatsEntry {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	return cs.StatsEntry
}

//GetError is from https://github.com/docker/cli/blob/3fb4fb83dfb5db0c0753a8316f21aea54dab32c5/cli/command/container/formatter_stats.go#L51-L57
func (cs *Stats) GetError() error {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	return cs.err
}

//SetErrorAndReset is from https://github.com/docker/cli/blob/3fb4fb83dfb5db0c0753a8316f21aea54dab32c5/cli/command/container/formatter_stats.go#L59-L75
func (cs *Stats) SetErrorAndReset(err error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	cs.CPUPercentage = 0
	cs.Memory = 0
	cs.MemoryPercentage = 0
	cs.MemoryLimit = 0
	cs.NetworkRx = 0
	cs.NetworkTx = 0
	cs.BlockRead = 0
	cs.BlockWrite = 0
	cs.PidsCurrent = 0
	cs.err = err
	cs.IsInvalid = true
}

//SetError is from https://github.com/docker/cli/blob/3fb4fb83dfb5db0c0753a8316f21aea54dab32c5/cli/command/container/formatter_stats.go#L77-L85
func (cs *Stats) SetError(err error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	cs.err = err
	if err != nil {
		cs.IsInvalid = true
	}
}

func calculateMemPercent(limit float64, usedNo float64) float64 {
	// Limit will never be 0 unless the container is not running and we haven't
	// got any data from cgroup
	if limit != 0 {
		return usedNo / limit * 100.0
	}
	return 0
}

// Rendering a FormattedStatsEntry from StatsEntry
func RenderEntry(in *StatsEntry, noTrunc bool) FormattedStatsEntry {
	return FormattedStatsEntry{
		Name:     in.EntryName(),
		ID:       in.EntryID(noTrunc),
		CPUPerc:  in.CPUPerc(),
		MemUsage: in.MemUsage(),
		MemPerc:  in.MemPerc(),
		NetIO:    in.NetIO(),
		BlockIO:  in.BlockIO(),
		PIDs:     in.PIDs(),
	}
}

/*
a set of functions to format container stats
*/
func (s *StatsEntry) EntryName() string {
	if len(s.Name) > 1 {
		if len(s.Name) > 12 {
			return s.Name[:12]
		}
		return s.Name
	}
	return "--"
}

func (s *StatsEntry) EntryID(noTrunc bool) string {
	if !noTrunc {
		if len(s.ID) > 12 {
			return s.ID[:12]
		}
	}
	return s.ID
}

func (s *StatsEntry) CPUPerc() string {
	if s.IsInvalid {
		return "--"
	}
	return fmt.Sprintf("%.2f%%", s.CPUPercentage)
}

func (s *StatsEntry) MemUsage() string {
	if s.IsInvalid {
		return "-- / --"
	}
	return fmt.Sprintf("%s / %s", units.BytesSize(s.Memory), units.BytesSize(s.MemoryLimit))
}

func (s *StatsEntry) MemPerc() string {
	if s.IsInvalid {
		return "--"
	}
	return fmt.Sprintf("%.2f%%", s.MemoryPercentage)
}

func (s *StatsEntry) NetIO() string {
	if s.IsInvalid {
		return "--"
	}
	return fmt.Sprintf("%s / %s", units.HumanSizeWithPrecision(s.NetworkRx, 3), units.HumanSizeWithPrecision(s.NetworkTx, 3))
}

func (s *StatsEntry) BlockIO() string {
	if s.IsInvalid {
		return "--"
	}
	return fmt.Sprintf("%s / %s", units.HumanSizeWithPrecision(s.BlockRead, 3), units.HumanSizeWithPrecision(s.BlockWrite, 3))
}

func (s *StatsEntry) PIDs() string {
	if s.IsInvalid {
		return "--"
	}
	return fmt.Sprintf("%d", s.PidsCurrent)
}
