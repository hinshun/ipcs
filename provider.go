package ipcs

import (
	"context"
	"io"
	"io/ioutil"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/remotes"
	"github.com/hinshun/ipcs/pkg/digestconv"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// ReaderAt only requires desc.Digest to be set.
// Other fields in the descriptor may be used internally for resolving
// the location of the actual data.
func (p *Peer) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	c, err := digestconv.DigestToCid(desc.Digest)
	if err != nil {
		return nil, err
	}

	file, err := p.GetFile(ctx, c.String())
	if err != nil {
		return nil, err
	}

	size := desc.Size
	if size == 0 {
		size, err = file.Size()
		if err != nil {
			return nil, err
		}
	}

	return &sizeReaderAt{
		size:   size,
		reader: file,
	}, nil
}

type sizeReaderAt struct {
	size   int64
	reader io.Reader
	n      int64
}

func (ra *sizeReaderAt) ReadAt(p []byte, offset int64) (n int, err error) {
	if offset < ra.n {
		return 0, errors.New("invalid offset")
	}

	diff := offset - ra.n
	written, err := io.CopyN(ioutil.Discard, ra.reader, diff)
	ra.n += written
	if err != nil {
		return int(written), err
	}

	n, err = ra.reader.Read(p)
	ra.n += int64(n)
	return
}

func (ra *sizeReaderAt) Size() int64 {
	return ra.size
}

func (ra *sizeReaderAt) Close() error {
	return nil
}

func FromFetcher(f remotes.Fetcher) content.Provider {
	return &fetchedProvider{
		f: f,
	}
}

type fetchedProvider struct {
	f remotes.Fetcher
}

func (p *fetchedProvider) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	rc, err := p.f.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}

	return &readerAt{Reader: rc, Closer: rc, size: desc.Size}, nil
}

type readerAt struct {
	io.Reader
	io.Closer
	size   int64
	offset int64
}

func (r *readerAt) ReadAt(b []byte, off int64) (int, error) {
	if ra, ok := r.Reader.(io.ReaderAt); ok {
		return ra.ReadAt(b, off)
	}

	if r.offset != off {
		if seeker, ok := r.Reader.(io.Seeker); ok {
			if _, err := seeker.Seek(off, io.SeekStart); err != nil {
				return 0, err
			}
			r.offset = off
		} else {
			return 0, errors.Errorf("unsupported offset")
		}
	}

	var totalN int
	for len(b) > 0 {
		n, err := r.Reader.Read(b)
		if err == io.EOF && n == len(b) {
			err = nil
		}
		r.offset += int64(n)
		totalN += n
		b = b[n:]
		if err != nil {
			return totalN, err
		}
	}
	return totalN, nil
}

func (r *readerAt) Size() int64 {
	return r.size
}
