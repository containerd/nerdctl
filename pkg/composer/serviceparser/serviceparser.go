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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/containerd/contrib/nvidia"
	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/reflectutil"
)

// ComposeExtensionKey defines fields used to implement extension features.
const (
	ComposeVerify                            = "x-nerdctl-verify"
	ComposeCosignPublicKey                   = "x-nerdctl-cosign-public-key"
	ComposeSign                              = "x-nerdctl-sign"
	ComposeCosignPrivateKey                  = "x-nerdctl-cosign-private-key"
	ComposeCosignCertificateIdentity         = "x-nerdctl-cosign-certificate-identity"
	ComposeCosignCertificateIdentityRegexp   = "x-nerdctl-cosign-certificate-identity-regexp"
	ComposeCosignCertificateOidcIssuer       = "x-nerdctl-cosign-certificate-oidc-issuer"
	ComposeCosignCertificateOidcIssuerRegexp = "x-nerdctl-cosign-certificate-oidc-issuer-regexp"
)

// Separator is used for naming components (e.g., service image or container)
// https://github.com/docker/compose/blob/8c39b5b7fd4210a69d07885835f7ff826aaa1cd8/pkg/api/api.go#L483
const Separator = "-"

func warnUnknownFields(svc types.ServiceConfig) {
	if unknown := reflectutil.UnknownNonEmptyFields(&svc,
		"Name",
		"Annotations",
		"Build",
		"BlkioConfig",
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
		"DNSSearch",
		"DNSOpts",
		"Entrypoint",
		"Environment",
		"Extends", // handled by the loader
		"Extensions",
		"ExtraHosts",
		"Hostname",
		"Image",
		"Init",
		"Labels",
		"Logging",
		"MemLimit",
		"Networks",
		"NetworkMode",
		"Pid",
		"PidsLimit",
		"Platform",
		"Ports",
		"Privileged",
		"PullPolicy",
		"ReadOnly",
		"Restart",
		"Runtime",
		"Secrets",
		"Scale",
		"SecurityOpt",
		"ShmSize",
		"StopGracePeriod",
		"StopSignal",
		"Sysctls",
		"StdinOpen",
		"Tmpfs",
		"Tty",
		"User",
		"WorkingDir",
		"Volumes",
		"Ulimits",
	); len(unknown) > 0 {
		log.L.Warnf("Ignoring: service %s: %+v", svc.Name, unknown)
	}

	if svc.BlkioConfig != nil {
		if unknown := reflectutil.UnknownNonEmptyFields(svc.BlkioConfig,
			"Weight",
		); len(unknown) > 0 {
			log.L.Warnf("Ignoring: service %s: blkio_config: %+v", svc.Name, unknown)
		}
	}

	for depName, dep := range svc.DependsOn {
		if unknown := reflectutil.UnknownNonEmptyFields(&dep,
			"Condition",
		); len(unknown) > 0 {
			log.L.Warnf("Ignoring: service %s: depends_on: %s: %+v", svc.Name, depName, unknown)
		}
		switch dep.Condition {
		case "", types.ServiceConditionStarted:
			// NOP
		default:
			log.L.Warnf("Ignoring: service %s: depends_on: %s: condition %s", svc.Name, depName, dep.Condition)
		}
	}

	if svc.Deploy != nil {
		if unknown := reflectutil.UnknownNonEmptyFields(svc.Deploy,
			"Replicas",
			"RestartPolicy",
			"Resources",
		); len(unknown) > 0 {
			log.L.Warnf("Ignoring: service %s: deploy: %+v", svc.Name, unknown)
		}
		if svc.Deploy.RestartPolicy != nil {
			if unknown := reflectutil.UnknownNonEmptyFields(svc.Deploy.RestartPolicy,
				"Condition",
			); len(unknown) > 0 {
				log.L.Warnf("Ignoring: service %s: deploy.restart_policy: %+v", svc.Name, unknown)
			}
		}
		if unknown := reflectutil.UnknownNonEmptyFields(svc.Deploy.Resources,
			"Limits",
			"Reservations",
		); len(unknown) > 0 {
			log.L.Warnf("Ignoring: service %s: deploy.resources: %+v", svc.Name, unknown)
		}
		if svc.Deploy.Resources.Limits != nil {
			if unknown := reflectutil.UnknownNonEmptyFields(svc.Deploy.Resources.Limits,
				"NanoCPUs",
				"MemoryBytes",
			); len(unknown) > 0 {
				log.L.Warnf("Ignoring: service %s: deploy.resources.resources: %+v", svc.Name, unknown)
			}
		}
		if svc.Deploy.Resources.Reservations != nil {
			if unknown := reflectutil.UnknownNonEmptyFields(svc.Deploy.Resources.Reservations,
				"Devices",
			); len(unknown) > 0 {
				log.L.Warnf("Ignoring: service %s: deploy.resources.resources.reservations: %+v", svc.Name, unknown)
			}
			for i, dev := range svc.Deploy.Resources.Reservations.Devices {
				if unknown := reflectutil.UnknownNonEmptyFields(dev,
					"Capabilities",
					"Driver",
					"Count",
					"IDs",
				); len(unknown) > 0 {
					log.L.Warnf("Ignoring: service %s: deploy.resources.resources.reservations.devices[%d]: %+v",
						svc.Name, i, unknown)
				}
			}
		}
	}

	// unknown fields of Build is checked in parseBuild().
}

type Container struct {
	Name    string   // e.g., "compose-wordpress_wordpress_1"
	RunArgs []string // {"--pull=never", ...}
	Mkdir   []string // For Bind.CreateHostPath
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
	Unparsed   *types.ServiceConfig
}

func getReplicas(svc types.ServiceConfig) (int, error) {
	replicas := 1

	// No need to check svc.Scale, as it is automatically transformed to svc.Deploy.Replicas by compose-go
	// https://github.com/compose-spec/compose-go/commit/958cb4f953330a3d1303961796d826b7f79132d7

	if svc.Deploy != nil && svc.Deploy.Replicas != nil {
		replicas = int(*svc.Deploy.Replicas) // nolint:unconvert
	}

	if replicas < 0 {
		return 0, fmt.Errorf("invalid replicas: %d", replicas)
	}
	return replicas, nil
}

func getCPULimit(svc types.ServiceConfig) (string, error) {
	var limit string
	if svc.CPUS > 0 {
		log.L.Warn("cpus is deprecated, use deploy.resources.limits.cpus")
		limit = fmt.Sprintf("%f", svc.CPUS)
	}
	if svc.Deploy != nil && svc.Deploy.Resources.Limits != nil {
		if nanoCPUs := svc.Deploy.Resources.Limits.NanoCPUs; nanoCPUs != 0 {
			if svc.CPUS > 0 {
				log.L.Warnf("deploy.resources.limits.cpus and cpus (deprecated) must not be set together, ignoring cpus=%f", svc.CPUS)
			}
			limit = strconv.FormatFloat(float64(nanoCPUs), 'f', 2, 32)
		}
	}
	return limit, nil
}

func getMemLimit(svc types.ServiceConfig) (types.UnitBytes, error) {
	var limit types.UnitBytes
	if svc.MemLimit > 0 {
		log.L.Warn("mem_limit is deprecated, use deploy.resources.limits.memory")
		limit = svc.MemLimit
	}
	if svc.Deploy != nil && svc.Deploy.Resources.Limits != nil {
		if memoryBytes := svc.Deploy.Resources.Limits.MemoryBytes; memoryBytes > 0 {
			if svc.MemLimit > 0 && memoryBytes != svc.MemLimit {
				log.L.Warnf("deploy.resources.limits.memory and mem_limit (deprecated) must not be set together, ignoring mem_limit=%d", svc.MemLimit)
			}
			limit = memoryBytes
		}
	}
	return limit, nil
}

func getGPUs(svc types.ServiceConfig) (reqs []string, _ error) {
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

var restartFailurePat = regexp.MustCompile(`^on-failure:\d+$`)

// getRestart returns `nerdctl run --restart` flag string
//
// restart:                         {"no" (default), "always", "on-failure", "unless-stopped"} (https://github.com/compose-spec/compose-spec/blob/167f207d0a8967df87c5ed757dbb1a2bb6025a1e/spec.md#restart)
// deploy.restart_policy.condition: {"none", "on-failure", "any" (default)}                    (https://github.com/compose-spec/compose-spec/blob/167f207d0a8967df87c5ed757dbb1a2bb6025a1e/deploy.md#restart_policy)
func getRestart(svc types.ServiceConfig) (string, error) {
	var restartFlag string
	switch svc.Restart {
	case "":
		restartFlag = "no"
	case "no", "always", "on-failure", "unless-stopped":
		restartFlag = svc.Restart
	default:
		if restartFailurePat.MatchString(svc.Restart) {
			restartFlag = svc.Restart
		} else {
			log.L.Warnf("Ignoring: service %s: restart=%q (unknown)", svc.Name, svc.Restart)
		}
	}

	if svc.Deploy != nil && svc.Deploy.RestartPolicy != nil {
		if svc.Restart != "" {
			log.L.Warnf("deploy.restart_policy and restart must not be set together, ignoring restart=%s", svc.Restart)
		}
		switch cond := svc.Deploy.RestartPolicy.Condition; cond {
		case "", "any":
			restartFlag = "always"
		case "always":
			return "", fmt.Errorf("deploy.restart_policy.condition: \"always\" is invalid, did you mean \"any\"?")
		case "none":
			restartFlag = "no"
		case "no":
			return "", fmt.Errorf("deploy.restart_policy.condition: \"no\" is invalid, did you mean \"none\"?")
		case "on-failure":
			log.L.Warnf("Ignoring: service %s: deploy.restart_policy.condition=%q (unimplemented)", svc.Name, cond)
		default:
			log.L.Warnf("Ignoring: service %s: deploy.restart_policy.condition=%q (unknown)", svc.Name, cond)
		}
	}

	return restartFlag, nil
}

type networkNamePair struct {
	shortNetworkName string
	fullName         string
}

// getNetworks returns full network names, e.g., {"compose-wordpress_default"}, or {"host"}
func getNetworks(project *types.Project, svc types.ServiceConfig) ([]networkNamePair, error) {
	var fullNames []networkNamePair // nolint: prealloc

	if svc.Net != "" {
		log.L.Warn("net is deprecated, use network_mode or networks")
		if len(svc.Networks) > 0 {
			return nil, errors.New("networks and net must not be set together")
		}

		fullNames = append(fullNames, networkNamePair{
			fullName:         svc.Net,
			shortNetworkName: "",
		})
	}

	if svc.NetworkMode != "" {
		if len(svc.Networks) > 0 {
			return nil, errors.New("networks and network_mode must not be set together")
		}
		if svc.Net != "" && svc.NetworkMode != svc.Net {
			return nil, errors.New("net and network_mode must not be set together")
		}
		if strings.Contains(svc.NetworkMode, ":") {
			if !strings.HasPrefix(svc.NetworkMode, "container:") {
				return nil, fmt.Errorf("unsupported network_mode: %q", svc.NetworkMode)
			}
		}
		fullNames = append(fullNames, networkNamePair{
			fullName:         svc.NetworkMode,
			shortNetworkName: "",
		})
	}

	for shortName := range svc.Networks {
		net, ok := project.Networks[shortName]
		if !ok {
			return nil, fmt.Errorf("invalid network %q", shortName)
		}
		fullNames = append(fullNames, networkNamePair{
			fullName:         net.Name,
			shortNetworkName: shortName,
		})
	}

	return fullNames, nil
}

func Parse(project *types.Project, svc types.ServiceConfig) (*Service, error) {
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
			return nil, fmt.Errorf("service %s: missing image", svc.Name)
		}
	} else {
		if parsed.Image == "" {
			parsed.Image = DefaultImageName(project.Name, svc.Name)
		}
		parsed.Build, err = parseBuildConfig(svc.Build, project, parsed.Image)
		if err != nil {
			return nil, fmt.Errorf("service %s: failed to parse build: %w", svc.Name, err)
		}
	}

	switch svc.PullPolicy {
	case "", types.PullPolicyMissing, types.PullPolicyIfNotPresent:
		// NOP
	case types.PullPolicyAlways, types.PullPolicyNever:
		parsed.PullMode = svc.PullPolicy
	case types.PullPolicyBuild:
		if parsed.Build == nil {
			return nil, fmt.Errorf("service %s: pull_policy \"build\" requires build config", svc.Name)
		}
		parsed.Build.Force = true
		parsed.PullMode = "never"
	default:
		log.L.Warnf("Ignoring: service %s: pull_policy: %q", svc.Name, svc.PullPolicy)
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

func newContainer(project *types.Project, parsed *Service, i int) (*Container, error) {
	svc := *parsed.Unparsed
	var c Container
	c.Name = DefaultContainerName(project.Name, svc.Name, strconv.Itoa(i+1))
	if svc.ContainerName != "" {
		if i != 0 {
			return nil, errors.New("container_name must not be specified when replicas != 1")
		}
		c.Name = svc.ContainerName
	}

	c.RunArgs = []string{
		"--name=" + c.Name,
		"--pull=never", // because image will be ensured before running replicas with `nerdctl run`.
	}

	for k, v := range svc.Annotations {
		if v == "" {
			c.RunArgs = append(c.RunArgs, fmt.Sprintf("--annotation=%s", k))
		} else {
			c.RunArgs = append(c.RunArgs, fmt.Sprintf("--annotation=%s=%s", k, v))
		}
	}

	if svc.BlkioConfig != nil && svc.BlkioConfig.Weight != 0 {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--blkio-weight=%d", svc.BlkioConfig.Weight))
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
	for _, v := range svc.DNSSearch {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--dns-search=%s", v))
	}
	for _, v := range svc.DNSOpts {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--dns-option=%s", v))
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
	for k, v := range svc.ExtraHosts {
		for _, h := range v {
			c.RunArgs = append(c.RunArgs, fmt.Sprintf("--add-host=%s:%s", k, h))
		}
	}

	if svc.Init != nil && *svc.Init {
		c.RunArgs = append(c.RunArgs, "--init")
	}

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

	if svc.Logging != nil {
		if svc.Logging.Driver != "" {
			c.RunArgs = append(c.RunArgs, fmt.Sprintf("--log-driver=%s", svc.Logging.Driver))
		}
		if svc.Logging.Options != nil {
			for k, v := range svc.Logging.Options {
				c.RunArgs = append(c.RunArgs, fmt.Sprintf("--log-opt=%s=%s", k, v))
			}
		}
	}

	networks, err := getNetworks(project, svc)
	if err != nil {
		return nil, err
	}
	netTypeContainer := false
	for _, net := range networks {
		if strings.HasPrefix(net.fullName, "container:") {
			netTypeContainer = true
		}
		c.RunArgs = append(c.RunArgs, "--net="+net.fullName)
		if value, ok := svc.Networks[net.shortNetworkName]; ok {
			if value != nil && value.Ipv4Address != "" {
				c.RunArgs = append(c.RunArgs, "--ip="+value.Ipv4Address)
			}
		}
	}

	if netTypeContainer && svc.Hostname != "" {
		return nil, fmt.Errorf("conflicting options: hostname and container network mode")
	}
	if !netTypeContainer {
		hostname := svc.Hostname
		if hostname == "" {
			hostname = svc.Name
		}
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--hostname=%s", hostname))
	}

	if svc.Pid != "" {
		c.RunArgs = append(c.RunArgs, "--pid="+svc.Pid)
	}

	if svc.PidsLimit > 0 {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--pids-limit=%d", svc.PidsLimit))
	}

	if svc.Ulimits != nil {
		for utype, ulimit := range svc.Ulimits {
			if ulimit.Single != 0 {
				c.RunArgs = append(c.RunArgs, fmt.Sprintf("--ulimit=%s=%d", utype, ulimit.Single))
			} else {
				c.RunArgs = append(c.RunArgs, fmt.Sprintf("--ulimit=%s=%d:%d", utype, ulimit.Soft, ulimit.Hard))
			}
		}
	}

	if svc.Platform != "" {
		c.RunArgs = append(c.RunArgs, "--platform="+svc.Platform)
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

	if svc.StopGracePeriod != nil {
		timeout := time.Duration(*svc.StopGracePeriod)
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--stop-timeout=%d", int(timeout.Seconds())))
	}
	if svc.StopSignal != "" {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--stop-signal=%s", svc.StopSignal))
	}

	if restart, err := getRestart(svc); err != nil {
		return nil, err
	} else if restart != "" {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--restart=%s", restart))
	}

	if svc.Runtime != "" {
		c.RunArgs = append(c.RunArgs, "--runtime="+svc.Runtime)
	}

	if svc.ShmSize > 0 {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--shm-size=%d", svc.ShmSize))
	}

	for _, v := range svc.SecurityOpt {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--security-opt=%s", v))
	}

	for k, v := range svc.Sysctls {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--sysctl=%s=%s", k, v))
	}

	if svc.StdinOpen {
		c.RunArgs = append(c.RunArgs, "--interactive")
	}

	if svc.User != "" {
		c.RunArgs = append(c.RunArgs, "--user="+svc.User)
	}

	for _, v := range svc.GroupAdd {
		c.RunArgs = append(c.RunArgs, fmt.Sprintf("--group-add=%s", v))
	}

	for _, v := range svc.Volumes {
		vStr, mkdir, err := serviceVolumeConfigToFlagV(v, project)
		if err != nil {
			return nil, err
		}
		c.RunArgs = append(c.RunArgs, "-v="+vStr)
		c.Mkdir = mkdir
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

	for _, tmpfs := range svc.Tmpfs {
		c.RunArgs = append(c.RunArgs, "--tmpfs="+tmpfs)
	}

	if svc.Tty {
		c.RunArgs = append(c.RunArgs, "--tty")
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
		log.L.Warnf("Ignoring: port: %+v", unknown)
	}
	switch c.Mode {
	case "", "ingress":
	default:
		return "", fmt.Errorf("unsupported port mode: %s", c.Mode)
	}
	if c.Target <= 0 {
		return "", fmt.Errorf("unsupported port number: %d", c.Target)
	}
	s := fmt.Sprintf("%s:%d", c.Published, c.Target)
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

func serviceVolumeConfigToFlagV(c types.ServiceVolumeConfig, project *types.Project) (flagV string, mkdir []string, err error) {
	if unknown := reflectutil.UnknownNonEmptyFields(&c,
		"Type",
		"Source",
		"Target",
		"ReadOnly",
		"Bind",
		"Volume",
	); len(unknown) > 0 {
		log.L.Warnf("Ignoring: volume: %+v", unknown)
	}
	if c.Bind != nil {
		// c.Bind is expected to be a non-nil reference to an empty Bind struct
		if unknown := reflectutil.UnknownNonEmptyFields(c.Bind, "CreateHostPath"); len(unknown) > 0 {
			log.L.Warnf("Ignoring: volume: Bind: %+v", unknown)
		}
	}
	if c.Volume != nil {
		// c.Volume is expected to be a non-nil reference to an empty Volume struct
		if unknown := reflectutil.UnknownNonEmptyFields(c.Volume); len(unknown) > 0 {
			log.L.Warnf("Ignoring: volume: Volume: %+v", unknown)
		}
	}

	if c.Target == "" {
		return "", nil, errors.New("volume target is missing")
	}
	if !filepath.IsAbs(c.Target) {
		return "", nil, fmt.Errorf("volume target must be an absolute path, got %q", c.Target)
	}

	if c.Source == "" {
		// anonymous volume
		s := c.Target
		if c.ReadOnly {
			s += ":ro"
		}
		return s, mkdir, nil
	}

	var src string
	switch c.Type {
	case "volume":
		vol, ok := project.Volumes[c.Source]
		if !ok {
			return "", nil, fmt.Errorf("invalid volume %q", c.Source)
		}
		// c.Source is like "db_data", vol.Name is like "compose-wordpress_db_data"
		src = vol.Name
	case "bind":
		src = project.RelativePath(c.Source)
		var err error
		src, err = filepath.Abs(src)
		if err != nil {
			return "", nil, fmt.Errorf("invalid relative path %q: %w", c.Source, err)
		}
		if c.Bind != nil && c.Bind.CreateHostPath {
			if _, stErr := os.Stat(src); errors.Is(stErr, os.ErrNotExist) {
				mkdir = append(mkdir, src)
			}
		}
	default:
		return "", nil, fmt.Errorf("unsupported volume type: %q", c.Type)
	}
	s := fmt.Sprintf("%s:%s", src, c.Target)
	if c.ReadOnly {
		s += ":ro"
	}
	return s, mkdir, nil
}

func fileReferenceConfigToFlagV(c types.FileReferenceConfig, project *types.Project, secret bool) (string, error) {
	objType := "config"
	if secret {
		objType = "secret"
	}
	if unknown := reflectutil.UnknownNonEmptyFields(&c,
		"Source", "Target", "UID", "GID", "Mode",
	); len(unknown) > 0 {
		log.L.Warnf("Ignoring: %s: %+v", objType, unknown)
	}

	if err := identifiers.Validate(c.Source); err != nil {
		return "", fmt.Errorf("%s source %q is invalid: %w", objType, c.Source, err)
	}

	var obj types.FileObjectConfig
	if secret {
		secret, ok := project.Secrets[c.Source]
		if !ok {
			return "", fmt.Errorf("secret %s is undefined", c.Source)
		}
		obj = types.FileObjectConfig(secret)
	} else {
		config, ok := project.Configs[c.Source]
		if !ok {
			return "", fmt.Errorf("config %s is undefined", c.Source)
		}
		obj = types.FileObjectConfig(config)
	}
	src := project.RelativePath(obj.File)
	var err error
	src, err = filepath.Abs(src)
	if err != nil {
		return "", fmt.Errorf("%s %s: invalid relative path %q: %w", objType, c.Source, src, err)
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
				return "", fmt.Errorf("config %s: target %q must be an absolute path", c.Source, c.Target)
			}
		}
	}

	if c.UID != "" {
		// Raise an error rather than ignoring the value, for avoiding any security issue
		return "", fmt.Errorf("%s %s: unsupported field: UID", objType, c.Source)
	}
	if c.GID != "" {
		return "", fmt.Errorf("%s %s: unsupported field: GID", objType, c.Source)
	}
	if c.Mode != nil {
		return "", fmt.Errorf("%s %s: unsupported field: Mode", objType, c.Source)
	}

	s := fmt.Sprintf("%s:%s:ro", src, target)
	return s, nil
}

// DefaultImageName returns the image name following compose naming logic.
func DefaultImageName(projectName string, serviceName string) string {
	return projectName + Separator + serviceName
}

// DefaultContainerName returns the service container name following compose naming logic.
func DefaultContainerName(projectName, serviceName, suffix string) string {
	return DefaultImageName(projectName, serviceName) + Separator + suffix
}
