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

package apparmorutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/moby/sys/userns"

	"github.com/containerd/containerd/v2/pkg/apparmor"
	"github.com/containerd/log"
)

// CanLoadNewProfile returns whether the current process can load a new AppArmor profile.
//
// CanLoadNewProfile needs root.
//
// CanLoadNewProfile checks both /sys/module/apparmor/parameters/enabled and /sys/kernel/security.
//
// Related: https://gitlab.com/apparmor/apparmor/-/blob/v3.0.3/libraries/libapparmor/src/kernel.c#L311
func CanLoadNewProfile() bool {
	return !userns.RunningInUserNS() && os.Geteuid() == 0 && apparmor.HostSupports()
}

var (
	paramEnabled     bool
	paramEnabledOnce sync.Once
)

// CanApplyExistingProfile returns whether the current process can apply an existing AppArmor profile
// to processes.
//
// CanApplyExistingProfile does NOT need root.
//
// CanApplyExistingProfile checks /sys/module/apparmor/parameters/enabled ,but does NOT check /sys/kernel/security/apparmor ,
// which might not be accessible from user namespaces (because securityfs cannot be mounted in a user namespace)
//
// Related: https://gitlab.com/apparmor/apparmor/-/blob/v3.0.3/libraries/libapparmor/src/kernel.c#L311
func CanApplyExistingProfile() bool {
	paramEnabledOnce.Do(func() {
		buf, err := os.ReadFile("/sys/module/apparmor/parameters/enabled")
		paramEnabled = err == nil && len(buf) > 1 && buf[0] == 'Y'
	})
	return paramEnabled
}

// CanApplySpecificExistingProfile attempts to run `aa-exec -p <NAME> -- true` to check whether
// the profile can be applied.
//
// CanApplySpecificExistingProfile does NOT depend on /sys/kernel/security/apparmor/profiles ,
// which might not be accessible from user namespaces (because securityfs cannot be mounted in a user namespace)
func CanApplySpecificExistingProfile(profileName string) bool {
	if !CanApplyExistingProfile() {
		return false
	}
	cmd := exec.Command("aa-exec", "-p", profileName, "--", "true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.L.WithError(err).Debugf("failed to run %v: %q", cmd.Args, string(out))
		return false
	}
	return true
}

type Profile struct {
	Name string `json:"Name"`           // e.g., "nerdctl-default"
	Mode string `json:"Mode,omitempty"` // e.g., "enforce"
}

// Profiles return profiles.
//
// Profiles does not need the root but needs access to /sys/kernel/security/apparmor/policy/profiles,
// which might not be accessible from user namespaces (because securityfs cannot be mounted in a user namespace)
//
// So, Profiles cannot be called from rootless child.
func Profiles() ([]Profile, error) {
	const profilesPath = "/sys/kernel/security/apparmor/policy/profiles"
	ents, err := os.ReadDir(profilesPath)
	if err != nil {
		return nil, err
	}
	res := make([]Profile, len(ents))
	for i, ent := range ents {
		namePath := filepath.Join(profilesPath, ent.Name(), "name")
		b, err := os.ReadFile(namePath)
		if err != nil {
			log.L.WithError(err).Warnf("failed to read %q", namePath)
			continue
		}
		profile := Profile{
			Name: strings.TrimSpace(string(b)),
		}
		modePath := filepath.Join(profilesPath, ent.Name(), "mode")
		b, err = os.ReadFile(modePath)
		if err != nil {
			log.L.WithError(err).Warnf("failed to read %q", namePath)
		} else {
			profile.Mode = strings.TrimSpace(string(b))
		}
		res[i] = profile
	}
	return res, nil
}

// Unload unloads a profile. Needs access to /sys/kernel/security/apparmor/.remove .
func Unload(target string) error {
	remover, err := os.OpenFile("/sys/kernel/security/apparmor/.remove", os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := remover.Write([]byte(target)); err != nil {
		remover.Close()
		return err
	}
	return remover.Close()
}
