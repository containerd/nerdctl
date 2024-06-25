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

package bypass4netnsutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/nerdctl/v2/pkg/annotations"
	"github.com/opencontainers/runtime-spec/specs-go"
	b4nnoci "github.com/rootless-containers/bypass4netns/pkg/oci"
)

func generateSecurityOpt(listenerPath string) (oci.SpecOpts, error) {
	opt := func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Linux.Seccomp == nil {
			s.Linux.Seccomp = b4nnoci.GetDefaultSeccompProfile(listenerPath)
		} else {
			sc, err := b4nnoci.TranslateSeccompProfile(*s.Linux.Seccomp, listenerPath)
			if err != nil {
				return err
			}
			s.Linux.Seccomp = sc
		}
		return nil
	}
	return opt, nil
}

func GenerateBypass4netnsOpts(securityOptsMaps map[string]string, annotationsMap map[string]string, id string) ([]oci.SpecOpts, error) {
	b4nn, ok := annotationsMap[annotations.Bypass4netns]
	if !ok {
		return nil, nil
	}

	b4nnEnable, err := strconv.ParseBool(b4nn)
	if err != nil {
		return nil, err
	}

	if !b4nnEnable {
		return nil, nil
	}

	socketPath, err := GetSocketPathByID(id)
	if err != nil {
		return nil, err
	}

	err = CreateSocketDir()
	if err != nil {
		return nil, err
	}

	opts := []oci.SpecOpts{}
	opt, err := generateSecurityOpt(socketPath)
	if err != nil {
		return nil, err
	}
	opts = append(opts, opt)

	return opts, nil
}

func getXDGRuntimeDir() (string, error) {
	if xrd := os.Getenv("XDG_RUNTIME_DIR"); xrd != "" {
		return xrd, nil
	}
	return "", fmt.Errorf("environment variable XDG_RUNTIME_DIR is not set")
}

func CreateSocketDir() error {
	xdgRuntimeDir, err := getXDGRuntimeDir()
	if err != nil {
		return err
	}
	dirPath := filepath.Join(xdgRuntimeDir, "bypass4netns")
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		err = os.MkdirAll(dirPath, 0775)
		if err != nil {
			return err
		}
	}

	return nil
}

func GetBypass4NetnsdDefaultSocketPath() (string, error) {
	xdgRuntimeDir, err := getXDGRuntimeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(xdgRuntimeDir, "bypass4netnsd.sock"), nil
}

func GetSocketPathByID(id string) (string, error) {
	xdgRuntimeDir, err := getXDGRuntimeDir()
	if err != nil {
		return "", err
	}

	socketPath := filepath.Join(xdgRuntimeDir, "bypass4netns", id[0:15]+".sock")
	return socketPath, nil
}

func GetPidFilePathByID(id string) (string, error) {
	xdgRuntimeDir, err := getXDGRuntimeDir()
	if err != nil {
		return "", err
	}

	socketPath := filepath.Join(xdgRuntimeDir, "bypass4netns", id[0:15]+".pid")
	return socketPath, nil
}

func IsBypass4netnsEnabled(annotationsMap map[string]string) (enabled, bindEnabled bool, err error) {
	if b4nn, ok := annotationsMap[annotations.Bypass4netns]; ok {
		enabled, err = strconv.ParseBool(b4nn)
		if err != nil {
			return
		}
		bindEnabled = enabled
		if s, ok := annotationsMap[annotations.Bypass4netnsIgnoreBind]; ok {
			var bindDisabled bool
			bindDisabled, err = strconv.ParseBool(s)
			if err != nil {
				return
			}
			bindEnabled = !bindDisabled
		}
	}
	return
}
