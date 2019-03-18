package ipcs

import (
	"context"
	"time"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/log"
	"github.com/hinshun/ipcs/digestconv"
	iface "github.com/ipfs/interface-go-ipfs-core"
	digest "github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

// Info will return metadata about content available in the content store.
//
// If the content is not present, ErrNotFound will be returned.
func (s *store) Info(ctx context.Context, dgst digest.Digest) (content.Info, error) {
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

// Update updates mutable information related to content.
// If one or more fieldpaths are provided, only those
// fields will be updated.
// Mutable fields:
//  labels.*
func (s *store) Update(ctx context.Context, info content.Info, fieldpaths ...string) (content.Info, error) {
	panic("unimplemented")
	return content.Info{}, nil
}

// Walk will call fn for each item in the content store which
// match the provided filters. If no filters are given all
// items will be walked.
func (s *store) Walk(ctx context.Context, fn content.WalkFunc, filters ...string) error {
	panic("unimplemented")
	return nil
}

// Delete removes the content from the store.
func (s *store) Delete(ctx context.Context, dgst digest.Digest) error {
	panic("unimplemented")
	return nil
}
