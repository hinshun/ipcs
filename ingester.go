package ipcs

import (
	"context"
	"io"
	"time"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/hinshun/ipcs/digestconv"
	files "github.com/ipfs/go-ipfs-files"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/ipfs/interface-go-ipfs-core/path"
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

	if wOpts.Desc.Digest != "" {
		c, err := digestconv.DigestToCid(wOpts.Desc.Digest)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert digest '%s' to cid", wOpts.Desc.Digest)
		}

		_, err = s.cln.Unixfs().Get(ctx, path.IpfsPath(c))
		if err == nil {
			return nil, errors.Wrapf(errdefs.ErrAlreadyExists, "content %v", wOpts.Desc.Digest)
		}
	}

	w := &writer{
		ctx:   ctx,
		cln:   s.cln,
		ref:   wOpts.Ref,
		total: wOpts.Desc.Size,
	}

	err := w.Truncate(0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to truncate writer")
	}

	return w, nil
}

type writer struct {
	ctx       context.Context
	cln       iface.CoreAPI
	ref       string
	offset    int64
	total     int64
	dgst      digest.Digest
	startedAt time.Time
	updatedAt time.Time
	pw        io.Writer
	ipfsErr   error
	cancel    func() error
}

// Write writes len(p) bytes from p to the underlying data stream.
// It returns the number of bytes written from p (0 <= n <= len(p))
// and any error encountered that caused the write to stop early.
// Write must return a non-nil error if it returns n < len(p).
// Write must not modify the slice data, even temporarily.
//
// Implementations must not retain p.
func (w *writer) Write(p []byte) (n int, err error) {
	if w.ipfsErr != nil {
		return 0, w.ipfsErr
	}

	n, err = w.pw.Write(p)
	w.offset += int64(n)
	w.updatedAt = time.Now()
	return n, err
}

// Close closes the writer, if the writer has not been
// committed this allows resuming or aborting.
// Calling Close on a closed writer will not error.
func (w *writer) Close() error {
	return w.cancel()
}

// Digest may return empty digest or panics until committed.
func (w *writer) Digest() digest.Digest {
	return w.dgst
}

// Commit commits the blob (but no roll-back is guaranteed on an error).
// size and expected can be zero-value when unknown.
// Commit always closes the writer, even on error.
// ErrAlreadyExists aborts the writer.
func (w *writer) Commit(ctx context.Context, size int64, expected digest.Digest, opts ...content.Opt) error {
	if size > 0 && size != w.offset {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "unexpected commit size %d, expected %d", w.offset, size)
	}

	if expected != "" && expected != w.dgst {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "unexpected commit digest %s, expected %s", w.dgst, expected)
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
	if size != 0 {
		return errors.New("Truncate: unsupported size")
	}

	if w.cancel != nil {
		err := w.cancel()
		if err != nil {
			return err
		}
	}

	var r io.ReadCloser
	r, w.pw = io.Pipe()

	ctx, cancel := context.WithCancel(w.ctx)
	go func() {
		p, err := w.cln.Unixfs().Add(ctx, files.NewReaderFile(r), options.Unixfs.Pin(true))
		if err != nil {
			w.ipfsErr = err
			return
		}

		dgst, err := digestconv.CidToDigest(p.Cid())
		if err != nil {
			w.ipfsErr = err
			return
		}

		w.dgst = dgst
	}()

	w.cancel = func() error {
		cancel()
		w.ipfsErr = nil

		err := w.Close()
		if err != nil {
			return err
		}

		return r.Close()
	}

	now := time.Now()
	w.startedAt = now
	w.updatedAt = now
	w.offset = 0
	return nil
}
