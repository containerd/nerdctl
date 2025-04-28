module github.com/containerd/nerdctl/v2

go 1.23.0

require (
	github.com/Masterminds/semver/v3 v3.3.1
	github.com/Microsoft/go-winio v0.6.2
	github.com/Microsoft/hcsshim v0.12.9
	github.com/compose-spec/compose-go/v2 v2.6.1
	github.com/containerd/accelerated-container-image v1.3.0
	github.com/containerd/cgroups/v3 v3.0.5
	github.com/containerd/console v1.0.4
	github.com/containerd/containerd/api v1.8.0
	github.com/containerd/containerd/v2 v2.0.5
	github.com/containerd/continuity v0.4.5
	github.com/containerd/errdefs v1.0.0
	github.com/containerd/fifo v1.1.0
	github.com/containerd/go-cni v1.1.12
	github.com/containerd/imgcrypt/v2 v2.0.1
	github.com/containerd/log v0.1.0
	github.com/containerd/nerdctl/mod/tigron v0.0.0
	github.com/containerd/nydus-snapshotter v0.15.1
	github.com/containerd/platforms v1.0.0-rc.1
	github.com/containerd/stargz-snapshotter v0.16.3
	github.com/containerd/stargz-snapshotter/estargz v0.16.3
	github.com/containerd/stargz-snapshotter/ipfs v0.16.3
	github.com/containerd/typeurl/v2 v2.2.3
	github.com/containernetworking/cni v1.3.0
	github.com/containernetworking/plugins v1.7.1
	github.com/coreos/go-iptables v0.8.0
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/cyphar/filepath-securejoin v0.4.1
	github.com/distribution/reference v0.6.0
	github.com/docker/cli v28.1.1+incompatible
	github.com/docker/docker v28.1.1+incompatible
	github.com/docker/go-connections v0.5.0
	github.com/docker/go-units v0.5.0
	github.com/fahedouch/go-logrotate v0.3.0
	github.com/fatih/color v1.18.0
	github.com/fluent/fluent-logger-golang v1.9.0
	github.com/fsnotify/fsnotify v1.9.0
	github.com/go-viper/mapstructure/v2 v2.2.1
	github.com/ipfs/go-cid v0.5.0
	github.com/klauspost/compress v1.18.0
	github.com/mattn/go-isatty v0.0.20
	github.com/moby/sys/mount v0.3.4
	github.com/moby/sys/signal v0.7.1
	github.com/moby/sys/userns v0.1.0
	github.com/moby/term v0.5.2
	github.com/muesli/cancelreader v0.2.2
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/opencontainers/runtime-spec v1.2.1
	github.com/pelletier/go-toml/v2 v2.2.4
	github.com/rootless-containers/bypass4netns v0.4.2
	github.com/rootless-containers/rootlesskit/v2 v2.3.4
	github.com/spf13/cobra v1.9.1
	github.com/spf13/pflag v1.0.6
	github.com/vishvananda/netlink v1.3.1-0.20250303224720-0e7078ed04c8
	github.com/vishvananda/netns v0.0.5
	github.com/yuchanns/srslog v1.1.0
	go.uber.org/mock v0.5.2
	golang.org/x/crypto v0.37.0
	golang.org/x/net v0.39.0
	golang.org/x/sync v0.13.0
	golang.org/x/sys v0.32.0
	golang.org/x/term v0.31.0
	golang.org/x/text v0.24.0
	gopkg.in/yaml.v3 v3.0.1
	gotest.tools/v3 v3.5.2
)

require (
	github.com/AdaLogics/go-fuzz-headers v0.0.0-20240806141605-e8a1dd7889d6 // indirect
	github.com/AdamKorcz/go-118-fuzz-build v0.0.0-20231105174938-2b5cbb29f3e2 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869 // indirect
	github.com/cilium/ebpf v0.16.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/go-runc v1.1.0 // indirect
	github.com/containerd/plugin v1.0.0 // indirect
	github.com/containerd/ttrpc v1.2.7 // indirect
	github.com/containers/ocicrypt v1.2.1 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/docker/docker-credential-helpers v0.8.2 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-jose/go-jose/v4 v4.0.5 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
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
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/symlink v0.3.0 // indirect
	github.com/moby/sys/user v0.3.0 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr v0.13.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/opencontainers/selinux v1.12.0 // indirect
	github.com/petermattis/goid v0.0.0-20240813172612-4fcff4a6cae7 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240612014219-fbbf4953d986 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sasha-s/go-deadlock v0.3.5 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/smallstep/pkcs7 v0.1.1 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stefanberger/go-pkcs11uri v0.0.0-20230803200340-78284954bff6 // indirect
	github.com/tinylib/msgp v1.2.0 // indirect
	github.com/vbatts/tar-split v0.11.6 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.56.0 // indirect
	go.opentelemetry.io/otel v1.31.0 // indirect
	go.opentelemetry.io/otel/metric v1.31.0 // indirect
	go.opentelemetry.io/otel/trace v1.31.0 // indirect
	golang.org/x/exp v0.0.0-20241108190413-2d47ceb2692f // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250106144421-5f5ef82da422 // indirect
	google.golang.org/grpc v1.69.4 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
	lukechampine.com/blake3 v1.3.0 // indirect
)

replace github.com/containerd/nerdctl/mod/tigron v0.0.0 => ./mod/tigron
