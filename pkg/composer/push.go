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
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
)

type PushOptions struct {
}

func (c *Composer) Push(ctx context.Context, po PushOptions, services []string) error {
	return c.project.WithServices(services, func(svc types.ServiceConfig) error {
		ps, err := serviceparser.Parse(c.project, svc)
		if err != nil {
			return err
		}
		return c.pushServiceImage(ctx, ps.Image, ps.Unparsed.Platform, ps, po)
	})
}

func (c *Composer) pushServiceImage(ctx context.Context, image string, platform string, ps *serviceparser.Service, po PushOptions) error {
	log.G(ctx).Infof("Pushing image %s", image)

	var args []string // nolint: prealloc
	if platform != "" {
		args = append(args, "--platform="+platform)
	}
	if signer, ok := ps.Unparsed.Extensions[serviceparser.ComposeSign]; ok {
		args = append(args, "--sign="+signer.(string))
	}
	if privateKey, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignPrivateKey]; ok {
		args = append(args, "--cosign-key="+privateKey.(string))
	}
	if c.Options.Experimental {
		args = append(args, "--experimental")
	}

	args = append(args, image)

	cmd := c.createNerdctlCmd(ctx, append([]string{"push"}, args...)...)
	if c.DebugPrintFull {
		log.G(ctx).Debugf("Running %v", cmd.Args)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error while pushing image %s: %w", image, err)
	}
	return nil
}
