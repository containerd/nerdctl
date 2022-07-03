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

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/runtime/restart"
	"github.com/containerd/nerdctl/pkg/strutil"
)

func generateRestartOpts(ctx context.Context, client *containerd.Client, restartFlag, logURI string) ([]containerd.NewContainerOpts, error) {
	if _, err := checkRestartCapabilities(ctx, client, restartFlag); err != nil {
		return nil, err
	}
	policy, err := restart.NewPolicy(restartFlag)
	if err != nil {
		return nil, err
	}
	opts := []containerd.NewContainerOpts{restart.WithPolicy(policy), restart.WithStatus(containerd.Running)}
	if logURI != "" {
		opts = append(opts, restart.WithLogURIString(logURI))
	}
	return opts, nil
}

func updateContainerStoppedLabel(ctx context.Context, container containerd.Container, stopped bool) error {
	opt := containerd.WithAdditionalContainerLabels(map[string]string{
		restart.ExplicitlyStoppedLabel: strconv.FormatBool(stopped),
	})
	return container.Update(ctx, containerd.UpdateContainerOpts(opt))
}

func updateContainerRestartPolicyLabel(ctx context.Context, client *containerd.Client, container containerd.Container, policyLabel string) error {
	if _, err := checkRestartCapabilities(ctx, client, policyLabel); err != nil {
		return err
	}
	policy, err := restart.NewPolicy(policyLabel)
	if err != nil {
		return err
	}
	return container.Update(ctx, restart.WithPolicy(policy))
}

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
		return false, fmt.Errorf("unsupported restart policy %q, supported policies are: %q", policySlice[0], restartPlugin.Capabilities)
	}
	return true, nil
}
