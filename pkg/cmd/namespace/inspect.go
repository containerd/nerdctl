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

package namespace

import (
	"context"
	"strings"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/mountutil/volumestore"
)

func Inspect(ctx context.Context, client *containerd.Client, inspectedNamespaces []string, options types.NamespaceInspectOptions) error {
	result := []interface{}{}

	warns := []error{}
	for _, ns := range inspectedNamespaces {
		ctx = namespaces.WithNamespace(ctx, ns)
		namespaceService := client.NamespaceService()
		if err := namespaceExists(ctx, namespaceService, ns); err != nil {
			warns = append(warns, err)
			continue
		}

		labels, err := namespaceService.Labels(ctx, ns)
		if err != nil {
			return err
		}

		containerInfo, err := containerInfo(ctx, client)
		if err != nil {
			warns = append(warns, err)
		}

		imageInfo, err := imageInfo(ctx, client)
		if err != nil {
			warns = append(warns, err)
		}

		volumeInfo, err := volumeInfo(ns, options)
		if err != nil {
			warns = append(warns, err)
		}

		nsInspect := native.Namespace{
			Name:       ns,
			Labels:     &labels,
			Containers: &containerInfo,
			Images:     &imageInfo,
			Volumes:    &volumeInfo,
		}
		result = append(result, nsInspect)
	}
	if err := formatter.FormatSlice(options.Format, options.Stdout, result); err != nil {
		return err
	}
	for _, warn := range warns {
		log.G(ctx).Warn(warn)
	}

	return nil
}
func containerInfo(ctx context.Context, client *containerd.Client) (native.ContainerInfo, error) {
	info := native.ContainerInfo{}
	containers, err := client.Containers(ctx)
	if err != nil {
		return info, err
	}

	info.Count = len(containers)
	ids := make([]string, info.Count)

	info.IDs = ids
	for idx, container := range containers {
		ids[idx] = container.ID()
	}

	return info, nil
}
func imageInfo(ctx context.Context, client *containerd.Client) (native.ImageInfo, error) {
	info := native.ImageInfo{}
	imageService := client.ImageService()
	images, err := imageService.List(ctx)
	if err != nil {
		return info, err
	}

	ids := make([]string, 0, len(images))

	for _, img := range images {
		digestStrSplit := strings.SplitN(img.Target.Digest.String(), ":", 2)
		if len(digestStrSplit) == 2 {
			ids = append(ids, digestStrSplit[1][:12])
		} else {
			log.G(ctx).Warnf("invalid image digest format:%s", img.Target.Digest.String())
		}
	}

	info.IDs = ids
	info.Count = len(ids)

	return info, nil
}

func volumeInfo(namespace string, options types.NamespaceInspectOptions) (native.VolumeInfo, error) {
	info := native.VolumeInfo{}

	dataStore, err := clientutil.DataStore(options.GOptions.DataRoot, options.GOptions.Address)
	if err != nil {
		return info, err
	}
	volStore, err := volumestore.New(dataStore, namespace)
	if err != nil {
		return info, err
	}

	volumes, err := volStore.List(false)
	if err != nil {
		return info, err
	}

	info.Count = len(volumes)
	names := make([]string, 0, info.Count)

	for _, v := range volumes {
		names = append(names, v.Name)
	}

	info.Names = names
	return info, nil
}
