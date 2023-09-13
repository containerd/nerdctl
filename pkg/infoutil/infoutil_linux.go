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

package infoutil

import (
	"fmt"
	"strings"

	"github.com/containerd/cgroups/v3"
	"github.com/containerd/nerdctl/pkg/apparmorutil"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/docker/docker/pkg/meminfo"
	"github.com/docker/docker/pkg/sysinfo"
)

const UnameO = "GNU/Linux"

func CgroupsVersion() string {
	if cgroups.Mode() == cgroups.Unified {
		return "2"
	}

	return "1"
}

func fulfillSecurityOptions(info *dockercompat.Info) {
	if apparmorutil.CanApplyExistingProfile() {
		info.SecurityOptions = append(info.SecurityOptions, "name=apparmor")
		if rootlessutil.IsRootless() && !apparmorutil.CanApplySpecificExistingProfile(defaults.AppArmorProfileName) {
			info.Warnings = append(info.Warnings, fmt.Sprintf(strings.TrimSpace(`
WARNING: AppArmor profile %q is not loaded.
         Use 'sudo nerdctl apparmor load' if you prefer to use AppArmor with rootless mode.
         This warning is negligible if you do not intend to use AppArmor.`), defaults.AppArmorProfileName))
		}
	}
	info.SecurityOptions = append(info.SecurityOptions, "name=seccomp,profile="+defaults.SeccompProfileName)
	if defaults.CgroupnsMode() == "private" {
		info.SecurityOptions = append(info.SecurityOptions, "name=cgroupns")
	}
	if rootlessutil.IsRootlessChild() {
		info.SecurityOptions = append(info.SecurityOptions, "name=rootless")
	}
}

// fulfillPlatformInfo fulfills cgroup and kernel info.
//
// fulfillPlatformInfo requires the following fields to be set:
// SecurityOptions, CgroupDriver, CgroupVersion
func fulfillPlatformInfo(info *dockercompat.Info) {
	fulfillSecurityOptions(info)
	mobySysInfo := mobySysInfo(info)

	if info.CgroupDriver == "none" {
		if info.CgroupVersion == "2" {
			info.Warnings = append(info.Warnings, "WARNING: Running in rootless-mode without cgroups. Systemd is required to enable cgroups in rootless-mode.")
		} else {
			info.Warnings = append(info.Warnings, "WARNING: Running in rootless-mode without cgroups. To enable cgroups in rootless-mode, you need to boot the system in cgroup v2 mode.")
		}
	} else {
		info.MemoryLimit = mobySysInfo.MemoryLimit
		if !info.MemoryLimit {
			info.Warnings = append(info.Warnings, "WARNING: No memory limit support")
		}
		info.SwapLimit = mobySysInfo.SwapLimit
		if !info.SwapLimit {
			info.Warnings = append(info.Warnings, "WARNING: No swap limit support")
		}
		info.CPUCfsPeriod = mobySysInfo.CPUCfs
		if !info.CPUCfsPeriod {
			info.Warnings = append(info.Warnings, "WARNING: No cpu cfs period support")
		}
		info.CPUCfsQuota = mobySysInfo.CPUCfs
		if !info.CPUCfsQuota {
			info.Warnings = append(info.Warnings, "WARNING: No cpu cfs quota support")
		}
		info.CPUShares = mobySysInfo.CPUShares
		if !info.CPUShares {
			info.Warnings = append(info.Warnings, "WARNING: No cpu shares support")
		}
		info.CPUSet = mobySysInfo.Cpuset
		if !info.CPUSet {
			info.Warnings = append(info.Warnings, "WARNING: No cpuset support")
		}
		info.PidsLimit = mobySysInfo.PidsLimit
		if !info.PidsLimit {
			info.Warnings = append(info.Warnings, "WARNING: No pids limit support")
		}
		info.OomKillDisable = mobySysInfo.OomKillDisable
		if !info.OomKillDisable && info.CgroupVersion == "1" {
			// no warning for cgroup v2
			info.Warnings = append(info.Warnings, "WARNING: No oom kill disable support")
		}
	}
	info.IPv4Forwarding = !mobySysInfo.IPv4ForwardingDisabled
	if !info.IPv4Forwarding {
		info.Warnings = append(info.Warnings, "WARNING: IPv4 forwarding is disabled")
	}
	info.BridgeNfIptables = !mobySysInfo.BridgeNFCallIPTablesDisabled
	if !info.BridgeNfIptables {
		info.Warnings = append(info.Warnings, "WARNING: bridge-nf-call-iptables is disabled")
	}
	info.BridgeNfIP6tables = !mobySysInfo.BridgeNFCallIP6TablesDisabled
	if !info.BridgeNfIP6tables {
		info.Warnings = append(info.Warnings, "WARNING: bridge-nf-call-ip6tables is disabled")
	}
	info.NCPU = sysinfo.NumCPU()
	memLimit, err := meminfo.Read()
	if err != nil {
		info.Warnings = append(info.Warnings, fmt.Sprintf("failed to read mem info: %v", err))
	} else {
		info.MemTotal = memLimit.MemTotal
	}
}

func mobySysInfo(info *dockercompat.Info) *sysinfo.SysInfo {
	var mobySysInfoOpts []sysinfo.Opt
	if info.CgroupDriver == "systemd" && info.CgroupVersion == "2" && rootlessutil.IsRootless() {
		g := fmt.Sprintf("/user.slice/user-%d.slice", rootlessutil.ParentEUID())
		mobySysInfoOpts = append(mobySysInfoOpts, sysinfo.WithCgroup2GroupPath(g))
	}
	mobySysInfo := sysinfo.New(mobySysInfoOpts...)
	return mobySysInfo
}
