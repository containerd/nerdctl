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

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/sirupsen/logrus"
)

func (c *Composer) Containers(ctx context.Context, services ...string) ([]containerd.Container, error) {
	projectLabel := fmt.Sprintf("labels.%q==%s", labels.ComposeProject, c.project.Name)
	filters := []string{}
	for _, service := range services {
		filters = append(filters, fmt.Sprintf("%s,labels.%q==%s", projectLabel, labels.ComposeService, service))
	}
	if len(services) == 0 {
		filters = append(filters, projectLabel)
	}
	logrus.Debugf("filters: %v", filters)
	containers, err := c.client.Containers(ctx, filters...)
	if err != nil {
		return nil, err
	}
	return containers, nil
}
