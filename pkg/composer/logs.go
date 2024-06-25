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

package composer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/composer/pipetagger"
	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

type LogsOptions struct {
	AbortOnContainerExit bool
	Follow               bool
	Timestamps           bool
	Tail                 string
	NoColor              bool
	NoLogPrefix          bool
}

func (c *Composer) Logs(ctx context.Context, lo LogsOptions, services []string) error {
	var serviceNames []string
	err := c.project.ForEachService(services, func(name string, svc *types.ServiceConfig) error {
		serviceNames = append(serviceNames, svc.Name)
		return nil
	}, types.IgnoreDependencies)
	if err != nil {
		return err
	}
	containers, err := c.Containers(ctx, serviceNames...)
	if err != nil {
		return err
	}
	return c.logs(ctx, containers, lo)
}

func (c *Composer) logs(ctx context.Context, containers []containerd.Container, lo LogsOptions) error {
	var logTagMaxLen int
	type containerState struct {
		name   string
		logTag string
		logCmd *exec.Cmd
	}

	containerStates := make(map[string]containerState, len(containers)) // key: containerID
	for _, container := range containers {
		info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			return err
		}
		name := info.Labels[labels.Name]
		logTag := strings.TrimPrefix(name, c.project.Name+serviceparser.Separator)
		if l := len(logTag); l > logTagMaxLen {
			logTagMaxLen = l
		}
		containerStates[container.ID()] = containerState{
			name:   name,
			logTag: logTag,
		}
	}

	logsEOFChan := make(chan string) // value: container name
	for id, state := range containerStates {
		// TODO: show logs without executing `nerdctl logs`
		args := []string{"logs"}
		if lo.Follow {
			args = append(args, "-f")
		}
		if lo.Timestamps {
			args = append(args, "-t")
		}
		if lo.Tail != "" {
			args = append(args, "-n")
			if lo.Tail == "all" {
				args = append(args, "+0")
			} else {
				args = append(args, lo.Tail)
			}
		}

		args = append(args, id)
		state.logCmd = c.createNerdctlCmd(ctx, args...)
		stdout, err := state.logCmd.StdoutPipe()
		if err != nil {
			return err
		}
		logWidth := logTagMaxLen + 1
		if lo.NoLogPrefix {
			logWidth = -1
		}
		stdoutTagger := pipetagger.New(os.Stdout, stdout, state.logTag, logWidth, lo.NoColor)
		stderr, err := state.logCmd.StderrPipe()
		if err != nil {
			return err
		}
		stderrTagger := pipetagger.New(os.Stderr, stderr, state.logTag, logWidth, lo.NoColor)
		if c.DebugPrintFull {
			log.G(ctx).Debugf("Running %v", state.logCmd.Args)
		}
		if err := state.logCmd.Start(); err != nil {
			return err
		}
		containerName := state.name
		go func() {
			stdoutTagger.Run()
			logsEOFChan <- containerName
		}()
		go stderrTagger.Run()
	}

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt)

	logsEOFMap := make(map[string]struct{}) // key: container name
	var containerError error
selectLoop:
	for {
		// Wait for Ctrl-C, or `nerdctl compose down` in another terminal
		select {
		case sig := <-interruptChan:
			log.G(ctx).Debugf("Received signal: %s", sig)
			break selectLoop
		case containerName := <-logsEOFChan:
			if lo.Follow {
				// When `nerdctl logs -f` has exited, we can assume that the container has exited
				log.G(ctx).Infof("Container %q exited", containerName)
				// In case a container has exited and the parameter --abort-on-container-exit,
				// we break the loop and set an error, so we can exit the program with 1
				if lo.AbortOnContainerExit {
					containerError = fmt.Errorf("container %q exited", containerName)
					break selectLoop
				}
			} else {
				log.G(ctx).Debugf("Logs for container %q reached EOF", containerName)
			}
			logsEOFMap[containerName] = struct{}{}
			if len(logsEOFMap) == len(containerStates) {
				if lo.Follow {
					log.G(ctx).Info("All the containers have exited")
				} else {
					log.G(ctx).Debug("All the logs reached EOF")
				}
				break selectLoop
			}
		}
	}

	for _, state := range containerStates {
		if state.logCmd != nil && state.logCmd.Process != nil {
			if err := state.logCmd.Process.Kill(); err != nil {
				log.G(ctx).Warn(err)
			}
		}
	}

	return containerError
}
