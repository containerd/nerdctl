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
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/pkg/labels"

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
		containers   = make(map[string]serviceparser.Container) // key: container ID
		services     = []string{}
		containersMu sync.Mutex
		runEG        errgroup.Group
	)
	for _, ps := range parsedServices {
		ps := ps
		services = append(services, ps.Unparsed.Name)
		for _, container := range ps.Containers {
			container := container
			runEG.Go(func() error {
				id, err := c.upServiceContainer(ctx, ps, container)
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
	if err := c.Logs(ctx, lo, services); err != nil {
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
func (c *Composer) upServiceContainer(ctx context.Context, service *serviceparser.Service, container serviceparser.Container) (string, error) {
	// check if container already exists
	exists, err := c.containerExists(ctx, container.Name, service.Unparsed.Name)
	if err != nil {
		return "", fmt.Errorf("error while checking for containers with name %q: %s", container.Name, err)
	}

	// delete container if it already exists
	if exists {
		logrus.Debugf("Container %q already exists, deleting", container.Name)
		delCmd := c.createNerdctlCmd(ctx, "rm", "-f", container.Name)
		if err = delCmd.Run(); err != nil {
			return "", fmt.Errorf("could not delete container %q: %s", container.Name, err)
		}
		logrus.Infof("Re-creating container %s", container.Name)
	} else {
		logrus.Infof("Creating container %s", container.Name)
	}

	tempDir, err := os.MkdirTemp(os.TempDir(), "compose-")
	if err != nil {
		return "", fmt.Errorf("error while creating/re-creating container %s: %w", container.Name, err)
	}
	defer os.RemoveAll(tempDir)
	cidFilename := filepath.Join(tempDir, "cid")

	if container.Detached {
		container.RunArgs = append([]string{"-d"}, container.RunArgs...)
	}

	//add metadata labels to container https://github.com/compose-spec/compose-spec/blob/master/spec.md#labels
	container.RunArgs = append([]string{
		"--cidfile=" + cidFilename,
		fmt.Sprintf("-l=%s=%s", labels.ComposeProject, c.project.Name),
		fmt.Sprintf("-l=%s=%s", labels.ComposeService, service.Unparsed.Name),
	}, container.RunArgs...)

	cmd := c.createNerdctlCmd(ctx, append([]string{"run"}, container.RunArgs...)...)
	if c.DebugPrintFull {
		logrus.Debugf("Running %v", cmd.Args)
	}
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error while creating container %s: %w", container.Name, err)
	}

	cid, err := os.ReadFile(cidFilename)
	if err != nil {
		return "", fmt.Errorf("error while creating container %s: %w", container.Name, err)
	}
	return strings.TrimSpace(string(cid)), nil
}
