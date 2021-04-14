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
	"strings"
	"sync"

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
	lo := LogsOptions{
		Follow:      true,
		NoColor:     uo.NoColor,
		NoLogPrefix: uo.NoLogPrefix,
	}
	if err := c.logs(ctx, containers, lo); err != nil {
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
