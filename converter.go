package ipcs

import (
	"context"
	"encoding/json"
	"log"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/hinshun/ipcs/digestconv"
	files "github.com/ipfs/go-ipfs-files"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"
	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

type Converter interface {
	Convert(ctx context.Context, desc ocispec.Descriptor) (ocispec.Descriptor, error)
}

type converter struct {
	cln      iface.CoreAPI
	provider content.Provider
}

func NewConverter(cln iface.CoreAPI, provider content.Provider) Converter {
	return &converter{
		cln:      cln,
		provider: provider,
	}
}

func (c *converter) Convert(ctx context.Context, desc ocispec.Descriptor) (ocispec.Descriptor, error) {
	mfst, err := images.Manifest(ctx, c.provider, desc, platforms.Default())
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "failed to get manifest")
	}

	origMfstJSON, err := json.MarshalIndent(mfst, "", "   ")
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "failed to marshal manifest JSON")
	}
	log.Printf("Original Manifest [%d] %s:\n%s", len(origMfstJSON), desc.Digest, origMfstJSON)

	origMfstConfigJSON, err := content.ReadBlob(ctx, c.provider, mfst.Config)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "failed to get original manifest config JSON")
	}
	log.Printf("Original Manifest Config [%d] %s:\n%s", len(origMfstConfigJSON), mfst.Config.Digest, origMfstConfigJSON)

	mfst.Config.Digest, err = copyFile(ctx, c.cln, c.provider, mfst.Config)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "failed to upload manifest config blob %q", mfst.Config.Digest)
	}

	for i, layer := range mfst.Layers {
		mfst.Layers[i].Digest, err = copyFile(ctx, c.cln, c.provider, layer)
		if err != nil {
			return ocispec.Descriptor{}, errors.Wrapf(err, "failed to upload blob %q", layer.Digest)
		}
	}

	mfstJSON, err := json.MarshalIndent(mfst, "", "   ")
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "failed to marshal manifest JSON")
	}

	mfstDigest, err := addFile(ctx, c.cln, files.NewBytesFile(mfstJSON))
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "failed to upload manifest")
	}
	log.Printf("Converted Manifest [%d] %s:\n%s", len(mfstJSON), mfstDigest, mfstJSON)

	return ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    mfstDigest,
		Size:      int64(len(mfstJSON)),
	}, nil
}

func copyFile(ctx context.Context, cln iface.CoreAPI, provider content.Provider, desc ocispec.Descriptor) (digest.Digest, error) {
	ra, err := provider.ReaderAt(ctx, desc)
	if err != nil {
		return "", errors.Wrap(err, "failed to create reader")
	}
	defer ra.Close()

	return addFile(ctx, cln, files.NewReaderFile(content.NewReader(ra)))
}

func addFile(ctx context.Context, cln iface.CoreAPI, n files.Node) (digest.Digest, error) {
	p, err := cln.Unixfs().Add(ctx, n, options.Unixfs.Pin(true))
	if err != nil {
		return "", errors.Wrap(err, "failed to put blob to ipfs")
	}

	dgst, err := digestconv.CidToDigest(p.Cid())
	if err != nil {
		return "", errors.Wrapf(err, "failed to convert cid %q to digest", p.Cid())
	}

	return dgst, nil
}
