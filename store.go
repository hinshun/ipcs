package ipcs

import (
	"github.com/containerd/containerd/content"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/pkg/errors"
)

type Config struct {
	IpfsPath string
}

type store struct {
	cln iface.CoreAPI
}

func NewContentStore(cfg Config) (content.Store, error) {
	cln, err := httpapi.NewPathApi(cfg.IpfsPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ipfs client")
	}

	return &store{
		cln: cln,
	}, nil
}

func NewContentStoreFromCoreAPI(cln iface.CoreAPI) content.Store {
	return &store{cln}
}
