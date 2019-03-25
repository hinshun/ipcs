package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/reference"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/hinshun/ipcs"
	"github.com/hinshun/ipcs/digestconv"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/moby/buildkit/util/contentutil"
	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"golang.org/x/net/context/ctxhttp"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("convert: requires exactly 2 args")
	}

	ctx := namespaces.WithNamespace(context.Background(), "ipfs")
	err := run(ctx, os.Args[1], os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, src, dst string) error {
	ipfsCln, err := httpapi.NewLocalApi()
	if err != nil {
		return errors.Wrap(err, "failed to create ipfs client")
	}

	ctrdCln, err := containerd.New("./tmp/containerd/containerd.sock")
	if err != nil {
		return errors.Wrap(err, "failed to create containerd client")
	}

	err = Convert(ctx, ipfsCln, ctrdCln, src, dst)
	if err != nil {
		return errors.Wrap(err, "failed to convert to p2p manifest")
	}

	err = RunContainer(ctx, ctrdCln, dst, "helloworld")
	if err != nil {
		return errors.Wrap(err, "failed to run container")
	}

	return nil
}

func Convert(ctx context.Context, ipfsCln iface.CoreAPI, ctrdCln *containerd.Client, src, dst string) error {
	resolver := docker.NewResolver(docker.ResolverOptions{
		Client: http.DefaultClient,
	})

	srcName, srcDesc, err := resolver.Resolve(ctx, src)
	if err != nil {
		return errors.Wrapf(err, "failed to resolve %q", src)
	}
	log.Printf("Resolved %q as \"%s@%s\"", src, srcName, srcDesc.Digest)

	fetcher, err := resolver.Fetcher(ctx, src)
	if err != nil {
		return errors.Wrapf(err, "failed to create fetcher for %q", src)
	}

	converter := ipcs.NewConverter(ipfsCln, contentutil.FromFetcher(fetcher))
	mfstDesc, err := converter.Convert(ctx, srcDesc)
	if err != nil {
		return errors.Wrapf(err, "failed to convert %q to ipfs manifest", srcName)
	}

	err = PullByDescriptor(ctx, ipfsCln, ctrdCln, dst, mfstDesc)
	if err != nil {
		return errors.Wrapf(err, "failed to pull descriptor %q", mfstDesc.Digest)
	}

	return nil
}

func RunContainer(ctx context.Context, cln *containerd.Client, ref, id string) error {
	image, err := cln.GetImage(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "failed to get image")
	}
	log.Printf("Successfully get image %q", image.Name())

	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
		s     specs.Spec
	)

	opts = append(opts,
		oci.WithTTY,
		oci.WithRootFSReadonly(),
		oci.WithProcessCwd("/"),
		oci.WithProcessArgs("/bin/sh"),
	)
	cOpts = append(cOpts,
		containerd.WithImage(image),
		containerd.WithSnapshotter("native"),
		// Even when "readonly" is set, we don't use KindView snapshot here. (#1495)
		// We pass writable snapshot to the OCI runtime, and the runtime remounts it as read-only,
		// after creating some mount points on demand.
		containerd.WithNewSnapshot(id, image),
		containerd.WithImageStopSignal(image, "SIGTERM"),
		containerd.WithSpec(&s, opts...),
	)

	container, err := cln.NewContainer(ctx, id, cOpts...)
	if err != nil {
		return errors.Wrap(err, "failed to create container")
	}
	log.Printf("Successfully create container %q", container.ID())

	return nil
}

func Pull(ctx context.Context, ipfsCln iface.CoreAPI, ctrdCln *containerd.Client, ref string) error {
	// resolver := docker.NewResolver(docker.ResolverOptions{
	//         Client: http.DefaultClient,
	// })

	// name, desc, err := resolver.Resolve(ctx, ref)
	// if err != nil {
	//         return errors.Wrapf(err, "failed to resolve %q", ref)
	// }

	name := ref
	desc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.Digest("sha256:c0ecc9c9fb27ebc68af1014d2f1962f1533fe606f2ed5963c61d066976ce8d5a"),
		Size:      456,
	}

	return PullByDescriptor(ctx, ipfsCln, ctrdCln, name, desc)
}

func PullByDescriptor(ctx context.Context, ipfsCln iface.CoreAPI, ctrdCln *containerd.Client, name string, desc ocispec.Descriptor) error {
	ingester := ctrdCln.ContentStore()
	provider, err := ipcs.NewContentStore(ipcs.Config{
		IpfsPath: "./tmp/ipfs",
	})
	if err != nil {
		return errors.Wrap(err, "failed to create ipcs")
	}

	mfst, err := images.Manifest(ctx, provider, desc, platforms.Default())
	if err != nil {
		return errors.Wrap(err, "failed to get manifest")
	}

	mfstJSON, err := json.MarshalIndent(mfst, "", "   ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal manifest JSON")
	}
	log.Printf("Pulled Manifest [%d]:\n%s", len(mfstJSON), mfstJSON)

	err = contentutil.Copy(ctx, ingester, provider, mfst.Config)
	if err != nil {
		return errors.Wrapf(err, "failed to ingest manifest config blob %q", mfst.Config.Digest)
	}

	for _, layer := range mfst.Layers {
		err = contentutil.Copy(ctx, ingester, provider, layer)
		if err != nil {
			return errors.Wrapf(err, "failed to ingest blob %q", layer.Digest)
		}
	}

	err = contentutil.Copy(ctx, ingester, provider, desc)
	if err != nil {
		return errors.Wrapf(err, "failed to ingest manifest blob %q", desc.Digest)
	}

	image := images.Image{
		Name:   name,
		Target: desc,
	}

	image, err = createImage(ctx, ctrdCln, image)
	if err != nil {
		return errors.Wrapf(err, "failed to create image %q", image.Name)
	}

	c, err := digestconv.DigestToCid(image.Target.Digest)
	if err != nil {
		return errors.Wrapf(err, "failed to convert image digest %q to cid", image.Target.Digest)
	}
	log.Printf("Successfully created image %q (%s)", image.Target.Digest, c)

	p := []ocispec.Platform{platforms.DefaultSpec()}
	for _, platform := range p {
		log.Printf("Unpacking %q %q...\n", platforms.Format(platform), image.Target.Digest)
		i := containerd.NewImageWithPlatform(ctrdCln, image, platforms.Only(platform))
		err = i.Unpack(ctx, "native")
		if err != nil {
			return errors.Wrap(err, "failed to unpack image")
		}
	}

	return nil
}

func createImage(ctx context.Context, cln *containerd.Client, img images.Image) (images.Image, error) {
	is := cln.ImageService()
	for {
		if created, err := is.Create(ctx, img); err != nil {
			if !errdefs.IsAlreadyExists(err) {
				return images.Image{}, err
			}

			updated, err := is.Update(ctx, img)
			if err != nil {
				// if image was removed, try create again
				if errdefs.IsNotFound(err) {
					continue
				}
				return images.Image{}, err
			}

			img = updated
		} else {
			img = created
		}

		return img, nil
	}
}

func pushTag(ctx context.Context, cln *http.Client, r io.Reader, ref string, desc ocispec.Descriptor) error {
	refspec, err := reference.Parse(ref)
	if err != nil {
		return errors.Wrapf(err, "failed to parse reference %q", ref)
	}

	u := url.URL{
		Host:   refspec.Hostname(),
		Scheme: "https",
	}
	if strings.HasPrefix(u.Host, "localhost:") {
		u.Scheme = "http"
	}
	prefix := strings.TrimPrefix(refspec.Locator, u.Host+"/")
	u.Path = path.Join("/v2", prefix, "manifests", refspec.Object)

	req, err := http.NewRequest(http.MethodPut, u.String(), nil)
	if err != nil {
		return errors.Wrap(err, "failed to create http requestname, ")
	}

	req.Header.Add("Content-Type", desc.MediaType)
	req.Body = ioutil.NopCloser(r)
	req.ContentLength = desc.Size

	resp, err := ctxhttp.Do(ctx, cln, req)
	if err != nil {
		return errors.Wrap(err, "failed to do request")
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	default:
		return errors.Wrapf(err, "failed to do request with status %q", resp.Status)
	}
}
