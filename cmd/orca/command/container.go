package command

import (
	"context"
	"fmt"
	"log"
	"syscall"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	"github.com/hinshun/ipcs"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	cli "github.com/urfave/cli/v2"
)

var containerExecCommand = &cli.Command{
	Name:      "exec",
	Usage:     "Run a command in a running container",
	ArgsUsage: "<container> <command> [arg...]",
	Flags:     []cli.Flag{},
	Action: func(c *cli.Context) error {
		return nil
	},
}

var containerLogsCommand = &cli.Command{
	Name:      "logs",
	Usage:     "Fetch the logs of a container",
	ArgsUsage: "<container>",
	Flags:     []cli.Flag{},
	Action: func(c *cli.Context) error {
		return nil
	},
}

var containerListCommand = &cli.Command{
	Name:      "list",
	Usage:     "List containers",
	Aliases:   []string{"ls"},
	ArgsUsage: "<container>",
	Flags:     []cli.Flag{},
	Action: func(c *cli.Context) error {
		cln, ctx, cancel, err := NewClient(c)
		if err != nil {
			return err
		}
		defer cancel()

		containers, err := cln.ContainerService().List(ctx)
		if err != nil {
			return err
		}

		for _, container := range containers {
			fmt.Println(container.ID)
		}
		return nil
	},
}

var containerRemoveCommand = &cli.Command{
	Name:      "remove",
	Usage:     "Remove one or more containers",
	Aliases:   []string{"rm"},
	ArgsUsage: "<container> [container...]",
	Flags:     []cli.Flag{},
	Action: func(c *cli.Context) error {
		if c.NArg() < 1 {
			return nil
		}

		cln, ctx, cancel, err := NewClient(c)
		if err != nil {
			return err
		}
		defer cancel()

		for _, id := range c.Args().Slice() {
			container, err := cln.LoadContainer(ctx, id)
			if err != nil {
				return err
			}

			task, err := container.Task(ctx, cio.Load)
			if err != nil {
				return err
			}

			err = task.Kill(ctx, syscall.SIGKILL, containerd.WithKillAll)
			if err != nil {
				return err
			}

			fmt.Printf("Deleting task with pid %d\n", task.Pid())
			_, err = task.Delete(ctx)
			if err != nil {
				return err
			}

			fmt.Printf("Deleting container %s\n", id)
			err = container.Delete(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("Removed container %q\n", id)
		}

		return nil
	},
}

var containerRunCommand = &cli.Command{
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
			Name:    "detach",
			Aliases: []string{"d"},
			Usage:   "run container in background and print container ID",
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

		cln, ctx, cancel, err := NewClient(c)
		if err != nil {
			return err
		}
		defer cancel()

		ref := c.Args().First()
		img, err := cln.GetImage(ctx, ref)
		if err != nil {
			if !errdefs.IsNotFound(err) {
				return errors.Wrap(err, "failed to get image")
			}

			contentdCln, err := ipcs.NewClient(c.String("contentd-address"))
			if err != nil {
				return errors.Wrap(err, "failed to create contentd client")
			}

			image, err := PullImage(ctx, contentdCln, cln, ref)
			if err != nil {
				return err
			}

			img = containerd.NewImageWithPlatform(cln, image, platforms.Default())
		}

		snapshotter := containerd.DefaultSnapshotter
		unpacked, err := img.IsUnpacked(ctx, snapshotter)
		if err != nil {
			return err
		}
		if !unpacked {
			if err := img.Unpack(ctx, snapshotter); err != nil {
				return err
			}
		}

		var (
			opts  []oci.SpecOpts
			cOpts []containerd.NewContainerOpts
			s     specs.Spec
		)

		id := "josh"

		opts = append(opts,
			oci.WithDefaultSpec(),
			oci.WithDefaultUnixDevices,
			oci.WithImageConfigArgs(img, args),
			oci.WithCgroup(""),
		)
		if c.Bool("tty") {
			opts = append(opts, oci.WithTTY)
		}

		cOpts = append(cOpts,
			containerd.WithImage(img),
			containerd.WithSnapshotter(snapshotter),
			containerd.WithNewSnapshot(id, img),
			containerd.WithImageStopSignal(img, "SIGTERM"),
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

func PullImage(ctx context.Context, contentdCln *ipcs.Client, containerdCln *containerd.Client, ref string) (img images.Image, err error) {
	var (
		store    = containerdCln.ContentStore()
		resolver = contentdCln.Resolver()
	)

	name, desc, err := resolver.Resolve(ctx, ref)
	if err != nil {
		return
	}

	fetcher, err := resolver.Fetcher(ctx, name)
	if err != nil {
		return
	}

	// Get all the children for a descriptor
	childrenHandler := images.ChildrenHandler(store)

	// Set any children labels for that content
	childrenHandler = images.SetChildrenMappedLabels(store, childrenHandler, nil)

	// Filter children by platforms if specified.
	childrenHandler = images.FilterPlatforms(childrenHandler, platforms.Default())

	handlers := []images.Handler{
		remotes.FetchHandler(store, fetcher),
		childrenHandler,
	}

	handler := images.Handlers(handlers...)
	err = images.Dispatch(ctx, handler, nil, desc)
	if err != nil {
		return
	}

	img = images.Image{
		Name:   name,
		Target: desc,
	}

	is := containerdCln.ImageService()
	for {
		created, err := is.Create(ctx, img)
		if err != nil {
			if !errdefs.IsAlreadyExists(err) {
				return img, err
			}

			updated, err := is.Update(ctx, img)
			if err != nil {
				// if image was removed, try create again
				if errdefs.IsNotFound(err) {
					continue
				}
				return img, err
			}

			return updated, nil
		}
		return created, nil
	}
}
