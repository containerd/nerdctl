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
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	composecli "github.com/compose-spec/compose-go/cli"
	compose "github.com/compose-spec/compose-go/types"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/reflectutil"
)

// Options groups the command line options recommended for a Compose implementation (ProjectOptions) and extra options for nerdctl
type Options struct {
	Project          string // empty for default
	ProjectDirectory string
	ConfigPaths      []string
	Profiles         []string
	Services         []string
	EnvFile          string
	NerdctlCmd       string
	NerdctlArgs      []string
	NetworkInUse     func(ctx context.Context, netName string) (bool, error)
	NetworkExists    func(string) (bool, error)
	VolumeExists     func(string) (bool, error)
	ImageExists      func(ctx context.Context, imageName string) (bool, error)
	EnsureImage      func(ctx context.Context, imageName, pullMode, platform string, ps *serviceparser.Service, quiet bool) error
	DebugPrintFull   bool // full debug print, may leak secret env var to logs
	Experimental     bool // enable experimental features
	IPFSAddress      string
}

func New(o Options, client *containerd.Client) (*Composer, error) {
	if o.NerdctlCmd == "" {
		return nil, errors.New("got empty nerdctl cmd")
	}
	if o.NetworkExists == nil || o.VolumeExists == nil || o.EnsureImage == nil {
		return nil, errors.New("got empty functions")
	}

	if o.Project != "" {
		if err := identifiers.Validate(o.Project); err != nil {
			return nil, fmt.Errorf("got invalid project name %q: %w", o.Project, err)
		}
	}

	var optionsFn []composecli.ProjectOptionsFn
	optionsFn = append(optionsFn,
		composecli.WithOsEnv,
		composecli.WithWorkingDirectory(o.ProjectDirectory),
	)
	if o.EnvFile != "" {
		optionsFn = append(optionsFn,
			composecli.WithEnvFiles(o.EnvFile),
		)
	}
	optionsFn = append(optionsFn,
		composecli.WithConfigFileEnv,
		composecli.WithDefaultConfigPath,
		composecli.WithDotEnv,
		composecli.WithName(o.Project),
	)

	projectOptions, err := composecli.NewProjectOptions(o.ConfigPaths, optionsFn...)
	if err != nil {
		return nil, err
	}
	project, err := composecli.ProjectFromOptions(projectOptions)
	if err != nil {
		return nil, err
	}

	if len(o.Services) > 0 {
		s, err := project.GetServices(o.Services...)
		if err != nil {
			return nil, err
		}
		o.Profiles = append(o.Profiles, s.GetProfiles()...)
	}
	project.ApplyProfiles(o.Profiles)

	if o.DebugPrintFull {
		projectJSON, _ := json.MarshalIndent(project, "", "    ")
		log.L.Debug("printing project JSON")
		log.L.Debugf("%s", projectJSON)
	}

	if unknown := reflectutil.UnknownNonEmptyFields(project,
		"Name",
		"WorkingDir",
		"Environment",
		"Services",
		"Networks",
		"Volumes",
		"Secrets",
		"Configs",
		"ComposeFiles"); len(unknown) > 0 {
		log.L.Warnf("Ignoring: %+v", unknown)
	}

	c := &Composer{
		Options: o,
		project: project,
		client:  client,
	}

	return c, nil
}

type Composer struct {
	Options
	project *compose.Project
	client  *containerd.Client
}

func (c *Composer) createNerdctlCmd(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, c.NerdctlCmd, append(c.NerdctlArgs, args...)...)
}

func (c *Composer) runNerdctlCmd(ctx context.Context, args ...string) error {
	cmd := c.createNerdctlCmd(ctx, args...)
	if c.DebugPrintFull {
		log.G(ctx).Debugf("Running %v", cmd.Args)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error while executing %v: %q: %w", cmd.Args, string(out), err)
	}
	return nil
}

// Services returns the parsed Service objects in dependency order.
func (c *Composer) Services(ctx context.Context, svcs ...string) ([]*serviceparser.Service, error) {
	var services []*serviceparser.Service
	if err := c.project.WithServices(svcs, func(svc compose.ServiceConfig) error {
		parsed, err := serviceparser.Parse(c.project, svc)
		if err != nil {
			return err
		}
		services = append(services, parsed)
		return nil
	}); err != nil {
		return nil, err
	}
	return services, nil
}

// ServiceNames returns service names in dependency order.
func (c *Composer) ServiceNames(svcs ...string) ([]string, error) {
	var names []string
	if err := c.project.WithServices(svcs, func(svc compose.ServiceConfig) error {
		names = append(names, svc.Name)
		return nil
	}); err != nil {
		return nil, err
	}
	return names, nil
}
