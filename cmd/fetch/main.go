package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/hinshun/ipcs"
	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

func main() {
	err := run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	dgst, err := digest.Parse(args[1])
	if err != nil {
		return errors.Wrap(err, "")
	}

	resolver := docker.NewResolver(docker.ResolverOptions{
		Client: http.DefaultClient,
	})

	ctx := context.Background()
	name, desc, err := resolver.Resolve(ctx, "docker.io/library/alpine:latest")
	if err != nil {
		return errors.Wrap(err, "")
	}

	fetcher, err := resolver.Fetcher(ctx, name)
	if err != nil {
		return errors.Wrap(err, "")
	}

	mfst, err := images.Manifest(ctx, ipcs.FromFetcher(fetcher), desc, platforms.Default())
	if err != nil {
		return errors.Wrap(err, "")
	}

	var target ocispec.Descriptor
	for _, layer := range mfst.Layers {
		if layer.Digest.String() == dgst.String() {
			target = layer
		}
	}

	rc, err := fetcher.Fetch(ctx, target)
	if err != nil {
		return errors.Wrap(err, "")
	}
	defer rc.Close()

	dt, err := ioutil.ReadAll(rc)
	if err != nil {
		return errors.Wrap(err, "")
	}

	return ioutil.WriteFile("blob", dt, 0644)
}
