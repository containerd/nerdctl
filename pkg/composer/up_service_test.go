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

package composer

import (
	"context"
	"errors"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
)

func TestEnsureImageMountSources(t *testing.T) {
	t.Parallel()

	type ensureCall struct {
		Image    string
		PullMode string
		Platform string
		Quiet    bool
	}
	var calls []ensureCall
	composer := &Composer{Options: Options{
		EnsureImage: func(_ context.Context, imageName, pullMode, platform string, _ *serviceparser.Service, quiet bool) error {
			calls = append(calls, ensureCall{
				Image:    imageName,
				PullMode: pullMode,
				Platform: platform,
				Quiet:    quiet,
			})
			return nil
		},
	}}
	service := &serviceparser.Service{
		Unparsed: &types.ServiceConfig{},
		ImageMountSources: []serviceparser.ImageMountSource{
			{Source: "nginx:alpine", Platform: "linux/amd64"},
			{Source: "nginx:alpine", Platform: "linux/amd64"},
			{Source: "nginx:alpine", Platform: "linux/arm64"},
			{Source: "caddy:alpine", Platform: "linux/arm64"},
		},
	}

	err := composer.ensureImageMountSources(context.Background(), service, types.PullPolicyAlways, true)
	assert.NilError(t, err)
	assert.DeepEqual(t, calls, []ensureCall{
		{Image: "nginx:alpine", PullMode: types.PullPolicyAlways, Platform: "linux/amd64", Quiet: true},
		{Image: "nginx:alpine", PullMode: types.PullPolicyAlways, Platform: "linux/arm64", Quiet: true},
		{Image: "caddy:alpine", PullMode: types.PullPolicyAlways, Platform: "linux/arm64", Quiet: true},
	})
}

func TestEnsureImageMountSourcesError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("pull failed")
	composer := &Composer{Options: Options{
		EnsureImage: func(context.Context, string, string, string, *serviceparser.Service, bool) error {
			return sentinel
		},
	}}
	service := &serviceparser.Service{
		Unparsed: &types.ServiceConfig{},
		ImageMountSources: []serviceparser.ImageMountSource{
			{Source: "nginx:alpine"},
		},
	}

	err := composer.ensureImageMountSources(context.Background(), service, types.PullPolicyMissing, false)
	assert.ErrorIs(t, err, sentinel)
	assert.ErrorContains(t, err, `failed to ensure image "nginx:alpine" for image volume`)
}
