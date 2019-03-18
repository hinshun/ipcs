package ipcs

import (
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/content/local"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/pkg/errors"
)

type Config struct {
	RootDir string
	IpfsPath string
}

type store struct {
	content.Store
	cln    iface.CoreAPI
}

func NewContentStore(cfg Config) (content.Store, error) {
	s, err := local.NewStore(cfg.RootDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create local content store")
	}

	cln, err := httpapi.NewPathApi(cfg.IpfsPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ipfs client")
	}

	return &store{
		Store:  s,
		cln:    cln,
	}, nil
}
