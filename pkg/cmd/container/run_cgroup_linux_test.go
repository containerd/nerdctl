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

package container

import "testing"

func TestGenerateCgroupPathSystemdDefaults(t *testing.T) {
	tests := []struct {
		name     string
		rootless bool
		want     string
	}{
		{name: "rootful", want: "system.slice:nerdctl:container-id"},
		{name: "rootless", rootless: true, want: ":nerdctl:container-id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := generateCgroupPathWithRootless("container-id", "systemd", "", tc.rootless)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGenerateCgroupPathWithExplicitParent(t *testing.T) {
	got, err := generateCgroupPathWithRootless("container-id", "systemd", "workload.slice", true)
	if err != nil {
		t.Fatal(err)
	}
	if want := "workload.slice:nerdctl:container-id"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
