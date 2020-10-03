package ipcs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"regexp"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

var (
	alreadyExistDigestPattern = regexp.MustCompile(`content (sha256:[^:]+):`)
)

// Converter converts OCI images to p2p distributed images via IPFS.
type Converter interface {
	Convert(ctx context.Context, desc ocispec.Descriptor) (ocispec.Descriptor, error)
}

type converter struct {
	store    content.Store
	provider content.Provider
}

// NewConverter returns a new image manifest converter.
func NewConverter(store content.Store, provider content.Provider) Converter {
	return &converter{
		store:    store,
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

	mfst.Config.Digest, err = copyFile(ctx, c.store, c.provider, mfst.Config)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "failed to upload manifest config blob %q", mfst.Config.Digest)
	}

	for i, layer := range mfst.Layers {
		mfst.Layers[i].Digest, err = copyFile(ctx, c.store, c.provider, layer)
		if err != nil {
			return ocispec.Descriptor{}, errors.Wrapf(err, "failed to upload blob %q", layer.Digest)
		}
	}

	mfstJSON, err := json.MarshalIndent(mfst, "", "   ")
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "failed to marshal manifest JSON")
	}

	mfstDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Size:      int64(len(mfstJSON)),
	}

	mfstDesc.Digest, err = addFile(ctx, c.store, bytes.NewReader(mfstJSON), mfstDesc)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "failed to upload manifest")
	}
	log.Printf("Converted Manifest [%d] %s:\n%s", len(mfstJSON), mfstDesc.Digest, mfstJSON)

	fn := images.SetChildrenMappedLabels(c.store, images.ChildrenHandler(c.store), nil)
	_, err = fn(ctx, mfstDesc)
	return mfstDesc, err
}

// copyFile copies content specified by its descriptor from a provider to IPFS.
func copyFile(ctx context.Context, ingester content.Ingester, provider content.Provider, desc ocispec.Descriptor) (digest.Digest, error) {
	ra, err := provider.ReaderAt(ctx, desc)
	if err != nil {
		return desc.Digest, errors.Wrap(err, "failed to create reader")
	}
	defer ra.Close()

	return addFile(ctx, ingester, content.NewReader(ra), desc)
}

// addFile adds a file to IPFS. In the case of layers, these are the layer
// tarballs with an optional "+gzip" compression.
func addFile(ctx context.Context, ingester content.Ingester, r io.Reader, desc ocispec.Descriptor) (dgst digest.Digest, err error) {
	ref := remotes.MakeRefKey(ctx, desc)
	desc.Digest = ""

	cw, err := content.OpenWriter(ctx, ingester, content.WithRef(ref), content.WithDescriptor(desc))
	if err != nil {
		return
	}
	defer cw.Close()

	size, err := io.Copy(cw, bufio.NewReader(r))
	if err != nil {
		return
	}

	err = cw.Commit(ctx, size, "")
	if err != nil {
		if !errdefs.IsAlreadyExists(err) {
			return
		}
	}

	dgst = cw.Digest()
	if dgst == "" {
		// return "", err
		fmt.Println("Commit with err", err)
		matches := alreadyExistDigestPattern.FindStringSubmatch(err.Error())
		return digest.Parse(matches[1])
	}
	return dgst, nil
}
