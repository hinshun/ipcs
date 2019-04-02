package ipcs

import (
	"context"
	"io"

	"github.com/hinshun/ipcs/digestconv"
	files "github.com/ipfs/go-ipfs-files"
	iface "github.com/ipfs/interface-go-ipfs-core"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

func (s *store) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	c, err := digestconv.DigestToCid(desc.Digest)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert digest '%s' to cid", desc.Digest)
	}

	n, err := s.cln.Unixfs().Get(ctx, iface.IpfsPath(c))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get unixfs node %q", c)
	}

	return files.ToFile(n), nil
}
