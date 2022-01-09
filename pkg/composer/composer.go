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
	"os"
	"os/exec"
	"path/filepath"

	composecli "github.com/compose-spec/compose-go/cli"
	compose "github.com/compose-spec/compose-go/types"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/nerdctl/pkg/composer/projectloader"
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/pkg/reflectutil"

	"github.com/sirupsen/logrus"
)

// Options groups the command line options recommended for a Compose implementation (ProjectOptions) and extra options for nerdctl
type Options struct {
	//TODO Specifying multiple Compose files
	ProjectOptions composecli.ProjectOptions
	Project        string // empty for default
	NerdctlCmd     string
	NerdctlArgs    []string
	NetworkExists  func(string) (bool, error)
	VolumeExists   func(string) (bool, error)
	ImageExists    func(ctx context.Context, imageName string) (bool, error)
	EnsureImage    func(ctx context.Context, imageName, pullMode, platform string, quiet bool) error
	DebugPrintFull bool // full debug print, may leak secret env var to logs
}

func New(o Options, client *containerd.Client) (*Composer, error) {
	if o.NerdctlCmd == "" {
		return nil, errors.New("got empty nerdctl cmd")
	}
	if o.NetworkExists == nil || o.VolumeExists == nil || o.EnsureImage == nil {
		return nil, errors.New("got empty functions")
	}
	// actually we support single ConfigPath
	// TODO support multiple ConfigPaths
	var err error
	if len(o.ProjectOptions.ConfigPaths) == 0 {
		composeYaml, err := findComposeYAML(&o)
		if err != nil {
			return nil, err
		}
		o.ProjectOptions.ConfigPaths = append(o.ProjectOptions.ConfigPaths, composeYaml)
	}

	if err := composecli.WithOsEnv(&o.ProjectOptions); err != nil {
		return nil, err
	}

	if err := composecli.WithDotEnv(&o.ProjectOptions); err != nil {
		return nil, err
	}

	for i := 0; i < len(o.ProjectOptions.ConfigPaths); i++ {
		o.ProjectOptions.ConfigPaths[i], err = filepath.Abs(o.ProjectOptions.ConfigPaths[i])
		if err != nil {
			return nil, err
		}
	}

	if o.Project == "" {
		o.Project = filepath.Base(filepath.Dir(o.ProjectOptions.ConfigPaths[0]))
	}

	if err := identifiers.Validate(o.Project); err != nil {
		return nil, fmt.Errorf("got invalid project name %q: %w", o.Project, err)
	}

	project, err := projectloader.Load(o.ProjectOptions.ConfigPaths[0], o.Project, o.ProjectOptions.Environment)
	if err != nil {
		return nil, err
	}

	if o.DebugPrintFull {
		projectJSON, _ := json.MarshalIndent(project, "", "    ")
		logrus.Debug("printing project JSON")
		logrus.Debugf("%s", projectJSON)
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
		logrus.Warnf("Ignoring: %+v", unknown)
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
		logrus.Debugf("Running %v", cmd.Args)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error while executing %v: %q: %w", cmd.Args, string(out), err)
	}
	return nil
}

//find compose Yaml file in current directory and its parents
func findComposeYAML(o *Options) (string, error) {
	pwd, err := o.ProjectOptions.GetWorkingDir()
	if err != nil {
		return "", err
	}
	for {
		yamlNames := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}
		for _, candidate := range yamlNames {
			f := filepath.Join(pwd, candidate)
			if _, err := os.Stat(f); err == nil {
				return f, nil
			} else if !os.IsNotExist(err) {
				return "", err
			}
		}
		parent := filepath.Dir(pwd)
		if parent == pwd {
			return "", fmt.Errorf("cannot find a compose YAML, supported file names: %+v in this directory or any parent", yamlNames)
		}
		pwd = parent
	}
}

func (c *Composer) Services(ctx context.Context) ([]*serviceparser.Service, error) {
	var services []*serviceparser.Service
	if err := c.project.WithServices(nil, func(svc compose.ServiceConfig) error {
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

func (c *Composer) ServiceNames(services ...string) ([]string, error) {
	var names []string
	if err := c.project.WithServices(services, func(svc compose.ServiceConfig) error {
		names = append(names, svc.Name)
		return nil
	}); err != nil {
		return nil, err
	}
	return names, nil
}
