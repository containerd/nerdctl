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

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"

	"github.com/sirupsen/logrus"
)

type PushOptions struct {
}

func (c *Composer) Push(ctx context.Context, po PushOptions) error {
	return c.project.WithServices(nil, func(svc types.ServiceConfig) error {
		ps, err := serviceparser.Parse(c.project, svc)
		if err != nil {
			return err
		}
		return c.pushServiceImage(ctx, ps.Image, ps.Unparsed.Platform, po)
	})
}

func (c *Composer) pushServiceImage(ctx context.Context, image string, platform string, po PushOptions) error {
	logrus.Infof("Pushing image %s", image)

	var args []string // nolint: prealloc
	if platform != "" {
		args = append(args, "--platform="+platform)
	}
	args = append(args, image)

	cmd := c.createNerdctlCmd(ctx, append([]string{"push"}, args...)...)
	if c.DebugPrintFull {
		logrus.Debugf("Running %v", cmd.Args)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error while pushing image %s: %w", image, err)
	}
	return nil
}
