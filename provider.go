package ipcs

import (
	"context"
	"io"

	"github.com/containerd/containerd/content"
	"github.com/hinshun/ipcs/digestconv"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/interface-go-ipfs-core/path"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// ReaderAt only requires desc.Digest to be set.
// Other fields in the descriptor may be used internally for resolving
// the location of the actual data.
func (s *store) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	c, err := digestconv.DigestToCid(desc.Digest)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert digest '%s' to cid", desc.Digest)
	}

	n, err := s.cln.Unixfs().Get(ctx, path.IpfsPath(c))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get unixfs node %q", c)
	}

	return &sizeReaderAt{
		size: desc.Size,
		rs:   files.ToFile(n),
	}, nil
}

type sizeReaderAt struct {
	size int64
	rs   io.ReadSeeker
}

func (ra *sizeReaderAt) ReadAt(p []byte, offset int64) (n int, err error) {
	_, err = ra.rs.Seek(offset, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	n, err = ra.rs.Read(p)
	if err == io.EOF {
		err = nil
	}
	return
}

func (ra *sizeReaderAt) Size() int64 {
	return ra.size
}

func (ra *sizeReaderAt) Close() error {
	return nil
}
