package ipcs

import (
	"context"
	"time"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/hinshun/ipcs/digestconv"
	iface "github.com/ipfs/interface-go-ipfs-core"
	digest "github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

// Info will return metadata about content available in the content store.
//
// If the content is not present, ErrNotFound will be returned.
func (s *store) Info(ctx context.Context, dgst digest.Digest) (content.Info, error) {
	c, err := digestconv.DigestToCid(dgst)
	if err != nil {
		return content.Info{}, errors.Wrapf(err, "failed to convert digest %q to cid", dgst)
	}

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
	return content.Info{}, errors.Wrapf(errdefs.ErrFailedPrecondition, "update not supported on immutable content store")
}

// Walk will call fn for each item in the content store which
// match the provided filters. If no filters are given all
// items will be walked.
func (s *store) Walk(ctx context.Context, fn content.WalkFunc, filters ...string) error {
	pins, err := s.cln.Pin().Ls(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list ipfs pins")
	}

	for _, pin := range pins {
		c := pin.Path().Cid()
		dgst, err := digestconv.CidToDigest(c)
		if err != nil {
			return errors.Wrap(err, "failed to convert digest")
		}

		info, err := s.Info(ctx, dgst)
		if err != nil {
			return errors.Wrap(err, "failed to get info")
		}

		err = fn(info)
		if err != nil {
			return errors.Wrap(err, "failed to walk info")
		}
	}

	return nil
}

// Delete removes the content from the store.
func (s *store) Delete(ctx context.Context, dgst digest.Digest) error {
	c, err := digestconv.DigestToCid(dgst)
	if err != nil {
		return errors.Wrap(err, "failed to convert digest")
	}

	err = s.cln.Pin().Rm(ctx, iface.IpfsPath(c))
	if err != nil {
		return errors.Wrap(err, "failed to remove pin")
	}

	return nil
}
