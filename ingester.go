package ipcs

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	"github.com/hinshun/ipcs/digestconv"
	files "github.com/ipfs/go-ipfs-files"
	iface "github.com/ipfs/interface-go-ipfs-core"
	digest "github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

func (s *store) Writer(ctx context.Context, opts ...content.WriterOpt) (content.Writer, error) {
	var wOpts content.WriterOpts
	for _, opt := range opts {
		if err := opt(&wOpts); err != nil {
			return nil, err
		}
	}

	log.L.WithField("desc.Digest", wOpts.Desc.Digest).Infof("Writer")
	if wOpts.Desc.Digest != "" {
		c, err := digestconv.DigestToCid(wOpts.Desc.Digest)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert digest '%s' to cid", wOpts.Desc.Digest)
		}

		log.L.WithField("c", c.String()).Infof("Unixfs.Get")
		_, err = s.cln.Unixfs().Get(ctx, iface.IpfsPath(c))
		if err == nil {
			return nil, errors.Wrapf(errdefs.ErrAlreadyExists, "content %v", wOpts.Desc.Digest)
		}
	}

	r, w := io.Pipe()
	errCh := make(chan error, 1)

	now := time.Now()
	addCtx, cancel := context.WithCancel(ctx)

	go func() {
		defer close(errCh)
		_, err := s.cln.Unixfs().Add(addCtx, files.NewReaderFile(r))
		errCh <- err
	}()

	return &writer{
		expected:  wOpts.Desc.Digest,
		startedAt: now,
		updatedAt: now,
		pw:        w,
		cancel: func() error {
			cancel()
			err := w.Close()
			if err != nil {
				return err
			}

			return r.Close()
		},
		errCh: errCh,
	}, nil
}

type writer struct {
	ref       string
	offset    int64
	total     int64
	expected  digest.Digest
	startedAt time.Time
	updatedAt time.Time
	pw        io.Writer
	cancel    func() error
	errCh     chan error
}

// Write writes len(p) bytes from p to the underlying data stream.
// It returns the number of bytes written from p (0 <= n <= len(p))
// and any error encountered that caused the write to stop early.
// Write must return a non-nil error if it returns n < len(p).
// Write must not modify the slice data, even temporarily.
//
// Implementations must not retain p.
func (w *writer) Write(p []byte) (n int, err error) {
	select {
	case err := <-w.errCh:
		return 0, err
	default:
		n, err = w.pw.Write(p)
		w.offset += int64(len(p))
		w.updatedAt = time.Now()
		return n, err
	}

}

// Close closes the writer, if the writer has not been
// committed this allows resuming or aborting.
// Calling Close on a closed writer will not error.
func (w *writer) Close() error {
	return w.cancel()
}

// Digest may return empty digest or panics until committed.
func (w *writer) Digest() digest.Digest {
	return w.expected
}

// Commit commits the blob (but no roll-back is guaranteed on an error).
// size and expected can be zero-value when unknown.
// Commit always closes the writer, even on error.
// ErrAlreadyExists aborts the writer.
func (w *writer) Commit(ctx context.Context, size int64, expected digest.Digest, opts ...content.Opt) error {
	if size > 0 && size != w.offset {
		return errors.Errorf("unexpected commit size %d, expected %d", w.offset, size)
	}
	return w.Close()
}

// Status returns the current state of write
func (w *writer) Status() (content.Status, error) {
	return content.Status{
		Ref:       w.ref,
		Offset:    w.offset,
		Total:     w.total,
		StartedAt: w.startedAt,
		UpdatedAt: w.updatedAt,
	}, nil
}

// Truncate updates the size of the target blob
func (w *writer) Truncate(size int64) error {
	panic("unimplemented")
	w.offset = 0
	return nil
}
