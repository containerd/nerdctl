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

	"github.com/compose-spec/compose-go/v2/types"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
)

type PullOptions struct {
	Quiet bool
}

func (c *Composer) Pull(ctx context.Context, po PullOptions, services []string) error {
	return c.project.ForEachService(services, func(name string, svc *types.ServiceConfig) error {
		ps, err := serviceparser.Parse(c.project, *svc)
		if err != nil {
			return err
		}
		return c.pullServiceImage(ctx, ps.Image, ps.Unparsed.Platform, ps, po)
	})
}

func (c *Composer) pullServiceImage(ctx context.Context, image string, platform string, ps *serviceparser.Service, po PullOptions) error {
	log.G(ctx).Infof("Pulling image %s", image)

	var args []string // nolint: prealloc
	if platform != "" {
		args = append(args, "--platform="+platform)
	}
	if po.Quiet {
		args = append(args, "--quiet")
	}
	if verifier, ok := ps.Unparsed.Extensions[serviceparser.ComposeVerify].(string); ok {
		args = append(args, "--verify="+verifier)
	}
	if publicKey, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignPublicKey].(string); ok {
		args = append(args, "--cosign-key="+publicKey)
	}
	if certificateIdentity, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignCertificateIdentity].(string); ok {
		args = append(args, "--cosign-certificate-identity="+certificateIdentity)
	}
	if certificateIdentityRegexp, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignCertificateIdentityRegexp].(string); ok {
		args = append(args, "--cosign-certificate-identity-regexp="+certificateIdentityRegexp)
	}
	if certificateOidcIssuer, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignCertificateOidcIssuer].(string); ok {
		args = append(args, "--cosign-certificate-oidc-issuer="+certificateOidcIssuer)
	}
	if certificateOidcIssuerRegexp, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignCertificateOidcIssuerRegexp].(string); ok {
		args = append(args, "--cosign-certificate-oidc-issuer-regexp="+certificateOidcIssuerRegexp)
	}

	if c.Options.Experimental {
		args = append(args, "--experimental")
	}

	args = append(args, image)

	cmd := c.createNerdctlCmd(ctx, append([]string{"pull"}, args...)...)
	if c.DebugPrintFull {
		log.G(ctx).Debugf("Running %v", cmd.Args)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error while pulling image %s: %w", image, err)
	}
	return nil
}
