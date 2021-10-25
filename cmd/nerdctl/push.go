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

package main

import (
	"errors"
	"fmt"

	"github.com/containerd/containerd/images/converter"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/pkg/imgutil/push"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newPushCommand() *cobra.Command {
	var pushCommand = &cobra.Command{
		Use:               "push NAME[:TAG]",
		Short:             "Push an image or a repository to a registry",
		RunE:              pushAction,
		ValidArgsFunction: pushShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	// #region platform flags
	pushCommand.Flags().StringSlice("platform", []string{}, "Push content for a specific platform")
	pushCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	pushCommand.Flags().Bool("all-platforms", false, "Push content for all platforms")
	// #endregion

	return pushCommand
}

func pushAction(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("image name needs to be specified")
	}
	rawRef := args[0]
	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return err
	}
	ref := named.String()
	refDomain := refdocker.Domain(named)

	insecure, err := cmd.Flags().GetBool("insecure-registry")
	if err != nil {
		return err
	}
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return err
	}
	platMC, err := platformutil.NewMatchComparer(allPlatforms, platform)
	if err != nil {
		return err
	}
	platRef := ref
	if !allPlatforms {
		platRef = ref + "-tmp-reduced-platform"
		// Push fails with "400 Bad Request" when the manifest is multi-platform but we do not locally have multi-platform blobs.
		// So we create a tmp reduced-platform image to avoid the error.
		platImg, err := converter.Convert(ctx, client, platRef, ref, converter.WithPlatform(platMC))
		if err != nil {
			if len(platform) == 0 {
				return fmt.Errorf("failed to create a tmp single-platform image %q: %w", platRef, err)
			}
			return fmt.Errorf("failed to create a tmp reduced-platform image %q (platform=%v): %w", platRef, platform, err)
		}
		defer client.ImageService().Delete(ctx, platImg.Name)
		logrus.Infof("pushing as a reduced-platform image (%s, %s)", platImg.Target.MediaType, platImg.Target.Digest)
	}

	pushFunc := func(r remotes.Resolver) error {
		return push.Push(ctx, client, r, cmd.OutOrStdout(), platRef, ref, platMC)
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

func pushShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return shellCompleteImageNames(cmd)
}
