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
	"os/user"
	"strconv"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/rootless-containers/rootlesskit/pkg/parent/idtools"
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

func generateUserNSOpts(userns string) ([]oci.SpecOpts, error) {
	switch userns {
	case "host":
		return []oci.SpecOpts{withResetUserNamespace()}, nil
	case "keep-id":
		min := func(a, b int) int {
			if a < b {
				return a
			}
			return b
		}

		uid := rootlessutil.ParentEUID()
		gid := rootlessutil.ParentEGID()

		u, err := user.LookupId(fmt.Sprintf("%d", uid))
		if err != nil {
			return nil, err
		}
		uids, gids, err := idtools.GetSubIDRanges(uid, u.Username)
		if err != nil {
			return nil, err
		}

		maxUID, maxGID := 0, 0
		for _, u := range uids {
			maxUID += u.Length
		}
		for _, g := range gids {
			maxGID += g.Length
		}

		uidmap := []specs.LinuxIDMapping{{
			ContainerID: uint32(uid),
			HostID:      0,
			Size:        1,
		}}
		if len(uids) > 0 {
			uidmap = append(uidmap, specs.LinuxIDMapping{
				ContainerID: 0,
				HostID:      1,
				Size:        uint32(min(uid, maxUID)),
			})
		}

		gidmap := []specs.LinuxIDMapping{{
			ContainerID: uint32(gid),
			HostID:      0,
			Size:        1,
		}}
		if len(gids) > 0 {
			gidmap = append(gidmap, specs.LinuxIDMapping{
				ContainerID: 0,
				HostID:      1,
				Size:        uint32(min(gid, maxGID)),
			})
		}
		return []oci.SpecOpts{
			oci.WithUserNamespace(uidmap, gidmap),
			oci.WithUIDGID(uint32(uid), uint32(gid)),
		}, nil
	default:
		return nil, fmt.Errorf("invalid UserNS Value:%s", userns)
	}
}

func withResetUserNamespace() oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		for i, ns := range s.Linux.Namespaces {
			if ns.Type == specs.UserNamespace {
				s.Linux.Namespaces = append(s.Linux.Namespaces[:i], s.Linux.Namespaces[i+1:]...)
			}
		}
		return nil
	}
}
