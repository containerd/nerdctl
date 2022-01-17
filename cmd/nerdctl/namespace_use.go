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
	"os"
	"path/filepath"

	ncdefaults "github.com/containerd/nerdctl/pkg/defaults"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	LastNamespace = "last-namespace"
)

func newNamespaceUseCommand() *cobra.Command {
	namespaceUseCommand := &cobra.Command{
		Use:           "use",
		Short:         "use a namespace",
		RunE:          namespaceUseAction,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return namespaceUseCommand
}

func namespaceUseAction(cmd *cobra.Command, args []string) error {
	tomlPath := ncdefaults.NerdctlTOML()
	if v, ok := os.LookupEnv("NERDCTL_TOML"); ok {
		tomlPath = v
	}
	cfg, err := NewConfigFromTOML(tomlPath)
	if err != nil {
		return err
	}
	namespace := args[0]
	if namespace == "-" {
		lastNS, err := os.ReadFile(filepath.Join(ncdefaults.NerdctlPath(), LastNamespace))
		if err == nil {
			namespace = string(lastNS)
		} else {
			logrus.Debugf("missing last namespace: %v", err)
			namespace = "default"
		}
	}
	if err := writeLastNamespace(cfg.Namespace); err != nil {
		return err
	}
	cfg.Namespace = namespace
	if err := cfg.MarshalToTOML(tomlPath); err != nil {
		return err
	}
	logrus.Infof("using namespace %q", namespace)
	return nil
}

func writeLastNamespace(namespace string) error {
	path := ncdefaults.NerdctlPath()
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	last := filepath.Join(path, LastNamespace)
	if err := os.WriteFile(last, []byte(namespace), 0644); err != nil {
		return err
	}
	return nil
}
