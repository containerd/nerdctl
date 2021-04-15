module github.com/containerd/nerdctl

go 1.16

require (
	github.com/compose-spec/compose-go v0.0.0-20210415072938-109132373677
	github.com/containerd/cgroups v0.0.0-20210114181951-8a68de567b68
	github.com/containerd/console v1.0.2
	github.com/containerd/containerd v1.5.0-rc.1
	github.com/containerd/go-cni v1.0.1
	github.com/containerd/imgcrypt v1.1.1
	github.com/containerd/stargz-snapshotter v0.5.0
	github.com/containerd/stargz-snapshotter/estargz v0.5.0
	github.com/containerd/typeurl v1.0.1
	github.com/containernetworking/cni v0.8.1
	github.com/containernetworking/plugins v0.9.1
	github.com/docker/cli v20.10.6+incompatible
	github.com/docker/docker v20.10.6+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.4.0
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/fatih/color v1.10.0
	github.com/gogo/protobuf v1.3.2
	github.com/mattn/go-isatty v0.0.12
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runtime-spec v1.0.3-0.20210303205135-43e4633e40c1
	github.com/pkg/errors v0.9.1
	github.com/rootless-containers/rootlesskit v0.14.1
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli/v2 v2.3.0
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210324051608-47abb6519492
	golang.org/x/term v0.0.0-20210406210042-72f3dc4e9b72
	gotest.tools/v3 v3.0.3
)
