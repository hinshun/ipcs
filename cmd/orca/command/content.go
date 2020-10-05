package command

import (
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/pkg/errors"
	cli "github.com/urfave/cli/v2"
)

var contentCatCommand = &cli.Command{
	Name:      "cat",
	Usage:     "print the content to standard output",
	ArgsUsage: "<digest>",
	Flags:     []cli.Flag{},
	Action: func(c *cli.Context) error {
		return nil
	},
}

var contentListCommand = &cli.Command{
	Name:    "list",
	Aliases: []string{"ls"},
	Usage:   "List all content",
	Flags:   []cli.Flag{},
	Action: func(c *cli.Context) error {
		ctx := namespaces.WithNamespace(c.Context, "ipcs")

		cln, err := containerd.New(c.String("containerd-addr"))
		if err != nil {
			return errors.Wrap(err, "failed to create containerd client")
		}

		contents, err := cln.ImageService().List(ctx)
		if err != nil {
			return err
		}

		for _, content := range contents {
			fmt.Println(content.Name)
		}
		return nil
	},
}

var contentRemoveCommand = &cli.Command{
	Name:      "remove",
	Aliases:   []string{"rm"},
	Usage:     "Remove one or more content by digest",
	ArgsUsage: "<digest> [digest...]",
	Flags:     []cli.Flag{},
	Action: func(c *cli.Context) error {
		return nil
	},
}
