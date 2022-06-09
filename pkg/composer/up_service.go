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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"

	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/logging"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

func (c *Composer) upServices(ctx context.Context, parsedServices []*serviceparser.Service, uo UpOptions) error {
	if len(parsedServices) == 0 {
		return errors.New("no service was provided")
	}

	// TODO: parallelize loop for ensuring images (make sure not to mess up tty)
	for _, ps := range parsedServices {
		if err := c.ensureServiceImage(ctx, ps, !uo.NoBuild, uo.ForceBuild, BuildOptions{IPFS: uo.IPFS}, uo.QuietPull); err != nil {
			return err
		}
	}

	var (
		containers   = make(map[string]serviceparser.Container) // key: container Name
		containersMu sync.Mutex
		runEG        errgroup.Group
	)

	logsEOFChan := make(chan string) // value: container name
	interruptChan := make(chan os.Signal, 1)
	logsChan := make(chan map[string]string)

	lo := logging.LogsOptions{
		Follow:      true,
		NoColor:     uo.NoColor,
		NoLogPrefix: uo.NoLogPrefix,
	}

	for _, ps := range parsedServices {
		ps := ps
		for _, container := range ps.Containers {
			if uo.Detach {
				cmd, _, _, err := c.upServiceContainer(ctx, ps, container, uo)
				if err != nil {
					return err
				}
				if err := cmd.Wait(); err != nil {
					return err
				}
			} else {
				container := container
				runEG.Go(func() error {
					_, rStdout, rStderr, err := c.upServiceContainer(ctx, ps, container, uo)
					if err != nil {
						return err
					}
					go func() {
						<-interruptChan
						rStdout.Close()
						rStderr.Close()
					}()
					// format and write to channel
					if err = c.FormatLogs(container.Name, logsChan, logsEOFChan, lo, rStdout, rStderr); err != nil {
						return err
					}
					containersMu.Lock()
					containers[container.Name] = container
					containersMu.Unlock()
					return nil
				})
			}

		}
	}

	if uo.Detach {
		return nil
	}

	if err := runEG.Wait(); err != nil {
		return err
	}

	if uo.Detach {
		logrus.Info("Attaching to logs")
	}
	interruptChann := make(chan os.Signal, 1)
	signal.Notify(interruptChann, os.Interrupt)

	go func() {
		for {
			select {
			case e := <-logsChan:
				for k, v := range e {
					if k == "stdout" {
						fmt.Fprintf(os.Stdout, v)
					} else if k == "stderr" {
						fmt.Fprintf(os.Stderr, v)
					}
				}
				break
			}
		}
	}()

	signal.Notify(interruptChan, os.Interrupt)
	logsEOFMap := make(map[string]struct{}) // key: container name
selectLoop:
	for {
		// Wait for Ctrl-C, or `nerdctl compose down` in another terminal
		select {
		case sig := <-interruptChann:
			logrus.Debugf("Received signal: %s", sig)
			close(logsChan)
			break selectLoop
		case containerName := <-logsEOFChan:
			if lo.Follow {
				// When `nerdctl logs -f` has exited, we can assume that the container has exited
				logrus.Infof("Container %q exited", containerName)
			} else {
				logrus.Debugf("Logs for container %q reached EOF", containerName)
			}
			logsEOFMap[containerName] = struct{}{}
			if len(logsEOFMap) == len(containers) {
				if lo.Follow {
					logrus.Info("All the containers have exited")
				} else {
					logrus.Debug("All the logs reached EOF")
				}
				close(logsChan)
				break selectLoop
			}
		}
	}

	logrus.Infof("Stopping containers (forcibly)") // TODO: support gracefully stopping
	var rmWG sync.WaitGroup
	for name, container := range containers {
		name := name
		container := container
		rmWG.Add(1)
		go func() {
			defer rmWG.Done()
			logrus.Infof("Stopping container %s", container.Name)
			if err := c.runNerdctlCmd(ctx, "rm", "-f", name); err != nil {
				logrus.Warn(err)
			}
		}()
	}
	rmWG.Wait()
	return nil
}

func (c *Composer) ensureServiceImage(ctx context.Context, ps *serviceparser.Service, allowBuild, forceBuild bool, bo BuildOptions, quiet bool) error {
	if ps.Build != nil && allowBuild {
		if ps.Build.Force || forceBuild {
			return c.buildServiceImage(ctx, ps.Image, ps.Build, ps.Unparsed.Platform, bo)
		}
		if ok, err := c.ImageExists(ctx, ps.Image); err != nil {
			return err
		} else if ok {
			logrus.Debugf("Image %s already exists, not building", ps.Image)
		} else {
			return c.buildServiceImage(ctx, ps.Image, ps.Build, ps.Unparsed.Platform, bo)
		}
	}

	// even when c.ImageExists returns true, we need to call c.EnsureImage
	// because ps.PullMode can be "always".
	logrus.Infof("Ensuring image %s", ps.Image)
	if err := c.EnsureImage(ctx, ps.Image, ps.PullMode, ps.Unparsed.Platform, quiet); err != nil {
		return err
	}
	return nil
}

// upServiceContainer must be called after ensureServiceImage
// upServiceContainer returns container ID
func (c *Composer) upServiceContainer(ctx context.Context, service *serviceparser.Service, container serviceparser.Container, uo UpOptions) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	// check if container already exists
	exists, err := c.containerExists(ctx, container.Name, service.Unparsed.Name)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error while checking for containers with name %q: %s", container.Name, err)
	}

	// delete container if it already exists
	if exists {
		logrus.Debugf("Container %q already exists, deleting", container.Name)
		delCmd := c.createNerdctlCmd(ctx, "rm", "-f", container.Name)
		if err = delCmd.Run(); err != nil {
			return nil, nil, nil, fmt.Errorf("could not delete container %q: %s", container.Name, err)
		}
		logrus.Infof("Re-creating container %s", container.Name)
	} else {
		logrus.Infof("Creating container %s", container.Name)
	}

	//add metadata labels to container https://github.com/compose-spec/compose-spec/blob/master/spec.md#labels
	container.RunArgs = append([]string{
		fmt.Sprintf("-l=%s=%s", labels.ComposeProject, c.project.Name),
		fmt.Sprintf("-l=%s=%s", labels.ComposeService, service.Unparsed.Name),
	}, container.RunArgs...)

	if uo.Detach {
		container.RunArgs = append([]string{"-d"}, container.RunArgs...)
	}
	cmd := c.createNerdctlCmd(ctx, append([]string{"run"}, container.RunArgs...)...)
	if c.DebugPrintFull {
		logrus.Debugf("Running %v", cmd.Args)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, err
	}

	if err = cmd.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("error while creating container %s: %w", container.Name, err)
	}
	return cmd, stdout, stderr, nil
}
