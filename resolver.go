package ipcs

import (
	"context"

	"github.com/containerd/containerd/api/types"
	"github.com/hinshun/ipcs/pkg/digestconv"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func (p *Peer) Resolve(ctx context.Context, req *ResolveRequest) (*ResolveResponse, error) {
	nd, err := p.Get(ctx, req.Ref)
	if err != nil {
		return nil, err
	}

	file, err := p.GetFile(ctx, req.Ref)
	if err != nil {
		return nil, err
	}

	size, err := file.Size()
	if err != nil {
		return nil, err
	}

	dgst, err := digestconv.CidToDigest(nd.Cid())
	if err != nil {
		return nil, err
	}

	return &ResolveResponse{
		Resolved: Resolved{
			Name: corepath.New(req.Ref).String(),
			Target: types.Descriptor{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    dgst,
				Size_:     size,
			},
		},
	}, nil
}
