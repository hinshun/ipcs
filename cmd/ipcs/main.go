package main

import (
	"github.com/containerd/containerd/plugin"
	"github.com/hinshun/image2ipfs/ipcs"
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

	c := ipcs.Config{
		RootDir: ic.Root,
	}

	s, err := ipcs.New(c)
	if err != nil {
		return nil, errors.Wrap(err, "ipcs: failed to create content store")
	}

	return s, nil
}
