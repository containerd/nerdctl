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

package serviceparser

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/reflectutil"

	"github.com/sirupsen/logrus"
)

func parseBuildConfig(c *types.BuildConfig, project *types.Project, imageName string) (*Build, error) {
	if unknown := reflectutil.UnknownNonEmptyFields(c,
		"Context", "Dockerfile", "Args", "CacheFrom", "Target", "Labels",
	); len(unknown) > 0 {
		logrus.Warnf("Ignoring: build: %+v", unknown)
	}

	if c.Context == "" {
		return nil, errors.New("build: context must be specified")
	}
	if strings.Contains(c.Context, "://") {
		return nil, fmt.Errorf("build: URL-style context (%q) is not supported yet: %w", c.Context, errdefs.ErrNotImplemented)
	}
	if filepath.IsAbs(c.Context) {
		logrus.Warnf("build.config should be relative path, got %q", c.Context)
	}
	ctxDir := project.RelativePath(c.Context)

	var b Build
	b.BuildArgs = append(b.BuildArgs, "-t="+imageName)
	if c.Dockerfile != "" {
		if filepath.IsAbs(c.Dockerfile) {
			logrus.Warnf("build.dockerfile should be relative path, got %q", c.Dockerfile)
			b.BuildArgs = append(b.BuildArgs, "-f="+c.Dockerfile)
		} else {
			// no need to use securejoin
			dockerfile := filepath.Join(ctxDir, c.Dockerfile)
			b.BuildArgs = append(b.BuildArgs, "-f="+dockerfile)
		}
	}

	for k, v := range c.Args {
		if v == nil {
			b.BuildArgs = append(b.BuildArgs, "--build-arg="+k)
		} else {
			b.BuildArgs = append(b.BuildArgs, "--build-arg="+k+"="+*v)
		}
	}

	for _, s := range c.CacheFrom {
		b.BuildArgs = append(b.BuildArgs, "--cache-from="+s)
	}

	if c.Target != "" {
		b.BuildArgs = append(b.BuildArgs, "--target="+c.Target)
	}

	if c.Labels != nil {
		for k, v := range c.Labels {
			b.BuildArgs = append(b.BuildArgs, "--label="+k+"="+v)
		}
	}

	b.BuildArgs = append(b.BuildArgs, ctxDir)
	return &b, nil
}
