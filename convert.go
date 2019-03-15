package image2ipfs

import (
	"context"
	"encoding/json"
	"log"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	"github.com/hinshun/image2ipfs/util/digestconv"
	files "github.com/ipfs/go-ipfs-files"
	iface "github.com/ipfs/interface-go-ipfs-core"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

func Convert(ctx context.Context, cln iface.CoreAPI, provider content.Provider, ingester content.Ingester, desc ocispec.Descriptor) ([]byte, ocispec.Descriptor, error) {
	mfst, err := images.Manifest(ctx, provider, desc, platforms.Default())
	if err != nil {
		return nil, ocispec.Descriptor{}, errors.Wrap(err, "failed to get manifest")
	}

	origMfstJSON, err := json.MarshalIndent(mfst, "", "   ")
	if err != nil {
		return nil, ocispec.Descriptor{}, errors.Wrap(err, "failed to marshal manifest JSON")
	}
	log.Printf("Original Manifest:\n%s", origMfstJSON)

	mfst.Config, err = uploadFromStore(ctx, cln, provider, ingester, mfst.Config)
	if err != nil {
		return nil, ocispec.Descriptor{}, errors.Wrapf(err, "failed to upload manifest config blob %q", mfst.Config.Digest)
	}

	for i, layer := range mfst.Layers {
		mfst.Layers[i], err = uploadFromStore(ctx, cln, provider, ingester, layer)
		if err != nil {
			return nil, ocispec.Descriptor{}, errors.Wrapf(err, "failed to upload blob %q", layer.Digest)
		}
	}

	mfstJSON, err := json.MarshalIndent(mfst, "", "   ")
	if err != nil {
		return nil, ocispec.Descriptor{}, errors.Wrap(err, "failed to marshal manifest JSON")
	}
	log.Printf("Converted Manifest:\n%s", mfstJSON)

	mfstDesc, err := uploadFromReader(ctx, cln, provider, ingester, files.NewBytesFile(mfstJSON), desc)
	if err != nil {
		return nil, ocispec.Descriptor{}, errors.Wrap(err, "failed to upload manifest")
	}

	return mfstJSON, mfstDesc, nil
}

func uploadFromStore(ctx context.Context, cln iface.CoreAPI, provider content.Provider, ingester content.Ingester, desc ocispec.Descriptor) (ocispec.Descriptor, error) {
	ra, err := provider.ReaderAt(ctx, desc)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "failed to create reader")
	}
	defer ra.Close()

	return uploadFromReader(ctx, cln, provider, ingester, files.NewReaderFile(content.NewReader(ra)), desc)
}

func uploadFromReader(ctx context.Context, cln iface.CoreAPI, provider content.Provider, ingester content.Ingester, n files.Node, desc ocispec.Descriptor) (ocispec.Descriptor, error) {
	p, err := cln.Unixfs().Add(ctx, n)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "failed to put blob to ipfs")
	}
	log.Printf("Added blob %q to ipfs as %q", desc.Digest, p.Cid())

	dgst, err := digestconv.CidToDigest(p.Cid())
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "failed to convert cid %q to digest", p.Cid())
	}

	n, err = cln.Unixfs().Get(ctx, p)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "failed to get ipld node %q", p.Cid())
	}

	size, err := n.Size()
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "failed to get ipld size %q", p.Cid())
	}

	ipldDesc := desc
	ipldDesc.Digest = dgst
	ipldDesc.Size = size

	err = content.WriteBlob(ctx, ingester, remotes.MakeRefKey(ctx, ipldDesc), files.ToFile(n), ipldDesc)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "failed to write blob %q", ipldDesc.Digest)
	}
	log.Printf("Added blob %q to containerd", ipldDesc.Digest)

	return ipldDesc, nil
}
