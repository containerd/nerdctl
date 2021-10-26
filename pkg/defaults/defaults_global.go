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

package defaults

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func GetglobalBool(cmd *cobra.Command, key string) (bool, error) {
	globalBoolMap := map[string]bool{
		"debug":             false,
		"debug-full":        false,
		"insecure-registry": false,
	}
	if cmd.Flags().Changed(key) {
		return cmd.Flags().GetBool(key)
	}
	vp := viper.GetViper()
	configBool := vp.Get(key)
	if configBool != nil {
		return configBool.(bool), nil
	} else {
		if val, ok := globalBoolMap[key]; ok {
			return val, nil
		} else {
			return false, nil
		}
	}
}

func GetglobalString(cmd *cobra.Command, key string) (string, error) {
	globalEnvMap := map[string]string{
		"address":         "CONTAINERD_ADDRESS",
		"namespace":       "CONTAINERD_NAMESPACE",
		"snapshotter":     "CONTAINERD_SNAPSHOTTER",
		"cni-path":        "CNI_PATH",
		"cni-netconfpath": "NETCONFPATH",
	}
	if cmd.Flags().Changed(key) {
		return cmd.Flags().GetString(key)
	}
	if envKey, ok := globalEnvMap[key]; ok {
		envString := os.Getenv(envKey)
		if envString != "" {
			return envString, nil
		}
	}
	vp := viper.GetViper()
	configString := vp.GetString(key)
	if configString != "" {
		return configString, nil
	}
	if defaultValue, err := cmd.Flags().GetString(key); err == nil {
		return defaultValue, nil
	} else {
		return "", nil
	}
}
