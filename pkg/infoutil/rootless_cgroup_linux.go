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

package infoutil

import (
	"context"
	"fmt"
	"strconv"

	"github.com/coreos/go-systemd/v22/dbus"
)

// systemdUserManagerControlGroup returns the cgroup containing the systemd user
// manager. Runc asks this manager to create rootless container scopes, so its
// ControlGroup is authoritative even when nerdctl runs in another cgroup.
func systemdUserManagerControlGroup(ctx context.Context) (string, error) {
	conn, err := dbus.NewUserConnectionContext(ctx)
	if err != nil {
		return "", fmt.Errorf("connecting to systemd user manager: %w", err)
	}
	defer conn.Close()

	property, err := conn.GetManagerProperty("ControlGroup")
	if err != nil {
		return "", fmt.Errorf("getting systemd user manager ControlGroup: %w", err)
	}
	groupPath, err := strconv.Unquote(property)
	if err != nil {
		return "", fmt.Errorf("decoding systemd user manager ControlGroup property %q: %w", property, err)
	}
	return groupPath, nil
}
