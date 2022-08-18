module github.com/containerd/nerdctl

go 1.19

require (
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/Microsoft/go-winio v0.5.2
	github.com/compose-spec/compose-go v1.5.0
	github.com/containerd/cgroups v1.0.4
	github.com/containerd/console v1.0.3
	github.com/containerd/containerd v1.6.8 // replaced
	github.com/containerd/continuity v0.3.0
	github.com/containerd/go-cni v1.1.7
	github.com/containerd/imgcrypt v1.1.6
	github.com/containerd/nydus-snapshotter v0.3.0-rc.1
	github.com/containerd/stargz-snapshotter v0.12.0
	github.com/containerd/stargz-snapshotter/estargz v0.12.0
	github.com/containerd/stargz-snapshotter/ipfs v0.12.0
	github.com/containerd/typeurl v1.0.3-0.20220422153119-7f6e6d160d67
	github.com/containernetworking/cni v1.1.2
	github.com/containernetworking/plugins v1.1.1
	github.com/coreos/go-systemd/v22 v22.3.2
	github.com/cyphar/filepath-securejoin v0.2.3
	github.com/docker/cli v20.10.17+incompatible
	github.com/docker/docker v20.10.17+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.4.0
	github.com/fahedouch/go-logrotate v0.1.2
	github.com/fatih/color v1.13.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/ipfs/go-cid v0.1.0
	github.com/ipfs/go-ipfs-files v0.1.1
	github.com/ipfs/go-ipfs-http-client v0.4.0
	github.com/ipfs/interface-go-ipfs-core v0.7.0
	github.com/mattn/go-isatty v0.0.16
	github.com/moby/sys/mount v0.3.3
	github.com/multiformats/go-multiaddr v0.6.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.3-0.20220303224323-02efb9a75ee1
	github.com/opencontainers/runtime-spec v1.0.3-0.20220311020903-6969a0a09ab1
	github.com/pelletier/go-toml v1.9.5
	github.com/rootless-containers/bypass4netns v0.2.2
	github.com/rootless-containers/rootlesskit v1.0.1
	github.com/sirupsen/logrus v1.9.0
	github.com/spf13/cobra v1.5.0
	github.com/spf13/pflag v1.0.5
	github.com/tidwall/gjson v1.14.3
	github.com/vishvananda/netlink v1.2.1-beta.2
	github.com/vishvananda/netns v0.0.0-20211101163701-50045581ed74
	golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e
	golang.org/x/net v0.0.0-20220615171555-694bf12d69de
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4
	golang.org/x/sys v0.0.0-20220811171246-fbc7d0a398ab
	golang.org/x/term v0.0.0-20220526004731-065cf7ba2467
	gopkg.in/yaml.v3 v3.0.1
	gotest.tools/v3 v3.3.0
)

require (
	github.com/AdaLogics/go-fuzz-headers v0.0.0-20220706123152-fef3fe1bab07 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Microsoft/hcsshim v0.9.4 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869 // indirect
	github.com/btcsuite/btcd v0.21.0-beta // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/cilium/ebpf v0.7.0 // indirect
	github.com/container-orchestrated-devices/container-device-interface v0.3.1 // indirect
	github.com/containerd/fifo v1.0.0 // indirect
	github.com/containerd/ttrpc v1.1.1-0.20220420014843-944ef4a40df3 // indirect
	github.com/containers/ocicrypt v1.1.5 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/crackcomm/go-gitignore v0.0.0-20170627025303-887ab5e44cc3 // indirect
	github.com/distribution/distribution/v3 v3.0.0-20220725133111-4bf3547399eb // indirect
	github.com/djherbis/times v1.5.0 // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/fluent/fluent-logger-golang v1.9.0
	github.com/godbus/dbus/v5 v5.0.6 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.8 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/ipfs/bbloom v0.0.4 // indirect
	github.com/ipfs/go-block-format v0.0.3 // indirect
	github.com/ipfs/go-blockservice v0.3.0 // indirect
	github.com/ipfs/go-datastore v0.5.0 // indirect
	github.com/ipfs/go-ipfs-blockstore v1.2.0 // indirect
	github.com/ipfs/go-ipfs-cmds v0.7.0 // indirect
	github.com/ipfs/go-ipfs-ds-help v1.1.0 // indirect
	github.com/ipfs/go-ipfs-exchange-interface v0.1.0 // indirect
	github.com/ipfs/go-ipfs-util v0.0.2 // indirect
	github.com/ipfs/go-ipld-cbor v0.0.5 // indirect
	github.com/ipfs/go-ipld-format v0.4.0 // indirect
	github.com/ipfs/go-ipld-legacy v0.1.0 // indirect
	github.com/ipfs/go-log v1.0.5 // indirect
	github.com/ipfs/go-log/v2 v2.3.0 // indirect
	github.com/ipfs/go-merkledag v0.6.0 // indirect
	github.com/ipfs/go-metrics-interface v0.0.1 // indirect
	github.com/ipfs/go-path v0.3.0 // indirect
	github.com/ipfs/go-unixfs v0.3.1 // indirect
	github.com/ipfs/go-verifcid v0.0.1 // indirect
	github.com/ipld/go-codec-dagpb v1.3.2 // indirect
	github.com/ipld/go-ipld-prime v0.11.0 // indirect
	github.com/jbenet/goprocess v0.1.4 // indirect
	github.com/klauspost/compress v1.15.7 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	// NOTE: P2P image distribution (IPFS) is completely optional. Your host is NOT connected to any P2P network, unless you opt in to install and run IPFS daemon.
	github.com/libp2p/go-buffer-pool v0.0.2 // indirect
	github.com/libp2p/go-libp2p-core v0.8.6 // indirect
	github.com/libp2p/go-openssl v0.0.7 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-shellwords v1.0.12 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/miekg/pkcs11 v1.1.1 // indirect
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1 // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/moby/sys/signal v0.7.0
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base32 v0.0.3 // indirect
	github.com/multiformats/go-base36 v0.1.0 // indirect
	github.com/multiformats/go-multibase v0.0.3 // indirect
	github.com/multiformats/go-multicodec v0.4.1 // indirect
	github.com/multiformats/go-multihash v0.1.0 // indirect
	github.com/multiformats/go-varint v0.0.6 // indirect
	github.com/opencontainers/runc v1.1.2 // indirect
	github.com/opencontainers/runtime-tools v0.0.0-20190417131837-cd1349b7c47e // indirect
	github.com/opencontainers/selinux v1.10.1 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/philhofer/fwd v1.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/polydawn/refmt v0.0.0-20201211092308-30ac6d18308e // indirect
	github.com/prometheus/client_golang v1.12.2 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rs/cors v1.7.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spacemonkeygo/spacelog v0.0.0-20180420211403-2296661a0572 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stefanberger/go-pkcs11uri v0.0.0-20201008174630-78d3cae3a980 // indirect
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tinylib/msgp v1.1.6 // indirect
	github.com/urfave/cli v1.22.9 // indirect
	github.com/vbatts/tar-split v0.11.2 // indirect
	github.com/whyrusleeping/cbor-gen v0.0.0-20200123233031-1cdf64d27158 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	go.mozilla.org/pkcs7 v0.0.0-20200128120323-432b2356ecb1 // indirect
	go.opencensus.io v0.23.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.19.0 // indirect
	golang.org/x/text v0.3.7
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/genproto v0.0.0-20220426171045-31bebdecfb46 // indirect
	google.golang.org/grpc v1.47.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/square/go-jose.v2 v2.5.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	lukechampine.com/blake3 v1.1.6 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	// containerd main
	github.com/containerd/containerd => github.com/containerd/containerd v1.6.1-0.20220807041520-6deb1174fbfc
	// Temporary fork for avoiding importing patent-protected code: https://github.com/hashicorp/golang-lru/issues/73
	github.com/hashicorp/golang-lru => github.com/ktock/golang-lru v0.5.5-0.20211029085301-ec551be6f75c
)
