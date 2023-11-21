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
	"errors"
	"strings"
	"sync"

	"github.com/containerd/containerd/contrib/apparmor"
	"github.com/containerd/containerd/contrib/seccomp"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/pkg/cap"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/apparmorutil"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/maputil"
	"github.com/containerd/nerdctl/pkg/strutil"
)

var privilegedOpts = []oci.SpecOpts{
	oci.WithPrivileged,
	oci.WithAllDevicesAllowed,
	oci.WithHostDevices,
	oci.WithNewPrivileges,
}

var privilegedWithoutDevicesOpts = []oci.SpecOpts{
	oci.WithPrivileged,
	oci.WithNewPrivileges,
}

func generateSecurityOpts(privileged bool, securityOptsMap map[string]string) ([]oci.SpecOpts, error) {
	for k := range securityOptsMap {
		switch k {
		case "seccomp", "apparmor", "no-new-privileges", "privileged-without-host-devices":
		default:
			log.L.Warnf("unknown security-opt: %q", k)
		}
	}
	var opts []oci.SpecOpts
	if seccompProfile, ok := securityOptsMap["seccomp"]; ok && seccompProfile != defaults.SeccompProfileName {
		if seccompProfile == "" {
			return nil, errors.New("invalid security-opt \"seccomp\"")
		}

		if seccompProfile != "unconfined" {
			opts = append(opts, seccomp.WithProfile(seccompProfile))
		}
	} else {
		opts = append(opts, seccomp.WithDefaultProfile())
	}

	canLoadNewAppArmor := apparmorutil.CanLoadNewProfile()
	canApplyExistingProfile := apparmorutil.CanApplyExistingProfile()
	if aaProfile, ok := securityOptsMap["apparmor"]; ok {
		if aaProfile == "" {
			return nil, errors.New("invalid security-opt \"apparmor\"")
		}
		if aaProfile != "unconfined" {
			if !canApplyExistingProfile {
				log.L.Warnf("the host does not support AppArmor. Ignoring profile %q", aaProfile)
			} else {
				opts = append(opts, apparmor.WithProfile(aaProfile))
			}
		}
	} else {
		if canLoadNewAppArmor {
			if err := apparmor.LoadDefaultProfile(defaults.AppArmorProfileName); err != nil {
				return nil, err
			}
		}
		if apparmorutil.CanApplySpecificExistingProfile(defaults.AppArmorProfileName) {
			opts = append(opts, apparmor.WithProfile(defaults.AppArmorProfileName))
		}
	}

	nnp, err := maputil.MapBoolValueAsOpt(securityOptsMap, "no-new-privileges")
	if err != nil {
		return nil, err
	}

	if !nnp {
		opts = append(opts, oci.WithNewPrivileges)
	}

	privilegedWithoutHostDevices, err := maputil.MapBoolValueAsOpt(securityOptsMap, "privileged-without-host-devices")
	if err != nil {
		return nil, err
	}

	if privilegedWithoutHostDevices && !privileged {
		return nil, errors.New("flag `--security-opt privileged-without-host-devices` can't be used without `--privileged` enabled")
	}

	if privileged {
		if privilegedWithoutHostDevices {
			opts = append(opts, privilegedWithoutDevicesOpts...)
		} else {
			opts = append(opts, privilegedOpts...)
		}
	}

	return opts, nil
}

func canonicalizeCapName(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ToUpper(s)
	if !strings.HasPrefix(s, "CAP_") {
		s = "CAP_" + s
	}
	if !isKnownCapName(s) {
		log.L.Warnf("unknown capability name %q", s)
		// Not a fatal error, because runtime might be aware of this cap
	}
	return s
}

var (
	knownCapNames     map[string]struct{}
	knownCapNamesOnce sync.Once
)

func isKnownCapName(s string) bool {
	knownCapNamesOnce.Do(func() {
		known := cap.Known()
		knownCapNames = make(map[string]struct{}, len(known))
		for _, f := range known {
			knownCapNames[f] = struct{}{}
		}
	})
	_, ok := knownCapNames[s]
	return ok
}

func generateCapOpts(capAdd, capDrop []string) ([]oci.SpecOpts, error) {
	if len(capAdd) == 0 && len(capDrop) == 0 {
		return nil, nil
	}

	var opts []oci.SpecOpts
	if strutil.InStringSlice(capDrop, "ALL") {
		opts = append(opts, oci.WithCapabilities(nil))
	}

	if strutil.InStringSlice(capAdd, "ALL") {
		opts = append(opts, oci.WithAllCurrentCapabilities)
	} else {
		var capsAdd []string
		for _, c := range capAdd {
			capsAdd = append(capsAdd, canonicalizeCapName(c))
		}
		opts = append(opts, oci.WithAddedCapabilities(capsAdd))
	}

	if !strutil.InStringSlice(capDrop, "ALL") {
		var capsDrop []string
		for _, c := range capDrop {
			capsDrop = append(capsDrop, canonicalizeCapName(c))
		}
		opts = append(opts, oci.WithDroppedCapabilities(capsDrop))
	}
	return opts, nil
}
