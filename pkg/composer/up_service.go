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
	"sync"

	"github.com/containerd/nerdctl/pkg/composer/pipetagger"
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

func (c *Composer) upServices(ctx context.Context, parsedServices []*serviceparser.Service, uo UpOptions) error {
	if len(parsedServices) == 0 {
		return errors.New("no service was provided")
	}

	// TODO: parallelize loop for ensuring images (make sure not to mess up tty)
	for _, ps := range parsedServices {
		if err := c.ensureServiceImage(ctx, ps); err != nil {
			return err
		}
	}

	var (
		containers   = make(map[string]serviceparser.Container) // key: container ID
		containersMu sync.Mutex
		runEG        errgroup.Group
	)
	for _, ps := range parsedServices {
		for _, container := range ps.Containers {
			container := container
			runEG.Go(func() error {
				id, err := c.upServiceContainer(ctx, container)
				if err != nil {
					return err
				}
				containersMu.Lock()
				containers[id] = container
				containersMu.Unlock()
				return nil
			})
		}
	}
	if err := runEG.Wait(); err != nil {
		return err
	}

	if uo.Detach {
		return nil
	}

	logrus.Info("Attaching to logs")
	if err := c.showLogs(ctx, containers); err != nil {
		return err
	}

	logrus.Infof("Stopping containers (forcibly)") // TODO: support gracefully stopping
	var rmWG sync.WaitGroup
	for id, container := range containers {
		id := id
		container := container
		rmWG.Add(1)
		go func() {
			defer rmWG.Done()
			logrus.Infof("Stopping container %s", container.Name)
			if err := c.runNerdctlCmd(ctx, "rm", "-f", id); err != nil {
				logrus.Warn(err)
			}
		}()
	}
	rmWG.Wait()
	return nil
}

func (c *Composer) showLogs(ctx context.Context, containers map[string]serviceparser.Container) error {
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
		state.logCmd = c.createNerdctlCmd(ctx, "logs", "-f", id)
		stdout, err := state.logCmd.StdoutPipe()
		if err != nil {
			return err
		}
		stdoutTagger := pipetagger.New(os.Stdout, stdout, state.logTag, logTagMaxLen+1)
		stderr, err := state.logCmd.StderrPipe()
		if err != nil {
			return err
		}
		stderrTagger := pipetagger.New(os.Stderr, stderr, state.logTag, logTagMaxLen+1)
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

	exited := make(map[string]struct{}) // key: container name
selectLoop:
	for {
		// Wait for Ctrl-C, or `nerdctl compose down` in another terminal
		select {
		case sig := <-interruptChan:
			logrus.Debugf("Received signal: %s", sig)
			break selectLoop
		case containerName := <-logsEOFChan:
			// When `nerdctl logs -f` has exited, we can assume that the container has exited
			logrus.Infof("Container %q exited", containerName)
			exited[containerName] = struct{}{}
			if len(exited) == len(containerStates) {
				logrus.Info("All the containers have exited")
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

func (c *Composer) ensureServiceImage(ctx context.Context, ps *serviceparser.Service) error {
	logrus.Infof("Ensuring image %s", ps.Image)
	if err := c.EnsureImage(ctx, ps.Image, ps.PullMode); err != nil {
		return err
	}
	return nil
}

// upServiceContainer must be called after ensureServiceImage
// upServiceContainer returns container ID
func (c *Composer) upServiceContainer(ctx context.Context, container serviceparser.Container) (string, error) {
	logrus.Infof("Creating container %s", container.Name)
	cmd := c.createNerdctlCmd(ctx, append([]string{"run"}, container.RunArgs...)...)
	if c.DebugPrintFull {
		logrus.Debugf("Running %v", cmd.Args)
	}
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", errors.Wrapf(err, "error while creating container %s", container.Name)
	}
	return strings.TrimSpace(string(out)), nil
}
