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

package network

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestNetworkLsFilter(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	var net1, net2 = tID + "net-1", tID + "net-2"
	var label1 = tID + "=label-1"
	netID1 := base.Cmd("network", "create", "--label="+label1, net1).Out()
	defer base.Cmd("network", "rm", "-f", netID1).Run()

	netID2 := base.Cmd("network", "create", net2).Out()
	defer base.Cmd("network", "rm", "-f", netID2).Run()

	base.Cmd("network", "ls", "--quiet", "--filter", "label="+tID).AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 1 {
			return errors.New("expected at least 1 lines")
		}
		netNames := map[string]struct{}{
			netID1[:12]: {},
		}

		for _, name := range lines {
			_, ok := netNames[name]
			if !ok {
				return fmt.Errorf("unexpected netume %s found", name)
			}
		}
		return nil
	})

	base.Cmd("network", "ls", "--quiet", "--filter", "name="+net2).AssertOutWithFunc(func(stdout string) error {
		var lines = strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 1 {
			return errors.New("expected at least 1 lines")
		}
		netNames := map[string]struct{}{
			netID2[:12]: {},
		}

		for _, name := range lines {
			_, ok := netNames[name]
			if !ok {
				return fmt.Errorf("unexpected netume %s found", name)
			}
		}
		return nil
	})
}
