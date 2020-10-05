package ipcs

import (
	"context"
	"time"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/hinshun/ipcs/pkg/digestconv"
	digest "github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

// Info will return metadata about content available in the content store.
//
// If the content is not present, ErrNotFound will be returned.
func (p *Peer) Info(ctx context.Context, dgst digest.Digest) (content.Info, error) {
	c, err := digestconv.DigestToCid(dgst)
	if err != nil {
		return content.Info{}, errors.Wrapf(err, "failed to convert digest %q to cid", dgst)
	}

	file, err := p.GetFile(ctx, c.String())
	if err != nil {
		return content.Info{}, errors.Wrapf(err, "failed to get node %q", c)
	}

	size, err := file.Size()
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
func (p *Peer) Update(ctx context.Context, info content.Info, fieldpaths ...string) (content.Info, error) {
	return content.Info{}, errors.Wrapf(errdefs.ErrFailedPrecondition, "update not supported on immutable content store")
}

// Walk will call fn for each item in the content store which
// match the provided filters. If no filters are given all
// items will be walked.
func (p *Peer) Walk(ctx context.Context, fn content.WalkFunc, filters ...string) error {
	return nil

	cids, err := p.bstore.AllKeysChan(ctx)
	if err != nil {
		return err
	}

	// TODO: Filters are also not supported in containerd's local store.
	// Since we replace the local store, and filters are implemented in the boltdb
	// metadata that wraps the local store, we can wait until upstream supports
	// it too.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case c, ok := <-cids:
			if !ok {
				return nil
			}

			dgst, err := digestconv.CidToDigest(c)
			if err != nil {
				return err
			}

			info, err := p.Info(ctx, dgst)
			if err != nil {
				return err
			}

			err = fn(info)
			if err != nil {
				return err
			}
		}
	}
}

// Delete removes the content from the store.
func (p *Peer) Delete(ctx context.Context, dgst digest.Digest) error {
	c, err := digestconv.DigestToCid(dgst)
	if err != nil {
		return errors.Wrap(err, "failed to convert digest")
	}

	return p.dserv.Remove(ctx, c)
}
