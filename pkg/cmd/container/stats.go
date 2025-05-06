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
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"text/tabwriter"
	"text/template"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/events"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/containerd/typeurl/v2"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/containerinspector"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/eventutil"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/infoutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/statsutil"
)

type stats struct {
	mu sync.Mutex
	cs []*statsutil.Stats
}

// add is from https://github.com/docker/cli/blob/3fb4fb83dfb5db0c0753a8316f21aea54dab32c5/cli/command/container/stats_helpers.go#L26-L34
func (s *stats) add(cs *statsutil.Stats) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.isKnownContainer(cs.ID); !exists {
		s.cs = append(s.cs, cs)
		return true
	}
	return false
}

// remove is from https://github.com/docker/cli/blob/3fb4fb83dfb5db0c0753a8316f21aea54dab32c5/cli/command/container/stats_helpers.go#L36-L42
func (s *stats) remove(id string) {
	s.mu.Lock()
	if i, exists := s.isKnownContainer(id); exists {
		s.cs = append(s.cs[:i], s.cs[i+1:]...)
	}
	s.mu.Unlock()
}

// isKnownContainer is from https://github.com/docker/cli/blob/3fb4fb83dfb5db0c0753a8316f21aea54dab32c5/cli/command/container/stats_helpers.go#L44-L51
func (s *stats) isKnownContainer(cid string) (int, bool) {
	for i, c := range s.cs {
		if c.ID == cid {
			return i, true
		}
	}
	return -1, false
}

// Stats displays a live stream of container(s) resource usage statistics.
func Stats(ctx context.Context, client *containerd.Client, containerIDs []string, options types.ContainerStatsOptions) error {
	// NOTE: rootless container does not rely on cgroupv1.
	// more details about possible ways to resolve this concern: #223
	if rootlessutil.IsRootless() && infoutil.CgroupsVersion() == "1" {
		return errors.New("stats requires cgroup v2 for rootless containers, see https://rootlesscontaine.rs/getting-started/common/cgroup2/")
	}

	showAll := len(containerIDs) == 0
	closeChan := make(chan error)

	var err error
	w := options.Stdout
	var tmpl *template.Template
	switch options.Format {
	case "", "table":
		w = tabwriter.NewWriter(options.Stdout, 10, 1, 3, ' ', 0)
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	default:
		tmpl, err = formatter.ParseTemplate(options.Format)
		if err != nil {
			return err
		}
	}

	// waitFirst is a WaitGroup to wait first stat data's reach for each container
	waitFirst := &sync.WaitGroup{}
	cStats := stats{}

	monitorContainerEvents := func(started chan<- struct{}, c chan *events.Envelope, closeEventChan chan struct{}) {
		eventsClient := client.EventService()
		eventsCh, errCh := eventsClient.Subscribe(ctx)

		// Whether we successfully subscribed to eventsCh or not, we can now
		// unblock the main goroutine.
		close(started)

		for {
			select {
			case <-closeEventChan:
				// c is closed, so we can stop monitoring events
				return
			case event := <-eventsCh:
				c <- event
			case err = <-errCh:
				closeChan <- err
				return
			}
		}
	}

	// getContainerList get all existing containers (only used when calling `nerdctl stats` without arguments).
	getContainerList := func() {
		containers, err := client.Containers(ctx)
		if err != nil {
			closeChan <- err
		}

		for _, c := range containers {
			cStatus := formatter.ContainerStatus(ctx, c)
			if !options.All {
				if !strings.HasPrefix(cStatus, "Up") {
					continue
				}
			}
			// if an error occurs when getting labels, the ID alone is sufficient for the stats screen.
			clabels, _ := c.Labels(ctx)
			s := statsutil.NewStats(c.ID(), containerutil.GetContainerName(clabels))
			if cStats.add(s) {
				waitFirst.Add(1)
				go collect(ctx, options.GOptions, s, waitFirst, c.ID(), !options.NoStream)
			}
		}
	}

	if showAll {
		started := make(chan struct{})
		var (
			datacc *eventstypes.ContainerCreate
			datacd *eventstypes.ContainerDelete
		)

		eh := eventutil.InitEventHandler()
		eh.Handle("/containers/create", func(e events.Envelope) {
			if e.Event != nil {
				anydata, err := typeurl.UnmarshalAny(e.Event)
				if err != nil {
					// just skip
					return
				}
				switch v := anydata.(type) {
				case *eventstypes.ContainerCreate:
					datacc = v
				default:
					// just skip
					return
				}
			}
			// Load the container to get its ID for retrieving the container name from Labels.
			// if an error occurs, the ID alone is sufficient for the stats screen.
			container, _ := client.LoadContainer(ctx, datacc.ID)
			clabels, _ := container.Labels(ctx)
			s := statsutil.NewStats(datacc.ID, containerutil.GetContainerName(clabels))
			if cStats.add(s) {
				waitFirst.Add(1)
				go collect(ctx, options.GOptions, s, waitFirst, datacc.ID, !options.NoStream)
			}
		})

		eh.Handle("/containers/delete", func(e events.Envelope) {
			if e.Event != nil {
				anydata, err := typeurl.UnmarshalAny(e.Event)
				if err != nil {
					// just skip
					return
				}
				switch v := anydata.(type) {
				case *eventstypes.ContainerDelete:
					datacd = v
				default:
					// just skip
					return
				}
			}
			cStats.remove(datacd.ID)
		})

		eventChan := make(chan *events.Envelope)
		closeEventChan := make(chan struct{})

		go eh.Watch(eventChan)
		go monitorContainerEvents(started, eventChan, closeEventChan)

		defer func() {
			closeEventChan <- struct{}{}
			close(eventChan)
		}()

		<-started

		// Start a goroutine to retrieve the initial list of containers stats.
		getContainerList()

		// make sure each container get at least one valid stat data
		waitFirst.Wait()

	} else {
		walker := &containerwalker.ContainerWalker{
			Client: client,
			OnFound: func(ctx context.Context, found containerwalker.Found) error {
				// if an error occurs when getting labels, the ID alone is sufficient for the stats screen.
				clabels, _ := found.Container.Labels(ctx)
				s := statsutil.NewStats(found.Container.ID(), containerutil.GetContainerName(clabels))
				if cStats.add(s) {
					waitFirst.Add(1)
					go collect(ctx, options.GOptions, s, waitFirst, found.Container.ID(), !options.NoStream)
				}
				return nil
			},
		}

		if err := walker.WalkAll(ctx, containerIDs, false); err != nil {
			return err
		}

		// make sure each container get at least one valid stat data
		waitFirst.Wait()

	}

	cleanScreen := func() {
		if !options.NoStream {
			fmt.Fprint(options.Stdout, "\033[2J")
			fmt.Fprint(options.Stdout, "\033[H")
		}
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// firstTick is for creating distant CPU readings.
	// firstTick stats are not displayed.
	firstTick := true
	for range ticker.C {
		cleanScreen()
		ccstats := []statsutil.StatsEntry{}
		cStats.mu.Lock()
		for _, c := range cStats.cs {
			if err := c.GetError(); err != nil {
				fmt.Fprintf(options.Stderr, "unable to get stat entry: %s\n", err)
			}
			ccstats = append(ccstats, c.GetStatistics())
		}
		cStats.mu.Unlock()

		if !firstTick {
			// print header for every tick
			if options.Format == "" || options.Format == "table" {
				fmt.Fprintln(w, "CONTAINER ID\tNAME\tCPU %\tMEM USAGE / LIMIT\tMEM %\tNET I/O\tBLOCK I/O\tPIDS")
			}
		}

		for _, c := range ccstats {
			if c.ID == "" {
				continue
			}
			rc := statsutil.RenderEntry(&c, options.NoTrunc)
			if !firstTick {
				if tmpl != nil {
					var b bytes.Buffer
					if err := tmpl.Execute(&b, rc); err != nil {
						break
					}
					if _, err = fmt.Fprintln(options.Stdout, b.String()); err != nil {
						break
					}
				} else {
					if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
						rc.ID,
						rc.Name,
						rc.CPUPerc,
						rc.MemUsage,
						rc.MemPerc,
						rc.NetIO,
						rc.BlockIO,
						rc.PIDs,
					); err != nil {
						break
					}
				}
			}
		}
		if f, ok := w.(formatter.Flusher); ok {
			f.Flush()
		}

		if len(cStats.cs) == 0 && !showAll {
			break
		}
		if options.NoStream && !firstTick {
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
		firstTick = false
	}

	return err
}

func collect(ctx context.Context, globalOptions types.GlobalCommandOptions, s *statsutil.Stats, waitFirst *sync.WaitGroup, id string, noStream bool) {
	log.G(ctx).Debugf("collecting stats for %s", s.ID)
	var (
		getFirst = true
		u        = make(chan error, 1)
	)

	defer func() {
		// if error happens and we get nothing of stats, release wait group whatever
		if getFirst {
			getFirst = false
			waitFirst.Done()
		}
	}()
	client, ctx, cancel, err := clientutil.NewClient(ctx, globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		s.SetError(err)
		return
	}
	defer func() {
		cancel()
		client.Close()
	}()
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		s.SetError(err)
		return
	}
	go func() {
		previousStats := new(statsutil.ContainerStats)
		firstSet := true
		for {
			// task is in the for loop to avoid nil task just after Container creation
			task, err := container.Task(ctx, nil)
			if err != nil {
				u <- err
				continue
			}

			metric, err := task.Metrics(ctx)
			if err != nil {
				u <- err
				continue
			}
			anydata, err := typeurl.UnmarshalAny(metric.Data)
			if err != nil {
				u <- err
				continue
			}

			netNS, err := containerinspector.InspectNetNS(ctx, int(task.Pid()))
			if err != nil {
				u <- err
				continue
			}

			// when (firstSet == true), we only set container stats without rendering stat entry
			statsEntry, err := setContainerStatsAndRenderStatsEntry(previousStats, firstSet, anydata, int(task.Pid()), netNS.Interfaces)
			if err != nil {
				u <- err
				continue
			}

			if firstSet {
				firstSet = false
			} else {
				s.SetStatistics(statsEntry)
			}
			u <- nil
			// sleep to create distant CPU readings
			time.Sleep(500 * time.Millisecond)
		}
	}()
	for {
		select {
		case <-time.After(6 * time.Second):
			// zero out the values if we have not received an update within
			// the specified duration.
			s.SetErrorAndReset(errors.New("timeout waiting for stats"))
			// if this is the first stat you get, release WaitGroup
			if getFirst {
				getFirst = false
				waitFirst.Done()
			}
		case err := <-u:
			if err != nil {
				if !errdefs.IsNotFound(err) && !strings.Contains(err.Error(), "no such file or directory") {
					s.SetError(err)
					continue
				}
			}
			// if this is the first stat you get, release WaitGroup
			if getFirst {
				getFirst = false
				waitFirst.Done()
			}
		}
	}
}
