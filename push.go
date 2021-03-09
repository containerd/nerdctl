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
	"context"

	"github.com/AkihiroSuda/nerdctl/pkg/imgutil"
	"github.com/AkihiroSuda/nerdctl/pkg/imgutil/dockerconfigresolver"
	"github.com/AkihiroSuda/nerdctl/pkg/imgutil/push"
	"github.com/containerd/containerd/images/converter"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/urfave/cli/v2"
)

var pushCommand = &cli.Command{
	Name:      "push",
	Usage:     "Push an image or a repository to a registry",
	ArgsUsage: "NAME[:TAG]",
	Action:    pushAction,
	Flags:     []cli.Flag{},
}

func pushAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 1 {
		return errors.New("image name needs to be specified")
	}
	rawRef := clicontext.Args().First()
	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return err
	}
	ref := named.String()
	refDomain := refdocker.Domain(named)

	insecure := clicontext.Bool("insecure-registry")

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	// Push fails with "400 Bad Request" when the manifest is multi-platform but we do not locally have multi-platform blobs.
	// So we create a tmp single-platform image to avoid the error.
	// TODO: support pushing multi-platform
	singlePlatform := platforms.DefaultStrict()
	singlePlatformRef := ref + "-tmp-single"
	singlePlatformImg, err := converter.Convert(ctx, client, singlePlatformRef, ref,
		converter.WithPlatform(singlePlatform))
	if err != nil {
		return errors.Wrapf(err, "failed to create a tmp single-platform image %q", singlePlatformRef)
	}
	defer client.ImageService().Delete(context.TODO(), singlePlatformImg.Name)
	logrus.Infof("pushing as a single-platform image (%s, %s)", singlePlatformImg.Target.MediaType, singlePlatformImg.Target.Digest)

	pushFunc := func(r remotes.Resolver) error {
		return push.Push(ctx, client, r, clicontext.App.Writer, singlePlatformRef, ref, singlePlatform)
	}

	var dOpts []dockerconfigresolver.Opt
	if insecure {
		logrus.Warnf("skipping verifying HTTPS certs for %q", refDomain)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	resolver, err := dockerconfigresolver.New(refDomain, dOpts...)
	if err != nil {
		return err
	}
	if err = pushFunc(resolver); err != nil {
		if !imgutil.IsErrHTTPResponseToHTTPSClient(err) {
			return err
		}
		if insecure {
			logrus.WithError(err).Warnf("server %q does not seem to support HTTPS, falling back to plain HTTP", refDomain)
			dOpts = append(dOpts, dockerconfigresolver.WithPlainHTTP(true))
			resolver, err = dockerconfigresolver.New(refDomain, dOpts...)
			if err != nil {
				return err
			}
			return pushFunc(resolver)
		} else {
			logrus.WithError(err).Errorf("server %q does not seem to support HTTPS", refDomain)
			logrus.Info("Hint: you may want to try --insecure-registry to allow plain HTTP (if you are in a trusted network)")
			return err
		}
	}
	return nil
}
