package command

import (
	"fmt"

	cli "github.com/urfave/cli/v2"
)

var imageListCommand = &cli.Command{
	Name:    "list",
	Aliases: []string{"ls"},
	Usage:   "List images",
	Flags:   []cli.Flag{},
	Action: func(c *cli.Context) error {
		cln, ctx, cancel, err := NewClient(c)
		if err != nil {
			return err
		}
		defer cancel()

		images, err := cln.ImageService().List(ctx)
		if err != nil {
			return err
		}

		for _, image := range images {
			fmt.Println(image.Name)
		}
		return nil
	},
}

var imagePullCommand = &cli.Command{
	Name:      "pull",
	Usage:     "Pull an image from a registry.",
	ArgsUsage: "<name>",
	Flags:     []cli.Flag{},
	Action: func(c *cli.Context) error {
		return nil
	},
}

var imageRemoveCommand = &cli.Command{
	Name:      "remove",
	Aliases:   []string{"rm"},
	Usage:     "Remove one or more images",
	ArgsUsage: "<image> [image...]",
	Flags:     []cli.Flag{},
	Action: func(c *cli.Context) error {
		return nil
	},
}
