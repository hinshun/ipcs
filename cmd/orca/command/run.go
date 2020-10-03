package command

import (
	"fmt"
	"log"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/docker/distribution/reference"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	cli "github.com/urfave/cli/v2"
)

var runCommand = &cli.Command{
	Name:      "run",
	Usage:     "Run a command in a new container",
	ArgsUsage: "<image> <command> <arg...>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "rm",
			Usage: "automatically remove the container when it exits",
		},
		&cli.BoolFlag{
			Name:    "tty",
			Aliases: []string{"t"},
			Usage:   "allocate a tty",
		},
		&cli.BoolFlag{
			Name:  "detach",
			Usage: "run container in background and print container ID",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() < 1 {
			return errors.Errorf("must specify an image")
		}

		var args []string
		if c.NArg() > 1 {
			args = c.Args().Slice()[1:]
		}

		ctx := namespaces.WithNamespace(c.Context, "ipcs")

		cln, err := containerd.New(c.String("addr"))
		if err != nil {
			return errors.Wrap(err, "failed to create containerd client")
		}

		ref := c.Args().First()
		named, err := reference.ParseNormalizedNamed(ref)
		if err != nil {
			return errors.Wrapf(err, "cannot parse %q", ref)
		}
		ref = reference.TagNameOnly(named).String()

		image, err := cln.GetImage(ctx, ref)
		if err != nil {
			return errors.Wrap(err, "failed to get image")
		}

		var (
			opts  []oci.SpecOpts
			cOpts []containerd.NewContainerOpts
			s     specs.Spec
		)

		id := "world"

		opts = append(opts,
			oci.WithDefaultSpec(),
			oci.WithDefaultUnixDevices,
			oci.WithImageConfigArgs(image, args),
			oci.WithCgroup(""),
		)
		if c.Bool("tty") {
			opts = append(opts, oci.WithTTY)
		}

		cOpts = append(cOpts,
			containerd.WithImage(image),
			containerd.WithSnapshotter(containerd.DefaultSnapshotter),
			containerd.WithNewSnapshot(id, image),
			containerd.WithImageStopSignal(image, "SIGTERM"),
			containerd.WithSpec(&s, opts...),
		)

		container, err := cln.NewContainer(ctx, id, cOpts...)
		if err != nil {
			return errors.Wrap(err, "failed to create container")
		}

		if c.Bool("rm") {
			defer container.Delete(ctx)
		}

		var con console.Console
		if c.Bool("tty") {
			con = console.Current()
			defer con.Reset()

			err = con.SetRaw()
			if err != nil {
				return err
			}
		}

		var (
			ioOpts = []cio.Opt{cio.WithFIFODir("/tmp/fifo-dir")}
		)

		task, err := tasks.NewTask(ctx, cln, container, "", con, false, "", ioOpts)
		if err != nil {
			return errors.Wrap(err, "failed to create task")
		}

		var statusC <-chan containerd.ExitStatus
		if !c.Bool("detach") {
			defer task.Delete(ctx)
			statusC, err = task.Wait(ctx)
			if err != nil {
				return err
			}
		}

		err = task.Start(ctx)
		if err != nil {
			return err
		}
		if c.Bool("detach") {
			fmt.Println(id)
			return nil
		}

		if c.Bool("tty") {
			err = tasks.HandleConsoleResize(ctx, task, con)
			if err != nil {
				log.Printf("console resize err: %s", err)
			}
		} else {
			sigc := commands.ForwardAllSignals(ctx, task)
			defer commands.StopCatch(sigc)
		}

		status := <-statusC
		code, _, err := status.Result()
		if err != nil {
			return err
		}

		_, err = task.Delete(ctx)
		if err != nil {
			return err
		}
		if code != 0 {
			return cli.NewExitError("", int(code))
		}
		return nil
	},
}
