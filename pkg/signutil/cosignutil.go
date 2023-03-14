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
	"os"
	"os/exec"
	"strings"

	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/sirupsen/logrus"
)

// SignCosign signs an image(`rawRef`) using a cosign private key (`keyRef`)
func SignCosign(rawRef string, keyRef string) error {
	cosignExecutable, err := exec.LookPath("cosign")
	if err != nil {
		logrus.WithError(err).Error("cosign executable not found in path $PATH")
		logrus.Info("you might consider installing cosign from: https://docs.sigstore.dev/cosign/installation")
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

	cosignCmd.Args = append(cosignCmd.Args, rawRef)

	logrus.Debugf("running %s %v", cosignExecutable, cosignCmd.Args)

	err = processCosignIO(cosignCmd)
	if err != nil {
		return err
	}

	if err := cosignCmd.Wait(); err != nil {
		return err
	}

	return nil
}

// VerifyCosign verifies an image(`rawRef`) with a cosign public key(`keyRef`)
// `hostsDirs` are used to resolve image `rawRef`
func VerifyCosign(ctx context.Context, rawRef string, keyRef string, hostsDirs []string) (string, error) {
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

	// if key is empty, use keyless mode(experimental)
	if keyRef != "" {
		cosignCmd.Args = append(cosignCmd.Args, "--key", keyRef)
	} else {
		cosignCmd.Env = append(cosignCmd.Env, "COSIGN_EXPERIMENTAL=true")
	}

	cosignCmd.Args = append(cosignCmd.Args, ref)

	logrus.Debugf("running %s %v", cosignExecutable, cosignCmd.Args)

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
		logrus.Warn("cosign: " + err.Error())
	}
	stderr, err := cosignCmd.StderrPipe()
	if err != nil {
		logrus.Warn("cosign: " + err.Error())
	}
	if err := cosignCmd.Start(); err != nil {
		// only return err if it's critical (cosign start failed.)
		return err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		logrus.Info("cosign: " + scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logrus.Warn("cosign: " + err.Error())
	}

	errScanner := bufio.NewScanner(stderr)
	for errScanner.Scan() {
		logrus.Info("cosign: " + errScanner.Text())
	}
	if err := errScanner.Err(); err != nil {
		logrus.Warn("cosign: " + err.Error())
	}

	return nil
}
