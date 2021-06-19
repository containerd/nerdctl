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

package imageinspector

import (
	"context"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	imgutil "github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/sirupsen/logrus"
)

func Inspect(ctx context.Context, client *containerd.Client, image images.Image) (*native.Image, error) {

	n := &native.Image{}

	img := containerd.NewImage(client, image)
	imageConfig, err := imgutil.ReadImageConfig(ctx, img)
	if err != nil {
		logrus.WithError(err).WithField("id", image.Name).Warnf("failed to inspect Rootfs")
		return nil, err
	}
	n.ImageConfig = imageConfig

	cs := client.ContentStore()
	config, err := image.Config(ctx, cs, platforms.DefaultStrict())
	if err != nil {
		logrus.WithError(err).WithField("id", image.Name).Warnf("failed to inspect Rootfs")
		return nil, err
	}

	n.ImageConfigDesc = config
	n.Image = image

	return n, nil
}
