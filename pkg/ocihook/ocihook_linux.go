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

package ocihook

import (
	"github.com/containerd/containerd/v2/contrib/apparmor"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/apparmorutil"
	"github.com/containerd/nerdctl/v2/pkg/defaults"
)

func loadAppArmor() {
	if !apparmorutil.CanLoadNewProfile() {
		return
	}
	// ensure that the default profile is loaded to the host
	if err := apparmor.LoadDefaultProfile(defaults.AppArmorProfileName); err != nil {
		log.L.WithError(err).Errorf("failed to load AppArmor profile %q", defaults.AppArmorProfileName)
		// We do not abort here. This is by design, and not a security issue.
		//
		// If the container is configured to use the default AppArmor profile
		// but the profile was not actually loaded, runc will fail.
	}
}
