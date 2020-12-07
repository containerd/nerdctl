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

	"github.com/containerd/containerd/contrib/seccomp"
	"github.com/containerd/containerd/oci"
	"github.com/pkg/errors"
)

var privilegedOpts = []oci.SpecOpts{
	oci.WithPrivileged,
	oci.WithAllDevicesAllowed,
	oci.WithHostDevices,
	oci.WithNewPrivileges,
}

func generateSecurityOpts(securityOptsMap map[string]string) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts
	if seccompProfile := securityOptsMap["seccomp"]; seccompProfile != "" {
		if seccompProfile != "unconfined" {
			opts = append(opts, seccomp.WithProfile(seccompProfile))
		}
	} else {
		opts = append(opts, seccomp.WithDefaultProfile())
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
