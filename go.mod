module github.com/hinshun/ipcs

go 1.12

require (
	github.com/Microsoft/go-winio v0.4.12 // indirect
	github.com/Microsoft/hcsshim v0.8.6 // indirect
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412 // indirect
	github.com/btcsuite/btcd v0.0.0-20190315201642-aa6e0f35703c // indirect
	github.com/containerd/aufs v0.0.0-20190114185352-f894a800659b
	github.com/containerd/cgroups v0.0.0-20190226200435-dbea6f2bd416 // indirect
	github.com/containerd/console v0.0.0-20181022165439-0650fd9eeb50 // indirect
	github.com/containerd/containerd v1.2.6
	github.com/containerd/continuity v0.0.0-20181203112020-004b46473808 // indirect
	github.com/containerd/fifo v0.0.0-20190226154929-a9fb20d87448 // indirect
	github.com/containerd/go-runc v0.0.0-20190226155025-7d11b49dc076 // indirect
	github.com/containerd/ttrpc v0.0.0-20190211042230-69144327078c // indirect
	github.com/containerd/typeurl v0.0.0-20190228175220-2a93cfde8c20 // indirect
	github.com/containerd/zfs v0.0.0-20181107152433-31af176f2ae8
	github.com/coreos/go-systemd v0.0.0-20161114122254-48702e0da86b // indirect
	github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible // indirect
	github.com/docker/docker v1.13.1 // indirect
	github.com/docker/go-events v0.0.0-20170721190031-9461782956ad // indirect
	github.com/docker/go-metrics v0.0.0-20181218153428-b84716841b82 // indirect
	github.com/docker/go-units v0.3.3 // indirect
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/gogo/googleapis v1.1.0 // indirect
	github.com/google/go-cmp v0.2.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/ipfs/go-cid v0.0.2
	github.com/ipfs/go-ipfs-files v0.0.3
	github.com/ipfs/go-ipfs-http-client v0.0.2
	github.com/ipfs/go-ipfs-util v0.0.1
	github.com/ipfs/go-merkledag v0.0.3
	github.com/ipfs/interface-go-ipfs-core v0.0.8
	github.com/mistifyio/go-zfs v2.1.1+incompatible // indirect
	github.com/moby/buildkit v0.3.3
	github.com/multiformats/go-multiaddr v0.0.4 // indirect
	github.com/multiformats/go-multihash v0.0.5
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/opencontainers/runtime-spec v0.1.2-0.20190207185410-29686dbc5559
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.3 // indirect
	github.com/sirupsen/logrus v1.4.0 // indirect
	github.com/stretchr/testify v1.3.0
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2 // indirect
	go.etcd.io/bbolt v1.3.2 // indirect
	golang.org/x/sync v0.0.0-20181221193216-37e7f081c4d4
	google.golang.org/grpc v1.19.0 // indirect
	gotest.tools v2.2.0+incompatible // indirect
)

replace github.com/containerd/containerd => github.com/hinshun/containerd v0.2.1-0.20190602215134-c3f4eaaf1470
