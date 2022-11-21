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
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/pkg/labels"
)

func (c *Composer) getOrphanContainers(ctx context.Context, parsedServices []*serviceparser.Service) ([]containerd.Container, error) {
	// get all running containers for project
	var filters = []string{fmt.Sprintf("labels.%q==%s", labels.ComposeProject, c.project.Name)}
	containers, err := c.client.Containers(ctx, filters...)
	if err != nil {
		return nil, err
	}

	parsedSvcNames := make(map[string]bool)
	for _, svc := range parsedServices {
		parsedSvcNames[svc.Unparsed.Name] = true
	}

	var orphanContainers []containerd.Container
	for _, container := range containers {
		// orphan containers doesn't have a `ComposeService` label corresponding
		// to any name of given services.
		containerLabels, err := container.Labels(ctx)
		if err != nil {
			return nil, fmt.Errorf("error getting container labels: %s", err)
		}
		containerSvc := containerLabels[labels.ComposeService]
		if inServices := parsedSvcNames[containerSvc]; !inServices {
			orphanContainers = append(orphanContainers, container)
		}
	}

	return orphanContainers, nil
}
