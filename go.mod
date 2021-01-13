module github.com/AkihiroSuda/nerdctl

go 1.15

require (
	github.com/containerd/cgroups v0.0.0-20200824123100-0b889c03f102
	github.com/containerd/console v1.0.1
	github.com/containerd/containerd v1.4.1-0.20201229174909-9067796ce4c8
	github.com/containerd/go-cni v1.0.1
	github.com/containerd/stargz-snapshotter v0.2.1-0.20210101143201-d58f43a8235e
	github.com/containerd/stargz-snapshotter/estargz v0.0.0-00010101000000-000000000000
	github.com/containerd/typeurl v1.0.1
	github.com/containernetworking/plugins v0.9.0
	github.com/docker/cli v20.10.0+incompatible
	github.com/docker/docker v17.12.0-ce-rc1.0.20200730172259-9f28837c1d93+incompatible
	github.com/docker/go-units v0.4.0
	github.com/fvbommel/sortorder v1.0.2 // indirect
	github.com/gogo/protobuf v1.3.1
	github.com/google/go-cmp v0.5.4 // indirect
	github.com/lib/pq v1.9.0 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/moby/sys/mount v0.2.0 // indirect
	github.com/onsi/ginkgo v1.14.2 // indirect
	github.com/onsi/gomega v1.10.4 // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runtime-spec v1.0.3-0.20200728170252-4d89ac9fbff6
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/cobra v1.1.1 // indirect
	github.com/spf13/viper v1.7.1 // indirect
	github.com/stretchr/testify v1.7.0 // indirect
	github.com/theupdateframework/notary v0.7.0 // indirect
	github.com/urfave/cli/v2 v2.3.0
	golang.org/x/crypto v0.0.0-20201117144127-c1f2f97bffc9
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a
	golang.org/x/sys v0.0.0-20210119212857-b64e53b001e4
	golang.org/x/text v0.3.4 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gotest.tools/v3 v3.0.2
)

// estargz: needs this replace because stargz-snapshotter git repo has two go.mod modules.
replace github.com/containerd/stargz-snapshotter/estargz => github.com/containerd/stargz-snapshotter/estargz v0.0.0-20210101143201-d58f43a8235e
