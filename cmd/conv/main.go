package main

import (
	"fmt"
	"os"

	"github.com/hinshun/ipcs/pkg/digestconv"
	digest "github.com/opencontainers/go-digest"
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "err: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	dgst, err := digest.Parse(os.Args[1])
	if err != nil {
		return err
	}

	c, err := digestconv.DigestToCid(dgst)
	if err != nil {
		return err
	}

	fmt.Println(c)
	return nil
}
