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

/*
   Portions from https://github.com/moby/moby/blob/cff4f20c44a3a7c882ed73934dec6a77246c6323/pkg/sysinfo/sysinfo_linux_test.go
   Copyright (C) Docker/Moby authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/moby/moby/blob/cff4f20c44a3a7c882ed73934dec6a77246c6323/NOTICE
*/

package sysinfo // import "github.com/docker/docker/pkg/sysinfo"

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

func TestReadProcBool(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-sysinfo-proc")
	assert.NilError(t, err)
	defer os.RemoveAll(tmpDir)

	procFile := filepath.Join(tmpDir, "read-proc-bool")
	err = os.WriteFile(procFile, []byte("1"), 0o644)
	assert.NilError(t, err)

	if !readProcBool(procFile) {
		t.Fatal("expected proc bool to be true, got false")
	}

	if err := os.WriteFile(procFile, []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if readProcBool(procFile) {
		t.Fatal("expected proc bool to be false, got true")
	}

	if readProcBool(path.Join(tmpDir, "no-exist")) {
		t.Fatal("should be false for non-existent entry")
	}
}

func TestCgroupEnabled(t *testing.T) {
	cgroupDir, err := os.MkdirTemp("", "cgroup-test")
	assert.NilError(t, err)
	defer os.RemoveAll(cgroupDir)

	if cgroupEnabled(cgroupDir, "test") {
		t.Fatal("cgroupEnabled should be false")
	}

	err = os.WriteFile(path.Join(cgroupDir, "test"), []byte{}, 0o644)
	assert.NilError(t, err)

	if !cgroupEnabled(cgroupDir, "test") {
		t.Fatal("cgroupEnabled should be true")
	}
}

func TestNew(t *testing.T) {
	sysInfo := New()
	assert.Assert(t, sysInfo != nil)
	checkSysInfo(t, sysInfo)
}

func checkSysInfo(t *testing.T, sysInfo *SysInfo) {
	// Check if Seccomp is supported, via CONFIG_SECCOMP.then sysInfo.Seccomp must be TRUE , else FALSE
	if err := unix.Prctl(unix.PR_GET_SECCOMP, 0, 0, 0, 0); err != unix.EINVAL {
		// Make sure the kernel has CONFIG_SECCOMP_FILTER.
		if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, 0, 0, 0); err != unix.EINVAL {
			assert.Assert(t, sysInfo.Seccomp)
		}
	} else {
		assert.Assert(t, !sysInfo.Seccomp)
	}
}

func TestNewAppArmorEnabled(t *testing.T) {
	// Check if AppArmor is supported. then it must be TRUE , else FALSE
	if _, err := os.Stat("/sys/kernel/security/apparmor"); err != nil {
		t.Skip("AppArmor Must be Enabled")
	}

	// FIXME: rootless is not allowed to read the profile
	if rootlessutil.IsRootless() {
		t.Skip("containerd v2 aftermath: test skipped for rootless")
	}
	sysInfo := New()
	assert.Assert(t, sysInfo.AppArmor)
}

func TestNewAppArmorDisabled(t *testing.T) {
	// Check if AppArmor is supported. then it must be TRUE , else FALSE
	if _, err := os.Stat("/sys/kernel/security/apparmor"); !os.IsNotExist(err) {
		t.Skip("AppArmor Must be Disabled")
	}

	sysInfo := New()
	assert.Assert(t, !sysInfo.AppArmor)
}

func TestNewCgroupNamespacesEnabled(t *testing.T) {
	// If cgroup namespaces are supported in the kernel, then sysInfo.CgroupNamespaces should be TRUE
	if _, err := os.Stat("/proc/self/ns/cgroup"); err != nil {
		t.Skip("cgroup namespaces must be enabled")
	}

	sysInfo := New()
	assert.Assert(t, sysInfo.CgroupNamespaces)
}

func TestNewCgroupNamespacesDisabled(t *testing.T) {
	// If cgroup namespaces are *not* supported in the kernel, then sysInfo.CgroupNamespaces should be FALSE
	if _, err := os.Stat("/proc/self/ns/cgroup"); !os.IsNotExist(err) {
		t.Skip("cgroup namespaces must be disabled")
	}

	sysInfo := New()
	assert.Assert(t, !sysInfo.CgroupNamespaces)
}

func TestNumCPU(t *testing.T) {
	cpuNumbers := NumCPU()
	if cpuNumbers <= 0 {
		t.Fatal("CPU returned must be greater than zero")
	}
}
