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

// SignNotation signs an image(`rawRef`) using a notation key name (`keyNameRef`)
func SignNotation(rawRef string, keyNameRef string) error {
	notationExecutable, err := exec.LookPath("notation")
	if err != nil {
		logrus.WithError(err).Error("notation executable not found in path $PATH")
		logrus.Info("you might consider installing notation from: https://notaryproject.dev/docs/installation/cli/")
		return err
	}

	notationCmd := exec.Command(notationExecutable, []string{"sign"}...)
	notationCmd.Env = os.Environ()

	// If keyNameRef is empty, don't append --key to notation command. This will cause using the notation default key.
	if keyNameRef != "" {
		notationCmd.Args = append(notationCmd.Args, "--key", keyNameRef)
	}

	notationCmd.Args = append(notationCmd.Args, rawRef)

	logrus.Debugf("running %s %v", notationExecutable, notationCmd.Args)

	err = processNotationIO(notationCmd)
	if err != nil {
		return err
	}

	if err := notationCmd.Wait(); err != nil {
		return err
	}

	return nil
}

// VerifyNotation verifies an image(`rawRef`) with the pre-configured notation trust policy
// `hostsDirs` are used to resolve image `rawRef`
func VerifyNotation(ctx context.Context, rawRef string, hostsDirs []string) (string, error) {
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

	notationExecutable, err := exec.LookPath("notation")
	if err != nil {
		logrus.WithError(err).Error("notation executable not found in path $PATH")
		logrus.Info("you might consider installing notation from: https://notaryproject.dev/docs/installation/cli/")
		return ref, err
	}

	notationCmd := exec.Command(notationExecutable, []string{"verify"}...)
	notationCmd.Env = os.Environ()

	notationCmd.Args = append(notationCmd.Args, ref)

	logrus.Debugf("running %s %v", notationExecutable, notationCmd.Args)

	err = processNotationIO(notationCmd)
	if err != nil {
		return ref, err
	}
	if err := notationCmd.Wait(); err != nil {
		return ref, err
	}

	return ref, nil
}

func processNotationIO(notationCmd *exec.Cmd) error {
	stdout, err := notationCmd.StdoutPipe()
	if err != nil {
		logrus.Warn("notation: " + err.Error())
	}
	stderr, err := notationCmd.StderrPipe()
	if err != nil {
		logrus.Warn("notation: " + err.Error())
	}
	if err := notationCmd.Start(); err != nil {
		// only return err if it's critical (notation start failed.)
		return err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		logrus.Info("notation: " + scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logrus.Warn("notation: " + err.Error())
	}

	errScanner := bufio.NewScanner(stderr)
	for errScanner.Scan() {
		logrus.Info("notation: " + errScanner.Text())
	}
	if err := errScanner.Err(); err != nil {
		logrus.Warn("notation: " + err.Error())
	}

	return nil
}
