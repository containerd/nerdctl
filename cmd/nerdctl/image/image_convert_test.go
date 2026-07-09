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

package image

import (
	"testing"

	"github.com/spf13/cobra"
	"gotest.tools/v3/assert"
)

func TestConvertOptionsOverlaybdVsize(t *testing.T) {
	testCases := []struct {
		name  string
		flags map[string]string
		want  int
	}{
		{
			name: "default",
			want: 64,
		},
		{
			name: "explicit value",
			flags: map[string]string{
				"overlaybd-vsize": "128",
			},
			want: 128,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cmd := convertCommand()
			addRootFlagsForConvertOptionsTest(t, cmd)
			assert.NilError(t, cmd.Flags().Set("overlaybd", "true"))
			for name, value := range tc.flags {
				assert.NilError(t, cmd.Flags().Set(name, value))
			}

			got, err := convertOptions(cmd)
			assert.NilError(t, err)
			assert.Equal(t, got.OverlaybdVsize, tc.want)
		})
	}
}

func addRootFlagsForConvertOptionsTest(t *testing.T, cmd *cobra.Command) {
	t.Helper()

	flags := cmd.Flags()
	flags.Bool("debug", false, "")
	flags.Bool("debug-full", false, "")
	flags.String("address", "", "")
	flags.String("namespace", "default", "")
	flags.String("snapshotter", "", "")
	flags.String("cni-path", "", "")
	flags.String("cni-netconfpath", "", "")
	flags.String("data-root", t.TempDir(), "")
	flags.String("cgroup-manager", "", "")
	flags.Bool("insecure-registry", false, "")
	flags.StringSlice("hosts-dir", nil, "")
	flags.Bool("experimental", false, "")
	flags.String("host-gateway-ip", "", "")
	flags.String("bridge-ip", "", "")
	flags.Bool("kube-hide-dupe", false, "")
	flags.StringSlice("cdi-spec-dirs", nil, "")
	flags.StringSlice("global-dns", nil, "")
	flags.StringSlice("global-dns-opts", nil, "")
	flags.StringSlice("global-dns-search", nil, "")
	flags.Bool("selinux-enabled", false, "")
}
