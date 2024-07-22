module github.com/containerd/nerdctl/v2

go 1.21.12

require (
	github.com/Masterminds/semver/v3 v3.2.1
	github.com/Microsoft/go-winio v0.6.2
	github.com/Microsoft/hcsshim v0.12.5
	github.com/awslabs/soci-snapshotter v0.6.1
	github.com/compose-spec/compose-go/v2 v2.1.4
	github.com/containerd/accelerated-container-image v1.1.4
	github.com/containerd/cgroups/v3 v3.0.3
	github.com/containerd/console v1.0.4
	github.com/containerd/containerd v1.7.19
	github.com/containerd/containerd/api v1.7.19
	github.com/containerd/continuity v0.4.3
	github.com/containerd/fifo v1.1.0
	github.com/containerd/go-cni v1.1.10
	github.com/containerd/imgcrypt v1.1.11
	github.com/containerd/log v0.1.0
	github.com/containerd/nydus-snapshotter v0.13.13
	github.com/containerd/platforms v0.2.1
	github.com/containerd/stargz-snapshotter v0.15.1
	github.com/containerd/stargz-snapshotter/estargz v0.15.1
	github.com/containerd/stargz-snapshotter/ipfs v0.15.1
	github.com/containerd/typeurl/v2 v2.2.0
	github.com/containernetworking/cni v1.2.3
	github.com/containernetworking/plugins v1.5.1
	github.com/coreos/go-iptables v0.7.0
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/cyphar/filepath-securejoin v0.3.0
	github.com/distribution/reference v0.6.0
	github.com/docker/cli v27.0.3+incompatible
	github.com/docker/docker v27.0.3+incompatible
	github.com/docker/go-connections v0.5.0
	github.com/docker/go-units v0.5.0
	github.com/fahedouch/go-logrotate v0.2.1
	github.com/fatih/color v1.17.0
	github.com/fluent/fluent-logger-golang v1.9.0
	github.com/ipfs/go-cid v0.4.1
	github.com/klauspost/compress v1.17.9
	github.com/mattn/go-isatty v0.0.20
	github.com/mitchellh/mapstructure v1.5.0
	github.com/moby/sys/mount v0.3.4
	github.com/moby/sys/signal v0.7.1
	github.com/moby/term v0.5.0
	github.com/muesli/cancelreader v0.2.2
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.0
	github.com/opencontainers/runtime-spec v1.2.0
	github.com/pelletier/go-toml/v2 v2.2.2
	github.com/rootless-containers/bypass4netns v0.4.1
	github.com/rootless-containers/rootlesskit/v2 v2.2.0
	github.com/spf13/cobra v1.8.1
	github.com/spf13/pflag v1.0.5
	github.com/tidwall/gjson v1.17.1
	github.com/vishvananda/netlink v1.2.1-beta.2
	github.com/vishvananda/netns v0.0.4
	github.com/yuchanns/srslog v1.1.0
	go.uber.org/mock v0.4.0
	golang.org/x/crypto v0.25.0
	golang.org/x/net v0.27.0
	golang.org/x/sync v0.7.0
	golang.org/x/sys v0.22.0
	golang.org/x/term v0.22.0
	golang.org/x/text v0.16.0
	gopkg.in/yaml.v3 v3.0.1
	gotest.tools/v3 v3.5.1
)

require (
	github.com/AdaLogics/go-fuzz-headers v0.0.0-20230811130428-ced1acdcaa24 // indirect
	github.com/AdamKorcz/go-118-fuzz-build v0.0.0-20231105174938-2b5cbb29f3e2 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869 // indirect
	github.com/cilium/ebpf v0.15.0 // indirect
	github.com/containerd/errdefs v0.1.0 // indirect
	github.com/containerd/go-runc v1.1.0 // indirect
	github.com/containerd/ttrpc v1.2.5 // indirect
	github.com/containerd/typeurl v1.0.2 // indirect
	github.com/containers/ocicrypt v1.2.0 // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/docker/docker-credential-helpers v0.8.2 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-jose/go-jose/v4 v4.0.3 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.0.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-shellwords v1.0.12 // indirect
	github.com/miekg/pkcs11 v1.1.1 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/moby/sys/sequential v0.5.0 // indirect
	github.com/moby/sys/symlink v0.2.0 // indirect
	github.com/moby/sys/user v0.1.0 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr v0.13.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/opencontainers/selinux v1.11.0 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240612014219-fbbf4953d986 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stefanberger/go-pkcs11uri v0.0.0-20230803200340-78284954bff6 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tinylib/msgp v1.2.0 // indirect
	github.com/vbatts/tar-split v0.11.5 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	go.mozilla.org/pkcs7 v0.0.0-20210826202110-33d05740a352 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.53.0 // indirect
	go.opentelemetry.io/otel v1.28.0 // indirect
	go.opentelemetry.io/otel/metric v1.28.0 // indirect
	go.opentelemetry.io/otel/trace v1.28.0 // indirect
	golang.org/x/exp v0.0.0-20240707233637-46b078467d37 // indirect
	google.golang.org/genproto v0.0.0-20240711142825-46eb208f015d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240711142825-46eb208f015d // indirect
	google.golang.org/grpc v1.65.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	lukechampine.com/blake3 v1.3.0 // indirect
)
