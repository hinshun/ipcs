package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/hinshun/ipcs"
	"github.com/hinshun/ipcs/pkg/digestconv"
	cid "github.com/ipfs/go-cid"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	merkledag "github.com/ipfs/go-merkledag"
	iface "github.com/ipfs/interface-go-ipfs-core"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

func main() {
	ctx := namespaces.WithNamespace(context.Background(), "ipcs")
	err := run(ctx, os.Args[1:]...)
	if err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, refs ...string) error {
	ipfsCln, err := httpapi.NewLocalApi()
	if err != nil {
		return errors.Wrap(err, "failed to create ipfs client")
	}

	ctrdCln, err := containerd.New("./tmp/containerd/containerd.sock")
	if err != nil {
		return errors.Wrap(err, "failed to create containerd client")
	}

	descs := make([]ocispec.Descriptor, len(refs))
	for i, ref := range refs {
		log.Printf("Converting manifest for %q", ref)
		desc, err := ConvertManifest(ctx, ipfsCln, ctrdCln, ref)
		if err != nil {
			return errors.Wrapf(err, "failed to convert manifest for ref %q", ref)
		}

		descs[i] = desc
	}

	blockParentByCid := make(map[string]*BlockParent)
	for i, desc := range descs {
		log.Printf("Comparing manifest blocks for %q (%q)", refs[i], desc.Digest)
		err = CompareManifestBlocks(ctx, ipfsCln, blockParentByCid, refs[i], desc)
		if err != nil {
			return errors.Wrap(err, "failed to compare manifest blocks")
		}
	}
	log.Printf("Found %d blocks", len(blockParentByCid))

	blockBuckets := make(map[string]uint64)
	for _, blockParent := range blockParentByCid {
		var refs []string
		for ref := range blockParent.Refs {
			refs = append(refs, ref)
		}
		sort.Strings(refs)

		bucket := strings.Join(refs, " n ")
		blockBuckets[bucket] += blockParent.Size
	}

	var bucketNames []string
	for name := range blockBuckets {
		bucketNames = append(bucketNames, name)
	}
	sort.Strings(bucketNames)

	for _, name := range bucketNames {
		fmt.Printf("%s: %d\n", name, blockBuckets[name])
	}

	return nil
}

func ConvertManifest(ctx context.Context, ipfsCln iface.CoreAPI, ctrdCln *containerd.Client, ref string) (ocispec.Descriptor, error) {
	resolver := docker.NewResolver(docker.ResolverOptions{
		Client: http.DefaultClient,
	})

	srcName, srcDesc, err := resolver.Resolve(ctx, ref)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "failed to resolve %q", ref)
	}
	log.Printf("Resolved %q as \"%s@%s\"", ref, srcName, srcDesc.Digest)

	fetcher, err := resolver.Fetcher(ctx, srcName)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "failed to create fetcher for %q", srcName)
	}

	converter := ipcs.NewConverter(ipfsCln, ipcs.FromFetcher(fetcher))
	dstDesc, err := converter.Convert(ctx, srcDesc)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "failed to convert %q to ipfs manifest", srcName)
	}

	return dstDesc, nil
}

type BlockParent struct {
	Size uint64
	Refs map[string]struct{}
}

func CompareManifestBlocks(ctx context.Context, ipfsCln iface.CoreAPI, blockParentByCid map[string]*BlockParent, ref string, desc ocispec.Descriptor) error {
	store, err := ipcs.New(ctx, "/ip4/0.0.0.0/udp/0/quic", "/run/user/1001/contentd")
	if err != nil {
		return err
	}

	mfst, err := images.Manifest(ctx, store, desc, platforms.Default())
	if err != nil {
		return errors.Wrap(err, "failed to get manifest")
	}

	descriptors := append([]ocispec.Descriptor{desc, mfst.Config}, mfst.Layers...)
	for _, desc := range descriptors {
		c, err := digestconv.DigestToCid(desc.Digest)
		if err != nil {
			return errors.Wrapf(err, "failed to convert digest %q to cid", desc.Digest)
		}

		var dagErr error
		err = merkledag.EnumerateChildrenAsync(ctx, merkledag.GetLinksWithDAG(ipfsCln.Dag()), c, func(v cid.Cid) bool {
			n, err := ipfsCln.Dag().Get(ctx, v)
			if err != nil {
				dagErr = err
				return false
			}

			blockParent, ok := blockParentByCid[v.String()]
			if !ok {
				size, err := n.Size()
				if err != nil {
					dagErr = err
					return false
				}

				blockParent = &BlockParent{
					Size: size,
					Refs: make(map[string]struct{}),
				}
			}

			blockParent.Refs[ref] = struct{}{}
			blockParentByCid[v.String()] = blockParent
			return true
		})
		if err != nil {
			return errors.Wrapf(err, "failed to enumerate children of cid %q", c)
		}
		if dagErr != nil {
			return errors.Wrapf(dagErr, "encountered dag err enumerating children of cid %q", c)
		}
	}

	return nil
}
