package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Netflix/p2plab/pkg/digestconv"
	"github.com/hinshun/ipcs"
)

func main() {
	err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx := context.Background()
	cln, err := ipcs.NewClient("/run/user/1001/contentd/contentd.sock")
	if err != nil {
		return err
	}

	name, desc, err := cln.Resolver().Resolve(ctx, args[0])
	if err != nil {
		return err
	}

	c, err := digestconv.DigestToCid(desc.Digest)
	if err != nil {
		return err
	}

	fmt.Printf("Resolved [%d] %s (%s) @ %s\n", desc.Size, name, c, desc.Digest)
	return nil
}
