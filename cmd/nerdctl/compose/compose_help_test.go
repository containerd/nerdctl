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

package compose

import (
	"bytes"
	"strings"
	"testing"
)

func TestComposeHelpHidesAliasImplementationFlags(t *testing.T) {
	cmd := Command()

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := stdout.String()
	if strings.Contains(out, "-f, --f") {
		t.Fatalf("help output unexpectedly contains alias implementation flag\n%s", out)
	}
	expected := "--file stringArray           Specify an alternate compose file (aliases: -f)"
	if !strings.Contains(out, expected) {
		t.Fatalf("help output missing %q\n%s", expected, out)
	}
}

func TestComposeHiddenFileAliasStillParses(t *testing.T) {
	cmd := Command()

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"-f", "compose.yaml", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if got := cmd.Flag("file").Value.String(); got != "[compose.yaml]" {
		t.Fatalf("file flag = %q, want %q", got, "[compose.yaml]")
	}
}
