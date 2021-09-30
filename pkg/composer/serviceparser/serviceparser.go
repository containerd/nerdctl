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
	"bytes"
	"encoding/csv"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/types"
	compose "github.com/compose-spec/compose-go/types"
	"github.com/containerd/containerd/contrib/nvidia"
	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/nerdctl/pkg/reflectutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func warnUnknownFields(svc compose.ServiceConfig) {
	if unknown := reflectutil.UnknownNonEmptyFields(&svc,
		"Name",
		"Build",
		"CapAdd",
		"CapDrop",
		"CPUS",
		"CPUSet",
		"CPUShares",
		"Command",
		"Configs",
		"ContainerName",
		"DependsOn",
		"Deploy",
		"Devices",
		"Dockerfile", // handled by the loader (normalizer)
		"DNS",
		"Entrypoint",
		"Environment",
		"Extends", // handled by the loader
		"ExtraHosts",
		"Hostname",
		"Image",
		"Labels",
		"MemLimit",
		"Networks",
		"NetworkMode",
		"Pid",
		"PidsLimit",
		"Ports",
		"Privileged",
		"PullPolicy",
		"ReadOnly",
		"Restart",
		"Runtime",
		"Secrets",
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
			"Reservations",
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
		if svc.Deploy.Resources.Reservations != nil {
			if unknown := reflectutil.UnknownNonEmptyFields(svc.Deploy.Resources.Reservations,
				"Devices",
			); len(unknown) > 0 {
				logrus.Warnf("Ignoring: service %s: deploy.resources.resources.reservations: %+v", svc.Name, unknown)
			}
			for i, dev := range svc.Deploy.Resources.Reservations.Devices {
				if unknown := reflectutil.UnknownNonEmptyFields(dev,
					"Capabilities",
					"Driver",
					"Count",
					"IDs",
				); len(unknown) > 0 {
					logrus.Warnf("Ignoring: service %s: deploy.resources.resources.reservations.devices[%d]: %+v",
						svc.Name, i, unknown)
				}
			}
		}
	}

	// unknown fields of Build is checked in parseBuild().
}

type Container struct {
	Name    string   // e.g., "compose-wordpress_wordpress_1"
	RunArgs []string // {"-d", "--pull=never", ...}
}

type Build struct {
	Force     bool     // force build even if already present
	BuildArgs []string // {"-t", "example.com/foo", "--target", "foo", "/path/to/ctx"}
	// TODO: call BuildKit API directly without executing `nerdctl build`
}

type Service struct {
	Image      string
	PullMode   string
	Containers []Container // length = replicas
	Build      *Build
	Unparsed   *compose.ServiceConfig
}

func getReplicas(svc compose.ServiceConfig) (int, error) {
	replicas := 1

	// No need to check svc.Scale, as it is automatically transformed to svc.Deploy.Replicas by compose-go
	// https://github.com/compose-spec/compose-go/commit/958cb4f953330a3d1303961796d826b7f79132d7

	if svc.Deploy != nil && svc.Deploy.Replicas != nil {
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

func getGPUs(svc compose.ServiceConfig) (reqs []string, _ error) {
	// "gpu" and "nvidia" are also allowed capabilities (but not used as nvidia driver capabilities)
	// https://github.com/moby/moby/blob/v20.10.7/daemon/nvidia_linux.go#L37
	capset := map[string]struct{}{"gpu": {}, "nvidia": {}}
	for _, c := range nvidia.AllCaps() {
		capset[string(c)] = struct{}{}
	}
	if svc.Deploy != nil && svc.Deploy.Resources.Reservations != nil {
		for _, dev := range svc.Deploy.Resources.Reservations.Devices {
			if len(dev.Capabilities) == 0 {
				// "capabilities" is required.
				// https://github.com/compose-spec/compose-spec/blob/74b933db994109616580eab8f47bf2ba226e0faa/deploy.md#devices
				return nil, fmt.Errorf("service %s: specifying \"capabilities\" is required for resource reservations", svc.Name)
			}

			var requiresGPU bool
			for _, c := range dev.Capabilities {
				if _, ok := capset[c]; ok {
					requiresGPU = true
				}
			}
			if !requiresGPU {
				continue
			}

			var e []string
			if len(dev.Capabilities) > 0 {
				e = append(e, fmt.Sprintf("capabilities=%s", strings.Join(dev.Capabilities, ",")))
			}
			if dev.Driver != "" {
				e = append(e, fmt.Sprintf("driver=%s", dev.Driver))
			}
			if len(dev.IDs) > 0 {
				e = append(e, fmt.Sprintf("device=%s", strings.Join(dev.IDs, ",")))
			}
			if dev.Count != 0 {
				e = append(e, fmt.Sprintf("count=%d", dev.Count))
			}

			buf := new(bytes.Buffer)
			w := csv.NewWriter(buf)
			if err := w.Write(e); err != nil {
				return nil, err
			}
			w.Flush()
			o := buf.Bytes()
			if len(o) > 0 {
				reqs = append(reqs, string(o[:len(o)-1])) // remove carriage return
			}
		}
	}
	return reqs, nil
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

	return fullNames, nil
}

func Parse(project *compose.Project, svc compose.ServiceConfig) (*Service, error) {
	warnUnknownFields(svc)

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

	if svc.Build == nil {
		if parsed.Image == "" {
			return nil, errors.Errorf("service %s: missing image", svc.Name)
		}
	} else {
		if parsed.Image == "" {
			parsed.Image = fmt.Sprintf("%s_%s", project.Name, svc.Name)
		}
		parsed.Build, err = parseBuildConfig(svc.Build, project, parsed.Image)
		if err != nil {
			return nil, errors.Wrapf(err, "service %s: failed to parse build", svc.Name)
		}
	}

	switch svc.PullPolicy {
	case "", types.PullPolicyMissing, types.PullPolicyIfNotPresent:
		// NOP
	case types.PullPolicyAlways, types.PullPolicyNever:
		parsed.PullMode = svc.PullPolicy
	case types.PullPolicyBuild:
		if parsed.Build == nil {
			return nil, errors.Errorf("service %s: pull_policy \"build\" requires build config", svc.Name)
		}
		parsed.Build.Force = true
		parsed.PullMode = "never"
	default:
		logrus.Warnf("Ignoring: service %s: pull_policy: %q", svc.Name, svc.PullPolicy)
	}

	for i := 0; i < replicas; i++ {
		container, err := newContainer(project, parsed, i)
		if err != nil {
			return nil, err
		}
		parsed.Containers[i] = *container
	}

	return parsed, nil
}

func newContainer(project *compose.Project, parsed *Service, i int) (*Container, error) {
	svc := *parsed.Unparsed
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

	for _, v := range svc.Devices {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--device=%s", v))
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
	for _, v := range svc.ExtraHosts {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--add-host=%s", v))
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

	if gpuReqs, err := getGPUs(svc); err != nil {
		return nil, err
	} else if len(gpuReqs) > 0 {
		for _, gpus := range gpuReqs {
			c.RunArgs = append(c.RunArgs, fmt.Sprintf("--gpus=%s", gpus))
		}
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

	if svc.Pid != "" {
		c.RunArgs = append(c.RunArgs, "--pid="+svc.Pid)
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
		vStr, err := serviceVolumeConfigToFlagV(v, project)
		if err != nil {
			return nil, err
		}
		c.RunArgs = append(c.RunArgs, "-v="+vStr)
	}

	for _, config := range svc.Configs {
		fileRef := types.FileReferenceConfig(config)
		vStr, err := fileReferenceConfigToFlagV(fileRef, project, false)
		if err != nil {
			return nil, err
		}
		c.RunArgs = append(c.RunArgs, "-v="+vStr)
	}

	for _, secret := range svc.Secrets {
		fileRef := types.FileReferenceConfig(secret)
		vStr, err := fileReferenceConfigToFlagV(fileRef, project, true)
		if err != nil {
			return nil, err
		}
		c.RunArgs = append(c.RunArgs, "-v="+vStr)
	}

	if svc.WorkingDir != "" {
		c.RunArgs = append(c.RunArgs, "-w="+svc.WorkingDir)
	}

	c.RunArgs = append(c.RunArgs, parsed.Image) // NOT svc.Image
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

func serviceVolumeConfigToFlagV(c types.ServiceVolumeConfig, project *types.Project) (string, error) {
	if unknown := reflectutil.UnknownNonEmptyFields(&c,
		"Type",
		"Source",
		"Target",
		"ReadOnly",
		"Bind",
		"Volume",
	); len(unknown) > 0 {
		logrus.Warnf("Ignoring: volume: %+v", unknown)
	}
	if c.Bind != nil {
		// c.Bind is expected to be a non-nil reference to an empty Bind struct
		if unknown := reflectutil.UnknownNonEmptyFields(c.Bind); len(unknown) > 0 {
			logrus.Warnf("Ignoring: volume: Bind: %+v", unknown)
		}
	}
	if c.Volume != nil {
		// c.Volume is expected to be a non-nil reference to an empty Volume struct
		if unknown := reflectutil.UnknownNonEmptyFields(c.Volume); len(unknown) > 0 {
			logrus.Warnf("Ignoring: volume: Volume: %+v", unknown)
		}
	}

	if c.Target == "" {
		return "", errors.New("volume target is missing")
	}
	if !filepath.IsAbs(c.Target) {
		return "", errors.Errorf("volume target must be an absolute path, got %q", c.Target)
	}

	if c.Source == "" {
		// anonymous volume
		s := c.Target
		if c.ReadOnly {
			s += ":ro"
		}
		return s, nil
	}

	var src string
	switch c.Type {
	case "volume":
		vol, ok := project.Volumes[c.Source]
		if !ok {
			return "", errors.Errorf("invalid volume %q", c.Source)
		}
		// c.Source is like "db_data", vol.Name is like "compose-wordpress_db_data"
		src = vol.Name
	case "bind":
		src = project.RelativePath(c.Source)
		var err error
		src, err = filepath.Abs(src)
		if err != nil {
			return "", errors.Wrapf(err, "invalid relative path %q", c.Source)
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

func fileReferenceConfigToFlagV(c types.FileReferenceConfig, project *types.Project, secret bool) (string, error) {
	objType := "config"
	if secret {
		objType = "secret"
	}
	if unknown := reflectutil.UnknownNonEmptyFields(&c,
		"Source", "Target", "UID", "GID", "Mode",
	); len(unknown) > 0 {
		logrus.Warnf("Ignoring: %s: %+v", objType, unknown)
	}

	if err := identifiers.Validate(c.Source); err != nil {
		return "", errors.Wrapf(err, "%s source %q is invalid", objType, c.Source)
	}

	var obj types.FileObjectConfig
	if secret {
		secret, ok := project.Secrets[c.Source]
		if !ok {
			return "", errors.Errorf("secret %s is undefined", c.Source)
		}
		obj = types.FileObjectConfig(secret)
	} else {
		config, ok := project.Configs[c.Source]
		if !ok {
			return "", errors.Errorf("config %s is undefined", c.Source)
		}
		obj = types.FileObjectConfig(config)
	}
	src := project.RelativePath(obj.File)
	var err error
	src, err = filepath.Abs(src)
	if err != nil {
		return "", errors.Wrapf(err, "%s %s: invalid relative path %q", objType, c.Source, src)
	}

	target := c.Target
	if target == "" {
		if secret {
			target = filepath.Join("/run/secrets", c.Source)
		} else {
			target = filepath.Join("/", c.Source)
		}
	} else {
		target = filepath.Clean(target)
		if !filepath.IsAbs(target) {
			if secret {
				target = filepath.Join("/run/secrets", target)
			} else {
				return "", errors.Errorf("config %s: target %q must be an absolute path", c.Source, c.Target)
			}
		}
	}

	if c.UID != "" {
		// Raise an error rather than ignoring the value, for avoiding any security issue
		return "", errors.Errorf("%s %s: unsupported field: UID", objType, c.Source)
	}
	if c.GID != "" {
		return "", errors.Errorf("%s %s: unsupported field: GID", objType, c.Source)
	}
	if c.Mode != nil {
		return "", errors.Errorf("%s %s: unsupported field: Mode", objType, c.Source)
	}

	s := fmt.Sprintf("%s:%s:ro", src, target)
	return s, nil
}
