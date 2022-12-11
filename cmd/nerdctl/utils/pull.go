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

package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/cosignutil"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/ipfs"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gotest.tools/v3/assert"
)

type CosignKeyPair struct {
	PublicKey  string
	PrivateKey string
	Cleanup    func()
}

func NewCosignKeyPair(t testing.TB, path string) *CosignKeyPair {
	td, err := os.MkdirTemp(t.TempDir(), path)
	assert.NilError(t, err)

	cmd := exec.Command("cosign", "generate-key-pair")
	cmd.Dir = td
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to run %v: %v (%q)", cmd.Args, err, string(out))
	}

	publicKey := filepath.Join(td, "cosign.pub")
	privateKey := filepath.Join(td, "cosign.key")

	return &CosignKeyPair{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
		Cleanup: func() {
			_ = os.RemoveAll(td)
		},
	}
}

func EnsureImage(ctx context.Context, cmd *cobra.Command, client *containerd.Client, rawRef string, ocispecPlatforms []v1.Platform,
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

		ref, err = cosignutil.VerifyCosign(ctx, rawRef, keyRef, hostsDirs)
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
