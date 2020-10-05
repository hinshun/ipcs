package main

import (
	"context"
	"fmt"
	"io/ioutil"
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
	p, err := ipcs.New(ctx, "/run/user/1001/contentd/contentd.sock", 0)
	if err != nil {
		return err
	}

	file, err := p.GetFile(ctx, args[1])
	if err != nil {
		return err
	}

	dt, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", string(dt))
	return nil
}
