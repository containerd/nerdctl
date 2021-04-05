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

	"github.com/containerd/nerdctl/pkg/reflectutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (c *Composer) upNetwork(ctx context.Context, shortName string) error {
	net, ok := c.project.Networks[shortName]
	if !ok {
		return errors.Errorf("invalid network name %q", shortName)
	}
	if net.External.External {
		// NOP
		return nil
	}

	if unknown := reflectutil.UnknownNonEmptyFields(&net, "Name"); len(unknown) > 0 {
		logrus.Warnf("Ignoring: network %s: %+v", shortName, unknown)
	}

	// shortName is like "default", fullName is like "compose-wordpress_default"
	fullName := net.Name
	netExists, err := c.NetworkExists(fullName)
	if err != nil {
		return err
	} else if !netExists {
		logrus.Infof("Creating network %s", fullName)
		if err := c.runNerdctlCmd(ctx, "network", "create", fullName); err != nil {
			return err
		}
	}
	return nil
}
