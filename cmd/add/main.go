package main

import (
	"context"
	"fmt"
	"os"

	"github.com/hinshun/ipcs"
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
	p, err := ipcs.New(ctx, "/ip4/0.0.0.0/udp/0/quic", "./tmp/contentd")
	if err != nil {
		return err
	}

	for _, arg := range args[1:] {
		f, err := os.Open(arg)
		if err != nil {
			return err
		}
		defer f.Close()

		nd, err := p.Add(ctx, f)
		if err != nil {
			return err
		}

		fmt.Println("Added file", nd.Cid())
	}

	return nil
}
