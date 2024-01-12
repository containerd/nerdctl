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

package container

import (
	"context"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/runtime/restart"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

func checkRestartCapabilities(ctx context.Context, client *containerd.Client, restartFlag string) (bool, error) {
	policySlice := strings.Split(restartFlag, ":")
	switch policySlice[0] {
	case "", "no":
		return true, nil
	}
	res, err := client.IntrospectionService().Plugins(ctx, []string{"id==restart"})
	if err != nil {
		return false, err
	}
	if len(res.Plugins) == 0 {
		return false, fmt.Errorf("no restart plugin found")
	}
	restartPlugin := res.Plugins[0]
	capabilities := restartPlugin.Capabilities
	if len(capabilities) == 0 {
		capabilities = []string{"always"}
	}
	if !strutil.InStringSlice(capabilities, policySlice[0]) {
		return false, fmt.Errorf("unsupported restart policy %q, supported policies are: %q", policySlice[0], capabilities)
	}
	return true, nil
}

func generateRestartOpts(ctx context.Context, client *containerd.Client, restartFlag, logURI string, inRun bool) ([]containerd.NewContainerOpts, error) {
	if restartFlag == "" || restartFlag == "no" {
		return nil, nil
	}
	if _, err := checkRestartCapabilities(ctx, client, restartFlag); err != nil {
		return nil, err
	}

	policy, err := restart.NewPolicy(restartFlag)
	if err != nil {
		return nil, err
	}
	desireStatus := containerd.Created
	if inRun {
		desireStatus = containerd.Running
	}
	opts := []containerd.NewContainerOpts{restart.WithPolicy(policy), restart.WithStatus(desireStatus)}
	if logURI != "" {
		opts = append(opts, restart.WithLogURIString(logURI))
	}
	return opts, nil
}

// UpdateContainerRestartPolicyLabel updates the restart policy label of the container.
func UpdateContainerRestartPolicyLabel(ctx context.Context, client *containerd.Client, container containerd.Container, restartFlag string) error {
	if _, err := checkRestartCapabilities(ctx, client, restartFlag); err != nil {
		return err
	}
	policy, err := restart.NewPolicy(restartFlag)
	if err != nil {
		return err
	}

	updateOpts := []containerd.UpdateContainerOpts{restart.WithPolicy(policy)}

	lables, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	_, statusLabelExist := lables[restart.StatusLabel]
	if !statusLabelExist {
		task, err := container.Task(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to get task:%w", err)
		}
		desireStatus := containerd.Running
		status, err := task.Status(ctx)
		if err == nil {
			switch status.Status {
			case containerd.Stopped:
				desireStatus = containerd.Stopped
			case containerd.Created:
				desireStatus = containerd.Created
			}
		}
		updateOpts = append(updateOpts, restart.WithStatus(desireStatus))
	}

	return container.Update(ctx, updateOpts...)
}
