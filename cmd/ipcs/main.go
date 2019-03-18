package main

import (
	"os"

	"github.com/containerd/containerd/plugin"
	"github.com/hinshun/ipcs"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	"github.com/pkg/errors"
)

func init() {
	plugin.Register(&plugin.Registration{
		Type:   plugin.ContentPlugin,
		ID:     "ipcs",
		Config: &ipcs.Config{},
		InitFn: initIPCSService,
	})
}

func initIPCSService(ic *plugin.InitContext) (interface{}, error) {
	ic.Meta.Exports["root"] = ic.Root

	ipfsPath := os.Getenv(httpapi.EnvDir)
	if ipfsPath == "" {
		ipfsPath = httpapi.DefaultPathRoot
	}

	c := ipcs.Config{
		RootDir:  ic.Root,
		IpfsPath: ipfsPath,
	}

	s, err := ipcs.NewContentStore(c)
	if err != nil {
		return nil, errors.Wrap(err, "ipcs: failed to create content store")
	}

	return s, nil
}
