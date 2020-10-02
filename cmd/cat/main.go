package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/hinshun/ipcs"
	cid "github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	iface "github.com/ipfs/interface-go-ipfs-core"
)

func main() {
	err := run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx := context.Background()
	p, err := ipcs.New(ctx, "./tmp/ipcsd", 0)
	if err != nil {
		return err
	}

	c, err := cid.Parse(args[1])
	if err != nil {
		return err
	}

	f, err := p.Get(ctx, c)
	if err != nil {
		return err
	}

	var file files.File
	switch f := f.(type) {
	case files.File:
		file = f
	case files.Directory:
		return iface.ErrIsDir
	default:
		return iface.ErrNotSupported
	}

	dt, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", string(dt))
	return nil
}
