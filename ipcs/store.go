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
}

type store struct {
	content.Store
	config Config
	cln    iface.CoreAPI
}

func New(config Config) (content.Store, error) {
	s, err := local.NewStore(config.RootDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create local content store")
	}

	cln, err := httpapi.NewLocalApi()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ipfs client")
	}

	return &store{
		Store:  s,
		config: config,
		cln:    cln,
	}, nil
}
