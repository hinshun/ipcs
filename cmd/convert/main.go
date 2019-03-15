package main

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/reference"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/hinshun/image2ipfs"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	"github.com/moby/buildkit/util/contentutil"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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

	ipfsCln, err := httpapi.NewLocalApi()
	if err != nil {
		return errors.Wrap(err, "failed to create ipfs client")
	}

	ctrdCln, err := containerd.New("./tmp/containerd/containerd.sock")
	if err != nil {
		return errors.Wrap(err, "failed to create containerd client")
	}

	_, mfstDesc, err := image2ipfs.Convert(ctx, ipfsCln, contentutil.FromFetcher(fetcher), ctrdCln.ContentStore(), srcDesc)
	if err != nil {
		return errors.Wrapf(err, "failed to convert %q to ipfs manifest", srcName)
	}
	log.Printf("Converted %q manifest from %q to %q", srcName, srcDesc.Digest, mfstDesc.Digest)

	dstImg := images.Image{
		Name:   dst,
		Target: mfstDesc,
	}

	dstImg, err = createImage(ctx, ctrdCln, dstImg)
	if err != nil {
		return errors.Wrapf(err, "failed to create image %q", dstImg.Name)
	}
	log.Printf("Successfully created image %q", dstImg.Name)

	// wd, err := os.Getwd()
	// if err != nil {
	// 	return errors.Wrap(err, "failed to get workdir")
	// }

	// ipcsStore, err := ipcs.NewContentStore(ipcs.Config{
	// 	RootDir: filepath.Join(wd, "tmp/containerd/root/io.containerd.content.v1.ipcs"),
	// })
	// if err != nil {
	// 	return errors.Wrap(err, "failed to create ipcs")
	// }

	// i, err := ipcsStore.Info(ctx, digest.Digest("sha256:2886587d8dd7f006c855ecd597a1b551d1e590598c102884d013e1c3e069fa1e"))
	// if err != nil {
	// 	return errors.Wrap(err, "failed to get info")
	// }
	// log.Printf("Info:\n%v", i)

	// p, err := images.Platforms(ctx, ctrdCln.ContentStore(), dstImg.Target)
	// if err != nil {
	// 	return errors.Wrap(err, "unable to resolve image platforms")
	// }

	// if len(p) == 0 {
	// 	p = append(p, platforms.DefaultSpec())
	// }

	log.Printf("Reading blob: %s", dstImg.Target.Digest)
	p, err := content.ReadBlob(ctx, ctrdCln.ContentStore(), dstImg.Target)
	if err != nil {
		return errors.Wrapf(err, "failed to read blob %q", dstImg.Target.Digest)
	}
	log.Printf("Read blob:\n%s", p)

	// p := []ocispec.Platform{platforms.DefaultSpec()}

	// for _, platform := range p {
	//         log.Printf("Unpacking %q %q...\n", platforms.Format(platform), dstImg.Target.Digest)
	//         i := containerd.NewImageWithPlatform(ctrdCln, dstImg, platforms.Only(platform))
	//         err = i.Unpack(ctx, containerd.DefaultSnapshotter)
	//         if err != nil {
	//                 return errors.Wrap(err, "failed to unpack image")
	//         }
	// }

	// err = ctrdCln.Push(ctx, dst, mfstDesc)
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to push %q", dstImg.Name)
	// }
	// log.Printf("Pushed '%s@%s'", dstImg.Name, dstImg.Target.Digest)

	// err = pushTag(ctx, http.DefaultClient, bytes.NewReader(mfstJSON), dstImg.Name, dstImg.Target)
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to push tag %q", dstImg.Name)
	// }

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
		return errors.Wrap(err, "failed to create http request")
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
