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
	"bufio"
	"context"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/containerd/cgroups"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/pkg/apparmor"
	"github.com/containerd/containerd/services/introspection"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	ptypes "github.com/gogo/protobuf/types"
	"golang.org/x/sys/unix"
)

// UnameR returns `uname -r`
func UnameR() string {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		// error is unlikely to happen
		return ""
	}
	var s string
	for _, f := range utsname.Release {
		if f == 0 {
			break
		}
		s += string(f)
	}
	return s
}

// UnameM returns `uname -m`
func UnameM() string {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		// error is unlikely to happen
		return ""
	}
	var s string
	for _, f := range utsname.Machine {
		if f == 0 {
			break
		}
		s += string(f)
	}
	return s
}

const UnameO = "GNU/Linux"

func DistroName() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return UnameO
	}
	defer f.Close()
	return distroName(f)
}

func distroName(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	var name, version string
	for scanner.Scan() {
		line := scanner.Text()
		k, v := getOSReleaseAttrib(line)
		switch k {
		case "PRETTY_NAME":
			return v
		case "NAME":
			name = v
		case "VERSION":
			version = v
		}
	}
	if name != "" {
		if version != "" {
			return name + " " + version
		}
		return name
	}
	return UnameO
}

var osReleaseAttribRegex = regexp.MustCompile(`([^\s=]+)\s*=\s*("{0,1})([^"]*)("{0,1})`)

func getOSReleaseAttrib(line string) (string, string) {
	splitBySlash := strings.SplitN(line, "#", 2)
	l := strings.TrimSpace(splitBySlash[0])
	x := osReleaseAttribRegex.FindAllStringSubmatch(l, -1)
	if len(x) >= 1 && len(x[0]) > 3 {
		return x[0][1], x[0][3]
	}
	return "", ""
}
