package command

import (
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/pkg/errors"
	cli "github.com/urfave/cli/v2"
)

var imagesCommand = &cli.Command{
	Name:      "images",
	Usage:     "List images",
	ArgsUsage: "",
	Flags:     []cli.Flag{},
	Action: func(c *cli.Context) error {
		ctx := namespaces.WithNamespace(c.Context, "ipcs")

		cln, err := containerd.New(c.String("containerd-addr"))
		if err != nil {
			return errors.Wrap(err, "failed to create containerd client")
		}

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
