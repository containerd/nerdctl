module github.com/containerd/nerdctl

go 1.16

require (
	github.com/Microsoft/go-winio v0.5.0
	github.com/compose-spec/compose-go v1.0.2
	github.com/containerd/cgroups v1.0.1
	github.com/containerd/console v1.0.3
	github.com/containerd/containerd v1.5.6 // replaced, see the bottom of this file
	github.com/containerd/containerd/api v0.0.0 // replaced, see the bottom of this file
	github.com/containerd/continuity v0.2.0
	github.com/containerd/go-cni v1.1.0
	github.com/containerd/imgcrypt v1.1.1
	github.com/containerd/stargz-snapshotter v0.9.0
	github.com/containerd/stargz-snapshotter/estargz v0.9.0
	github.com/containerd/typeurl v1.0.2
	github.com/containernetworking/cni v1.0.1
	github.com/containernetworking/plugins v1.0.1
	github.com/cyphar/filepath-securejoin v0.2.3
	github.com/docker/cli v20.10.8+incompatible
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v20.10.9+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.4.0
	github.com/fatih/color v1.13.0
	github.com/gogo/protobuf v1.3.2
	github.com/mattn/go-isatty v0.0.14
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2-0.20210819154149-5ad6f50d6283
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417
	github.com/pkg/errors v0.9.1
	github.com/rootless-containers/rootlesskit v0.14.5
	github.com/sirupsen/logrus v1.8.1
	github.com/tidwall/gjson v1.9.2
	github.com/urfave/cli/v2 v2.3.0
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210809222454-d867a43fc93e
	golang.org/x/term v0.0.0-20210615171337-6886f2dfbf5b
	gotest.tools/v3 v3.0.3
)

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v1.5.1-0.20210921144419-90c6ff97a88f
	github.com/containerd/containerd/api => github.com/containerd/containerd/api v0.0.0-20210921144419-90c6ff97a88f
)
