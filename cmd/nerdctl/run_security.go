/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"strconv"
	"strings"

	"github.com/AkihiroSuda/nerdctl/pkg/defaults"
	"github.com/containerd/containerd/contrib/apparmor"
	"github.com/containerd/containerd/contrib/seccomp"
	"github.com/containerd/containerd/oci"
	pkgapparmor "github.com/containerd/containerd/pkg/apparmor"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var privilegedOpts = []oci.SpecOpts{
	oci.WithPrivileged,
	oci.WithAllDevicesAllowed,
	oci.WithHostDevices,
	oci.WithNewPrivileges,
}

func generateSecurityOpts(securityOptsMap map[string]string) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts
	if seccompProfile, ok := securityOptsMap["seccomp"]; ok {
		if seccompProfile == "" {
			return nil, errors.New("invalid security-opt \"seccomp\"")
		}

		if seccompProfile != "unconfined" {
			opts = append(opts, seccomp.WithProfile(seccompProfile))
		}
	} else {
		opts = append(opts, seccomp.WithDefaultProfile())
	}

	aaSupported := pkgapparmor.HostSupports()
	if aaProfile, ok := securityOptsMap["apparmor"]; ok {
		if aaProfile == "" {
			return nil, errors.New("invalid security-opt \"apparmor\"")
		}
		if aaProfile != "unconfined" {
			if !aaSupported {
				logrus.Warnf("The host does not support AppArmor. Ignoring profile %q", aaProfile)
			} else {
				opts = append(opts, apparmor.WithProfile(aaProfile))
			}
		}
	} else if aaSupported {
		opts = append(opts, apparmor.WithDefaultProfile(defaults.AppArmorProfileName))
	}

	nnp := false
	if nnpStr, ok := securityOptsMap["no-new-privileges"]; ok {
		if nnpStr == "" {
			nnp = true
		} else {
			var err error
			nnp, err = strconv.ParseBool(nnpStr)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid \"no-new-privileges\" value: %q", nnpStr)
			}
		}
	}
	if !nnp {
		opts = append(opts, oci.WithNewPrivileges)
	}
	return opts, nil
}

func generateCapOpts(capAdd, capDrop []string) ([]oci.SpecOpts, error) {
	if len(capAdd) == 0 && len(capDrop) == 0 {
		return nil, nil
	}

	var opts []oci.SpecOpts
	if InStringSlice(capDrop, "ALL") {
		opts = append(opts, oci.WithCapabilities(nil))
	}

	if InStringSlice(capAdd, "ALL") {
		opts = append(opts, oci.WithAllCurrentCapabilities)
	} else {
		var capsAdd []string
		for _, c := range capAdd {
			capsAdd = append(capsAdd, "CAP_"+strings.ToUpper(c))
		}
		opts = append(opts, oci.WithCapabilities(capsAdd))
	}

	if !InStringSlice(capDrop, "ALL") {
		var capsDrop []string
		for _, c := range capDrop {
			capsDrop = append(capsDrop, "CAP_"+strings.ToUpper(c))
		}
		opts = append(opts, oci.WithDroppedCapabilities(capsDrop))
	}
	return opts, nil
}
