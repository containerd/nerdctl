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

package serviceparser

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/types"
	compose "github.com/compose-spec/compose-go/types"
	"github.com/containerd/nerdctl/pkg/reflectutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func warnUnknownFields(svc compose.ServiceConfig) {
	if unknown := reflectutil.UnknownNonEmptyFields(&svc,
		"Name",
		"CapAdd",
		"CapDrop",
		"CPUS",
		"CPUSet",
		"CPUShares",
		"Command",
		"ContainerName",
		"DependsOn",
		"Deploy",
		"DNS",
		"Entrypoint",
		"Environment",
		"Extends", // handled by the loader
		"Hostname",
		"Image",
		"Labels",
		"MemLimit",
		"Networks",
		"PidsLimit",
		"Ports",
		"Privileged",
		"PullPolicy",
		"ReadOnly",
		"Restart",
		"Runtime",
		"Scale",
		"SecurityOpt",
		"Sysctls",
		"User",
		"WorkingDir",
		"Volumes",
	); len(unknown) > 0 {
		logrus.Warnf("Ignoring: service %s: %+v", svc.Name, unknown)
	}

	for depName, dep := range svc.DependsOn {
		if unknown := reflectutil.UnknownNonEmptyFields(&dep,
			"Condition",
		); len(unknown) > 0 {
			logrus.Warnf("Ignoring: service %s: depends_on: %s: %+v", svc.Name, depName, unknown)
		}
		switch dep.Condition {
		case "", types.ServiceConditionStarted:
			// NOP
		default:
			logrus.Warnf("Ignoring: service %s: depends_on: %s: condition %s", svc.Name, depName, dep.Condition)
		}
	}

	if svc.Deploy != nil {
		if unknown := reflectutil.UnknownNonEmptyFields(svc.Deploy,
			"Replicas",
			"RestartPolicy",
			"Resources",
		); len(unknown) > 0 {
			logrus.Warnf("Ignoring: service %s: deploy: %+v", svc.Name, unknown)
		}
		if svc.Deploy.RestartPolicy != nil {
			if unknown := reflectutil.UnknownNonEmptyFields(svc.Deploy.RestartPolicy,
				"Condition",
			); len(unknown) > 0 {
				logrus.Warnf("Ignoring: service %s: deploy.restart_policy: %+v", svc.Name, unknown)
			}
		}
		if unknown := reflectutil.UnknownNonEmptyFields(svc.Deploy.Resources,
			"Limits",
		); len(unknown) > 0 {
			logrus.Warnf("Ignoring: service %s: deploy.resources: %+v", svc.Name, unknown)
		}
		if svc.Deploy.Resources.Limits != nil {
			if unknown := reflectutil.UnknownNonEmptyFields(svc.Deploy.Resources.Limits,
				"NanoCPUs",
				"MemoryBytes",
			); len(unknown) > 0 {
				logrus.Warnf("Ignoring: service %s: deploy.resources.resources: %+v", svc.Name, unknown)
			}
		}
	}
}

type Container struct {
	Name    string   // e.g., "compose-wordpress_wordpress_1"
	RunArgs []string // {"-d", "--pull=never", ...}
}

type Service struct {
	Image      string
	PullMode   string
	Containers []Container // length = replicas
	Unparsed   *compose.ServiceConfig
}

func getReplicas(svc compose.ServiceConfig) (int, error) {
	replicas := 1
	if svc.Scale > 0 {
		logrus.Warn("scale is deprecated, use deploy.replicas")
		replicas = svc.Scale
	}

	if svc.Deploy != nil && svc.Deploy.Replicas != nil {
		if svc.Scale > 0 && int(*svc.Deploy.Replicas) != svc.Scale {
			return 0, errors.New("deploy.replicas and scale (deprecated) must not be set together")
		}
		replicas = int(*svc.Deploy.Replicas)
	}

	if replicas < 1 {
		return 0, errors.Errorf("invalid replicas: %d", replicas)
	}
	return replicas, nil
}

func getCPULimit(svc compose.ServiceConfig) (string, error) {
	var limit string
	if svc.CPUS > 0 {
		logrus.Warn("cpus is deprecated, use deploy.resources.limits.cpus")
		limit = fmt.Sprintf("%f", svc.CPUS)
	}
	if svc.Deploy != nil && svc.Deploy.Resources.Limits != nil {
		if nanoCPUs := svc.Deploy.Resources.Limits.NanoCPUs; nanoCPUs != "" {
			if svc.CPUS > 0 {
				logrus.Warnf("deploy.resources.limits.cpus and cpus (deprecated) must not be set together, ignoring cpus=%f", svc.CPUS)
			}
			limit = nanoCPUs
		}
	}
	return limit, nil
}

func getMemLimit(svc compose.ServiceConfig) (types.UnitBytes, error) {
	var limit types.UnitBytes
	if svc.MemLimit > 0 {
		logrus.Warn("mem_limit is deprecated, use deploy.resources.limits.memory")
		limit = svc.MemLimit
	}
	if svc.Deploy != nil && svc.Deploy.Resources.Limits != nil {
		if memoryBytes := svc.Deploy.Resources.Limits.MemoryBytes; memoryBytes > 0 {
			if svc.MemLimit > 0 && memoryBytes != svc.MemLimit {
				logrus.Warnf("deploy.resources.limits.memory and mem_limit (deprecated) must not be set together, ignoring mem_limit=%d", svc.MemLimit)
			}
			limit = memoryBytes
		}
	}
	return limit, nil
}

// getRestart returns `nerdctl run --restart` flag string ("no" or "always")
//
// restart:                         {"no" (default), "always", "on-failure", "unless-stopped"} (https://github.com/compose-spec/compose-spec/blob/167f207d0a8967df87c5ed757dbb1a2bb6025a1e/spec.md#restart)
// deploy.restart_policy.condition: {"none", "on-falure", "any" (default)}                     (https://github.com/compose-spec/compose-spec/blob/167f207d0a8967df87c5ed757dbb1a2bb6025a1e/deploy.md#restart_policy)
func getRestart(svc compose.ServiceConfig) (string, error) {
	var restartFlag string
	switch svc.Restart {
	case "":
		restartFlag = "no"
	case "no", "always":
		restartFlag = svc.Restart
	case "on-failure", "unless-stopped":
		logrus.Warnf("Ignoring: service %s: restart=%q (unimplemented)", svc.Name, svc.Restart)
	default:
		logrus.Warnf("Ignoring: service %s: restart=%q (unknown)", svc.Name, svc.Restart)
	}

	if svc.Deploy != nil && svc.Deploy.RestartPolicy != nil {
		if svc.Restart != "" {
			logrus.Warnf("deploy.restart_policy and restart must not be set together, ignoring restart=%s", svc.Restart)
		}
		switch cond := svc.Deploy.RestartPolicy.Condition; cond {
		case "", "any":
			restartFlag = "always"
		case "always":
			return "", errors.Errorf("deploy.restart_policy.condition: \"always\" is invalid, did you mean \"any\"?")
		case "none":
			restartFlag = "no"
		case "no":
			return "", errors.Errorf("deploy.restart_policy.condition: \"no\" is invalid, did you mean \"none\"?")
		case "on-failure":
			logrus.Warnf("Ignoring: service %s: deploy.restart_policy.condition=%q (unimplemented)", svc.Name, cond)
		default:
			logrus.Warnf("Ignoring: service %s: deploy.restart_policy.condition=%q (unknown)", svc.Name, cond)
		}
	}

	return restartFlag, nil
}

// getNetworks returns full network names, e.g., {"compose-wordpress_default"}, or {"host"}
func getNetworks(project *compose.Project, svc compose.ServiceConfig) ([]string, error) {
	var fullNames []string // nolint: prealloc

	if svc.Net != "" {
		logrus.Warn("net is deprecated, use network_mode or networks")
		if len(svc.Networks) > 0 {
			return nil, errors.New("networks and net must not be set together")
		}
		fullNames = append(fullNames, svc.Net)
	}

	if svc.NetworkMode != "" {
		if len(svc.Networks) > 0 {
			return nil, errors.New("networks and network_mode must not be set together")
		}
		if svc.Net != "" && svc.NetworkMode != svc.Net {
			return nil, errors.New("net and network_mode must not be set together")
		}
		if strings.Contains(svc.NetworkMode, ":") {
			return nil, errors.Errorf("unsupported network_mode: %q", svc.NetworkMode)
		}
		fullNames = append(fullNames, svc.NetworkMode)
	}

	for shortName := range svc.Networks {
		net, ok := project.Networks[shortName]
		if !ok {
			return nil, errors.Errorf("invalid network %q", shortName)
		}
		fullNames = append(fullNames, net.Name)
	}

	if len(fullNames) > 1 {
		return nil, errors.Errorf("service %s: specifying multiple networks (%v) is not supported yet",
			svc.Name, fullNames)
	}

	return fullNames, nil
}

func Parse(project *compose.Project, svc compose.ServiceConfig) (*Service, error) {
	warnUnknownFields(svc)

	if svc.Image == "" {
		return nil, errors.Errorf("service %s: missing image", svc.Name)
	}

	replicas, err := getReplicas(svc)
	if err != nil {
		return nil, err
	}

	parsed := &Service{
		Image:      svc.Image,
		PullMode:   "missing",
		Containers: make([]Container, replicas),
		Unparsed:   &svc,
	}

	switch svc.PullPolicy {
	case "", types.PullPolicyMissing, types.PullPolicyIfNotPresent:
		// NOP
	case types.PullPolicyAlways, types.PullPolicyNever:
		parsed.PullMode = svc.PullPolicy
	default:
		logrus.Warnf("Ignoring: service %s: pull_policy: %q", svc.Name, svc.PullPolicy)
	}

	for i := 0; i < replicas; i++ {
		container, err := newContainer(project, svc, i)
		if err != nil {
			return nil, err
		}
		parsed.Containers[i] = *container
	}

	return parsed, nil
}

func newContainer(project *compose.Project, svc compose.ServiceConfig, i int) (*Container, error) {
	var c Container
	c.Name = fmt.Sprintf("%s_%s_%d", project.Name, svc.Name, i+1)
	if svc.ContainerName != "" {
		if i != 0 {
			return nil, errors.New("container_name must not be specified when replicas != 1")
		}
		c.Name = svc.ContainerName
	}

	c.RunArgs = []string{
		"--name=" + c.Name,
		"-d",
		"--pull=never", // because image will be ensured before running replicas with `nerdctl run`.
	}

	for _, v := range svc.CapAdd {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--cap-add=%s", v))
	}

	for _, v := range svc.CapDrop {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--cap-drop=%s", v))
	}

	if cpuLimit, err := getCPULimit(svc); err != nil {
		return nil, err
	} else if cpuLimit != "" {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--cpus=%s", cpuLimit))
	}

	if svc.CPUSet != "" {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--cpuset-cpus=%s", svc.CPUSet))
	}

	if svc.CPUShares != 0 {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--cpu-shares=%d", svc.CPUShares))
	}

	for _, v := range svc.DNS {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--dns=%s", v))
	}

	if len(svc.Entrypoint) > 1 {
		return nil, errors.Errorf("service %s: specifying entrypoint with multiple strings (%v) is not supported yet",
			svc.Name, svc.Entrypoint)
	}

	for _, v := range svc.Entrypoint {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--entrypoint=%s", v))
	}

	for k, v := range svc.Environment {
		if v == nil {
			c.RunArgs = append(c.RunArgs, fmt.Sprintf("-e=%s", k))
		} else {
			c.RunArgs = append(c.RunArgs, fmt.Sprintf("-e=%s=%s", k, *v))
		}
	}

	hostname := svc.Hostname
	if hostname == "" {
		hostname = svc.Name
	}
	c.RunArgs = append(c.RunArgs, fmt.Sprintf("--hostname=%s", hostname))

	if memLimit, err := getMemLimit(svc); err != nil {
		return nil, err
	} else if memLimit > 0 {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("-m=%d", memLimit))
	}

	for k, v := range svc.Labels {
		if v == "" {
			c.RunArgs = append(c.RunArgs, fmt.Sprintf("-l=%s", k))
		} else {
			c.RunArgs = append(c.RunArgs, fmt.Sprintf("-l=%s=%s", k, v))
		}
	}

	if networks, err := getNetworks(project, svc); err != nil {
		return nil, err
	} else {
		for _, net := range networks {
			c.RunArgs = append(c.RunArgs, "--net="+net)
		}
	}

	if svc.PidsLimit > 0 {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--pids-limit=%d", svc.PidsLimit))
	}

	for _, p := range svc.Ports {
		pStr, err := servicePortConfigToFlagP(p)
		if err != nil {
			return nil, err
		}
		c.RunArgs = append(c.RunArgs, "-p="+pStr)
	}

	if svc.Privileged {
		c.RunArgs = append(c.RunArgs, "--privileged")
	}

	if svc.ReadOnly {
		c.RunArgs = append(c.RunArgs, "--read-only")
	}

	if restart, err := getRestart(svc); err != nil {
		return nil, err
	} else if restart != "" {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--restart=%s", restart))
	}

	if svc.Runtime != "" {
		c.RunArgs = append(c.RunArgs, "--runtime="+svc.Runtime)
	}

	for _, v := range svc.SecurityOpt {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--security-opt=%s", v))
	}

	for k, v := range svc.Sysctls {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--sysctl=%s=%s", k, v))
	}

	if svc.User != "" {
		c.RunArgs = append(c.RunArgs, "--user="+svc.User)
	}

	for _, v := range svc.Volumes {
		vStr, err := serviceVolumeConfigToFlagV(v, project.Volumes, project.WorkingDir)
		if err != nil {
			return nil, err
		}
		c.RunArgs = append(c.RunArgs, "-v="+vStr)
	}

	if svc.WorkingDir != "" {
		c.RunArgs = append(c.RunArgs, "-w="+svc.WorkingDir)
	}

	c.RunArgs = append(c.RunArgs, svc.Image)
	c.RunArgs = append(c.RunArgs, svc.Command...)
	return &c, nil
}

func servicePortConfigToFlagP(c types.ServicePortConfig) (string, error) {
	if unknown := reflectutil.UnknownNonEmptyFields(&c,
		"Mode",
		"HostIP",
		"Target",
		"Published",
		"Protocol",
	); len(unknown) > 0 {
		logrus.Warnf("Ignoring: port: %+v", unknown)
	}
	switch c.Mode {
	case "", "ingress":
	default:
		return "", errors.Errorf("unsupported port mode: %s", c.Mode)
	}
	if c.Published <= 0 {
		return "", errors.Errorf("unsupported port number: %d", c.Published)
	}
	if c.Target <= 0 {
		return "", errors.Errorf("unsupported port number: %d", c.Target)
	}
	s := fmt.Sprintf("%d:%d", c.Published, c.Target)
	if c.HostIP != "" {
		if strings.Contains(c.HostIP, ":") {
			s = fmt.Sprintf("[%s]:%s", c.HostIP, s)
		} else {
			s = fmt.Sprintf("%s:%s", c.HostIP, s)
		}
	}
	if c.Protocol != "" {
		s = fmt.Sprintf("%s/%s", s, c.Protocol)
	}
	return s, nil
}

func serviceVolumeConfigToFlagV(c types.ServiceVolumeConfig, volumes types.Volumes, projectDir string) (string, error) {
	if unknown := reflectutil.UnknownNonEmptyFields(&c,
		"Type",
		"Source",
		"Target",
		"ReadOnly",
	); len(unknown) > 0 {
		logrus.Warnf("Ignoring: volume: %+v", unknown)
	}

	if c.Target == "" {
		return "", errors.New("volume target is missing")
	}
	if !filepath.IsAbs(c.Target) {
		return "", errors.Errorf("volume target must be an absolute path, got %q", c.Target)
	}

	if c.Source == "" {
		return "", errors.New("volume source is missing")
	}
	var src string
	switch c.Type {
	case "volume":
		vol, ok := volumes[c.Source]
		if !ok {
			return "", errors.Errorf("invalid volume %q", c.Source)
		}
		// c.Source is like "db_data", vol.Name is like "compose-wordpress_db_data"
		src = vol.Name
	case "bind":
		if filepath.IsAbs(c.Source) {
			src = c.Source
		} else {
			// Source can be "../../../../foo", but we do NOT need to use securejoin here.
			src = filepath.Join(projectDir, c.Source)
			var err error
			src, err = filepath.Abs(src)
			if err != nil {
				return "", errors.Wrapf(err, "invalid relative path %q", c.Source)
			}
		}
	default:
		return "", errors.Errorf("unsupported volume type: %q", c.Type)
	}
	s := fmt.Sprintf("%s:%s", src, c.Target)
	if c.ReadOnly {
		s += ":ro"
	}
	return s, nil
}
