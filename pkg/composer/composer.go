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
	"os"
	"os/exec"
	"path/filepath"

	compose "github.com/compose-spec/compose-go/types"
	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/nerdctl/pkg/composer/projectloader"
	"github.com/containerd/nerdctl/pkg/reflectutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Options struct {
	File           string // empty for default
	Project        string // empty for default
	NerdctlCmd     string
	NerdctlArgs    []string
	NetworkExists  func(string) (bool, error)
	VolumeExists   func(string) (bool, error)
	EnsureImage    func(ctx context.Context, imageName, pullMode string) error
	DebugPrintFull bool // full debug print, may leak secret env var to logs
}

func New(o Options) (*Composer, error) {
	if o.NerdctlCmd == "" {
		return nil, errors.New("got empty nerdctl cmd")
	}
	if o.NetworkExists == nil || o.VolumeExists == nil || o.EnsureImage == nil {
		return nil, errors.New("got empty functions")
	}

	var err error
	if o.File == "" {
		o.File, err = findComposeYAML()
		if err != nil {
			return nil, err
		}
	}

	o.File, err = filepath.Abs(o.File)
	if err != nil {
		return nil, err
	}

	if o.Project == "" {
		o.Project = filepath.Base(filepath.Dir(o.File))
	}

	if err := identifiers.Validate(o.Project); err != nil {
		return nil, errors.Wrapf(err, "got invalid project name %q", o.Project)
	}

	project, err := projectloader.Load(o.File, o.Project)
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
	}

	return c, nil
}

type Composer struct {
	Options
	project *compose.Project
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
		return errors.Wrapf(err, "error while executing %v: %q", cmd.Args, string(out))
	}
	return nil
}

func findComposeYAML() (string, error) {
	yamlNames := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}
	for _, candidate := range yamlNames {
		fullPath, err := filepath.Abs(candidate)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", errors.Errorf("cannot find a compose YAML, supported file names: %+v", yamlNames)
}
