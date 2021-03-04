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
	//"errors"
	"sync"
	"time"
	"fmt"
	//"io"
    //"strings"
	//"os"
   	"github.com/docker/docker/api/types"
	//wstats "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	v1 "github.com/containerd/containerd/metrics/types/v1"
	v2 "github.com/containerd/containerd/metrics/types/v2"
	//"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/typeurl"
	"github.com/urfave/cli/v2"
	"github.com/sirupsen/logrus"
	"github.com/docker/cli/cli/command/formatter"
)

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
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:        "all",
			Aliases:     []string{"a"},
			Usage:       "Show all containers (default shows just running)",
			Destination: &optionsStats.all,
		},
		&cli.StringFlag{
			Name:        "format",
			Usage:       "Pretty-print images using a Go template",
			Destination: &optionsStats.format,
		},
		&cli.BoolFlag{
			Name:        "no-stream",
			Usage:       "Disable streaming stats and only pull the first result",
			Destination: &optionsStats.noStream,
		},
		&cli.BoolFlag{
			Name:        "no-trunc",
			Usage:       "Do not truncate output",
			Destination: &optionsStats.noTrunc,
		},
	},
	Action: func(clicontext *cli.Context) error {
		showAll := len(optionsStats.containers) == 0
	    optionsStats.containers = clicontext.Args().Slice()
	    closeChan := make(chan error)
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
        daemonOSType := "linux"

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

		// We don't expect any asynchronous errors: closeChan can be closed.
		close(closeChan)

		// make sure each container get at least one valid stat data
		waitFirst.Wait()

        format := optionsStats.format
        if len(format) == 0 {
            /*if len(dockerCli.ConfigFile().StatsFormat) > 0 {
                format = dockerCli.ConfigFile().StatsFormat
            } else {
                format = formatter.TableFormatKey
            }*/
            format = formatter.TableFormatKey
        }
        statsCtx := formatter.Context{
            Output: clicontext.App.Writer,
            Format: NewStatsFormat(format, "linux"),
        }
        cleanScreen := func() {
            if !optionsStats.noStream {
                fmt.Fprint(clicontext.App.Writer, "\033[2J")
                fmt.Fprint(clicontext.App.Writer, "\033[H")
            }
        }

        var err error
        ticker := time.NewTicker(500 * time.Millisecond)
        defer ticker.Stop()
        for range ticker.C {
            cleanScreen()
            ccstats := []StatsEntry{}
            cStats.mu.Lock()
            for _, c := range cStats.cs {
                ccstats = append(ccstats, c.GetStatistics())
            }
            cStats.mu.Unlock()
            if err = statsFormatWrite(statsCtx, ccstats, daemonOSType, !optionsStats.noTrunc); err != nil {
                break
            }
            if len(cStats.cs) == 0 && !showAll {
                break
            }
            if optionsStats.noStream {
                break
            }
            select {
            case err, ok := <-closeChan:
                if ok {
                    if err != nil {
                        return err
                    }
                }
            default:
                // just skip
            }
        }

	return err
	},
}

func collect(clicontext *cli.Context, s *Stats, waitFirst *sync.WaitGroup) error {
	logrus.Debugf("collecting stats for %s", s.Container)
	var (
		getFirst         bool
		//previousCPU    uint64
		//previousSystem uint64
	    u  = make(chan error, 1)
	)

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

	go func() {
		for  {
		    time.Sleep(1 * time.Second)
            var (
                   memPercent             float64
                   blkRead, blkWrite      uint64 // Only used on Linux
                   mem, memLimit          float64
                   netRx, netTx           float64
                   pidsStatsCurrent       uint64
                )

                	metric, err := task.Metrics(ctx)
                	if err != nil {
                		u <- err
                	}
                	anydata, err := typeurl.UnmarshalAny(metric.Data)
                	if err != nil {
                		u <- err
                	}

                    var (
                        data         *v1.Metrics
                        data2        *v2.Metrics
                        //windowsStats *wstats.Statistics
                    )
                    switch v := anydata.(type) {
                    case *v1.Metrics:
                        data = v
                    case *v2.Metrics:
                        data2 = v
                    //case *wstats.Statistics:
                        //windowsStats = v
                    default:
                        //return errors.New("cannot convert metric data to cgroups.Metrics or windows.Statistics")
                        return
                    }

                    daemonOSType := "linux"
                    if daemonOSType != "windows" {
                       if data != nil {
                          //previousCPU = data.CPU.Usage.Total
                          //previousSystem = data.CPU.Usage.Kernel
                          //cpuPercent = calculateCPUPercentUnix(previousCPU, previousSystem, v)
                          blkRead, blkWrite = calculateBlockIO(data)
                          mem = calculateMemUsageUnixNoCache(data)
                          memLimit = float64(data.Memory.Usage.Limit)
                          memPercent = calculateMemPercentUnixNoCache(memLimit, mem)
                          pidsStatsCurrent = data.Pids.Current
                          netRx, netTx = calculateNetwork(data)
                        }else if data2 != nil {
                          //previousCPU = data2.CPU.UsageUsec
                          //previousSystem = data2.CPU.SystemUsec
                        }else { }

                    } else {
                       // do not handle windows yet
                    }
                    value, _ := container.Labels(ctx)

                    s.SetStatistics(StatsEntry{
                    Name:             value["name"],
                    ID:               container.ID(),
                    CPUPercentage:    10,
                    Memory:           mem,
                    MemoryPercentage: memPercent,
                    MemoryLimit:      memLimit,
                    NetworkRx:        netRx,
                    NetworkTx:        netTx,
                    BlockRead:        float64(blkRead),
                    BlockWrite:       float64(blkWrite),
                    PidsCurrent:      pidsStatsCurrent,
                    })
                    u <- nil
   	    }
   	}()
	for {
		select {
		case <-time.After(2 * time.Second):
			// zero out the values if we have not received an update within
			// the specified duration.
			//s.SetErrorAndReset(errors.New("timeout waiting for stats"))
			// if this is the first stat you get, release WaitGroup
			if !getFirst {
				getFirst = true
				waitFirst.Done()
			}
		case err := <-u:
			//s.SetError(err)
			if err != nil {
				continue
			}
			// if this is the first stat you get, release WaitGroup
			if !getFirst {
				getFirst = true
				waitFirst.Done()
			}
		}
	}
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

func calculateCPUPercentUnix(previousCPU, previousSystem uint64, v *types.StatsJSON) float64 {
	var (
		cpuPercent = 0.0
		// calculate the change for the cpu usage of the container in between readings
		cpuDelta = float64(v.CPUStats.CPUUsage.TotalUsage) - float64(previousCPU)
		// calculate the change for the entire system between readings
		systemDelta = float64(v.CPUStats.SystemUsage) - float64(previousSystem)
		onlineCPUs  = float64(v.CPUStats.OnlineCPUs)
	)

	if onlineCPUs == 0.0 {
		onlineCPUs = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
	}
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}
	return cpuPercent
}


func calculateMemUsageUnixNoCache(stats *v1.Metrics) float64 {
	// cgroup v1
	if v := stats.Memory.TotalInactiveFile; v < stats.Memory.Usage.Usage {
		return float64(stats.Memory.Usage.Usage - v)
	}
	// cgroup v2
	//if v := mem.Stats["inactive_file"]; v < mem.Usage {
		//return float64(mem.Usage - v)
	//}
	return float64(stats.Memory.Usage.Usage)
}

func calculateMemPercentUnixNoCache(limit float64, usedNoCache float64) float64 {
	// MemoryStats.Limit will never be 0 unless the container is not running and we haven't
	// got any data from cgroup
	if limit != 0 {
		return usedNoCache / limit * 100.0
	}
	return 0
}

func calculateNetwork(metrics *v1.Metrics) (float64, float64) {
	var rx, tx float64

	for _, v := range metrics.Network {
		rx += float64(v.RxBytes)
		tx += float64(v.TxBytes)
	}
	return rx, tx
}

func calculateBlockIO(metrics *v1.Metrics) (uint64, uint64) {
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