package ipcs

import (
	"context"
	"io"
	"io/ioutil"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/log"
	"github.com/hinshun/image2ipfs/util/digestconv"
	files "github.com/ipfs/go-ipfs-files"
	iface "github.com/ipfs/interface-go-ipfs-core"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

func (s *store) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
    	// panic("ReaderAt")
	log.L.WithField("desc.Digest", desc.Digest).Infof("ReaderAt")
	c, err := digestconv.DigestToCid(desc.Digest)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert digest '%s' to cid", desc.Digest)
	}

	log.L.WithField("c", c.String()).Infof("Unixfs.Get")
	n, err := s.cln.Unixfs().Get(ctx, iface.IpfsPath(c))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get unixfs node %q", c)
	}

	size, err := n.Size()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get size of %q", c)
	}

	return &sizeReaderAt{
		size:   size,
		reader: files.ToFile(n),
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
