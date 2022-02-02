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
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images/converter"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/pkg/imgutil/push"
	"github.com/containerd/nerdctl/pkg/ipfs"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"
	estargzconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newPushCommand() *cobra.Command {
	var pushCommand = &cobra.Command{
		Use:               "push NAME[:TAG]",
		Short:             "Push an image or a repository to a registry. Optionally specify \"ipfs://\" or \"ipns://\" scheme to push image to IPFS.",
		Args:              cobra.ExactArgs(1),
		RunE:              pushAction,
		ValidArgsFunction: pushShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	pushCommand.Flags().StringSlice("platform", []string{}, "Push content for a specific platform")
	pushCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	pushCommand.Flags().Bool("all-platforms", false, "Push content for all platforms")
	// #endregion

	pushCommand.Flags().Bool("estargz", false, "Convert the image into eStargz")
	pushCommand.Flags().Bool("ipfs-ensure-image", true, "Ensure the entire contents of the image is locally available before push")

	// #region sign flags
	pushCommand.Flags().String("sign", "none", "Sign the image (none|cosign")
	pushCommand.RegisterFlagCompletionFunc("sign", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"none", "cosign"}, cobra.ShellCompDirectiveNoFileComp
	})
	pushCommand.Flags().String("cosign-key", "", "Path to the private key file, KMS URI or Kubernetes Secret for --sign=cosign")
	// #endregion

	return pushCommand
}

func pushAction(cmd *cobra.Command, args []string) error {
	rawRef := args[0]

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
	convertEStargz, err := cmd.Flags().GetBool("estargz")
	if err != nil {
		return err
	}

	if scheme, ref, err := referenceutil.ParseIPFSRefWithScheme(rawRef); err == nil {
		if scheme != "ipfs" {
			return fmt.Errorf("ipfs scheme is only supported but got %q", scheme)
		}
		ensureImage, err := cmd.Flags().GetBool("ipfs-ensure-image")
		if err != nil {
			return err
		}
		logrus.Infof("pushing image %q to IPFS", ref)
		ipfsClient, err := httpapi.NewLocalApi()
		if err != nil {
			return err
		}
		var layerConvert converter.ConvertFunc
		if convertEStargz {
			layerConvert = eStargzConvertFunc()
		}
		c, err := ipfs.Push(ctx, client, ipfsClient, ref, layerConvert, allPlatforms, platform, ensureImage)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), c.String())
		return err
	}

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

	platMC, err := platformutil.NewMatchComparer(allPlatforms, platform)
	if err != nil {
		return err
	}
	pushRef := ref
	if !allPlatforms {
		pushRef = ref + "-tmp-reduced-platform"
		// Push fails with "400 Bad Request" when the manifest is multi-platform but we do not locally have multi-platform blobs.
		// So we create a tmp reduced-platform image to avoid the error.
		platImg, err := converter.Convert(ctx, client, pushRef, ref, converter.WithPlatform(platMC))
		if err != nil {
			if len(platform) == 0 {
				return fmt.Errorf("failed to create a tmp single-platform image %q: %w", pushRef, err)
			}
			return fmt.Errorf("failed to create a tmp reduced-platform image %q (platform=%v): %w", pushRef, platform, err)
		}
		defer client.ImageService().Delete(ctx, platImg.Name)
		logrus.Infof("pushing as a reduced-platform image (%s, %s)", platImg.Target.MediaType, platImg.Target.Digest)
	}

	if convertEStargz {
		pushRef = ref + "-tmp-esgz"
		esgzImg, err := converter.Convert(ctx, client, pushRef, ref, converter.WithPlatform(platMC), converter.WithLayerConvertFunc(eStargzConvertFunc()))
		if err != nil {
			return fmt.Errorf("failed to convert to eStargz: %v", err)
		}
		defer client.ImageService().Delete(ctx, esgzImg.Name)
		logrus.Infof("pushing as an eStargz image (%s, %s)", esgzImg.Target.MediaType, esgzImg.Target.Digest)
	}

	pushFunc := func(r remotes.Resolver) error {
		return push.Push(ctx, client, r, cmd.OutOrStdout(), pushRef, ref, platMC)
	}

	var dOpts []dockerconfigresolver.Opt
	if insecure {
		logrus.Warnf("skipping verifying HTTPS certs for %q", refDomain)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	hostsDirs, err := cmd.Flags().GetStringSlice("hosts-dir")
	if err != nil {
		return err
	}
	dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(hostsDirs))
	resolver, err := dockerconfigresolver.New(ctx, refDomain, dOpts...)
	if err != nil {
		return err
	}
	if err = pushFunc(resolver); err != nil {
		// In some circumstance (e.g. people just use 80 port to support pure http), the error will contain message like "dial tcp <port>: connection refused"
		if !imgutil.IsErrHTTPResponseToHTTPSClient(err) && !imgutil.IsErrConnectionRefused(err) {
			return err
		}
		if insecure {
			logrus.WithError(err).Warnf("server %q does not seem to support HTTPS, falling back to plain HTTP", refDomain)
			dOpts = append(dOpts, dockerconfigresolver.WithPlainHTTP(true))
			resolver, err = dockerconfigresolver.New(ctx, refDomain, dOpts...)
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

	signer, err := cmd.Flags().GetString("sign")

	if err != nil {
		return err
	}
	switch signer {
	case "cosign":
		keyRef, err := cmd.Flags().GetString("cosign-key")
		if err != nil {
			return err
		}

		err = signCosign(rawRef, keyRef)
		if err != nil {
			return err
		}
	case "none":
		logrus.Debugf("signing process skipped")
	default:
		return fmt.Errorf("no signers found: %s", signer)

	}

	return nil
}

func pushShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return shellCompleteImageNames(cmd)
}

func eStargzConvertFunc() converter.ConvertFunc {
	convertToESGZ := estargzconvert.LayerConvertFunc()
	return func(ctx context.Context, cs content.Store, desc ocispec.Descriptor) (*ocispec.Descriptor, error) {
		if isReusableESGZ(ctx, cs, desc) {
			logrus.Infof("reusing estargz %s without conversion", desc.Digest)
			return nil, nil
		}
		newDesc, err := convertToESGZ(ctx, cs, desc)
		if err != nil {
			return nil, err
		}
		logrus.Infof("converted %q to %s", desc.MediaType, newDesc.Digest)
		return newDesc, err
	}

}

func isReusableESGZ(ctx context.Context, cs content.Store, desc ocispec.Descriptor) bool {
	dgstStr, ok := desc.Annotations[estargz.TOCJSONDigestAnnotation]
	if !ok {
		return false
	}
	tocdgst, err := digest.Parse(dgstStr)
	if err != nil {
		return false
	}
	ra, err := cs.ReaderAt(ctx, desc)
	if err != nil {
		return false
	}
	defer ra.Close()
	r, err := estargz.Open(io.NewSectionReader(ra, 0, desc.Size), estargz.WithDecompressors(new(zstdchunked.Decompressor)))
	if err != nil {
		return false
	}
	if _, err := r.VerifyTOC(tocdgst); err != nil {
		return false
	}
	return true
}

func signCosign(rawRef string, keyRef string) error {
	cosignExecutable, err := exec.LookPath("cosign")
	if err != nil {
		logrus.WithError(err).Error("cosign executable not found in path $PATH")
		logrus.Info("you might consider installing cosign from: https://docs.sigstore.dev/cosign/installation")
		return err
	}

	cosignCmd := exec.Command(cosignExecutable, []string{"sign"}...)
	cosignCmd.Env = os.Environ()

	if keyRef != "" {
		cosignCmd.Args = append(cosignCmd.Args, "--key", keyRef)
	} else {
		cosignCmd.Env = append(cosignCmd.Env, "COSIGN_EXPERIMENTAL=true")
	}

	cosignCmd.Args = append(cosignCmd.Args, rawRef)

	logrus.Debugf("running %s %v", cosignExecutable, cosignCmd.Args)

	stdout, _ := cosignCmd.StdoutPipe()
	stderr, _ := cosignCmd.StderrPipe()
	if err := cosignCmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		logrus.Info("cosign: " + scanner.Text())
	}

	errScanner := bufio.NewScanner(stderr)
	for errScanner.Scan() {
		logrus.Info("cosign: " + errScanner.Text())
	}

	if err := cosignCmd.Wait(); err != nil {
		return err
	}

	return nil
}
