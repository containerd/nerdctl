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
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/flagutil"
	"github.com/containerd/nerdctl/v2/pkg/idgen"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/ipcutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/logging"
	"github.com/containerd/nerdctl/v2/pkg/mountutil"
	"github.com/containerd/nerdctl/v2/pkg/namestore"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	dockercliopts "github.com/docker/cli/opts"
	dockeropts "github.com/docker/docker/opts"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// Create will create a container.
func Create(ctx context.Context, client *containerd.Client, args []string, netManager containerutil.NetworkOptionsManager, options types.ContainerCreateOptions) (containerd.Container, func(), error) {
	// simulate the behavior of double dash
	newArg := []string{}
	if len(args) >= 2 && args[1] == "--" {
		newArg = append(newArg, args[:1]...)
		newArg = append(newArg, args[2:]...)
		args = newArg
	}
	var internalLabels internalLabels
	internalLabels.platform = options.Platform
	internalLabels.namespace = options.GOptions.Namespace

	var (
		id    = idgen.GenerateID()
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
	)

	if options.CidFile != "" {
		if err := writeCIDFile(options.CidFile, id); err != nil {
			return nil, nil, err
		}
	}
	dataStore, err := clientutil.DataStore(options.GOptions.DataRoot, options.GOptions.Address)
	if err != nil {
		return nil, nil, err
	}

	internalLabels.stateDir, err = containerutil.ContainerStateDirPath(options.GOptions.Namespace, dataStore, id)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(internalLabels.stateDir, 0700); err != nil {
		return nil, nil, err
	}

	opts = append(opts,
		oci.WithDefaultSpec(),
	)

	platformOpts, err := setPlatformOptions(ctx, client, id, netManager.NetworkOptions().UTSNamespace, &internalLabels, options)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, platformOpts...)

	var ensuredImage *imgutil.EnsuredImage
	if !options.Rootfs {
		var platformSS []string // len: 0 or 1
		if options.Platform != "" {
			platformSS = append(platformSS, options.Platform)
		}
		ocispecPlatforms, err := platformutil.NewOCISpecPlatformSlice(false, platformSS)
		if err != nil {
			return nil, nil, err
		}
		rawRef := args[0]

		ensuredImage, err = image.EnsureImage(ctx, client, rawRef, ocispecPlatforms, options.Pull, nil, false, options.ImagePullOpt)
		if err != nil {
			return nil, nil, err
		}
	}

	rootfsOpts, rootfsCOpts, err := generateRootfsOpts(args, id, ensuredImage, options)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, rootfsOpts...)
	cOpts = append(cOpts, rootfsCOpts...)

	if options.Workdir != "" {
		opts = append(opts, oci.WithProcessCwd(options.Workdir))
	}

	envs, err := flagutil.MergeEnvFileAndOSEnv(options.EnvFile, options.Env)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, oci.WithEnv(envs))

	if options.Interactive {
		if options.Detach {
			return nil, nil, errors.New("currently flag -i and -d cannot be specified together (FIXME)")
		}
	}

	if options.TTY {
		opts = append(opts, oci.WithTTY)
	}

	var mountOpts []oci.SpecOpts
	mountOpts, internalLabels.anonVolumes, internalLabels.mountPoints, err = generateMountOpts(ctx, client, ensuredImage, options)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, mountOpts...)

	// Always set internalLabels.logURI
	// to support restart the container that run with "-it", like
	//
	// 1, nerdctl run --name demo -it imagename
	// 2, ctrl + c to stop demo container
	// 3, nerdctl start/restart demo
	logConfig, err := generateLogConfig(dataStore, id, options.LogDriver, options.LogOpt, options.GOptions.Namespace)
	if err != nil {
		return nil, nil, err
	}
	internalLabels.logURI = logConfig.LogURI

	restartOpts, err := generateRestartOpts(ctx, client, options.Restart, logConfig.LogURI, options.InRun)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, restartOpts...)
	cOpts = append(cOpts, withStop(options.StopSignal, options.StopTimeout, ensuredImage))

	if err = netManager.VerifyNetworkOptions(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to verify networking settings: %s", err)
	}

	netOpts, netNewContainerOpts, err := netManager.ContainerNetworkingOpts(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate networking spec options: %s", err)
	}
	opts = append(opts, netOpts...)
	cOpts = append(cOpts, netNewContainerOpts...)

	netLabelOpts, err := netManager.InternalNetworkingOptionLabels(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate internal networking labels: %s", err)
	}
	// TODO(aznashwan): more formal way to load net opts into internalLabels:
	internalLabels.hostname = netLabelOpts.Hostname
	internalLabels.ports = netLabelOpts.PortMappings
	internalLabels.ipAddress = netLabelOpts.IPAddress
	internalLabels.ip6Address = netLabelOpts.IP6Address
	internalLabels.networks = netLabelOpts.NetworkSlice
	internalLabels.macAddress = netLabelOpts.MACAddress

	// NOTE: OCI hooks are currently not supported on Windows so we skip setting them altogether.
	// The OCI hooks we define (whose logic can be found in pkg/ocihook) primarily
	// perform network setup and teardown when using CNI networking.
	// On Windows, we are forced to set up and tear down the networking from within nerdctl.
	if runtime.GOOS != "windows" {
		hookOpt, err := withNerdctlOCIHook(options.NerdctlCmd, options.NerdctlArgs)
		if err != nil {
			return nil, nil, err
		}
		opts = append(opts, hookOpt)
	}

	uOpts, err := generateUserOpts(options.User)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, uOpts...)
	gOpts, err := generateGroupsOpts(options.GroupAdd)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, gOpts...)

	umaskOpts, err := generateUmaskOpts(options.Umask)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, umaskOpts...)

	rtCOpts, err := generateRuntimeCOpts(options.GOptions.CgroupManager, options.Runtime)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, rtCOpts...)

	lCOpts, err := withContainerLabels(options.Label, options.LabelFile)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, lCOpts...)

	var containerNameStore namestore.NameStore
	if options.Name == "" && !options.NameChanged {
		// Automatically set the container name, unless `--name=""` was explicitly specified.
		var imageRef string
		if ensuredImage != nil {
			imageRef = ensuredImage.Ref
		}
		options.Name = referenceutil.SuggestContainerName(imageRef, id)
	}
	if options.Name != "" {
		containerNameStore, err = namestore.New(dataStore, options.GOptions.Namespace)
		if err != nil {
			return nil, nil, err
		}
		if err := containerNameStore.Acquire(options.Name, id); err != nil {
			return nil, nil, err
		}
	}
	internalLabels.name = options.Name
	internalLabels.pidFile = options.PidFile
	internalLabels.extraHosts = strutil.DedupeStrSlice(netManager.NetworkOptions().AddHost)
	for i, host := range internalLabels.extraHosts {
		if _, err := dockercliopts.ValidateExtraHost(host); err != nil {
			return nil, nil, err
		}
		parts := strings.SplitN(host, ":", 2)
		// If the IP Address is a string called "host-gateway", replace this value with the IP address stored
		// in the daemon level HostGateway IP config variable.
		if parts[1] == dockeropts.HostGatewayName {
			if options.GOptions.HostGatewayIP == "" {
				return nil, nil, fmt.Errorf("unable to derive the IP value for host-gateway")
			}
			parts[1] = options.GOptions.HostGatewayIP
			internalLabels.extraHosts[i] = fmt.Sprintf(`%s:%s`, parts[0], parts[1])
		}
	}

	ilOpt, err := withInternalLabels(internalLabels)
	if err != nil {
		return nil, nil, err
	}
	cOpts = append(cOpts, ilOpt)

	opts = append(opts, propagateContainerdLabelsToOCIAnnotations())

	var s specs.Spec
	spec := containerd.WithSpec(&s, opts...)

	cOpts = append(cOpts, spec)

	c, containerErr := client.NewContainer(ctx, id, cOpts...)
	var netSetupErr error
	// NOTE: on non-Windows platforms, network setup is performed by OCI hooks.
	// Seeing as though Windows does not currently support OCI hooks, we must explicitly
	// perform network setup/teardown in the main nerdctl executable.
	if containerErr == nil && runtime.GOOS == "windows" {
		netSetupErr = netManager.SetupNetworking(ctx, id)
		if netSetupErr != nil {
			log.G(ctx).WithError(netSetupErr).Warnf("networking setup error has occurred")
		}
	}

	if containerErr != nil || netSetupErr != nil {
		returnedError := containerErr
		if netSetupErr != nil {
			returnedError = netSetupErr // mutually exclusive
		}
		return nil, generateGcFunc(ctx, c, options.GOptions.Namespace, id, options.Name, dataStore, containerErr, containerNameStore, netManager, internalLabels), returnedError
	}

	return c, nil, nil
}

func generateRootfsOpts(args []string, id string, ensured *imgutil.EnsuredImage, options types.ContainerCreateOptions) (opts []oci.SpecOpts, cOpts []containerd.NewContainerOpts, err error) {
	if !options.Rootfs {
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
			}
			if ind == len(ensured.ImageConfig.Env)-1 {
				opts = append(opts, oci.WithDefaultPathEnv)
			}
		}
	} else {
		absRootfs, err := filepath.Abs(args[0])
		if err != nil {
			return nil, nil, err
		}
		opts = append(opts, oci.WithRootFSPath(absRootfs), oci.WithDefaultPathEnv)
	}

	if !options.Rootfs && !options.EntrypointChanged {
		opts = append(opts, oci.WithImageConfigArgs(ensured.Image, args[1:]))
	} else {
		if !options.Rootfs {
			opts = append(opts, oci.WithImageConfig(ensured.Image))
		}
		var processArgs []string
		if len(options.Entrypoint) != 0 {
			processArgs = append(processArgs, options.Entrypoint...)
		}
		if len(args) > 1 {
			processArgs = append(processArgs, args[1:]...)
		}
		if len(processArgs) == 0 {
			// error message is from Podman
			return nil, nil, errors.New("no command or entrypoint provided, and no CMD or ENTRYPOINT from image")
		}
		opts = append(opts, oci.WithProcessArgs(processArgs...))
	}
	if options.InitBinary != nil {
		options.InitProcessFlag = true
	}
	if options.InitProcessFlag {
		binaryPath, err := exec.LookPath(*options.InitBinary)
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return nil, nil, fmt.Errorf(`init binary %q not found`, *options.InitBinary)
			}
			return nil, nil, err
		}
		inContainerPath := filepath.Join("/sbin", filepath.Base(*options.InitBinary))
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
	if options.ReadOnly {
		opts = append(opts, oci.WithRootFSReadonly())
	}
	return opts, cOpts, nil
}

// GenerateLogURI generates a log URI for the current container store
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

func withNerdctlOCIHook(cmd string, args []string) (oci.SpecOpts, error) {
	args = append([]string{cmd}, append(args, "internal", "oci-hook")...)
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Hooks == nil {
			s.Hooks = &specs.Hooks{}
		}
		crArgs := append(args, "createRuntime")
		s.Hooks.CreateRuntime = append(s.Hooks.CreateRuntime, specs.Hook{
			Path: cmd,
			Args: crArgs,
			Env:  os.Environ(),
		})
		argsCopy := append([]string(nil), args...)
		psArgs := append(argsCopy, "postStop")
		s.Hooks.Poststop = append(s.Hooks.Poststop, specs.Hook{
			Path: cmd,
			Args: psArgs,
			Env:  os.Environ(),
		})
		return nil
	}, nil
}

func withContainerLabels(label, labelFile []string) ([]containerd.NewContainerOpts, error) {
	labelMap, err := readKVStringsMapfFromLabel(label, labelFile)
	if err != nil {
		return nil, err
	}
	o := containerd.WithAdditionalContainerLabels(labelMap)
	return []containerd.NewContainerOpts{o}, nil
}

func readKVStringsMapfFromLabel(label, labelFile []string) (map[string]string, error) {
	labelsMap := strutil.DedupeStrSlice(label)
	labelsFilePath := strutil.DedupeStrSlice(labelFile)
	kvStrings, err := dockercliopts.ReadKVStrings(labelsFilePath, labelsMap)
	if err != nil {
		return nil, err
	}
	return strutil.ConvertKVStringsToMap(kvStrings), nil
}

// parseKVStringsMapFromLogOpt parse log options KV entries and convert to Map
func parseKVStringsMapFromLogOpt(logOpt []string, logDriver string) (map[string]string, error) {
	logOptArray := strutil.DedupeStrSlice(logOpt)
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

func withStop(stopSignal string, stopTimeout int, ensuredImage *imgutil.EnsuredImage) containerd.NewContainerOpts {
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
			c.Labels[labels.StopTimeout] = strconv.Itoa(stopTimeout)
		}
		return nil
	}
}

type internalLabels struct {
	// labels from cmd options
	namespace  string
	platform   string
	extraHosts []string
	pidFile    string
	// labels from cmd options or automatically set
	name     string
	hostname string
	// automatically generated
	stateDir string
	// network
	networks   []string
	ipAddress  string
	ip6Address string
	ports      []gocni.PortMapping
	macAddress string
	// volume
	mountPoints []*mountutil.Processed
	anonVolumes []string
	// pid namespace
	pidContainer string
	// ipc namespace & dev/shm
	ipc string
	// log
	logURI string
}

// WithInternalLabels sets the internal labels for a container.
func withInternalLabels(internalLabels internalLabels) (containerd.NewContainerOpts, error) {
	m := make(map[string]string)
	m[labels.Namespace] = internalLabels.namespace
	if internalLabels.name != "" {
		m[labels.Name] = internalLabels.name
	}
	m[labels.Hostname] = internalLabels.hostname
	extraHostsJSON, err := json.Marshal(internalLabels.extraHosts)
	if err != nil {
		return nil, err
	}
	m[labels.ExtraHosts] = string(extraHostsJSON)
	m[labels.StateDir] = internalLabels.stateDir
	networksJSON, err := json.Marshal(internalLabels.networks)
	if err != nil {
		return nil, err
	}
	m[labels.Networks] = string(networksJSON)
	if len(internalLabels.ports) > 0 {
		portsJSON, err := json.Marshal(internalLabels.ports)
		if err != nil {
			return nil, err
		}
		m[labels.Ports] = string(portsJSON)
	}
	if internalLabels.logURI != "" {
		m[labels.LogURI] = internalLabels.logURI
	}
	if len(internalLabels.anonVolumes) > 0 {
		anonVolumeJSON, err := json.Marshal(internalLabels.anonVolumes)
		if err != nil {
			return nil, err
		}
		m[labels.AnonymousVolumes] = string(anonVolumeJSON)
	}

	if internalLabels.pidFile != "" {
		m[labels.PIDFile] = internalLabels.pidFile
	}

	if internalLabels.ipAddress != "" {
		m[labels.IPAddress] = internalLabels.ipAddress
	}

	if internalLabels.ip6Address != "" {
		m[labels.IP6Address] = internalLabels.ip6Address
	}

	m[labels.Platform], err = platformutil.NormalizeString(internalLabels.platform)
	if err != nil {
		return nil, err
	}

	if len(internalLabels.mountPoints) > 0 {
		mounts := dockercompatMounts(internalLabels.mountPoints)
		mountPointsJSON, err := json.Marshal(mounts)
		if err != nil {
			return nil, err
		}
		m[labels.Mounts] = string(mountPointsJSON)
	}

	if internalLabels.macAddress != "" {
		m[labels.MACAddress] = internalLabels.macAddress
	}

	if internalLabels.pidContainer != "" {
		m[labels.PIDContainer] = internalLabels.pidContainer
	}

	if internalLabels.ipc != "" {
		m[labels.IPC] = internalLabels.ipc
	}

	return containerd.WithAdditionalContainerLabels(m), nil
}

func dockercompatMounts(mountPoints []*mountutil.Processed) []dockercompat.MountPoint {
	result := make([]dockercompat.MountPoint, len(mountPoints))
	for i := range mountPoints {
		mp := mountPoints[i]
		result[i] = dockercompat.MountPoint{
			Type:        mp.Type,
			Name:        mp.Name,
			Source:      mp.Mount.Source,
			Destination: mp.Mount.Destination,
			Driver:      "",
			Mode:        mp.Mode,
		}

		// it's an anonymous volume
		if mp.AnonymousVolume != "" {
			result[i].Name = mp.AnonymousVolume
		}

		// volume only support local driver
		if mp.Type == "volume" {
			result[i].Driver = "local"
		}
	}
	return result
}

func processeds(mountPoints []dockercompat.MountPoint) []*mountutil.Processed {
	result := make([]*mountutil.Processed, len(mountPoints))
	for i := range mountPoints {
		mp := mountPoints[i]
		result[i] = &mountutil.Processed{
			Type: mp.Type,
			Name: mp.Name,
			Mount: specs.Mount{
				Source:      mp.Source,
				Destination: mp.Destination,
			},
			Mode: mp.Mode,
		}
	}
	return result
}

func propagateContainerdLabelsToOCIAnnotations() oci.SpecOpts {
	return func(ctx context.Context, oc oci.Client, c *containers.Container, s *oci.Spec) error {
		return oci.WithAnnotations(c.Labels)(ctx, oc, c, s)
	}
}

func writeCIDFile(path, id string) error {
	_, err := os.Stat(path)
	if err == nil {
		return fmt.Errorf("container ID file found, make sure the other container isn't running or delete %s", path)
	} else if errors.Is(err, os.ErrNotExist) {
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("failed to create the container ID file: %s for %s, err: %s", path, id, err)
		}
		defer f.Close()

		if _, err := f.WriteString(id); err != nil {
			return err
		}
		return nil
	}
	return err
}

// generateLogConfig creates a LogConfig for the current container store
func generateLogConfig(dataStore string, id string, logDriver string, logOpt []string, ns string) (logConfig logging.LogConfig, err error) {
	var u *url.URL
	if u, err = url.Parse(logDriver); err == nil && u.Scheme != "" {
		logConfig.LogURI = logDriver
	} else {
		logConfig.Driver = logDriver
		logConfig.Opts, err = parseKVStringsMapFromLogOpt(logOpt, logDriver)
		if err != nil {
			return
		}
		var (
			logDriverInst logging.Driver
			logConfigB    []byte
			lu            *url.URL
		)
		logDriverInst, err = logging.GetDriver(logDriver, logConfig.Opts)
		if err != nil {
			return
		}
		if err = logDriverInst.Init(dataStore, ns, id); err != nil {
			return
		}

		logConfigB, err = json.Marshal(logConfig)
		if err != nil {
			return
		}

		logConfigFilePath := logging.LogConfigFilePath(dataStore, ns, id)
		if err = os.WriteFile(logConfigFilePath, logConfigB, 0600); err != nil {
			return
		}

		lu, err = GenerateLogURI(dataStore)
		if err != nil {
			return
		}
		if lu != nil {
			log.L.Debugf("generated log driver: %s", lu.String())
			logConfig.LogURI = lu.String()
		}
	}
	return logConfig, nil
}

func generateGcFunc(ctx context.Context, container containerd.Container, ns, id, name, dataStore string, containerErr error, containerNameStore namestore.NameStore, netManager containerutil.NetworkOptionsManager, internalLabels internalLabels) func() {
	return func() {
		if containerErr == nil {
			netGcErr := netManager.CleanupNetworking(ctx, container)
			if netGcErr != nil {
				log.G(ctx).WithError(netGcErr).Warnf("failed to revert container %q networking settings", id)
			}
		}

		ipc, ipcErr := ipcutil.DecodeIPCLabel(internalLabels.ipc)
		if ipcErr != nil {
			log.G(ctx).WithError(ipcErr).Warnf("failed to decode ipc label for container %q", id)
		}
		if ipcErr := ipcutil.CleanUp(ipc); ipcErr != nil {
			log.G(ctx).WithError(ipcErr).Warnf("failed to clean up ipc for container %q", id)
		}
		if rmErr := os.RemoveAll(internalLabels.stateDir); rmErr != nil {
			log.G(ctx).WithError(rmErr).Warnf("failed to remove container %q state dir %q", id, internalLabels.stateDir)
		}

		if name != "" {
			var errE error
			if containerNameStore, errE = namestore.New(dataStore, ns); errE != nil {
				log.G(ctx).WithError(errE).Warnf("failed to instantiate container name store during cleanup for container %q", id)
			}
			if errE = containerNameStore.Release(name, id); errE != nil {
				log.G(ctx).WithError(errE).Warnf("failed to release container name store for container %q (%s)", name, id)
			}
		}
	}
}
