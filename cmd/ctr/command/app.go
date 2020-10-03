package command

import (
	cli "github.com/urfave/cli/v2"
)

func App() *cli.App {
	app := cli.NewApp()
	app.Name = "ctr"
	app.Usage = "cli for container management"

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "addr",
			Usage: "containerd address",
			Value: "./tmp/containerd/containerd.sock",
		},
	}

	app.Commands = []*cli.Command{
		runCommand,
		imagesCommand,
	}

	return app
}
