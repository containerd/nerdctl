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
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/pkg/labels"
	"golang.org/x/sync/errgroup"
)

// FYI: https://github.com/docker/compose/blob/v2.14.1/pkg/api/api.go#L423
const (
	// RecreateNever specifies never recreating existing service containers
	RecreateNever = "never"
	// RecreateForce specifies always force-recreating service containers
	RecreateForce = "force"
	// RecreateDiverged specifies only recreating service containers which diverges from compose model.
	// (Unimplemented, currently equal to `RecreateNever`) In docker-compose,
	// service config is hashed and stored in a label.
	// FYI: https://github.com/docker/compose/blob/v2.14.1/pkg/compose/convergence.go#L244
	RecreateDiverged = "diverged"
)

// CreateOptions stores all option input from `nerdctl compose create`
type CreateOptions struct {
	Build         bool
	NoBuild       bool
	ForceRecreate bool
	NoRecreate    bool
	Pull          *string
}

func (opts CreateOptions) recreateStrategy() string {
	switch {
	case opts.ForceRecreate:
		return RecreateForce
	case opts.NoRecreate:
		return RecreateNever
	default:
		return RecreateDiverged
	}
}

// Create creates containers for given services.
func (c *Composer) Create(ctx context.Context, opt CreateOptions, services []string) error {
	// preprocess services based on options (for all project services, in case
	// there are dependencies not in `services`)
	for i, service := range c.project.Services {
		if opt.Pull != nil {
			service.PullPolicy = *opt.Pull
		}
		if opt.Build && service.Build != nil {
			service.PullPolicy = types.PullPolicyBuild
		}
		if opt.NoBuild {
			service.Build = nil
			if service.Image == "" {
				service.Image = fmt.Sprintf("%s_%s", c.project.Name, service.Name)
			}
		}
		c.project.Services[i] = service
	}

	// prepare other components (networks, volumes, configs)
	for shortName := range c.project.Networks {
		if err := c.upNetwork(ctx, shortName); err != nil {
			return err
		}
	}

	for shortName := range c.project.Volumes {
		if err := c.upVolume(ctx, shortName); err != nil {
			return err
		}
	}

	for shortName, secret := range c.project.Secrets {
		obj := types.FileObjectConfig(secret)
		if err := validateFileObjectConfig(obj, shortName, "service", c.project); err != nil {
			return err
		}
	}

	for shortName, config := range c.project.Configs {
		obj := types.FileObjectConfig(config)
		if err := validateFileObjectConfig(obj, shortName, "config", c.project); err != nil {
			return err
		}
	}

	// ensure images
	// TODO: parallelize loop for ensuring images (make sure not to mess up tty)
	parsedServices, err := c.Services(ctx, services...)
	if err != nil {
		return err
	}
	for _, ps := range parsedServices {
		if err := c.ensureServiceImage(ctx, ps, !opt.NoBuild, opt.Build, BuildOptions{}, false); err != nil {
			return err
		}
	}

	for _, ps := range parsedServices {
		if err := c.createService(ctx, ps, opt); err != nil {
			return err
		}
	}

	return nil
}

func (c *Composer) createService(ctx context.Context, ps *serviceparser.Service, opt CreateOptions) error {
	recreate := opt.recreateStrategy()
	var runEG errgroup.Group
	for _, container := range ps.Containers {
		container := container
		runEG.Go(func() error {
			_, err := c.createServiceContainer(ctx, ps, container, recreate)
			if err != nil {
				return err
			}
			return nil
		})
	}
	return runEG.Wait()
}

// createServiceContainer must be called after ensureServiceImage
// createServiceContainer returns container ID
// TODO(djdongjin): refactor needed:
// 1. the logic is similar to `upServiceContainer`, need to decouple some of the logic.
// 2. ideally, `compose up` should equal to `compose create` + `compose start`, we should decouple and reuse the logic in `compose up`.
// 3. it'll be easier to refactor after related `compose` logic are moved to `pkg` from `cmd`.
func (c *Composer) createServiceContainer(ctx context.Context, service *serviceparser.Service, container serviceparser.Container, recreate string) (string, error) {
	// check if container already exists
	exists, err := c.containerExists(ctx, container.Name, service.Unparsed.Name)
	if err != nil {
		return "", fmt.Errorf("error while checking for containers with name %q: %s", container.Name, err)
	}

	// delete container if it already exists and force-recreate is enabled
	if exists {
		if recreate != RecreateForce {
			log.G(ctx).Infof("Container %s exists, skipping", container.Name)
			return "", nil
		}

		log.G(ctx).Debugf("Container %q already exists and force-created is enabled, deleting", container.Name)
		delCmd := c.createNerdctlCmd(ctx, "rm", "-f", container.Name)
		if err = delCmd.Run(); err != nil {
			return "", fmt.Errorf("could not delete container %q: %s", container.Name, err)
		}
		log.G(ctx).Infof("Re-creating container %s", container.Name)
	} else {
		log.G(ctx).Infof("Creating container %s", container.Name)
	}

	tempDir, err := os.MkdirTemp(os.TempDir(), "compose-")
	if err != nil {
		return "", fmt.Errorf("error while creating/re-creating container %s: %w", container.Name, err)
	}
	defer os.RemoveAll(tempDir)
	cidFilename := filepath.Join(tempDir, "cid")

	//add metadata labels to container https://github.com/compose-spec/compose-spec/blob/master/spec.md#labels
	container.RunArgs = append([]string{
		"--cidfile=" + cidFilename,
		fmt.Sprintf("-l=%s=%s", labels.ComposeProject, c.project.Name),
		fmt.Sprintf("-l=%s=%s", labels.ComposeService, service.Unparsed.Name),
	}, container.RunArgs...)

	cmd := c.createNerdctlCmd(ctx, append([]string{"create"}, container.RunArgs...)...)
	if c.DebugPrintFull {
		log.G(ctx).Debugf("Running %v", cmd.Args)
	}

	// FIXME
	if service.Unparsed.StdinOpen != service.Unparsed.Tty {
		return "", fmt.Errorf("currently StdinOpen(-i) and Tty(-t) should be same")
	}

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
