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
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/nerdctl/pkg/composer/pipetagger"
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/sirupsen/logrus"
)

type LogsOptions struct {
	Follow      bool
	NoColor     bool
	NoLogPrefix bool
}

func (c *Composer) Logs(ctx context.Context, lo LogsOptions) error {
	containers := make(map[string]serviceparser.Container) // key: container name
	if err := c.project.WithServices(nil, func(svc types.ServiceConfig) error {
		ps, err := serviceparser.Parse(c.project, svc)
		if err != nil {
			return err
		}
		for _, container := range ps.Containers {
			containers[container.Name] = container
		}
		return nil
	}); err != nil {
		return err
	}

	return c.logs(ctx, containers, lo)
}

func (c *Composer) logs(ctx context.Context, containers map[string]serviceparser.Container, lo LogsOptions) error {
	var logTagMaxLen int
	type containerState struct {
		name   string
		logTag string
		logCmd *exec.Cmd
	}

	containerStates := make(map[string]containerState, len(containers)) // key: containerID
	for id, container := range containers {
		logTag := strings.TrimPrefix(container.Name, c.project.Name+"_")
		if l := len(logTag); l > logTagMaxLen {
			logTagMaxLen = l
		}
		containerStates[id] = containerState{
			name:   container.Name,
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
			logrus.Debugf("Running %v", state.logCmd.Args)
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
selectLoop:
	for {
		// Wait for Ctrl-C, or `nerdctl compose down` in another terminal
		select {
		case sig := <-interruptChan:
			logrus.Debugf("Received signal: %s", sig)
			break selectLoop
		case containerName := <-logsEOFChan:
			if lo.Follow {
				// When `nerdctl logs -f` has exited, we can assume that the container has exited
				logrus.Infof("Container %q exited", containerName)
			} else {
				logrus.Debugf("Logs for container %q reached EOF", containerName)
			}
			logsEOFMap[containerName] = struct{}{}
			if len(logsEOFMap) == len(containerStates) {
				if lo.Follow {
					logrus.Info("All the containers have exited")
				} else {
					logrus.Debug("All the logs reached EOF")
				}
				break selectLoop
			}
		}
	}

	for _, state := range containerStates {
		if state.logCmd != nil && state.logCmd.Process != nil {
			if err := state.logCmd.Process.Kill(); err != nil {
				logrus.Warn(err)
			}
		}
	}

	return nil
}
