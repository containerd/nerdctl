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

package imgutil

import (
	"context"
	"io"
	"strings"

	"github.com/AkihiroSuda/nerdctl/pkg/imgutil/dockerconfigresolver"
	"github.com/AkihiroSuda/nerdctl/pkg/imgutil/pull"
	"github.com/containerd/containerd"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/imgcrypt"
	"github.com/containerd/imgcrypt/images/encryption"
	"github.com/containerd/stargz-snapshotter/fs/source"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type EnsuredImage struct {
	Ref         string
	Image       containerd.Image
	Snapshotter string
	Remote      bool // true for stargz
}

// PullMode is either one of "always", "missing", "never"
type PullMode = string

// EnsureImage ensures the image.
//
// When insecure is set, skips verifying certs, and also falls back to HTTP when the registry does not speak HTTPS
func EnsureImage(ctx context.Context, client *containerd.Client, stdout io.Writer, snapshotter, rawRef string, mode PullMode, insecure bool) (*EnsuredImage, error) {
	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return nil, err
	}
	ref := named.String()

	if mode != "always" {
		if i, err := client.ImageService().Get(ctx, ref); err == nil {
			image := containerd.NewImage(client, i)
			res := &EnsuredImage{
				Ref:         ref,
				Image:       image,
				Snapshotter: snapshotter,
				Remote:      isStargz(snapshotter),
			}
			if unpacked, err := image.IsUnpacked(ctx, snapshotter); err == nil && !unpacked {
				if err := image.Unpack(ctx, snapshotter); err != nil {
					return nil, err
				}
			}
			return res, nil
		}
	}

	if mode == "never" {
		return nil, errors.Errorf("image %q is not available", rawRef)
	}

	refDomain := refdocker.Domain(named)

	var dOpts []dockerconfigresolver.Opt
	if insecure {
		logrus.Warnf("skipping verifying HTTPS certs for %q", refDomain)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	resolver, err := dockerconfigresolver.New(refDomain, dOpts...)
	if err != nil {
		return nil, err
	}

	img, err := pullImage(ctx, client, stdout, snapshotter, resolver, ref)
	if err != nil {
		if !IsErrHTTPResponseToHTTPSClient(err) {
			return nil, err
		}
		if insecure {
			logrus.WithError(err).Warnf("server %q does not seem to support HTTPS, falling back to plain HTTP", refDomain)
			dOpts = append(dOpts, dockerconfigresolver.WithPlainHTTP(true))
			resolver, err = dockerconfigresolver.New(refDomain, dOpts...)
			if err != nil {
				return nil, err
			}
			return pullImage(ctx, client, stdout, snapshotter, resolver, ref)
		} else {
			logrus.WithError(err).Errorf("server %q does not seem to support HTTPS", refDomain)
			logrus.Info("Hint: you may want to try --insecure-registry to allow plain HTTP (if you are in a trusted network)")
			return nil, err
		}
	}
	return img, nil
}

// IsErrHTTPResponseToHTTPSClient returns whether err is
// "http: server gave HTTP response to HTTPS client"
func IsErrHTTPResponseToHTTPSClient(err error) bool {
	// The error string is unexposed as of Go 1.16, so we can't use `errors.Is`.
	// https://github.com/golang/go/issues/44855
	const unexposed = "server gave HTTP response to HTTPS client"
	return strings.Contains(err.Error(), unexposed)
}

func pullImage(ctx context.Context, client *containerd.Client, stdout io.Writer, snapshotter string, resolver remotes.Resolver, ref string) (*EnsuredImage, error) {
	ctx, done, err := client.WithLease(ctx)
	if err != nil {
		return nil, err
	}
	defer done(ctx)

	var containerdImage containerd.Image
	config := &pull.Config{
		Resolver:       resolver,
		ProgressOutput: stdout,
		RemoteOpts: []containerd.RemoteOpt{
			containerd.WithPullUnpack,
			containerd.WithPullSnapshotter(snapshotter),
		},
	}

	imgcryptPayload := imgcrypt.Payload{}
	imgcryptUnpackOpt := encryption.WithUnpackConfigApplyOpts(encryption.WithDecryptedUnpack(&imgcryptPayload))
	config.RemoteOpts = append(config.RemoteOpts,
		containerd.WithUnpackOpts([]containerd.UnpackOpt{imgcryptUnpackOpt}))

	sgz := isStargz(snapshotter)
	if sgz {
		// TODO: support "skip-content-verify"
		config.RemoteOpts = append(
			config.RemoteOpts,
			containerd.WithImageHandlerWrapper(source.AppendDefaultLabelsHandlerWrapper(ref, 10*1024*1024)),
		)
	}
	containerdImage, err = pull.Pull(ctx, client, ref, config)
	if err != nil {
		return nil, err
	}
	res := &EnsuredImage{
		Ref:         ref,
		Image:       containerdImage,
		Snapshotter: snapshotter,
		Remote:      sgz,
	}
	return res, nil

}

func isStargz(sn string) bool {
	if !strings.Contains(sn, "stargz") {
		return false
	}
	if sn != "stargz" {
		logrus.Debugf("assuming %q to be a stargz-compatible snapshotter", sn)
	}
	return true
}
