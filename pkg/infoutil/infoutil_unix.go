//go:build unix

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
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/containerd/log"
	"github.com/docker/go-units"
	"golang.org/x/sys/unix"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
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

func PrettyPrintInfoDockerCompat(stdout io.Writer, stderr io.Writer, info *dockercompat.Info, globalOptions types.GlobalCommandOptions) error {
	w := stdout
	debug := globalOptions.Debug
	fmt.Fprintf(w, "Client:\n")
	fmt.Fprintf(w, " Namespace:\t%s\n", globalOptions.Namespace)
	fmt.Fprintf(w, " Debug Mode:\t%v\n", debug)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Server:\n")
	fmt.Fprintf(w, " Server Version: %s\n", info.ServerVersion)
	// Storage Driver is not really Server concept for nerdctl, but mimics `docker info` output
	fmt.Fprintf(w, " Storage Driver: %s\n", info.Driver)
	fmt.Fprintf(w, " Logging Driver: %s\n", info.LoggingDriver)
	fmt.Fprintf(w, " Cgroup Driver: %s\n", info.CgroupDriver)
	fmt.Fprintf(w, " Cgroup Version: %s\n", info.CgroupVersion)
	fmt.Fprintf(w, " Plugins:\n")
	fmt.Fprintf(w, "  Log: %s\n", strings.Join(info.Plugins.Log, " "))
	fmt.Fprintf(w, "  Storage: %s\n", strings.Join(info.Plugins.Storage, " "))
	fmt.Fprintf(w, " Security Options:\n")
	for _, s := range info.SecurityOptions {
		m, err := strutil.ParseCSVMap(s)
		if err != nil {
			log.L.WithError(err).Warnf("unparsable security option %q", s)
			continue
		}
		name := m["name"]
		if name == "" {
			log.L.Warnf("unparsable security option %q", s)
			continue
		}
		fmt.Fprintf(w, "  %s\n", name)
		for k, v := range m {
			if k == "name" {
				continue
			}
			fmt.Fprintf(w, "   %s: %s\n", cases.Title(language.English).String(k), v)
		}
	}
	fmt.Fprintf(w, " Kernel Version: %s\n", info.KernelVersion)
	fmt.Fprintf(w, " Operating System: %s\n", info.OperatingSystem)
	fmt.Fprintf(w, " OSType: %s\n", info.OSType)
	fmt.Fprintf(w, " Architecture: %s\n", info.Architecture)
	fmt.Fprintf(w, " CPUs: %d\n", info.NCPU)
	fmt.Fprintf(w, " Total Memory: %s\n", units.BytesSize(float64(info.MemTotal)))
	fmt.Fprintf(w, " Name: %s\n", info.Name)
	fmt.Fprintf(w, " ID: %s\n", info.ID)

	fmt.Fprintln(w)
	if len(info.Warnings) > 0 {
		fmt.Fprintln(stderr, strings.Join(info.Warnings, "\n"))
	}
	return nil
}
