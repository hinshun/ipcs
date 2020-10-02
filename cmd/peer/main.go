package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/hinshun/ipcs"
)

func main() {
	err := run(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	root := "./tmp/peer"
	err := os.MkdirAll(root, 0711)
	if err != nil {
		return err
	}

	p, err := ipcs.New(ctx, root, 0)
	if err != nil {
		return err
	}

	var addrs []string
	for _, ma := range p.Host().Addrs() {
		addrs = append(addrs, ma.String())
	}
	fmt.Printf("Starting libp2p peer. ID=%q Listen=%s\n", p.Host().ID(), addrs)

	fmt.Println("Press 'Enter' to terminate peer...")
	_, err = bufio.NewReader(os.Stdin).ReadBytes('\n')
	return err
}
