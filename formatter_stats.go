package main

import (
	"fmt"
	"sync"

	"github.com/docker/cli/cli/command/formatter"
	"github.com/docker/docker/pkg/stringid"
	units "github.com/docker/go-units"
)

const (
	winOSType                  = "windows"
	defaultStatsTableFormat    = "table {{.ID}}\t{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.NetIO}}\t{{.BlockIO}}\t{{.PIDs}}"
	winDefaultStatsTableFormat = "table {{.ID}}\t{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}"

	containerHeader = "CONTAINER"
	cpuPercHeader   = "CPU %"
	netIOHeader     = "NET I/O"
	blockIOHeader   = "BLOCK I/O"
	memPercHeader   = "MEM %"             // Used only on Linux
	winMemUseHeader = "PRIV WORKING SET"  // Used only on Windows
	memUseHeader    = "MEM USAGE / LIMIT" // Used only on Linux
	pidsHeader      = "PIDS"              // Used only on Linux
)

// StatsEntry represents represents the statistics data collected from a container
type StatsEntry struct {
	Container        string
	Name             string
	ID               string
	CPUPercentage    float64
	Memory           float64 // On Windows this is the private working set
	MemoryLimit      float64 // Not used on Windows
	MemoryPercentage float64 // Not used on Windows
	NetworkRx        float64
	NetworkTx        float64
	BlockRead        float64
	BlockWrite       float64
	PidsCurrent      uint64 // Not used on Windows
	IsInvalid        bool
}

// Stats represents an entity to store containers statistics synchronously
type Stats struct {
	mutex sync.Mutex
	StatsEntry
	err error
}

// GetError returns the container statistics error.
// This is used to determine whether the statistics are valid or not
func (cs *Stats) GetError() error {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	return cs.err
}

// SetErrorAndReset zeroes all the container statistics and store the error.
// It is used when receiving time out error during statistics collecting to reduce lock overhead
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

// SetError sets container statistics error
func (cs *Stats) SetError(err error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	cs.err = err
	if err != nil {
		cs.IsInvalid = true
	}
}

// SetStatistics set the container statistics
func (cs *Stats) SetStatistics(s StatsEntry) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	s.Container = cs.Container
	cs.StatsEntry = s
}

// GetStatistics returns container statistics with other meta data such as the container name
func (cs *Stats) GetStatistics() StatsEntry {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	return cs.StatsEntry
}

// NewStatsFormat returns a format for rendering an CStatsContext
func NewStatsFormat(source, osType string) formatter.Format {
	if source == formatter.TableFormatKey {
		if osType == winOSType {
			return winDefaultStatsTableFormat
		}
		return defaultStatsTableFormat
	}
	return formatter.Format(source)
}

// NewStats returns a new Stats entity and sets in it the given name
func NewStats(container string) *Stats {
	return &Stats{StatsEntry: StatsEntry{Container: container}}
}

// statsFormatWrite renders the context for a list of containers statistics
func statsFormatWrite(ctx formatter.Context, Stats []StatsEntry, osType string, trunc bool) error {
	render := func(format func(subContext formatter.SubContext) error) error {
		for _, cstats := range Stats {
			statsCtx := &statsContext{
				s:     cstats,
				os:    osType,
				trunc: trunc,
			}
			if err := format(statsCtx); err != nil {
				return err
			}
		}
		return nil
	}
	memUsage := memUseHeader
	if osType == winOSType {
		memUsage = winMemUseHeader
	}
	statsCtx := statsContext{}
	statsCtx.Header = formatter.SubHeaderContext{
		"Container": containerHeader,
		"Name":      formatter.NameHeader,
		"ID":        formatter.ContainerIDHeader,
		"CPUPerc":   cpuPercHeader,
		"MemUsage":  memUsage,
		"MemPerc":   memPercHeader,
		"NetIO":     netIOHeader,
		"BlockIO":   blockIOHeader,
		"PIDs":      pidsHeader,
	}
	statsCtx.os = osType
	return ctx.Write(&statsCtx, render)
}

type statsContext struct {
	formatter.HeaderContext
	s     StatsEntry
	os    string
	trunc bool
}

func (c *statsContext) MarshalJSON() ([]byte, error) {
	return formatter.MarshalJSON(c)
}

func (c *statsContext) Container() string {
	return c.s.Container
}

func (c *statsContext) Name() string {
	if len(c.s.Name) > 1 {
		return c.s.Name[1:]
	}
	return "--"
}

func (c *statsContext) ID() string {
	if c.trunc {
		return stringid.TruncateID(c.s.ID)
	}
	return c.s.ID
}

func (c *statsContext) CPUPerc() string {
	if c.s.IsInvalid {
		return fmt.Sprintf("--")
	}
	return fmt.Sprintf("%.2f%%", c.s.CPUPercentage)
}

func (c *statsContext) MemUsage() string {
	if c.s.IsInvalid {
		return fmt.Sprintf("-- / --")
	}
	if c.os == winOSType {
		return units.BytesSize(c.s.Memory)
	}
	return fmt.Sprintf("%s / %s", units.BytesSize(c.s.Memory), units.BytesSize(c.s.MemoryLimit))
}

func (c *statsContext) MemPerc() string {
	if c.s.IsInvalid || c.os == winOSType {
		return fmt.Sprintf("--")
	}
	return fmt.Sprintf("%.2f%%", c.s.MemoryPercentage)
}

func (c *statsContext) NetIO() string {
	if c.s.IsInvalid {
		return fmt.Sprintf("--")
	}
	return fmt.Sprintf("%s / %s", units.HumanSizeWithPrecision(c.s.NetworkRx, 3), units.HumanSizeWithPrecision(c.s.NetworkTx, 3))
}

func (c *statsContext) BlockIO() string {
	if c.s.IsInvalid {
		return fmt.Sprintf("--")
	}
	return fmt.Sprintf("%s / %s", units.HumanSizeWithPrecision(c.s.BlockRead, 3), units.HumanSizeWithPrecision(c.s.BlockWrite, 3))
}

func (c *statsContext) PIDs() string {
	if c.s.IsInvalid || c.os == winOSType {
		return fmt.Sprintf("--")
	}
	return fmt.Sprintf("%d", c.s.PidsCurrent)
}
