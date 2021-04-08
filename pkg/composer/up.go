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

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/pkg/reflectutil"
	"github.com/sirupsen/logrus"
)

type UpOptions struct {
	Detach bool
}

func (c *Composer) Up(ctx context.Context, uo UpOptions) error {
	if unknown := reflectutil.UnknownNonEmptyFields(c.project, "Name", "WorkingDir", "Services", "Networks", "Volumes", "ComposeFiles"); len(unknown) > 0 {
		logrus.Warnf("Ignoring: %+v", unknown)
	}

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

	var parsedServices []*serviceparser.Service
	// use WithServices to sort the services in dependency order
	if err := c.project.WithServices(nil, func(svc types.ServiceConfig) error {
		ps, err := serviceparser.Parse(c.project, svc)
		if err != nil {
			return err
		}
		parsedServices = append(parsedServices, ps)
		return nil
	}); err != nil {
		return err
	}

	return c.upServices(ctx, parsedServices, uo)
}
