module github.com/containerd/nerdctl

go 1.16

require (
	github.com/Microsoft/go-winio v0.5.1
	github.com/compose-spec/compose-go v1.0.3
	github.com/containerd/cgroups v1.0.2
	github.com/containerd/console v1.0.3
	github.com/containerd/containerd v1.5.7 // replaced, see the bottom of this file
	github.com/containerd/containerd/api v0.0.0 // replaced, see the bottom of this file
	github.com/containerd/continuity v0.2.1
	github.com/containerd/go-cni v1.1.0
	github.com/containerd/imgcrypt v1.1.1
	github.com/containerd/stargz-snapshotter v0.9.0
	github.com/containerd/stargz-snapshotter/estargz v0.9.0
	github.com/containerd/typeurl v1.0.2
	github.com/containernetworking/cni v1.0.1
	github.com/containernetworking/plugins v1.0.1
	github.com/cyphar/filepath-securejoin v0.2.3
	github.com/docker/cli v20.10.10+incompatible
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v20.10.9+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.4.0
	github.com/fatih/color v1.13.0
	github.com/gogo/protobuf v1.3.2
	github.com/mattn/go-isatty v0.0.14
	github.com/moby/sys/mount v0.2.0 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2-0.20210819154149-5ad6f50d6283
	github.com/opencontainers/runtime-spec v1.0.3-0.20210910115017-0d6cc581aeea
	github.com/pkg/errors v0.9.1
	github.com/rootless-containers/rootlesskit v0.14.5
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1 // replaced, see the bottom of this file
	github.com/spf13/pflag v1.0.5 // replaced, see the bottom of this file
	github.com/tidwall/gjson v1.10.1
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20211004093028-2c5d950f24ef
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211
	gotest.tools/v3 v3.0.3
)

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v1.5.1-0.20211004173205-d193dc2b8afb
	github.com/containerd/containerd/api => github.com/containerd/containerd/api v0.0.0-20211004173205-d193dc2b8afb
	github.com/spf13/cobra => github.com/robberphex/cobra v1.2.2-0.20211012081327-8e3ac9400ac4 // https://github.com/spf13/cobra/pull/1503
	github.com/spf13/pflag => github.com/robberphex/pflag v1.0.6-0.20211014094653-9df3e45100fd // https://github.com/spf13/pflag/pull/333
)
