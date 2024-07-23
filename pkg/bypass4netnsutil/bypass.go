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

package bypass4netnsutil

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"

	"github.com/containerd/errdefs"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/v2/pkg/annotations"
	b4nnapi "github.com/rootless-containers/bypass4netns/pkg/api"
	"github.com/rootless-containers/bypass4netns/pkg/api/daemon/client"
	rlkclient "github.com/rootless-containers/rootlesskit/v2/pkg/api/client"
)

func NewBypass4netnsCNIBypassManager(client client.Client, rlkClient rlkclient.Client, annotationsMap map[string]string) (*Bypass4netnsCNIBypassManager, error) {
	if client == nil || rlkClient == nil {
		return nil, errdefs.ErrInvalidArgument
	}
	enabled, bindEnabled, err := IsBypass4netnsEnabled(annotationsMap)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, errdefs.ErrInvalidArgument
	}
	var ignoreSubnets []string
	if v := annotationsMap[annotations.Bypass4netnsIgnoreSubnets]; v != "" {
		if err := json.Unmarshal([]byte(v), &ignoreSubnets); err != nil {
			return nil, fmt.Errorf("failed to unmarshal annotation %q: %q: %w", annotations.Bypass4netnsIgnoreSubnets, v, err)
		}
	}
	pm := &Bypass4netnsCNIBypassManager{
		Client:        client,
		rlkClient:     rlkClient,
		ignoreSubnets: ignoreSubnets,
		ignoreBind:    !bindEnabled,
	}
	return pm, nil
}

type Bypass4netnsCNIBypassManager struct {
	client.Client
	rlkClient     rlkclient.Client
	ignoreSubnets []string
	ignoreBind    bool
}

func (b4nnm *Bypass4netnsCNIBypassManager) StartBypass(ctx context.Context, ports []gocni.PortMapping, id, stateDir string) error {
	socketPath, err := GetSocketPathByID(id)
	if err != nil {
		return err
	}
	pidFilePath, err := GetPidFilePathByID(id)
	if err != nil {
		return err
	}
	logFilePath := filepath.Join(stateDir, "bypass4netns.log")

	rlkInfo, err := b4nnm.rlkClient.Info(ctx)
	if err != nil {
		return err
	}
	if rlkInfo.NetworkDriver == nil {
		return fmt.Errorf("no network driver is set in RootlessKit info: %+v", rlkInfo)
	}
	rlkIP := rlkInfo.NetworkDriver.ChildIP
	const mask = 24 // currently hard-coded
	rlkCIDR := fmt.Sprintf("%s/%d", rlkIP.Mask(net.CIDRMask(mask, 32)), mask)

	spec := b4nnapi.BypassSpec{
		ID:          id,
		SocketPath:  socketPath,
		PidFilePath: pidFilePath,
		LogFilePath: logFilePath,
		// "auto" can detect CNI CIDRs automatically
		IgnoreSubnets: append([]string{"127.0.0.0/8", rlkCIDR, "auto"}, b4nnm.ignoreSubnets...),
		IgnoreBind:    b4nnm.ignoreBind,
	}
	if !b4nnm.ignoreBind {
		portMap := []b4nnapi.PortSpec{}
		for _, p := range ports {
			portMap = append(portMap, b4nnapi.PortSpec{
				ParentIP:   p.HostIP,
				ParentPort: int(p.HostPort),
				ChildPort:  int(p.ContainerPort),
				Protos:     []string{p.Protocol},
			})
		}
		spec.PortMapping = portMap
	}
	_, err = b4nnm.BypassManager().StartBypass(ctx, spec)
	if err != nil {
		return err
	}

	return nil
}

func (b4nnm *Bypass4netnsCNIBypassManager) StopBypass(ctx context.Context, id string) error {
	err := b4nnm.BypassManager().StopBypass(ctx, id)
	if err != nil {
		return err
	}

	return nil
}
