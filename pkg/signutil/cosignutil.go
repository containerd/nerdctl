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
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/imgutil"
)

// SignCosign signs an image(`rawRef`) using a cosign private key (`keyRef`)
func SignCosign(rawRef string, keyRef string) error {
	cosignExecutable, err := exec.LookPath("cosign")
	if err != nil {
		log.L.WithError(err).Error("cosign executable not found in path $PATH")
		log.L.Info("you might consider installing cosign from: https://docs.sigstore.dev/cosign/installation")
		return err
	}

	cosignCmd := exec.Command(cosignExecutable, []string{"sign"}...)
	cosignCmd.Env = os.Environ()

	// if key is empty, use keyless mode(experimental)
	if keyRef != "" {
		cosignCmd.Args = append(cosignCmd.Args, "--key", keyRef)
	} else {
		cosignCmd.Env = append(cosignCmd.Env, "COSIGN_EXPERIMENTAL=true")
	}

	cosignCmd.Args = append(cosignCmd.Args, "--yes")
	cosignCmd.Args = append(cosignCmd.Args, rawRef)

	log.L.Debugf("running %s %v", cosignExecutable, cosignCmd.Args)

	err = processCosignIO(cosignCmd)
	if err != nil {
		return err
	}

	return cosignCmd.Wait()
}

// VerifyCosign verifies an image(`rawRef`) with a cosign public key(`keyRef`)
// `hostsDirs` are used to resolve image `rawRef`
// Either --cosign-certificate-identity or --cosign-certificate-identity-regexp and either --cosign-certificate-oidc-issuer or --cosign-certificate-oidc-issuer-regexp must be set for keyless flows.
func VerifyCosign(ctx context.Context, rawRef string, keyRef string, hostsDirs []string,
	certIdentity string, certIdentityRegexp string, certOidcIssuer string, certOidcIssuerRegexp string) (string, error) {
	digest, err := imgutil.ResolveDigest(ctx, rawRef, false, hostsDirs)
	if err != nil {
		log.G(ctx).WithError(err).Errorf("unable to resolve digest for an image %s: %v", rawRef, err)
		return rawRef, err
	}
	ref := rawRef
	if !strings.Contains(ref, "@") {
		ref += "@" + digest
	}

	log.G(ctx).Debugf("verifying image: %s", ref)

	cosignExecutable, err := exec.LookPath("cosign")
	if err != nil {
		log.G(ctx).WithError(err).Error("cosign executable not found in path $PATH")
		log.G(ctx).Info("you might consider installing cosign from: https://docs.sigstore.dev/cosign/installation")
		return ref, err
	}

	cosignCmd := exec.Command(cosignExecutable, []string{"verify"}...)
	cosignCmd.Env = os.Environ()

	// if key is empty, use keyless mode(experimental)
	if keyRef != "" {
		cosignCmd.Args = append(cosignCmd.Args, "--key", keyRef)
	} else {
		if certIdentity == "" && certIdentityRegexp == "" {
			return ref, errors.New("--cosign-certificate-identity or --cosign-certificate-identity-regexp is required for Cosign verification in keyless mode")
		}
		if certIdentity != "" {
			cosignCmd.Args = append(cosignCmd.Args, "--certificate-identity", certIdentity)
		}
		if certIdentityRegexp != "" {
			cosignCmd.Args = append(cosignCmd.Args, "--certificate-identity-regexp", certIdentityRegexp)
		}
		if certOidcIssuer == "" && certOidcIssuerRegexp == "" {
			return ref, errors.New("--cosign-certificate-oidc-issuer or --cosign-certificate-oidc-issuer-regexp is required for Cosign verification in keyless mode")
		}
		if certOidcIssuer != "" {
			cosignCmd.Args = append(cosignCmd.Args, "--certificate-oidc-issuer", certOidcIssuer)
		}
		if certOidcIssuerRegexp != "" {
			cosignCmd.Args = append(cosignCmd.Args, "--certificate-oidc-issuer-regexp", certOidcIssuerRegexp)
		}
		cosignCmd.Env = append(cosignCmd.Env, "COSIGN_EXPERIMENTAL=true")
	}

	cosignCmd.Args = append(cosignCmd.Args, ref)

	log.G(ctx).Debugf("running %s %v", cosignExecutable, cosignCmd.Args)

	err = processCosignIO(cosignCmd)
	if err != nil {
		return ref, err
	}
	if err := cosignCmd.Wait(); err != nil {
		return ref, err
	}

	return ref, nil
}

func processCosignIO(cosignCmd *exec.Cmd) error {
	stdout, err := cosignCmd.StdoutPipe()
	if err != nil {
		log.L.Warn("cosign: " + err.Error())
	}
	stderr, err := cosignCmd.StderrPipe()
	if err != nil {
		log.L.Warn("cosign: " + err.Error())
	}
	if err := cosignCmd.Start(); err != nil {
		// only return err if it's critical (cosign start failed.)
		return err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		log.L.Info("cosign: " + scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.L.Warn("cosign: " + err.Error())
	}

	errScanner := bufio.NewScanner(stderr)
	for errScanner.Scan() {
		log.L.Info("cosign: " + errScanner.Text())
	}
	if err := errScanner.Err(); err != nil {
		log.L.Warn("cosign: " + err.Error())
	}

	return nil
}
