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
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/docker/docker/pkg/system"
	"github.com/sirupsen/logrus"
)

type CopyOptions struct {
	Source      string
	Destination string
	Index       int
	FollowLink  bool
	DryRun      bool
}

type copyDirection int

const (
	fromService copyDirection = 0
	toService   copyDirection = 1
)

func (c *Composer) Copy(ctx context.Context, co CopyOptions) error {
	srcService, srcPath := splitCpArg(co.Source)
	destService, dstPath := splitCpArg(co.Destination)
	var serviceName string
	var direction copyDirection

	if srcService != "" && destService != "" {
		return errors.New("copying between services is not supported")
	}
	if srcService == "" && destService == "" {
		return errors.New("unknown copy direction")
	}

	if srcService != "" {
		direction = fromService
		serviceName = srcService
	}
	if destService != "" {
		direction = toService
		serviceName = destService
	}

	containers, err := c.listContainersTargetedForCopy(ctx, co.Index, direction, serviceName)
	if err != nil {
		return err
	}

	for _, container := range containers {
		args := []string{"cp"}
		if co.FollowLink {
			args = append(args, "--follow-link")
		}
		if direction == fromService {
			args = append(args, fmt.Sprintf("%s:%s", container.ID(), srcPath), dstPath)
		}
		if direction == toService {
			args = append(args, srcPath, fmt.Sprintf("%s:%s", container.ID(), dstPath))
		}
		err := c.logCopyMsg(ctx, container, direction, srcService, srcPath, destService, dstPath, co.DryRun)
		if err != nil {
			return err
		}
		if !co.DryRun {
			if err := c.runNerdctlCmd(ctx, args...); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Composer) logCopyMsg(ctx context.Context, container containerd.Container, direction copyDirection, srcService string, srcPath string, destService string, dstPath string, dryRun bool) error {
	containerLabels, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	containerName := containerLabels[labels.Name]
	msg := ""
	if dryRun {
		msg = "DRY-RUN MODE - "
	}
	if direction == fromService {
		msg = msg + fmt.Sprintf("copy %s:%s to %s", containerName, srcPath, dstPath)
	}
	if direction == toService {
		msg = msg + fmt.Sprintf("copy %s to %s:%s", srcPath, containerName, dstPath)
	}
	logrus.Info(msg)
	return nil
}

func (c *Composer) listContainersTargetedForCopy(ctx context.Context, index int, direction copyDirection, serviceName string) ([]containerd.Container, error) {
	var containers []containerd.Container
	var err error

	containers, err = c.Containers(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	if index > 0 {
		if index > len(containers) {
			return nil, fmt.Errorf("index (%d) out of range: only %d running instances from service %s",
				index, len(containers), serviceName)
		}
		container := containers[index-1]
		return []containerd.Container{container}, nil
	}

	if len(containers) < 1 {
		return nil, fmt.Errorf("no container found for service %q", serviceName)
	}
	if direction == fromService {
		return containers[:1], err

	}
	return containers, err
}

// https://github.com/docker/compose/blob/v2.21.0/pkg/compose/cp.go#L307
func splitCpArg(arg string) (container, path string) {
	if system.IsAbs(arg) {
		// Explicit local absolute path, e.g., `C:\foo` or `/foo`.
		return "", arg
	}

	parts := strings.SplitN(arg, ":", 2)

	if len(parts) == 1 || strings.HasPrefix(parts[0], ".") {
		// Either there's no `:` in the arg
		// OR it's an explicit local relative path like `./file:name.txt`.
		return "", arg
	}

	return parts[0], parts[1]
}
