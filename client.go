package ipcs

import (
	"context"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	"github.com/hinshun/ipcs/digestconv"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/path"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// Client is a client for containerd using ipcs.
type Client struct {
	ipfsCln iface.CoreAPI
	ctrdCln *containerd.Client
	ipcs    *store
}

// NewClient returns a new ipcs client.
func NewClient(ipfsCln iface.CoreAPI, ctrdCln *containerd.Client) *Client {
	return &Client{
		ipfsCln: ipfsCln,
		ctrdCln: ctrdCln,
		ipcs: &store{
			cln: ipfsCln,
		},
	}
}

// Pull pulls an image specified by its descriptor and creates an image named
// ref.
func (c *Client) Pull(ctx context.Context, ref string, desc ocispec.Descriptor) (containerd.Image, error) {
	ctx, done, err := c.ctrdCln.WithLease(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create lease on context")
	}
	defer done(ctx)

	img, err := c.Fetch(ctx, ref, desc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch image")
	}

	i := containerd.NewImageWithPlatform(c.ctrdCln, img, platforms.Default())

	// if err := i.Unpack(ctx, containerd.DefaultSnapshotter); err != nil {
	// 	return nil, errors.Wrapf(err, "failed to unpack image on snapshotter %s", containerd.DefaultSnapshotter)
	// }

	return i, nil
}

// Fetch fetches all the content referenced by a p2p manifest descriptor.
func (c *Client) Fetch(ctx context.Context, ref string, desc ocispec.Descriptor) (images.Image, error) {
	store := c.ctrdCln.ContentStore()
	fetcher := c.ipcs

	// Get all the children for a descriptor
	childrenHandler := images.ChildrenHandler(store)
	// Set any children labels for that content
	childrenHandler = images.SetChildrenLabels(store, childrenHandler)
	// Filter children by platforms
	childrenHandler = images.FilterPlatforms(childrenHandler, platforms.Default())
	// Sort and limit manifests if a finite number is needed
	childrenHandler = images.LimitManifests(childrenHandler, platforms.Default(), 1)

	handler := images.Handlers(
		PinHandler(c.ipfsCln),
		remotes.FetchHandler(store, fetcher),
		childrenHandler,
	)

	if err := images.Dispatch(ctx, handler, desc); err != nil {
		return images.Image{}, err
	}

	img := images.Image{
		Name:   ref,
		Target: desc,
	}

	is := c.ctrdCln.ImageService()
	for {
		if created, err := is.Create(ctx, img); err != nil {
			if !errdefs.IsAlreadyExists(err) {
				return images.Image{}, err
			}

			updated, err := is.Update(ctx, img)
			if err != nil {
				// if image was removed, try create again
				if errdefs.IsNotFound(err) {
					continue
				}
				return images.Image{}, err
			}

			img = updated
		} else {
			img = created
		}

		return img, nil
	}
}

// Push is unimplemented. If reference resolution is centralized in a
// metadata-only registry, push may just update the tag to new p2p manifest
// digest.
func (c *Client) Push(ctx context.Context, ref string, desc ocispec.Descriptor) error {
	panic("unimplemented")
	return nil
}

// PinHandler returns a handler that will recursive pin all content discovered
// in a call to Dispatch. Use with ChildrenHandler to do a full recursive pin.
func PinHandler(ipfsCln iface.CoreAPI) images.HandlerFunc {
	return func(ctx context.Context, desc ocispec.Descriptor) (subdescs []ocispec.Descriptor, err error) {
		switch desc.MediaType {
		case images.MediaTypeDockerSchema1Manifest:
			return nil, fmt.Errorf("%v not supported", desc.MediaType)
		default:
			err := pin(ctx, ipfsCln, desc)
			return nil, err
		}
	}
}

func pin(ctx context.Context, ipfsCln iface.CoreAPI, desc ocispec.Descriptor) error {
	c, err := digestconv.DigestToCid(desc.Digest)
	if err != nil {
		return errors.Wrapf(err, "failed to convert digest %q to cid", desc.Digest)
	}

	err = ipfsCln.Pin().Add(ctx, path.IpfsPath(c))
	if err != nil {
		return errors.Wrapf(err, "failed to pin %q", c)
	}

	return nil
}
