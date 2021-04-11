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

package main

import (
	"context"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/pkg/cap"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/dnsutil"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/portutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"net/url"
	"os"

	"path/filepath"
	"strings"
)

func NewContainer(ctx context.Context, clicontext *cli.Context, client *containerd.Client, dataStore, id string) (containerd.Container, error) {
	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
	)

	opts = append(opts,
		oci.WithDefaultSpec(),
		oci.WithDefaultUnixDevices,
		oci.WithMounts([]specs.Mount{
			{Type: "cgroup", Source: "cgroup", Destination: "/sys/fs/cgroup", Options: []string{"ro", "nosuid", "noexec", "nodev"}},
		}),
		WithoutRunMount, // unmount default tmpfs on "/run": https://github.com/containerd/nerdctl/issues/157
	)

	rootfsOpts, rootfsCOpts, ensuredImage, err := generateRootfsOpts(ctx, client, clicontext, id)
	if err != nil {
		return nil, err
	}
	opts = append(opts, rootfsOpts...)
	cOpts = append(cOpts, rootfsCOpts...)

	if wd := clicontext.String("workdir"); wd != "" {
		opts = append(opts, oci.WithProcessCwd(wd))
	}

	if env := strutil.DedupeStrSlice(clicontext.StringSlice("env")); len(env) > 0 {
		opts = append(opts, oci.WithEnv(env))
	}

	flagI := clicontext.Bool("i")
	flagT := clicontext.Bool("t")
	flagD := clicontext.Bool("d")
	name := clicontext.String("name")
	ns := clicontext.String("namespace")

	stateDir, err := getContainerStateDirPath(clicontext, dataStore, id)
	if err != nil {
		return nil, err
	}

	if flagI {
		if flagD {
			return nil, errors.New("currently flag -i and -d cannot be specified together (FIXME)")
		}
	}

	if flagT {
		if flagD {
			return nil, errors.New("currently flag -t and -d cannot be specified together (FIXME)")
		}
		if !flagI {
			return nil, errors.New("currently flag -t needs -i to be specified together (FIXME)")
		}
		opts = append(opts, oci.WithTTY)
	}

	var imageVolumes map[string]struct{}
	if ensuredImage != nil {
		imageVolumes = ensuredImage.ImageConfig.Volumes
	}
	mountOpts, anonVolumes, err := generateMountOpts(clicontext, imageVolumes)
	if err != nil {
		return nil, err
	} else {
		opts = append(opts, mountOpts...)
	}

	logURI, err := generateLogUri(flagD, dataStore)
	if err != nil {
		return nil, err
	}

	restartOpts, err := generateRestartOpts(clicontext.String("restart"), logURI)
	if err != nil {
		return nil, err
	}
	cOpts = append(cOpts, restartOpts...)

	// DedupeStrSlice is required as a workaround for urfave/cli bug
	// https://github.com/containerd/nerdctl/issues/108
	// https://github.com/urfave/cli/issues/1254
	portSlice := strutil.DedupeStrSlice(clicontext.StringSlice("p"))
	netSlice := strutil.DedupeStrSlice(clicontext.StringSlice("net"))

	ports := make([]gocni.PortMapping, len(portSlice))
	if len(netSlice) != 1 {
		return nil, errors.New("currently, number of networks must be 1")
	}
	switch netstr := netSlice[0]; netstr {
	case "none":
		// NOP
	case "host":
		opts = append(opts, oci.WithHostNamespace(specs.NetworkNamespace), oci.WithHostHostsFile, oci.WithHostResolvconf)
	default:
		// We only verify flags and generate resolv.conf here.
		// The actual network is configured in the oci hook.
		e := &netutil.CNIEnv{
			Path:        clicontext.String("cni-path"),
			NetconfPath: clicontext.String("cni-netconfpath"),
		}
		ll, err := netutil.ConfigLists(e)
		if err != nil {
			return nil, err
		}
		var netconflist *netutil.NetworkConfigList
		for _, f := range ll {
			if f.Name == netstr {
				netconflist = f
				break
			}
		}
		if netconflist == nil {
			return nil, errors.Errorf("no such network: %q", netstr)
		}

		resolvConfPath := filepath.Join(stateDir, "resolv.conf")
		if err := dnsutil.WriteResolvConfFile(resolvConfPath, strutil.DedupeStrSlice(clicontext.StringSlice("dns"))); err != nil {
			return nil, err
		}

		ns := clicontext.String("namespace")
		// the content of /etc/hosts is created in OCI Hook
		etcHostsPath, err := hostsstore.AllocHostsFile(dataStore, ns, id)
		if err != nil {
			return nil, err
		}
		opts = append(opts, withCustomResolvConf(resolvConfPath), withCustomHosts(etcHostsPath))
		for i, p := range portSlice {
			pm, err := portutil.ParseFlagP(p)
			if err != nil {
				return nil, err
			}
			ports[i] = *pm
		}
	}

	hostname := id[0:12]
	if customHostname := clicontext.String("hostname"); customHostname != "" {
		hostname = customHostname
	}
	opts = append(opts, oci.WithHostname(hostname))

	hookOpt, err := withNerdctlOCIHook(clicontext, id, stateDir)
	if err != nil {
		return nil, err
	}
	opts = append(opts, hookOpt)

	if cgOpts, err := generateCgroupOpts(clicontext, id); err != nil {
		return nil, err
	} else {
		opts = append(opts, cgOpts...)
	}

	if uOpts, err := generateUserOpts(clicontext); err != nil {
		return nil, err
	} else {
		opts = append(opts, uOpts...)
	}

	securityOptsMaps := strutil.ConvertKVStringsToMap(strutil.DedupeStrSlice(clicontext.StringSlice("security-opt")))
	if secOpts, err := generateSecurityOpts(securityOptsMaps); err != nil {
		return nil, err
	} else {
		opts = append(opts, secOpts...)
	}

	if capOpts, err := generateCapOpts(
		strutil.DedupeStrSlice(clicontext.StringSlice("cap-add")),
		strutil.DedupeStrSlice(clicontext.StringSlice("cap-drop"))); err != nil {
		return nil, err
	} else {
		opts = append(opts, capOpts...)
	}

	if clicontext.Bool("privileged") {
		opts = append(opts, privilegedOpts...)
	}

	rtCOpts, err := generateRuntimeCOpts(clicontext)
	if err != nil {
		return nil, err
	}
	cOpts = append(cOpts, rtCOpts...)

	lCOpts, err := withContainerLabels(clicontext)
	if err != nil {
		return nil, err
	}
	cOpts = append(cOpts, lCOpts...)

	ilOpt, err := withInternalLabels(ns, name, hostname, stateDir, netSlice, ports, logURI, anonVolumes)
	if err != nil {
		return nil, err
	}
	cOpts = append(cOpts, ilOpt)

	opts = append(opts, propagateContainerdLabelsToOCIAnnotations())

	opts = append(opts, WithSysctls(strutil.ConvertKVStringsToMap(clicontext.StringSlice("sysctl"))))

	var s specs.Spec
	spec := containerd.WithSpec(&s, opts...)
	cOpts = append(cOpts, spec)

	return client.NewContainer(ctx, id, cOpts...)
}

func runComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring {
		defaultBashComplete(clicontext)
		return
	}
	if coco.flagTakesValue {
		w := clicontext.App.Writer
		switch coco.flagName {
		case "restart":
			fmt.Fprintln(w, "always")
			fmt.Fprintln(w, "no")
			return
		case "pull":
			fmt.Fprintln(w, "always")
			fmt.Fprintln(w, "missing")
			fmt.Fprintln(w, "never")
			return
		case "cgroupns":
			fmt.Fprintln(w, "host")
			fmt.Fprintln(w, "private")
			return
		case "security-opt":
			fmt.Fprintln(w, "seccomp=")
			fmt.Fprintln(w, "apparmor="+defaults.AppArmorProfileName)
			fmt.Fprintln(w, "no-new-privileges")
			return
		case "cap-add", "cap-drop":
			for _, c := range cap.Known() {
				// "CAP_SYS_ADMIN" -> "sys_admin"
				s := strings.ToLower(strings.TrimPrefix(c, "CAP_"))
				fmt.Fprintln(w, s)
			}
			return
		case "net", "network":
			bashCompleteNetworkNames(clicontext, nil)
			return
		}
		defaultBashComplete(clicontext)
		return
	}
	// show image names, unless we have "--rootfs" flag
	if clicontext.Bool("rootfs") {
		defaultBashComplete(clicontext)
		return
	}
	bashCompleteImageNames(clicontext)
}

func generateLogUri(flagD bool, dataStore string) (string, error) {
	var logURI string
	if flagD {
		if lu, err := generateLogURI(dataStore); err != nil {
			return "", err
		} else if lu != nil {
			logURI = lu.String()
		}
	}
	return logURI, nil
}

func generateLogURI(dataStore string) (*url.URL, error) {
	selfExe, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return nil, err
	}
	args := map[string]string{
		logging.MagicArgv1: dataStore,
	}
	return cio.LogURIGenerator("binary", selfExe, args)
}
