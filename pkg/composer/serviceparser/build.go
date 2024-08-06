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

	"github.com/compose-spec/compose-go/v2/types"
	securejoin "github.com/cyphar/filepath-securejoin"

	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/identifiers"
	"github.com/containerd/nerdctl/v2/pkg/reflectutil"
)

func parseBuildConfig(c *types.BuildConfig, project *types.Project, imageName string) (*Build, error) {
	if unknown := reflectutil.UnknownNonEmptyFields(c,
		"Context", "Dockerfile", "Args", "CacheFrom", "Target", "Labels", "Secrets",
	); len(unknown) > 0 {
		log.L.Warnf("Ignoring: build: %+v", unknown)
	}

	if c.Context == "" {
		return nil, errors.New("build: context must be specified")
	}
	if strings.Contains(c.Context, "://") {
		return nil, fmt.Errorf("build: URL-style context (%q) is not supported yet: %w", c.Context, errdefs.ErrNotImplemented)
	}
	ctxDir := project.RelativePath(c.Context)

	var b Build
	b.BuildArgs = append(b.BuildArgs, "-t="+imageName)
	if c.Dockerfile != "" {
		if filepath.IsAbs(c.Dockerfile) {
			log.L.Warnf("build.dockerfile should be relative path, got %q", c.Dockerfile)
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

	for _, s := range c.Secrets {
		fileRef := types.FileReferenceConfig(s)

		if err := identifiers.ValidateDockerCompat(fileRef.Source); err != nil {
			return nil, fmt.Errorf("invalid secret source name: %w", err)
		}

		projectSecret, ok := project.Secrets[fileRef.Source]
		if !ok {
			return nil, fmt.Errorf("build: secret %s is undefined", fileRef.Source)
		}
		var src string
		if filepath.IsAbs(projectSecret.File) {
			log.L.Warnf("build.secrets should be relative path, got %q", projectSecret.File)
			src = projectSecret.File
		} else {
			var err error
			src, err = securejoin.SecureJoin(ctxDir, projectSecret.File)
			if err != nil {
				return nil, err
			}
		}
		id := fileRef.Source
		if fileRef.Target != "" {
			id = fileRef.Target
		}
		b.BuildArgs = append(b.BuildArgs, "--secret=id="+id+",src="+src)
	}

	b.BuildArgs = append(b.BuildArgs, ctxDir)
	return &b, nil
}
