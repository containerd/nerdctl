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

package apparmor

import (
	"fmt"

	"github.com/containerd/containerd/contrib/apparmor"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newApparmorLoadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "load",
		Short:         fmt.Sprintf("Load the default AppArmor profile %q. Requires root.", defaults.AppArmorProfileName),
		Args:          cobra.NoArgs,
		RunE:          LoadAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return cmd
}

func LoadAction(cmd *cobra.Command, args []string) error {
	logrus.Infof("Loading profile %q", defaults.AppArmorProfileName)
	return apparmor.LoadDefaultProfile(defaults.AppArmorProfileName)
}
