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
	"fmt"
	"os/exec"
	"strings"

	cniutils "github.com/containernetworking/plugins/pkg/utils"

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

// cleanupIptablesRules cleans up iptables rules related to the container
func cleanupIptablesRules(containerID string, cniNames []string) error {
	// Check if iptables command exists
	if _, err := exec.LookPath("iptables"); err != nil {
		return fmt.Errorf("iptables command not found: %w", err)
	}

	// Tables to check for rules
	tables := []string{"nat", "filter", "mangle"}

	for _, table := range tables {
		// Get all iptables rules for this table
		cmd := exec.Command("iptables", "-t", table, "-S")
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.L.WithError(err).Warnf("failed to list iptables rules for table %s", table)
			continue
		}

		// Find and delete rules related to the container
		rules := strings.Split(string(output), "\n")
		for _, rule := range rules {
			if strings.Contains(rule, containerID) {
				// Execute delete command
				deleteCmd := exec.Command("sh", "-c", "--", fmt.Sprintf(`iptables -t %s -D %s`, table, rule[3:]))
				if deleteOutput, err := deleteCmd.CombinedOutput(); err != nil {
					log.L.WithError(err).Warnf("failed to delete iptables rule: %s, output: %s", rule, string(deleteOutput))
				} else {
					log.L.Debugf("deleted iptables rule: %s", rule)
				}
			}
		}
	}

	// Delete CNI chains related to the container
	for _, cniName := range cniNames {
		chain := cniutils.FormatChainName(cniName, containerID)
		deleteCmd := exec.Command("iptables", "-t", "nat", "-X", chain)
		if deleteOutput, err := deleteCmd.CombinedOutput(); err != nil {
			log.L.WithError(err).Warnf("failed to delete iptables chain: %s, output: %s", chain, string(deleteOutput))
		} else {
			log.L.Debugf("deleted iptables chain: %s", chain)
		}
	}

	return nil
}
