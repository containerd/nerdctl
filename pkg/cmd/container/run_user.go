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
	"context"
	"fmt"
	"strconv"

	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/oci"
)

func generateUserOpts(user string) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts
	if user != "" {
		opts = append(opts, oci.WithUser(user), withResetAdditionalGIDs(), oci.WithAdditionalGIDs(user))
	}
	return opts, nil
}

func generateUmaskOpts(umask string) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts

	if umask != "" {
		decVal, err := strconv.ParseUint(umask, 8, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid Umask Value:%s", umask)
		}
		opts = append(opts, withAdditionalUmask(uint32(decVal)))
	}
	return opts, nil
}

func generateGroupsOpts(groups []string) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts

	if len(groups) != 0 {
		opts = append(opts, oci.WithAppendAdditionalGroups(groups...))
	}
	return opts, nil
}

func withResetAdditionalGIDs() oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Process.User.AdditionalGids = nil
		return nil
	}
}

func withAdditionalUmask(umask uint32) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Process.User.Umask = &umask
		return nil
	}
}
