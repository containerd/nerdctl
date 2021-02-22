/*
   Copyright (C) nerdctl authors.

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
	//"encoding/json"
	"errors"
	"sync"
	"fmt"
	//"os"
   	//"github.com/docker/docker/api/types"
	//wstats "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	//v1 "github.com/containerd/cgroups/stats/v1"
	v2 "github.com/containerd/cgroups/v2/stats"
	//"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/typeurl"
	"github.com/urfave/cli/v2"
	"github.com/sirupsen/logrus"
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

type stats struct {
	mu sync.Mutex
	cs []*Stats
}

type statsOptions struct {
	all        bool
	noStream   bool
	noTrunc    bool
	format     string
	containers []string
}

var optionsStats = new (statsOptions)

var statsCommand = &cli.Command{
	Name:      "stats",
	Usage:     "Display a live stream of container(s) resource usage statistics",
	ArgsUsage: "[OPTIONS] [CONTAINER...]",
	/*Flags: []cli.Flag{
		cli.StringFlag{
			Name:  formatFlag,
			Usage: `"table" or "json"`,
			Value: formatTable,
		},
	},*/
	Action: func(clicontext *cli.Context) error {
	    optionsStats.containers = clicontext.Args().Slice()
	    _ = len(optionsStats.containers) == 0

	    /*ctx := clicontext.Background()

        // Get the daemonOSType if not set already
        if daemonOSType == "" {
            svctx := clicontext.Background()
            sv, err := dockerCli.Client().ServerVersion(svctx)
            if err != nil {
                return err
            }
            daemonOSType = sv.Os
        }*/

        // waitFirst is a WaitGroup to wait first stat data's reach for each container
        waitFirst := &sync.WaitGroup{}
	    cStats := stats{}

		// Artificially send creation events for the containers we were asked to
		// monitor (same code path than we use when monitoring all containers).
		for _, name := range optionsStats.containers {
			s := NewStats(name)
            			if cStats.add(s) {
            				waitFirst.Add(1)
            				go collect(clicontext, s, waitFirst)
            			}
		}

		waitFirst.Wait()

	    return nil
	},
}

func collect(clicontext *cli.Context, s *Stats, waitFirst *sync.WaitGroup) error {
	logrus.Debugf("collecting stats for %s", s.Container)
	var (
		getFirst       bool
		previousCPU    uint64
		previousSystem uint64
	)

	defer func() {
		// if error happens and we get nothing of stats, release wait group whatever
		if !getFirst {
			getFirst = true
			waitFirst.Done()
		}
	}()

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()
	container, err := client.LoadContainer(ctx, clicontext.Args().First())
	if err != nil {
		return err
	}
	task, err := container.Task(ctx, nil)
	if err != nil {
		return err
	}
	metric, err := task.Metrics(ctx)
	if err != nil {
		return err
	}
	anydata, err := typeurl.UnmarshalAny(metric.Data)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	var (
		//data         *v1.Metrics
		data2        *v2.Metrics
		//windowsStats *wstats.Statistics
	)
	switch v := anydata.(type) {
	//case *v1.Metrics:
		//data = v
	case *v2.Metrics:
		data2 = v
	//case *wstats.Statistics:
		//windowsStats = v
	default:
		return errors.New("cannot convert metric data to cgroups.Metrics or windows.Statistics")
	}


			var (
				//memPercent, cpuPercent float64
				//blkRead, blkWrite      uint64 // Only used on Linux
				//mem, memLimit          float64
				//pidsStatsCurrent       uint64
			)

			daemonOSType := "linux"

			if daemonOSType != "windows" {
			    //v.PreCPUStats.CPUUsage.TotalUsage
				previousCPU = data2.CPU.UsageUsec
				previousSystem = data2.CPU.SystemUsec
				//cpuPercent = calculateCPUPercentUnix(previousCPU, previousSystem, v)
				//blkRead, blkWrite = calculateBlockIO(v.BlkioStats)
				//mem = calculateMemUsageUnixNoCache(v.MemoryStats)
				//memLimit = float64(v.MemoryStats.Limit)
				//memPercent = calculateMemPercentUnixNoCache(memLimit, mem)
				//pidsStatsCurrent = v.PidsStats.Current
			} else {
                // do not handle windows yet
			}
			fmt.Println(previousCPU)
			fmt.Println(previousSystem)
			//netRx, netTx := calculateNetwork(v.Networks)
			/*s.SetStatistics(StatsEntry{
				Name:             v.Name,
				ID:               v.ID,
				CPUPercentage:    cpuPercent,
				Memory:           mem,
				MemoryPercentage: memPercent,
				MemoryLimit:      memLimit,
				NetworkRx:        netRx,
				NetworkTx:        netTx,
				BlockRead:        float64(blkRead),
				BlockWrite:       float64(blkWrite),
				PidsCurrent:      pidsStatsCurrent,
			})*/


	/*for {
		select {
		case <-time.After(2 * time.Second):
			// zero out the values if we have not received an update within
			// the specified duration.
			s.SetErrorAndReset(errors.New("timeout waiting for stats"))
			// if this is the first stat you get, release WaitGroup
			if !getFirst {
				getFirst = true
				waitFirst.Done()
			}
		case err := <-u:
			s.SetError(err)
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}
			// if this is the first stat you get, release WaitGroup
			if !getFirst {
				getFirst = true
				waitFirst.Done()
			}
		}
		if !streamStats {
			return
		}
	}*/
	return nil
}

// NewStats returns a new Stats entity and sets in it the given name
func NewStats(container string) *Stats {
	return &Stats{StatsEntry: StatsEntry{Container: container}}
}

func (s *stats) add(cs *Stats) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.isKnownContainer(cs.Container); !exists {
		s.cs = append(s.cs, cs)
		return true
	}
	return false
}

func (s *stats) isKnownContainer(cid string) (int, bool) {
	for i, c := range s.cs {
		if c.Container == cid {
			return i, true
		}
	}
	return -1, false
}