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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/ipfs"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

func newPullCommand() *cobra.Command {
	var pullCommand = &cobra.Command{
		Use:           "pull",
		Short:         "Pull an image from a registry. Optionally specify \"ipfs://\" or \"ipns://\" scheme to pull image from IPFS.",
		Args:          cobra.ExactArgs(1),
		RunE:          pullAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	pullCommand.Flags().String("unpack", "auto", "Unpack the image for the current single platform (auto/true/false)")
	pullCommand.RegisterFlagCompletionFunc("unpack", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"auto", "true", "false"}, cobra.ShellCompDirectiveNoFileComp
	})

	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	pullCommand.Flags().StringSlice("platform", nil, "Pull content for a specific platform")
	pullCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	pullCommand.Flags().Bool("all-platforms", false, "Pull content for all platforms")
	// #endregion

	// #region verify flags
	pullCommand.Flags().String("verify", "none", "Verify the image (none|cosign)")
	pullCommand.RegisterFlagCompletionFunc("verify", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"none", "cosign"}, cobra.ShellCompDirectiveNoFileComp
	})
	pullCommand.Flags().String("cosign-key", "", "Path to the public key file, KMS, URI or Kubernetes Secret for --verify=cosign")
	// #endregion

	pullCommand.Flags().BoolP("quiet", "q", false, "Suppress verbose output")

	return pullCommand
}

func pullAction(cmd *cobra.Command, args []string) error {
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
	ocispecPlatforms, err := platformutil.NewOCISpecPlatformSlice(allPlatforms, platform)
	if err != nil {
		return err
	}

	unpackStr, err := cmd.Flags().GetString("unpack")
	if err != nil {
		return err
	}
	unpack, err := strutil.ParseBoolOrAuto(unpackStr)
	if err != nil {
		return err
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}

	_, err = ensureImage(cmd, ctx, client, rawRef, ocispecPlatforms, "always", unpack, quiet)
	if err != nil {
		return err
	}

	return nil
}

func ensureImage(cmd *cobra.Command, ctx context.Context, client *containerd.Client, rawRef string, ocispecPlatforms []v1.Platform,
	pull string, unpack *bool, quiet bool) (*imgutil.EnsuredImage, error) {

	var ensured *imgutil.EnsuredImage
	snapshotter, err := cmd.Flags().GetString("snapshotter")
	if err != nil {
		return nil, err
	}
	insecureRegistry, err := cmd.Flags().GetBool("insecure-registry")
	if err != nil {
		return nil, err
	}
	hostsDirs, err := cmd.Flags().GetStringSlice("hosts-dir")
	if err != nil {
		return nil, err
	}
	verifier, err := cmd.Flags().GetString("verify")
	if err != nil {
		return nil, err
	}

	if scheme, ref, err := referenceutil.ParseIPFSRefWithScheme(rawRef); err == nil {
		if verifier != "none" {
			return nil, errors.New("--verify flag is not supported on IPFS as of now")
		}

		ipfsClient, err := httpapi.NewLocalApi()
		if err != nil {
			return nil, err
		}
		ensured, err = ipfs.EnsureImage(ctx, client, ipfsClient, cmd.OutOrStdout(), cmd.ErrOrStderr(), snapshotter, scheme, ref,
			pull, ocispecPlatforms, unpack, quiet)
		if err != nil {
			return nil, err
		}
		return ensured, nil
	}

	ref := rawRef
	switch verifier {
	case "cosign":
		experimental, err := cmd.Flags().GetBool("experimental")
		if err != nil {
			return nil, err
		}

		if !experimental {
			return nil, fmt.Errorf("cosign only work with enable experimental feature")
		}

		keyRef, err := cmd.Flags().GetString("cosign-key")
		if err != nil {
			return nil, err
		}

		ref, err = verifyCosign(ctx, rawRef, keyRef, hostsDirs)
		if err != nil {
			return nil, err
		}
	case "none":
		logrus.Debugf("verification process skipped")
	default:
		return nil, fmt.Errorf("no verifier found: %s", verifier)
	}

	ensured, err = imgutil.EnsureImage(ctx, client, cmd.OutOrStdout(), cmd.ErrOrStderr(), snapshotter, ref,
		pull, insecureRegistry, hostsDirs, ocispecPlatforms, unpack, quiet)
	if err != nil {
		return nil, err
	}
	return ensured, err
}

func verifyCosign(ctx context.Context, rawRef string, keyRef string, hostsDirs []string) (string, error) {
	digest, err := imgutil.ResolveDigest(ctx, rawRef, false, hostsDirs)
	if err != nil {
		logrus.WithError(err).Errorf("unable to resolve digest for an image %s: %v", rawRef, err)
		return rawRef, err
	}
	ref := rawRef
	if !strings.Contains(ref, "@") {
		ref += "@" + digest
	}

	logrus.Debugf("verifying image: %s", ref)

	cosignExecutable, err := exec.LookPath("cosign")
	if err != nil {
		logrus.WithError(err).Error("cosign executable not found in path $PATH")
		logrus.Info("you might consider installing cosign from: https://docs.sigstore.dev/cosign/installation")
		return ref, err
	}

	cosignCmd := exec.Command(cosignExecutable, []string{"verify"}...)
	cosignCmd.Env = os.Environ()

	if keyRef != "" {
		cosignCmd.Args = append(cosignCmd.Args, "--key", keyRef)
	} else {
		cosignCmd.Env = append(cosignCmd.Env, "COSIGN_EXPERIMENTAL=true")
	}

	cosignCmd.Args = append(cosignCmd.Args, ref)

	logrus.Debugf("running %s %v", cosignExecutable, cosignCmd.Args)

	stdout, _ := cosignCmd.StdoutPipe()
	stderr, _ := cosignCmd.StderrPipe()
	if err := cosignCmd.Start(); err != nil {
		return ref, err
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
		return ref, err
	}

	return ref, nil
}
