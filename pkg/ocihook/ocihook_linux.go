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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/contrib/apparmor"
	pkgapparmor "github.com/containerd/containerd/pkg/apparmor"
	"github.com/containerd/go-cni"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	rlkclient "github.com/rootless-containers/rootlesskit/pkg/api/client"
	"github.com/sirupsen/logrus"
)

func loadapparmor() {
	// ensure that the default profile is loaded to the host
	if err := apparmor.LoadDefaultProfile(defaults.AppArmorProfileName); err != nil {
		logrus.WithError(err).Errorf("failed to load AppArmor profile %q", defaults.AppArmorProfileName)
	}
}
