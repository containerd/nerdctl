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

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	nerdClient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/run"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/namestore"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	opts2 "github.com/docker/cli/opts"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// FIXME: split to smaller functions
func CreateContainer(ctx context.Context, cmd *cobra.Command, client *containerd.Client, args []string, platform string, flagI, flagT, flagD bool) (containerd.Container, func(), error) {
	// simulate the behavior of double dash
	newArg := []string{}
	if len(args) >= 2 && args[1] == "--" {
		newArg = append(newArg, args[:1]...)
		newArg = append(newArg, args[2:]...)
		args = newArg
	}
	var internalLabels common.InternalLabels
	internalLabels.Platform = platform

	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return nil, nil, err
	}
	internalLabels.Namespace = ns

	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
		id    = idgen.GenerateID()
	)

	cidfile, err := cmd.Flags().GetString("cidfile")
	if err != nil {
		return nil, nil, err
	}
	if cidfile != "" {
		if err := WriteCIDFile(cidfile, id); err != nil {
			return nil, nil, err
		}
	}

	dataStore, err := nerdClient.GetDataStore(cmd)
	if err != nil {
		return nil, nil, err
	}

	stateDir, err := common.GetContainerStateDirPath(cmd, dataStore, id)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, nil, err
	}
	internalLabels.StateDir = stateDir

	opts = append(opts,
		oci.WithDefaultSpec(),
	)

	opts, internalLabels, err = run.SetPlatformOptions(ctx, opts, cmd, client, id, internalLabels)
	if err != nil {
		return nil, nil, err
	}

	rootfsOpts, rootfsCOpts, ensuredImage, err := GenerateRootfsOpts(ctx, client, platform, cmd, args, id)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, rootfsOpts...)
	cOpts = append(cOpts, rootfsCOpts...)

	wd, err := cmd.Flags().GetString("workdir")
	if err != nil {
		return nil, nil, err
	}
	if wd != "" {
		opts = append(opts, oci.WithProcessCwd(wd))
	}

	envFile, err := cmd.Flags().GetStringSlice("env-file")
	if err != nil {
		return nil, nil, err
	}
	env, err := cmd.Flags().GetStringArray("env")
	if err != nil {
		return nil, nil, err
	}
	envs, err := GenerateEnvs(envFile, env)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, oci.WithEnv(envs))

	if flagI {
		if flagD {
			return nil, nil, errors.New("currently flag -i and -d cannot be specified together (FIXME)")
		}
	}

	if flagT {
		if flagD {
			return nil, nil, errors.New("currently flag -t and -d cannot be specified together (FIXME)")
		}
		opts = append(opts, oci.WithTTY)
	}

	mountOpts, anonVolumes, mountPoints, err := utils.GenerateMountOpts(ctx, cmd, client, ensuredImage)
	if err != nil {
		return nil, nil, err
	}
	internalLabels.AnonVolumes = anonVolumes
	internalLabels.MountPoints = mountPoints
	opts = append(opts, mountOpts...)

	var logURI string
	if flagD {
		// json-file is the built-in and default log driver for nerdctl
		logDriver, err := cmd.Flags().GetString("log-driver")
		if err != nil {
			return nil, nil, err
		}

		// check if log driver is a valid uri. If it is a valid uri and scheme is not
		if u, err := url.Parse(logDriver); err == nil && u.Scheme != "" {
			logURI = logDriver
		} else {
			logOptMap, err := ParseKVStringsMapFromLogOpt(cmd, logDriver)
			if err != nil {
				return nil, nil, err
			}
			logDriverInst, err := logging.GetDriver(logDriver, logOptMap)
			if err != nil {
				return nil, nil, err
			}
			if err := logDriverInst.Init(dataStore, ns, id); err != nil {
				return nil, nil, err
			}
			logConfig := &logging.LogConfig{
				Driver: logDriver,
				Opts:   logOptMap,
			}
			logConfigB, err := json.Marshal(logConfig)
			if err != nil {
				return nil, nil, err
			}
			logConfigFilePath := logging.LogConfigFilePath(dataStore, ns, id)
			if err = os.WriteFile(logConfigFilePath, logConfigB, 0600); err != nil {
				return nil, nil, err
			}
			if lu, err := GenerateLogURI(dataStore); err != nil {
				return nil, nil, err
			} else if lu != nil {
				logrus.Debugf("generated log driver: %s", lu.String())

				logURI = lu.String()
			}
		}
	}
	internalLabels.LogURI = logURI

	restartValue, err := cmd.Flags().GetString("restart")
	if err != nil {
		return nil, nil, err
	}
	restartOpts, err := run.GenerateRestartOpts(ctx, client, restartValue, logURI)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, restartOpts...)

	stopSignal, err := cmd.Flags().GetString("stop-signal")
	if err != nil {
		return nil, nil, err
	}
	stopTimeout, err := cmd.Flags().GetInt("stop-timeout")
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, WithStop(stopSignal, stopTimeout, ensuredImage))

	hostname := id[0:12]
	customHostname, err := cmd.Flags().GetString("hostname")
	if err != nil {
		return nil, nil, err
	}
	if customHostname != "" {
		hostname = customHostname
	}
	opts = append(opts, oci.WithHostname(hostname))
	internalLabels.Hostname = hostname
	// `/etc/hostname` does not exist on FreeBSD
	if runtime.GOOS == "linux" {
		hostnamePath := filepath.Join(stateDir, "hostname")
		if err := os.WriteFile(hostnamePath, []byte(hostname+"\n"), 0644); err != nil {
			return nil, nil, err
		}
		opts = append(opts, run.WithCustomEtcHostname(hostnamePath))
	}

	netOpts, netSlice, ipAddress, ports, macAddress, err := run.GenerateNetOpts(cmd, dataStore, stateDir, ns, id)
	if err != nil {
		return nil, nil, err
	}
	internalLabels.Networks = netSlice
	internalLabels.IPAddress = ipAddress
	internalLabels.Ports = ports
	internalLabels.MacAddress = macAddress
	opts = append(opts, netOpts...)

	hookOpt, err := WithNerdctlOCIHook(cmd, id)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, hookOpt)

	uOpts, err := run.GenerateUserOpts(cmd)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, uOpts...)

	gOpts, err := run.GenerateGroupsOpts(cmd)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, gOpts...)

	umaskOpts, err := run.GenerateUmaskOpts(cmd)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, umaskOpts...)

	rtCOpts, err := run.GenerateRuntimeCOpts(cmd)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, rtCOpts...)

	lCOpts, err := WithContainerLabels(cmd)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, lCOpts...)

	var containerNameStore namestore.NameStore
	name, err := cmd.Flags().GetString("name")
	if err != nil {
		return nil, nil, err
	}
	if name == "" && !cmd.Flags().Changed("name") {
		// Automatically set the container Name, unless `--Name=""` was explicitly specified.
		var imageRef string
		if ensuredImage != nil {
			imageRef = ensuredImage.Ref
		}
		name = referenceutil.SuggestContainerName(imageRef, id)
	}
	if name != "" {
		containerNameStore, err = namestore.New(dataStore, ns)
		if err != nil {
			return nil, nil, err
		}
		if err := containerNameStore.Acquire(name, id); err != nil {
			return nil, nil, err
		}
	}
	internalLabels.Name = name

	var pidFile string
	if cmd.Flags().Lookup("pidfile").Changed {
		pidFile, err = cmd.Flags().GetString("pidfile")
		if err != nil {
			return nil, nil, err
		}
	}
	internalLabels.PidFile = pidFile

	extraHosts, err := cmd.Flags().GetStringSlice("add-host")
	if err != nil {
		return nil, nil, err
	}
	extraHosts = strutil.DedupeStrSlice(extraHosts)
	for _, host := range extraHosts {
		if _, err := opts2.ValidateExtraHost(host); err != nil {
			return nil, nil, err
		}
	}
	internalLabels.ExtraHosts = extraHosts

	ilOpt, err := common.WithInternalLabels(internalLabels)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, ilOpt)

	opts = append(opts, PropagateContainerdLabelsToOCIAnnotations())

	var s specs.Spec
	spec := containerd.WithSpec(&s, opts...)
	cOpts = append(cOpts, spec)

	container, err := client.NewContainer(ctx, id, cOpts...)
	if err != nil {
		gcContainer := func() {
			var isErr bool
			if errE := os.RemoveAll(stateDir); errE != nil {
				isErr = true
			}
			if name != "" {
				var errE error
				if containerNameStore, errE = namestore.New(dataStore, ns); errE != nil {
					isErr = true
				}
				if errE = containerNameStore.Release(name, id); errE != nil {
					isErr = true
				}

			}
			if isErr {
				logrus.Warnf("failed to remove container %q", id)
			}
		}
		return nil, gcContainer, err
	}
	return container, nil, nil
}

func GenerateRootfsOpts(ctx context.Context, client *containerd.Client, platform string, cmd *cobra.Command, args []string, id string) ([]oci.SpecOpts, []containerd.NewContainerOpts, *imgutil.EnsuredImage, error) {
	var (
		ensured *imgutil.EnsuredImage
		err     error
	)
	imageless, err := cmd.Flags().GetBool("rootfs")
	if err != nil {
		return nil, nil, nil, err
	}
	if !imageless {
		pull, err := cmd.Flags().GetString("pull")
		if err != nil {
			return nil, nil, nil, err
		}
		var platformSS []string // len: 0 or 1
		if platform != "" {
			platformSS = append(platformSS, platform)
		}
		ocispecPlatforms, err := platformutil.NewOCISpecPlatformSlice(false, platformSS)
		if err != nil {
			return nil, nil, nil, err
		}
		rawRef := args[0]
		ensured, err = utils.EnsureImage(ctx, cmd, client, rawRef, ocispecPlatforms, pull, nil, false)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
	)
	if !imageless {
		cOpts = append(cOpts,
			containerd.WithImage(ensured.Image),
			containerd.WithSnapshotter(ensured.Snapshotter),
			containerd.WithNewSnapshot(id, ensured.Image),
			containerd.WithImageStopSignal(ensured.Image, "SIGTERM"),
		)

		if len(ensured.ImageConfig.Env) == 0 {
			opts = append(opts, oci.WithDefaultPathEnv)
		}
		for ind, env := range ensured.ImageConfig.Env {
			if strings.HasPrefix(env, "PATH=") {
				break
			} else {
				if ind == len(ensured.ImageConfig.Env)-1 {
					opts = append(opts, oci.WithDefaultPathEnv)
				}
			}
		}
	} else {
		absRootfs, err := filepath.Abs(args[0])
		if err != nil {
			return nil, nil, nil, err
		}
		opts = append(opts, oci.WithRootFSPath(absRootfs), oci.WithDefaultPathEnv)
	}

	// NOTE: "--entrypoint" can be set to an empty string, see TestRunEntrypoint* in run_test.go .
	entrypoint, err := cmd.Flags().GetStringArray("entrypoint")
	if err != nil {
		return nil, nil, nil, err
	}

	if !imageless && !cmd.Flag("entrypoint").Changed {
		opts = append(opts, oci.WithImageConfigArgs(ensured.Image, args[1:]))
	} else {
		if !imageless {
			opts = append(opts, oci.WithImageConfig(ensured.Image))
		}
		var processArgs []string
		if len(entrypoint) != 0 {
			processArgs = append(processArgs, entrypoint...)
		}
		if len(args) > 1 {
			processArgs = append(processArgs, args[1:]...)
		}
		if len(processArgs) == 0 {
			// error message is from Podman
			return nil, nil, nil, errors.New("no command or entrypoint provided, and no CMD or ENTRYPOINT from image")
		}
		opts = append(opts, oci.WithProcessArgs(processArgs...))
	}

	initProcessFlag, err := cmd.Flags().GetBool("init")
	if err != nil {
		return nil, nil, nil, err
	}
	initBinary, err := cmd.Flags().GetString("init-binary")
	if err != nil {
		return nil, nil, nil, err
	}
	if cmd.Flags().Changed("init-binary") {
		initProcessFlag = true
	}
	if initProcessFlag {
		binaryPath, err := exec.LookPath(initBinary)
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return nil, nil, nil, fmt.Errorf(`init binary %q not found`, initBinary)
			}
			return nil, nil, nil, err
		}
		inContainerPath := filepath.Join("/sbin", filepath.Base(initBinary))
		opts = append(opts, func(_ context.Context, _ oci.Client, _ *containers.Container, spec *oci.Spec) error {
			spec.Process.Args = append([]string{inContainerPath, "--"}, spec.Process.Args...)
			spec.Mounts = append([]specs.Mount{{
				Destination: inContainerPath,
				Type:        "bind",
				Source:      binaryPath,
				Options:     []string{"bind", "ro"},
			}}, spec.Mounts...)
			return nil
		})
	}

	readonly, err := cmd.Flags().GetBool("read-only")
	if err != nil {
		return nil, nil, nil, err
	}
	if readonly {
		opts = append(opts, oci.WithRootFSReadonly())
	}
	return opts, cOpts, ensured, nil
}

func GenerateLogURI(dataStore string) (*url.URL, error) {
	selfExe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := map[string]string{
		logging.MagicArgv1: dataStore,
	}

	return cio.LogURIGenerator("binary", selfExe, args)
}

func WithNerdctlOCIHook(cmd *cobra.Command, id string) (oci.SpecOpts, error) {
	selfExe, f := utils.GlobalFlags(cmd)
	args := append([]string{selfExe}, append(f, "internal", "oci-hook")...)
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Hooks == nil {
			s.Hooks = &specs.Hooks{}
		}
		crArgs := append(args, "createRuntime")
		s.Hooks.CreateRuntime = append(s.Hooks.CreateRuntime, specs.Hook{
			Path: selfExe,
			Args: crArgs,
			Env:  os.Environ(),
		})
		argsCopy := append([]string(nil), args...)
		psArgs := append(argsCopy, "postStop")
		s.Hooks.Poststop = append(s.Hooks.Poststop, specs.Hook{
			Path: selfExe,
			Args: psArgs,
			Env:  os.Environ(),
		})
		return nil
	}, nil
}

func WithContainerLabels(cmd *cobra.Command) ([]containerd.NewContainerOpts, error) {
	labelMap, err := common.ReadKVStringsMapfFromLabel(cmd)
	if err != nil {
		return nil, err
	}
	o := containerd.WithAdditionalContainerLabels(labelMap)
	return []containerd.NewContainerOpts{o}, nil
}

// parseKVStringsMapFromLogOpt parse log options KV entries and convert to Map
func ParseKVStringsMapFromLogOpt(cmd *cobra.Command, logDriver string) (map[string]string, error) {
	logOptArray, err := cmd.Flags().GetStringArray("log-opt")
	if err != nil {
		return nil, err
	}
	logOptArray = strutil.DedupeStrSlice(logOptArray)
	logOptMap := strutil.ConvertKVStringsToMap(logOptArray)
	if logDriver == "json-file" {
		if _, ok := logOptMap[logging.MaxSize]; !ok {
			delete(logOptMap, logging.MaxFile)
		}
	}
	if err := logging.ValidateLogOpts(logDriver, logOptMap); err != nil {
		return nil, err
	}
	return logOptMap, nil
}

func WithStop(stopSignal string, stopTimeout int, ensuredImage *imgutil.EnsuredImage) containerd.NewContainerOpts {
	return func(ctx context.Context, _ *containerd.Client, c *containers.Container) error {
		if c.Labels == nil {
			c.Labels = make(map[string]string)
		}
		var err error
		if ensuredImage != nil {
			stopSignal, err = containerd.GetOCIStopSignal(ctx, ensuredImage.Image, stopSignal)
			if err != nil {
				return err
			}
		}
		c.Labels[containerd.StopSignalLabel] = stopSignal
		if stopTimeout != 0 {
			c.Labels[labels.StopTimout] = strconv.Itoa(stopTimeout)
		}
		return nil
	}
}

func PropagateContainerdLabelsToOCIAnnotations() oci.SpecOpts {
	return func(ctx context.Context, oc oci.Client, c *containers.Container, s *oci.Spec) error {
		return oci.WithAnnotations(c.Labels)(ctx, oc, c, s)
	}
}

func WriteCIDFile(path, id string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("container ID file found, make sure the other container isn't running or delete %s", path)
	} else if errors.Is(err, os.ErrNotExist) {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if err != nil {
			return fmt.Errorf("failed to create the container ID file: %s", err)
		}
		if _, err := f.WriteString(id); err != nil {
			return err
		}
		return nil
	} else {
		return err
	}
}

func ParseEnvVars(paths []string) ([]string, error) {
	vars := make([]string, 0)
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open env file %s: %w", path, err)
		}
		defer f.Close()

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			// skip comment lines
			if strings.HasPrefix(line, "#") {
				continue
			}
			vars = append(vars, line)
		}
		if err = sc.Err(); err != nil {
			return nil, err
		}
	}
	return vars, nil
}

func WithOSEnv(envs []string) ([]string, error) {
	newEnvs := make([]string, len(envs))

	// from https://github.com/docker/cli/blob/v22.06.0-beta.0/opts/env.go#L18
	getEnv := func(val string) (string, error) {
		arr := strings.SplitN(val, "=", 2)
		if arr[0] == "" {
			return "", errors.New("invalid environment variable: " + val)
		}
		if len(arr) > 1 {
			return val, nil
		}
		if envVal, ok := os.LookupEnv(arr[0]); ok {
			return arr[0] + "=" + envVal, nil
		}
		return val, nil
	}
	for i := range envs {
		env, err := getEnv(envs[i])
		if err != nil {
			return nil, err
		}
		newEnvs[i] = env
	}

	return newEnvs, nil
}

// generateEnvs combines environment variables from `--env-file` and `--env`.
// Pass an empty slice if any arg is not used.
func GenerateEnvs(envFile []string, env []string) ([]string, error) {
	var envs []string
	var err error

	if envFiles := strutil.DedupeStrSlice(envFile); len(envFiles) > 0 {
		envs, err = ParseEnvVars(envFiles)
		if err != nil {
			return nil, err
		}
	}

	if env := strutil.DedupeStrSlice(env); len(env) > 0 {
		envs = append(envs, env...)
	}

	if envs, err = WithOSEnv(envs); err != nil {
		return nil, err
	}

	return envs, nil
}
