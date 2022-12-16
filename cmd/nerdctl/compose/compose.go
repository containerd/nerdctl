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

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/volume"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/pkg/cosignutil"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/ipfs"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

func NewComposeCommand() *cobra.Command {
	var composeCommand = &cobra.Command{
		Use:              "compose [flags] COMMAND",
		Short:            "Compose",
		RunE:             completion.UnknownSubcommandAction,
		SilenceUsage:     true,
		SilenceErrors:    true,
		TraverseChildren: true, // required for global short hands like -f
	}
	// `-f` is a nonPersistentAlias, as it conflicts with `nerdctl compose logs --follow`
	utils.AddPersistentStringArrayFlag(composeCommand, "file", nil, []string{"f"}, nil, "", "Specify an alternate compose file")
	composeCommand.PersistentFlags().String("project-directory", "", "Specify an alternate working directory")
	composeCommand.PersistentFlags().StringP("project-name", "p", "", "Specify an alternate project name")
	composeCommand.PersistentFlags().String("env-file", "", "Specify an alternate environment file")

	composeCommand.AddCommand(
		newComposeUpCommand(),
		newComposeLogsCommand(),
		newComposeConfigCommand(),
		newComposeBuildCommand(),
		newComposeExecCommand(),
		newComposeImagesCommand(),
		newComposePortCommand(),
		newComposePushCommand(),
		newComposePullCommand(),
		newComposeDownCommand(),
		newComposePsCommand(),
		newComposeKillCommand(),
		newComposeRestartCommand(),
		newComposeRemoveCommand(),
		newComposeRunCommand(),
		newComposeVersionCommand(),
		newComposeStopCommand(),
		newComposePauseCommand(),
		newComposeUnpauseCommand(),
		newComposeTopCommand(),
	)

	return composeCommand
}

func getComposer(cmd *cobra.Command, client *containerd.Client) (*composer.Composer, error) {
	nerdctlCmd, nerdctlArgs := utils.GlobalFlags(cmd)
	projectDirectory, err := cmd.Flags().GetString("project-directory")
	if err != nil {
		return nil, err
	}
	envFile, err := cmd.Flags().GetString("env-file")
	if err != nil {
		return nil, err
	}
	projectName, err := cmd.Flags().GetString("project-name")
	if err != nil {
		return nil, err
	}
	debugFull, err := cmd.Flags().GetBool("debug-full")
	if err != nil {
		return nil, err
	}
	files, err := cmd.Flags().GetStringArray("file")
	if err != nil {
		return nil, err
	}
	insecure, err := cmd.Flags().GetBool("insecure-registry")
	if err != nil {
		return nil, err
	}
	cniPath, err := cmd.Flags().GetString("cni-path")
	if err != nil {
		return nil, err
	}
	cniNetconfpath, err := cmd.Flags().GetString("cni-netconfpath")
	if err != nil {
		return nil, err
	}
	snapshotter, err := cmd.Flags().GetString("snapshotter")
	if err != nil {
		return nil, err
	}
	hostsDirs, err := cmd.Flags().GetStringSlice("hosts-dir")
	if err != nil {
		return nil, err
	}
	experimental, err := cmd.Flags().GetBool("experimental")
	if err != nil {
		return nil, err
	}

	o := composer.Options{
		Project:          projectName,
		ProjectDirectory: projectDirectory,
		ConfigPaths:      files,
		EnvFile:          envFile,
		NerdctlCmd:       nerdctlCmd,
		NerdctlArgs:      nerdctlArgs,
		DebugPrintFull:   debugFull,
		Experimental:     experimental,
	}

	cniEnv, err := netutil.NewCNIEnv(cniPath, cniNetconfpath, netutil.WithDefaultNetwork())
	if err != nil {
		return nil, err
	}
	networkConfigs, err := cniEnv.NetworkList()
	if err != nil {
		return nil, err
	}

	o.NetworkExists = func(netName string) (bool, error) {
		for _, f := range networkConfigs {
			if f.Name == netName {
				return true, nil
			}
		}
		return false, nil
	}

	volStore, err := volume.Store(cmd)
	if err != nil {
		return nil, err
	}

	o.VolumeExists = func(volName string) (bool, error) {
		if _, volGetErr := volStore.Get(volName, false); volGetErr == nil {
			return true, nil
		} else if errors.Is(volGetErr, errdefs.ErrNotFound) {
			return false, nil
		} else {
			return false, volGetErr
		}
	}

	o.ImageExists = func(ctx context.Context, rawRef string) (bool, error) {
		refNamed, err := referenceutil.ParseAny(rawRef)
		if err != nil {
			return false, err
		}
		ref := refNamed.String()
		if _, err := client.ImageService().Get(ctx, ref); err != nil {
			if errors.Is(err, errdefs.ErrNotFound) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	o.EnsureImage = func(ctx context.Context, imageName, pullMode, platform string, ps *serviceparser.Service, quiet bool) error {
		ocispecPlatforms := []ocispec.Platform{platforms.DefaultSpec()}
		if platform != "" {
			parsed, err := platforms.Parse(platform)
			if err != nil {
				return err
			}
			ocispecPlatforms = []ocispec.Platform{parsed} // no append
		}

		// IPFS reference
		if scheme, ref, err := referenceutil.ParseIPFSRefWithScheme(imageName); err == nil {
			ipfsClient, err := httpapi.NewLocalApi()
			if err != nil {
				return err
			}
			_, err = ipfs.EnsureImage(ctx, client, ipfsClient, cmd.OutOrStdout(), cmd.ErrOrStderr(), snapshotter, scheme, ref,
				pullMode, ocispecPlatforms, nil, quiet)
			return err
		}

		ref := imageName
		if verifier, ok := ps.Unparsed.Extensions[serviceparser.ComposeVerify]; ok {
			switch verifier {
			case "cosign":
				if !o.Experimental {
					return fmt.Errorf("cosign only work with enable experimental feature")
				}

				// if key is given, use key mode, otherwise use keyless mode.
				keyRef := ""
				if keyVal, ok := ps.Unparsed.Extensions[serviceparser.ComposeCosignPublicKey]; ok {
					keyRef = keyVal.(string)
				}

				ref, err = cosignutil.VerifyCosign(ctx, ref, keyRef, hostsDirs)
				if err != nil {
					return err
				}
			case "none":
				logrus.Debugf("verification process skipped")
			default:
				return fmt.Errorf("no verifier found: %s", verifier)
			}
		}
		_, err = imgutil.EnsureImage(ctx, client, cmd.OutOrStdout(), cmd.ErrOrStderr(), snapshotter, ref,
			pullMode, insecure, hostsDirs, ocispecPlatforms, nil, quiet)
		return err
	}

	return composer.New(o, client)
}
