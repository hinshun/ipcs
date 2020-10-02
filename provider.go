package ipcs

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/containerd/containerd/content"
	"github.com/hinshun/ipcs/pkg/digestconv"
	files "github.com/ipfs/go-ipfs-files"
	unixfile "github.com/ipfs/go-unixfs/file"
	iface "github.com/ipfs/interface-go-ipfs-core"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// ReaderAt only requires desc.Digest to be set.
// Other fields in the descriptor may be used internally for resolving
// the location of the actual data.
func (p *Peer) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	fmt.Println("ReaderAt", desc.Digest)

	c, err := digestconv.DigestToCid(desc.Digest)
	if err != nil {
		return nil, err
	}

	nd, err := p.dserv.Get(ctx, c)
	if err != nil {
		return nil, err
	}

	f, err := unixfile.NewUnixfsFile(ctx, p.dserv, nd)
	if err != nil {
		return nil, err
	}

	var file files.File
	switch f := f.(type) {
	case files.File:
		file = f
	case files.Directory:
		return nil, iface.ErrIsDir
	default:
		return nil, iface.ErrNotSupported
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
