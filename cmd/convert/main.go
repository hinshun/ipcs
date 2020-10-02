package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/hinshun/ipcs"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("convert: requires exactly 1 arg")
	}

	ctx := namespaces.WithNamespace(context.Background(), "ipcs")
	err := run(ctx, os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, ref string) error {
	cln, err := containerd.New("./tmp/containerd/containerd.sock")
	if err != nil {
		return errors.Wrap(err, "failed to create containerd client")
	}

	return Convert(ctx, cln, ref)
}

func Convert(ctx context.Context, cln *containerd.Client, ref string) error {
	resolver := docker.NewResolver(docker.ResolverOptions{
		Client: http.DefaultClient,
	})

	name, desc, err := resolver.Resolve(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "failed to resolve %q", ref)
	}
	log.Printf("Resolved %q as \"%s@%s\"", name, ref, desc.Digest)

	fetcher, err := resolver.Fetcher(ctx, name)
	if err != nil {
		return errors.Wrapf(err, "failed to create fetcher for %q", name)
	}

	converter := ipcs.NewConverter(cln.ContentStore(), ipcs.FromFetcher(fetcher))
	mfstDesc, err := converter.Convert(ctx, desc)
	if err != nil {
		return errors.Wrapf(err, "failed to convert %q to ipfs manifest", name)
	}
	log.Printf("Successfully converted manifest %s", mfstDesc.Digest)

	img := images.Image{
		Name:   name,
		Target: mfstDesc,
	}

	is := cln.ImageService()
	for {
		if created, err := is.Create(ctx, img); err != nil {
			if !errdefs.IsAlreadyExists(err) {
				return err
			}

			updated, err := is.Update(ctx, img)
			if err != nil {
				// if image was removed, try create again
				if errdefs.IsNotFound(err) {
					continue
				}
				return err
			}

			img = updated
		} else {
			img = created
		}
		break
	}
	log.Printf("Successfully created image %s", img.Name)

	i := containerd.NewImageWithPlatform(cln, img, platforms.Default())
	err = i.Unpack(ctx, containerd.DefaultSnapshotter)
	if err != nil {
		return err
	}
	log.Printf("Successfully unpacked image %s", img.Name)

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
		oci.WithRootFSReadonly(),
		oci.WithProcessCwd("/"),
		oci.WithProcessArgs("/bin/sleep", "1"),
	)
	cOpts = append(cOpts,
		containerd.WithImage(image),
		containerd.WithSnapshotter(containerd.DefaultSnapshotter),
		containerd.WithNewSnapshot(id, image),
		containerd.WithImageStopSignal(image, "SIGTERM"),
		containerd.WithSpec(&s, opts...),
	)

	log.Printf("Creating container %q", id)
	container, err := cln.NewContainer(ctx, id, cOpts...)
	if err != nil {
		return errors.Wrap(err, "failed to create container")
	}
	log.Printf("Successfully create container %q", container.ID())

	return nil
}
