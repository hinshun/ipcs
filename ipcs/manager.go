package ipcs

import (
	"context"
	"time"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/log"
	"github.com/hinshun/image2ipfs/util/digestconv"
	iface "github.com/ipfs/interface-go-ipfs-core"
	digest "github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

// Info will return metadata about content available in the content store.
//
// If the content is not present, ErrNotFound will be returned.
func (s *store) Info(ctx context.Context, dgst digest.Digest) (content.Info, error) {
    	// panic("Info")
	log.L.WithField("dgst", dgst).Infof("Info")

	c, err := digestconv.DigestToCid(dgst)
	if err != nil {
		return content.Info{}, errors.Wrapf(err, "failed to convert digest %q to cid", dgst)
	}

	log.L.WithField("c", c.String()).Infof("Unixfs.Get")
	n, err := s.cln.Unixfs().Get(ctx, iface.IpfsPath(c))
	if err != nil {
		return content.Info{}, errors.Wrapf(err, "failed to get unixfs node %q", c)
	}

	size, err := n.Size()
	if err != nil {
		return content.Info{}, errors.Wrapf(err, "failed to get size of %q", c)
	}

	now := time.Now()
	return content.Info{
		Digest:    dgst,
		Size:      size,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// func (s *store) Update(ctx context.Context, info content.Info, fieldpaths ...string) (content.Info, error) {
//     panic("Update")
//     return content.Info{}, nil
// }

// func (s *store) Walk(ctx context.Context, fn content.WalkFunc, filters ...string) error {
//     panic("Walk")
//     return nil
// }

// func (s *store)  Delete(ctx context.Context, dgst digest.Digest) error {
//     panic("Delete")
//     return nil
// }
