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

package signutil

import (
	"context"
	"fmt"

	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

// Sign signs an image using a signer and options provided in options.
func Sign(rawRef string, experimental bool, options types.ImageSignOptions) error {
	switch options.Provider {
	case "cosign":
		if !experimental {
			return fmt.Errorf("cosign only work with enable experimental feature")
		}

		if err := SignCosign(rawRef, options.CosignKey); err != nil {
			return err
		}
	case "notation":
		if !experimental {
			return fmt.Errorf("notation only work with enable experimental feature")
		}

		if err := SignNotation(rawRef, options.NotationKeyName); err != nil {
			return err
		}
	case "", "none":
		log.L.Debugf("signing process skipped")
	default:
		return fmt.Errorf("no signers found: %s", options.Provider)
	}
	return nil
}

// Verify verifies an image using a verifier and options provided in options.
func Verify(ctx context.Context, rawRef string, hostsDirs []string, experimental bool, options types.ImageVerifyOptions) (ref string, err error) {
	switch options.Provider {
	case "cosign":
		if !experimental {
			return "", fmt.Errorf("cosign only work with enable experimental feature")
		}

		if ref, err = VerifyCosign(ctx, rawRef, options.CosignKey, hostsDirs, options.CosignCertificateIdentity, options.CosignCertificateIdentityRegexp, options.CosignCertificateOidcIssuer, options.CosignCertificateOidcIssuerRegexp); err != nil {
			return "", err
		}
	case "notation":
		if !experimental {
			return "", fmt.Errorf("notation only work with enable experimental feature")
		}

		if ref, err = VerifyNotation(ctx, rawRef, hostsDirs); err != nil {
			return "", err
		}
	case "", "none":
		ref = rawRef
		log.G(ctx).Debugf("verifying process skipped")
	default:
		return "", fmt.Errorf("no verifiers found: %s", options.Provider)
	}
	return ref, nil
}
