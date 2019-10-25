package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/hinshun/ipcs"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/moby/buildkit/util/contentutil"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("convert: requires exactly 2 args")
	}

	ctx := namespaces.WithNamespace(context.Background(), "ipfs")
	err := run(ctx, os.Args[1], os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, src, dst string) error {
	ipfsCln, err := httpapi.NewLocalApi()
	if err != nil {
		return errors.Wrap(err, "failed to create ipfs client")
	}

	ctrdCln, err := containerd.New("./tmp/containerd/containerd.sock")
	if err != nil {
		return errors.Wrap(err, "failed to create containerd client")
	}

	err = Convert(ctx, ipfsCln, ctrdCln, src, dst)
	if err != nil {
		return errors.Wrap(err, "failed to convert to p2p manifest")
	}

	// err = RunContainer(ctx, ctrdCln, dst, "helloworld")
	// if err != nil {
	// 	return errors.Wrap(err, "failed to run container")
	// }

	return nil
}

func Convert(ctx context.Context, ipfsCln iface.CoreAPI, ctrdCln *containerd.Client, src, dst string) error {
	resolver := docker.NewResolver(docker.ResolverOptions{
		Client: http.DefaultClient,
	})

	srcName, srcDesc, err := resolver.Resolve(ctx, src)
	if err != nil {
		return errors.Wrapf(err, "failed to resolve %q", src)
	}
	log.Printf("Resolved %q as \"%s@%s\"", src, srcName, srcDesc.Digest)

	fetcher, err := resolver.Fetcher(ctx, src)
	if err != nil {
		return errors.Wrapf(err, "failed to create fetcher for %q", src)
	}

	converter := ipcs.NewContinuityConverter(ipfsCln, contentutil.FromFetcher(fetcher))
	mfstDesc, err := converter.Convert(ctx, srcDesc)
	if err != nil {
		return errors.Wrapf(err, "failed to convert %q to ipfs manifest", srcName)
	}

	ipcsCln := ipcs.NewClient(ipfsCln, ctrdCln)
	img, err := ipcsCln.Pull(ctx, dst, mfstDesc)
	if err != nil {
		return errors.Wrapf(err, "failed to pull descriptor %q", mfstDesc.Digest)
	}

	log.Printf("Successfully pulled image %q", img.Name())
	return nil
}

func RunContainer(ctx context.Context, cln *containerd.Client, ref, id string) error {
	image, err := cln.GetImage(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "failed to get image")
	}
	log.Printf("Successfully get image %q", image.Name())

	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
		s     specs.Spec
	)

	opts = append(opts,
		oci.WithTTY,
		oci.WithRootFSReadonly(),
		oci.WithProcessCwd("/"),
		oci.WithProcessArgs("/bin/sh"),
	)
	cOpts = append(cOpts,
		containerd.WithImage(image),
		containerd.WithSnapshotter(containerd.DefaultSnapshotter),
		// Even when "readonly" is set, we don't use KindView snapshot here. (#1495)
		// We pass writable snapshot to the OCI runtime, and the runtime remounts it as read-only,
		// after creating some mount points on demand.
		containerd.WithNewSnapshot(id, image),
		containerd.WithImageStopSignal(image, "SIGTERM"),
		containerd.WithSpec(&s, opts...),
	)

	container, err := cln.NewContainer(ctx, id, cOpts...)
	if err != nil {
		return errors.Wrap(err, "failed to create container")
	}
	log.Printf("Successfully create container %q", container.ID())

	return nil
}
