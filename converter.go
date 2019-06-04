package ipcs

import (
	"archive/tar"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/containerd/containerd/archive"
	"github.com/containerd/containerd/archive/compression"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/hinshun/ipcs/digestconv"
	files "github.com/ipfs/go-ipfs-files"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/ipfs/interface-go-ipfs-core/path"
	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// Converter converts OCI images to p2p distributed images via IPFS.
type Converter interface {
	Convert(ctx context.Context, desc ocispec.Descriptor) (ocispec.Descriptor, error)
}

type converter struct {
	api      iface.CoreAPI
	provider content.Provider
}

// NewConverter returns a new image manifest converter.
func NewConverter(api iface.CoreAPI, provider content.Provider) Converter {
	return &converter{
		api:      api,
		provider: provider,
	}
}

// Convert converts a manifest specified by its descriptor to a new manifest
// where every descriptor (manifest config and layers) is modified to point to
// the root IPLD node of the respective content added to IPFS.
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

	mfst.Config.Digest, err = copyFile(ctx, c.api, c.provider, mfst.Config)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "failed to upload manifest config blob %q", mfst.Config.Digest)
	}

	for i, layer := range mfst.Layers {
		mfst.Layers[i].Digest, err = copyFile(ctx, c.api, c.provider, layer)
		if err != nil {
			return ocispec.Descriptor{}, errors.Wrapf(err, "failed to upload blob %q", layer.Digest)
		}
	}

	mfstJSON, err := json.MarshalIndent(mfst, "", "   ")
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "failed to marshal manifest JSON")
	}

	mfstDigest, err := addFile(ctx, c.api, files.NewBytesFile(mfstJSON))
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

// copyFile copies content specified by its descriptor from a provider to IPFS.
func copyFile(ctx context.Context, api iface.CoreAPI, provider content.Provider, desc ocispec.Descriptor) (digest.Digest, error) {
	ra, err := provider.ReaderAt(ctx, desc)
	if err != nil {
		return "", errors.Wrap(err, "failed to create reader")
	}
	defer ra.Close()

	return addFile(ctx, api, files.NewReaderFile(content.NewReader(ra)))
}

// addFile adds a file to IPFS. In the case of layers, these are the layer
// tarballs with an optional "+gzip" compression.
func addFile(ctx context.Context, api iface.CoreAPI, n files.Node) (digest.Digest, error) {
	p, err := api.Unixfs().Add(ctx, n, options.Unixfs.Pin(true))
	if err != nil {
		return "", errors.Wrap(err, "failed to put blob to ipfs")
	}

	dgst, err := digestconv.CidToDigest(p.Cid())
	if err != nil {
		return "", errors.Wrapf(err, "failed to convert cid %q to digest", p.Cid())
	}

	return dgst, nil
}

// copyLayer decompresses and untars the layer into a temporary directory and
// adds the directory recursively into IPFS as individual files. This can
// potentially increase the deduplication at a per-file basis, but early tests
// show that similar images often don't have many byte-for-byte equivalent
// files.
//
// Another disadvantage is that the layers are now uncompressed so layer sizes
// and number of IPLD blocks increase roughly 10x. Perhaps every individual
// file can be gzipped to reduce this.
//
// IPFS also doesn't support uid/gid, modtime, xattrs, and other file system
// features to have a working container rootfs atm, so this is just a POC.
func copyLayer(ctx context.Context, api iface.CoreAPI, provider content.Provider, desc ocispec.Descriptor) (digest.Digest, error) {
	ra, err := provider.ReaderAt(ctx, desc)
	if err != nil {
		return "", errors.Wrap(err, "failed to create reader")
	}
	defer ra.Close()

	root, err := ioutil.TempDir("", "ipcs-root")
	if err != nil {
		return "", errors.Wrap(err, "failed to create tmp root directory")
	}

	isCompressed, err := images.IsCompressedDiff(ctx, desc.MediaType)
	if err != nil {
		return "", errors.Wrapf(err, "unsupported diff media type: %v", desc.MediaType)
	}

	r := content.NewReader(ra)

	if isCompressed {
		ds, err := compression.DecompressStream(r)
		if err != nil {
			return "", errors.Wrap(err, "failed to decompress stream")
		}
		defer ds.Close()
		r = ds
	}

	if _, err := archive.Apply(ctx, root, r, archive.WithFilter(RegularTypeFilter)); err != nil {
		return "", errors.Wrapf(err, "failed to apply tar archive to directory %q", root)
	}

	// Read any trailing data
	if _, err := io.Copy(ioutil.Discard, r); err != nil {
		return "", errors.Wrap(err, "failed to discard trailing data after tar archive")
	}

	stat, err := os.Stat(root)
	if err != nil {
		return "", errors.Wrap(err, "failed to stat root")
	}

	n, err := files.NewSerialFile(root, false, stat)
	if err != nil {
		return "", errors.Wrap(err, "failed to create serial file out of root")
	}

	dir := files.NewSliceDirectory([]files.DirEntry{
		files.FileEntry("", n),
	})

	var p path.Resolved
	entries := dir.Entries()
	for entries.Next() {
		p, err = api.Unixfs().Add(ctx, entries.Node(), options.Unixfs.Pin(true))
		if err != nil {
			return "", errors.Wrapf(err, "failed to add node %q", entries.Name())
		}

	}

	if entries.Err() != nil {
		return "", entries.Err()
	}

	log.Printf("Added as %s", p.Cid())

	dgst, err := digestconv.CidToDigest(p.Cid())
	if err != nil {
		return "", errors.Wrapf(err, "failed to convert cid %q to digest", p.Cid())
	}

	return dgst, nil
}

// RegularTypeFilter filters out tar headers that are not regular, symlinks,
// or directories.
//
// IPFS UnixFs does not support special files yet:
// https://github.com/ipfs/go-ipfs/issues/1642
func RegularTypeFilter(header *tar.Header) (bool, error) {
	switch header.Typeflag {
	case tar.TypeReg, tar.TypeSymlink, tar.TypeDir:
		return true, nil
	default:
		return false, nil
	}
}
