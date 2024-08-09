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
	"runtime"
	"strings"

	"github.com/docker/docker/pkg/meminfo"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/sysinfo"
)

const UnameO = "Microsoft Windows"

// MsiNTProductType is the product type of the operating system.
// https://learn.microsoft.com/en-us/windows/win32/msi/msintproducttype
// Ref: https://docs.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-osversioninfoexa
const (
	verNTServer = 0x0000003
)

type windowsInfoUtil interface {
	RtlGetVersion() *windows.OsVersionInfoEx
	GetRegistryStringValue(key registry.Key, path string, name string) (string, error)
	GetRegistryIntValue(key registry.Key, path string, name string) (int, error)
}

type winInfoUtil struct{}

// RtlGetVersion implements the RtlGetVersion method using the actual windows package
func (sw *winInfoUtil) RtlGetVersion() *windows.OsVersionInfoEx {
	return windows.RtlGetVersion()
}

// UnameR returns the Kernel version
func UnameR() string {
	util := &winInfoUtil{}
	version, err := getKernelVersion(util)
	if err != nil {
		log.L.Error(err.Error())
	}

	return version
}

// UnameM returns the architecture of the system
func UnameM() string {
	arch := runtime.GOARCH

	if strings.ToLower(arch) == "amd64" {
		return "x86_64"
	}

	// "386": 32-bit Intel/AMD processors (x86 architecture)
	if strings.ToLower(arch) == "386" {
		return "x86"
	}

	// arm, s390x, and so on
	return arch
}

// DistroName returns version information about the currently running operating system
func DistroName() string {
	util := &winInfoUtil{}
	version, err := distroName(util)
	if err != nil {
		log.L.Error(err.Error())
	}

	return version
}

func distroName(sw windowsInfoUtil) (string, error) {
	// Get the OS version information from the Windows registry
	regPath := `SOFTWARE\Microsoft\Windows NT\CurrentVersion`

	// Eg. 22631 (REG_SZ)
	currBuildNo, err := sw.GetRegistryStringValue(registry.LOCAL_MACHINE, regPath, "CurrentBuildNumber")
	if err != nil {
		return "", fmt.Errorf("failed to get os version (build number) %v", err)
	}

	// Eg. 23H2 (REG_SZ)
	displayVersion, err := sw.GetRegistryStringValue(registry.LOCAL_MACHINE, regPath, "DisplayVersion")
	if err != nil {
		return "", fmt.Errorf("failed to get os version (display version) %v", err)
	}

	// UBR: Update Build Revision. Eg. 3737 (REG_DWORD 32-bit Value)
	ubr, err := sw.GetRegistryIntValue(registry.LOCAL_MACHINE, regPath, "UBR")
	if err != nil {
		return "", fmt.Errorf("failed to get os version (ubr) %v", err)
	}

	productType := ""
	if isWindowsServer(sw) {
		productType = "Server"
	}

	// Concatenate the reg.key values to get the OS version information
	// Example: "Microsoft Windows Version 23H2 (OS Build 22631.3737)"
	versionString := fmt.Sprintf("%s %s Version %s (OS Build %s.%d)",
		UnameO,
		productType,
		displayVersion,
		currBuildNo,
		ubr,
	)

	// Replace double spaces with single spaces
	versionString = strings.ReplaceAll(versionString, "  ", " ")

	return versionString, nil
}

func getKernelVersion(sw windowsInfoUtil) (string, error) {
	// Get BuildLabEx value from the Windows registry
	// [buiild number].[revision number].[architecture].[branch].[date]-[time]
	// Eg. "BuildLabEx: 10240.16412.amd64fre.th1.150729-1800"
	buildLab, err := sw.GetRegistryStringValue(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "BuildLabEx")
	if err != nil {
		return "", err
	}

	// Get Version: Contains the major and minor version numbers of the operating system.
	// Eg. "10.0"
	osvi := sw.RtlGetVersion()

	// Concatenate the OS version and BuildLabEx values to get the Kernel version information
	// Example: "10.0 22631 (10240.16412.amd64fre.th1.150729-1800)"
	version := fmt.Sprintf("%d.%d %d (%s)", osvi.MajorVersion, osvi.MinorVersion, osvi.BuildNumber, buildLab)
	return version, nil
}

// GetRegistryStringValue retrieves a string value from the Windows registry
func (sw *winInfoUtil) GetRegistryStringValue(key registry.Key, path string, name string) (string, error) {
	k, err := registry.OpenKey(key, path, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()

	v, _, err := k.GetStringValue(name)
	if err != nil {
		return "", err
	}
	return v, nil
}

// GetRegistryIntValue retrieves an integer value from the Windows registry
func (sw *winInfoUtil) GetRegistryIntValue(key registry.Key, path string, name string) (int, error) {
	k, err := registry.OpenKey(key, path, registry.QUERY_VALUE)
	if err != nil {
		return 0, err
	}
	defer k.Close()

	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

func isWindowsServer(sw windowsInfoUtil) bool {
	osvi := sw.RtlGetVersion()
	return osvi.ProductType == verNTServer
}

// Cgroups not supported on Windows
func CgroupsVersion() string {
	return ""
}

func fulfillPlatformInfo(info *dockercompat.Info) {
	mobySysInfo := mobySysInfo(info)

	// NOTE: cgroup fields are not available on Windows
	// https://techcommunity.microsoft.com/t5/containers/introducing-the-host-compute-service-hcs/ba-p/382332

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

func mobySysInfo(_ *dockercompat.Info) *sysinfo.SysInfo {
	var sysinfo sysinfo.SysInfo
	return &sysinfo
}
