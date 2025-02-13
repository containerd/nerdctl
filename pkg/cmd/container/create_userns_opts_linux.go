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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/moby/sys/user"
	"github.com/opencontainers/runtime-spec/specs-go"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/oci"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/netutil/nettype"
)

// IDMap contains a single entry for user namespace range remapping. An array
// of IDMap entries represents the structure that will be provided to the Linux
// kernel for creating a user namespace.
type IDMap struct {
	ContainerID int `json:"container_id"`
	HostID      int `json:"host_id"`
	Size        int `json:"size"`
}

// IdentityMapping contains a mappings of UIDs and GIDs.
// The zero value represents an empty mapping.
type IdentityMapping struct {
	UIDMaps []IDMap `json:"UIDMaps"`
	GIDMaps []IDMap `json:"GIDMaps"`
}

const (
	capabRemapIDs = "remap-ids"
)

func getUserNamespaceOpts(
	ctx context.Context,
	client *containerd.Client,
	options *types.ContainerCreateOptions,
	ensuredImage imgutil.EnsuredImage,
	id string,
) ([]oci.SpecOpts, []containerd.NewContainerOpts, error) {
	if isDefaultUserns(options) {
		return nil, createDefaultSnapshotOpts(id, ensuredImage), nil
	}

	supportsRemap, err := snapshotterSupportsRemapLabels(ctx, client, ensuredImage.Snapshotter)
	if err != nil {
		return nil, nil, err
	} else if !supportsRemap {
		return nil, nil, errors.New("snapshotter does not support remap-ids capability")
	}

	idMapping, err := loadAndValidateIDMapping(options.UserNS)
	if err != nil {
		return nil, nil, err
	}

	uidMaps, gidMaps := convertMappings(idMapping)
	specOpts := getUserNamespaceSpecOpts(uidMaps, gidMaps)
	snapshotOpts, err := createSnapshotOpts(id, ensuredImage, uidMaps, gidMaps)
	if err != nil {
		return nil, nil, err
	}

	return specOpts, snapshotOpts, nil
}

// getContainerUserNamespaceNetOpts retrieves the user namespace path for the specified container.
func getContainerUserNamespaceNetOpts(
	ctx context.Context,
	client *containerd.Client,
	netManager containerutil.NetworkOptionsManager,
) ([]oci.SpecOpts, error) {
	netOpts, err := netManager.InternalNetworkingOptionLabels(ctx)
	if err != nil {
		return nil, err
	}
	netType, err := nettype.Detect(netOpts.NetworkSlice)
	if err != nil {
		return nil, err
	} else if netType == nettype.Container {
		containerName, err := getContainerNameFromNetworkSlice(netOpts)
		if err != nil {
			return nil, err
		}

		container, err := findContainer(ctx, client, containerName)
		if err != nil {
			return nil, err
		}

		if err := validateContainerStatus(ctx, container); err != nil {
			return nil, err
		}

		userNsPath, err := getUserNamespacePath(ctx, container)
		if err != nil {
			return nil, err
		}

		var userNameSpaceSpecOpts []oci.SpecOpts
		userNameSpaceSpecOpts = append(userNameSpaceSpecOpts, oci.WithLinuxNamespace(specs.LinuxNamespace{
			Type: specs.UserNamespace,
			Path: userNsPath,
		}))
		return userNameSpaceSpecOpts, nil
	} else if netType == nettype.Namespace {
		netNsPath, err := getNamespacePathFromNetworkSlice(netOpts)
		if err != nil {
			return nil, err
		}
		userNsPath, err := getUserNamespacePathFromNetNsPath(netNsPath)
		if err != nil {
			return nil, err
		}
		var userNameSpaceSpecOpts []oci.SpecOpts
		userNameSpaceSpecOpts = append(userNameSpaceSpecOpts, oci.WithLinuxNamespace(specs.LinuxNamespace{
			Type: specs.UserNamespace,
			Path: userNsPath,
		}))
		return userNameSpaceSpecOpts, nil

	}
	return []oci.SpecOpts{}, nil
}

func getNamespacePathFromNetworkSlice(netOpts types.NetworkOptions) (string, error) {
	if len(netOpts.NetworkSlice) > 1 {
		return "", fmt.Errorf("only one network namespace is supported")
	}
	netItems := strings.Split(netOpts.NetworkSlice[0], ":")
	if len(netItems) < 2 {
		return "", fmt.Errorf("namespace networking argument format must be 'ns:<path>', got: %q", netOpts.NetworkSlice[0])
	}
	return netItems[1], nil
}

func getUserNamespacePathFromNetNsPath(netNsPath string) (string, error) {
	var path string
	var maxSymlinkDepth = 255
	depth := 0
	for {
		var err error
		path, err = os.Readlink(netNsPath)
		if err != nil {
			break
		} else if depth > maxSymlinkDepth {
			return "", fmt.Errorf("EvalSymlinks: too many links")
		}

		depth++
		_, err = os.Readlink(path)
		if err != nil {
			break
		} else if depth > maxSymlinkDepth {
			return "", fmt.Errorf("EvalSymlinks: too many links")
		}

		netNsPath = path
		depth++
	}
	matched, err := regexp.MatchString(`^/proc/\d+/ns/net$`, netNsPath)
	if err != nil {
		return "", err
	} else if !matched {
		return "", fmt.Errorf("path is not of the form /proc/<pid>/ns/net, unable to resolve user namespace")
	}
	userNsPath := filepath.Join(filepath.Dir(netNsPath), "user")

	return userNsPath, nil
}

func convertIDMapToLinuxIDMapping(idMaps []IDMap) []specs.LinuxIDMapping {
	// Create a slice to hold the resulting LinuxIDMapping structs
	linuxIDMappings := make([]specs.LinuxIDMapping, len(idMaps))

	// Iterate through the IDMap slice and convert each one
	for i, idMap := range idMaps {
		linuxIDMappings[i] = specs.LinuxIDMapping{
			ContainerID: uint32(idMap.ContainerID),
			HostID:      uint32(idMap.HostID),
			Size:        uint32(idMap.Size),
		}
	}

	// Return the converted slice
	return linuxIDMappings
}

// findContainer searches for a container by name and returns it if found.
func findContainer(
	ctx context.Context,
	client *containerd.Client,
	containerName string,
) (containerd.Container, error) {
	var container containerd.Container

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(_ context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple containers found with prefix: %s", containerName)
			}
			container = found.Container
			return nil
		},
	}

	if n, err := walker.Walk(ctx, containerName); err != nil {
		return container, err
	} else if n == 0 {
		return container, fmt.Errorf("container not found: %s", containerName)
	}

	return container, nil
}

// validateContainerStatus checks if the container is running.
func validateContainerStatus(ctx context.Context, container containerd.Container) error {
	task, err := container.Task(ctx, nil)
	if err != nil {
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	if status.Status != containerd.Running {
		return fmt.Errorf("container %s is not running", container.ID())
	}

	return nil
}

// getUserNamespacePath returns the path to the container's user namespace.
func getUserNamespacePath(ctx context.Context, container containerd.Container) (string, error) {
	task, err := container.Task(ctx, nil)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("/proc/%d/ns/user", task.Pid()), nil
}

// Determines if the default UserNS should be used.
func isDefaultUserns(options *types.ContainerCreateOptions) bool {
	return options.UserNS == "" || options.UserNS == "host"
}

// Creates default snapshot options.
func createDefaultSnapshotOpts(id string, image imgutil.EnsuredImage) []containerd.NewContainerOpts {
	return []containerd.NewContainerOpts{
		containerd.WithNewSnapshot(id, image.Image),
	}
}

// parseGroup parses a string identifier (name or ID) and returns the corresponding group
func parseGroup(identifier string) (user.Group, bool, error) {
	id, err := strconv.Atoi(identifier)
	if err == nil {
		grp, err := user.LookupGid(id)
		if err != nil {
			return user.Group{}, true, fmt.Errorf("could not get group for gid %d: %w", id, err)
		}
		return grp, true, nil
	}

	grp, err := user.LookupGroup(identifier)
	if err != nil {
		return user.Group{}, false, fmt.Errorf("could not get group for groupname %s: %w", identifier, err)
	}
	return grp, false, nil
}

// parseIdentifier parses a string identifier (name or ID) and returns the corresponding user
func parseUser(identifier string) (user.User, bool, error) {
	id, err := strconv.Atoi(identifier)
	if err == nil {
		usr, err := user.LookupUid(id)
		if err != nil {
			return user.User{}, true, fmt.Errorf("could not get user for uid %d: %w", id, err)
		}
		return usr, true, nil
	}

	usr, err := user.LookupUser(identifier)
	if err != nil {
		return user.User{}, false, fmt.Errorf("could not get user for username %s: %w", identifier, err)
	}
	return usr, false, nil
}

func getUserAndGroup(spec string) (user.User, user.Group, error) {
	parts := strings.Split(spec, ":")
	if len(parts) > 2 {
		return user.User{}, user.Group{}, fmt.Errorf("invalid identity mapping format: %s", spec)
	} else if len(parts) == 2 && (parts[0] == "" || parts[1] == "") {
		return user.User{}, user.Group{}, fmt.Errorf("invalid identity mapping format: %s", spec)
	}

	userPart := parts[0]
	usr, _, err := parseUser(userPart)
	if err != nil {
		return user.User{}, user.Group{}, err
	}

	var groupPart string
	if len(parts) == 1 {
		groupPart = userPart
	} else {
		groupPart = parts[1]
	}

	group, _, err := parseGroup(groupPart)
	if err != nil {
		return user.User{}, user.Group{}, err
	}

	return usr, group, nil
}

// LoadIdentityMapping takes a requested identity mapping specification and
// using the data from /etc/sub{uid,gid} ranges, creates the
// uid and gid remapping ranges for that user/group pair.
// The specification can be in the following formats:
// (format: <name|uid>[:<group|gid>])
func LoadIdentityMapping(spec string) (IdentityMapping, error) {
	usr, groupUsr, err := getUserAndGroup(spec)
	if err != nil {
		return IdentityMapping{}, err
	}
	subuidRanges, err := lookupUserSubRangesFile("/etc/subuid", usr)
	if err != nil {
		return IdentityMapping{}, err
	}

	subgidRanges, err := lookupGroupSubRangesFile("/etc/subgid", groupUsr)
	if err != nil {
		return IdentityMapping{}, err
	}

	return IdentityMapping{
		UIDMaps: subuidRanges,
		GIDMaps: subgidRanges,
	}, nil
}

func lookupUserSubRangesFile(path string, usr user.User) ([]IDMap, error) {
	uidstr := strconv.Itoa(usr.Uid)
	rangeList, err := user.ParseSubIDFileFilter(path, func(sid user.SubID) bool {
		return sid.Name == usr.Name || sid.Name == uidstr
	})
	if err != nil {
		return nil, err
	}
	if len(rangeList) == 0 {
		return nil, fmt.Errorf("no subuid ranges found for user %q", usr.Name)
	}

	idMap := []IDMap{}

	containerID := 0
	for _, idrange := range rangeList {
		idMap = append(idMap, IDMap{
			ContainerID: containerID,
			HostID:      int(idrange.SubID),
			Size:        int(idrange.Count),
		})
		containerID = containerID + int(idrange.Count)
	}
	return idMap, nil
}

func lookupGroupSubRangesFile(path string, grp user.Group) ([]IDMap, error) {
	gidstr := strconv.Itoa(grp.Gid)
	rangeList, err := user.ParseSubIDFileFilter(path, func(sid user.SubID) bool {
		return sid.Name == grp.Name || sid.Name == gidstr
	})
	if err != nil {
		return nil, err
	}
	if len(rangeList) == 0 {
		return nil, fmt.Errorf("no subuid ranges found for user %q", grp.Name)
	}

	idMap := []IDMap{}
	containerID := 0
	for _, idrange := range rangeList {
		idMap = append(idMap, IDMap{
			ContainerID: containerID,
			HostID:      int(idrange.SubID),
			Size:        int(idrange.Count),
		})
		containerID = containerID + int(idrange.Count)
	}
	return idMap, nil
}

// Loads and validates the ID mapping from the given UserNS.
func loadAndValidateIDMapping(userNS string) (IdentityMapping, error) {
	idMapping, err := LoadIdentityMapping(userNS)
	if err != nil {
		return IdentityMapping{}, err
	}
	if !validIDMapping(idMapping) {
		return IdentityMapping{}, errors.New("no valid UID/GID mappings found")
	}
	return idMapping, nil
}

// Validates that both UID and GID mappings are available.
func validIDMapping(mapping IdentityMapping) bool {
	return len(mapping.UIDMaps) > 0 && len(mapping.GIDMaps) > 0
}

// Converts IDMapping into LinuxIDMapping structures.
func convertMappings(mapping IdentityMapping) ([]specs.LinuxIDMapping, []specs.LinuxIDMapping) {
	return convertIDMapToLinuxIDMapping(mapping.UIDMaps),
		convertIDMapToLinuxIDMapping(mapping.GIDMaps)
}

// Builds OCI spec options for the user namespace.
func getUserNamespaceSpecOpts(
	uidMaps, gidMaps []specs.LinuxIDMapping,
) []oci.SpecOpts {
	return []oci.SpecOpts{oci.WithUserNamespace(uidMaps, gidMaps)}
}

// Creates snapshot options based on ID mappings and snapshotter capabilities.
func createSnapshotOpts(
	id string,
	image imgutil.EnsuredImage,
	uidMaps, gidMaps []specs.LinuxIDMapping,
) ([]containerd.NewContainerOpts, error) {
	if !isValidMapping(uidMaps, gidMaps) {
		return nil, errors.New("snapshotter uidmap gidmap config invalid")
	}
	return []containerd.NewContainerOpts{containerd.WithNewSnapshot(id, image.Image, WithUserNSRemapperLabels(uidMaps, gidMaps))}, nil
}

func WithUserNSRemapperLabels(uidmaps, gidmaps []specs.LinuxIDMapping) snapshots.Opt {
	idMap := ContainerdIDMap{
		UidMap: uidmaps,
		GidMap: gidmaps,
	}
	uidmapLabel, gidmapLabel := idMap.Marshal()
	return snapshots.WithLabels(map[string]string{
		snapshots.LabelSnapshotUIDMapping: uidmapLabel,
		snapshots.LabelSnapshotGIDMapping: gidmapLabel,
	})
}

func isValidMapping(uidMaps, gidMaps []specs.LinuxIDMapping) bool {
	return len(uidMaps) > 0 && len(gidMaps) > 0
}

func getContainerNameFromNetworkSlice(netOpts types.NetworkOptions) (string, error) {
	netItems := strings.Split(netOpts.NetworkSlice[0], ":")
	if len(netItems) < 2 || netItems[1] == "" {
		return "", fmt.Errorf("container networking argument format must be 'container:<id|name>', got: %q", netOpts.NetworkSlice[0])
	}
	containerName := netItems[1]
	return containerName, nil
}

func snapshotterSupportsRemapLabels(
	ctx context.Context,
	client *containerd.Client,
	snapshotterName string,
) (bool, error) {
	caps, err := client.GetSnapshotterCapabilities(ctx, snapshotterName)
	if err != nil {
		return false, err
	}
	return hasCapability(caps, capabRemapIDs), nil
}

// Checks if the given capability exists in the list.
func hasCapability(caps []string, capability string) bool {
	for _, cap := range caps {
		if cap == capability {
			return true
		}
	}
	return false
}
