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
	"sync"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type RunOptions struct {
	ServiceName string
	Args        []string

	NoBuild       bool
	NoColor       bool
	NoLogPrefix   bool
	ForceBuild    bool
	IPFS          bool
	QuietPull     bool
	RemoveOrphans bool

	Name         string
	Detach       bool
	NoDeps       bool
	Tty          bool
	Interactive  bool
	Rm           bool
	User         string
	Volume       []string
	Entrypoint   []string
	Env          []string
	Label        []string
	WorkDir      string
	ServicePorts bool
	Publish      []string
}

func (c *Composer) Run(ctx context.Context, ro RunOptions) error {
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

	var svcs []types.ServiceConfig

	if ro.NoDeps {
		svc, err := c.project.GetService(ro.ServiceName)
		if err != nil {
			return err
		}
		svcs = append(svcs, svc)
	} else {
		if err := c.project.WithServices([]string{ro.ServiceName}, func(svc types.ServiceConfig) error {
			svcs = append(svcs, svc)
			return nil
		}); err != nil {
			return err
		}
	}

	var targetSvc *types.ServiceConfig
	for i := range svcs {
		if svcs[i].Name == ro.ServiceName {
			targetSvc = &svcs[i]
			break
		}
	}
	if targetSvc == nil {
		return fmt.Errorf("error cannot find service name: %s", ro.ServiceName)
	}

	targetSvc.Tty = ro.Tty
	targetSvc.StdinOpen = ro.Interactive

	if ro.Name != "" {
		targetSvc.ContainerName = ro.Name
	}
	if ro.User != "" {
		targetSvc.User = ro.User
	}
	if ro.Volume != nil && len(ro.Volume) > 0 {
		for _, v := range ro.Volume {
			vc, err := loader.ParseVolume(v)
			if err != nil {
				return err
			}
			targetSvc.Volumes = append(targetSvc.Volumes, vc)
		}
	}
	if ro.Entrypoint != nil && len(ro.Entrypoint) > 0 {
		targetSvc.Entrypoint = make([]string, len(ro.Entrypoint))
		copy(targetSvc.Entrypoint, ro.Entrypoint)
	}
	if ro.Env != nil && len(ro.Env) > 0 {
		envs := types.NewMappingWithEquals(ro.Env)
		targetSvc.Environment.OverrideBy(envs)
	}
	if ro.Label != nil && len(ro.Label) > 0 {
		label := types.NewMappingWithEquals(ro.Label)
		for k, v := range label {
			if v != nil {
				targetSvc.Labels.Add(k, *v)
			}
		}
	}
	if ro.WorkDir != "" {
		c.project.WorkingDir = ro.WorkDir
	}

	// `compose run` command does not create any of the ports specified in the service configuration.
	if !ro.ServicePorts {
		for k := range svcs {
			svcs[k].Ports = []types.ServicePortConfig{}
		}
		if ro.Publish != nil && len(ro.Publish) > 0 {
			for _, p := range ro.Publish {
				pc, err := types.ParsePortConfig(p)
				if err != nil {
					return fmt.Errorf("error parse --publish: %s", err)
				}
				targetSvc.Ports = append(targetSvc.Ports, pc...)
			}
		}
	}

	// `compose run` command overrides the command defined in the service configuration.
	if len(ro.Args) != 0 {
		targetSvc.Command = make([]string, len(ro.Args))
		copy(targetSvc.Command, ro.Args)
	}

	parsedServices := make([]*serviceparser.Service, 0)
	for _, svc := range svcs {
		ps, err := serviceparser.Parse(c.project, svc)
		if err != nil {
			return err
		}
		parsedServices = append(parsedServices, ps)
	}

	// remove orphan containers before the service has be started
	// FYI: https://github.com/docker/compose/blob/v2.3.4/pkg/compose/create.go#L91-L112
	orphans, err := c.getOrphanContainers(ctx, parsedServices)
	if err != nil && ro.RemoveOrphans {
		return fmt.Errorf("error getting orphaned containers: %s", err)
	}
	if len(orphans) > 0 {
		if ro.RemoveOrphans {
			if err := c.downContainers(ctx, orphans, true); err != nil {
				return fmt.Errorf("error removing orphaned containers: %s", err)
			}
		} else {
			logrus.Warnf("found %d orphaned containers: %v, you can run this command with the --remove-orphans flag to clean it up", len(orphans), orphans)
		}
	}

	if err := c.runServices(ctx, parsedServices, ro); err != nil {
		return err
	}

	return nil
}

func (c *Composer) runServices(ctx context.Context, parsedServices []*serviceparser.Service, ro RunOptions) error {
	if len(parsedServices) == 0 {
		return errors.New("no service was provided")
	}

	// TODO: parallelize loop for ensuring images (make sure not to mess up tty)
	for _, ps := range parsedServices {
		if err := c.ensureServiceImage(ctx, ps, !ro.NoBuild, ro.ForceBuild, BuildOptions{IPFS: ro.IPFS}, ro.QuietPull); err != nil {
			return err
		}
	}

	var (
		containers   = make(map[string]serviceparser.Container) // key: container ID
		services     = []string{}
		containersMu sync.Mutex
		runEG        errgroup.Group
		cid          string // For printing cid when -d exists
	)

	for _, ps := range parsedServices {
		ps := ps
		services = append(services, ps.Unparsed.Name)

		if len(ps.Containers) != 1 {
			logrus.Warnf("compose run does not support scale but %s is currently %v, automatically it will configure 1", ps.Unparsed.Name, len(ps.Containers))
		}

		if len(ps.Containers) == 0 {
			return fmt.Errorf("error, a service should have at least one container but %s does not have any container", ps.Unparsed.Name)
		}
		container := ps.Containers[0]

		runEG.Go(func() error {
			id, err := c.upServiceContainer(ctx, ps, container)
			if err != nil {
				return err
			}
			containersMu.Lock()
			containers[id] = container
			containersMu.Unlock()
			if ps.Unparsed.Name == ro.ServiceName {
				cid = id
			}
			return nil
		})
	}
	if err := runEG.Wait(); err != nil {
		return err
	}

	if ro.Detach {
		logrus.Printf("%s\n", cid)
		return nil
	}

	// TODO: fix it when `nerdctl logs` supports `nerdctl run` without detach
	// https://github.com/containerd/nerdctl/blob/v0.22.2/pkg/taskutil/taskutil.go#L55
	if !ro.Interactive && !ro.Tty {
		logrus.Info("Attaching to logs")
		lo := LogsOptions{
			Follow:      true,
			NoColor:     ro.NoColor,
			NoLogPrefix: ro.NoLogPrefix,
		}
		// it finally causes to show logs of some containers which are stopped but not deleted.
		if err := c.Logs(ctx, lo, services); err != nil {
			return err
		}
	}

	logrus.Infof("Stopping containers (forcibly)") // TODO: support gracefully stopping
	for id, container := range containers {
		var stopWG sync.WaitGroup
		id := id
		container := container
		stopWG.Add(1)
		go func() {
			defer stopWG.Done()
			logrus.Infof("Stopping container %s", container.Name)
			if err := c.runNerdctlCmd(ctx, "stop", id); err != nil {
				logrus.Warn(err)
			}
		}()
		stopWG.Wait()
	}

	if ro.Rm {
		var rmWG sync.WaitGroup
		for id, container := range containers {
			id := id
			container := container
			rmWG.Add(1)
			go func() {
				defer rmWG.Done()
				logrus.Infof("Removing container %s", container.Name)
				if err := c.runNerdctlCmd(ctx, "rm", "-f", id); err != nil {
					logrus.Warn(err)
				}
			}()
		}
		rmWG.Wait()
	}
	return nil
}
