package ipcs

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"time"

	bitswap "github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	blockservice "github.com/ipfs/go-blockservice"
	cid "github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	chunker "github.com/ipfs/go-ipfs-chunker"
	config "github.com/ipfs/go-ipfs-config"
	files "github.com/ipfs/go-ipfs-files"
	provider "github.com/ipfs/go-ipfs-provider"
	"github.com/ipfs/go-ipfs-provider/queue"
	"github.com/ipfs/go-ipfs-provider/simple"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	ipns "github.com/ipfs/go-ipns"
	merkledag "github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipfs/go-unixfs/importer/balanced"
	"github.com/ipfs/go-unixfs/importer/helpers"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	host "github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	noise "github.com/libp2p/go-libp2p-noise"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	quic "github.com/libp2p/go-libp2p-quic-transport"
	record "github.com/libp2p/go-libp2p-record"
	tls "github.com/libp2p/go-libp2p-tls"
	yamux "github.com/libp2p/go-libp2p-yamux"
	multihash "github.com/multiformats/go-multihash"
	"github.com/pkg/errors"
)

func init() {
	ipld.Register(cid.DagProtobuf, merkledag.DecodeProtobufBlock)
	ipld.Register(cid.DagCBOR, cbor.DecodeBlock) // need to decode CBOR
	ipld.Register(cid.Raw, merkledag.DecodeRawBlock)
}

var (
	ReprovideInterval = 12 * time.Hour
)

type Peer struct {
	host     host.Host
	dstore   datastore.Batching
	bstore   blockstore.Blockstore
	bserv    blockservice.BlockService
	routing  routing.ContentRouting
	provider provider.System
	bswap    *bitswap.Bitswap
	dserv    ipld.DAGService
}

func New(ctx context.Context, root string, port int) (*Peer, error) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate private key")
	}

	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return nil, err
	}

	pstore := pstoremem.NewPeerstore()
	err = pstore.AddPubKey(id, pub)
	if err != nil {
		return nil, err
	}

	err = pstore.AddPrivKey(id, priv)
	if err != nil {
		return nil, err
	}

	dstore, err := badger.NewDatastore(root, &badger.DefaultOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create datastore")
	}

	validator := record.NamespacedValidator{
		"pk":   record.PublicKeyValidator{},
		"ipns": ipns.Validator{KeyBook: pstore},
	}

	bootstrapPeers, err := config.DefaultBootstrapPeers()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bootstrap peers")
	}

	var r *dual.DHT
	newRouting := func(h host.Host) (routing.PeerRouting, error) {
		var err error
		r, err = dual.New(
			ctx, h,
			dual.DHTOption(
				dht.Concurrency(10),
				dht.Mode(dht.ModeAuto),
				dht.Datastore(dstore),
				dht.Validator(validator),
			),
			dual.WanDHTOption(dht.BootstrapPeers(bootstrapPeers...)),
		)
		return r, err
	}

	h, err := libp2p.New(
		ctx,
		libp2p.Identity(pstore.PrivKey(id)),
		libp2p.Peerstore(pstore),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Security(tls.ID, tls.New),
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
		libp2p.Transport(quic.NewTransport),
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic", port)),
		libp2p.Routing(newRouting),
	)

	bstore, err := NewBlockstore(ctx, dstore)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create blockstore")
	}

	bswapnet := network.NewFromIpfsHost(h, r)
	rem := bitswap.New(ctx, bswapnet, bstore)

	bswap, ok := rem.(*bitswap.Bitswap)
	if !ok {
		return nil, errors.New("expected to be able to cast exchange interface to bitswap")
	}

	bserv := blockservice.New(bstore, rem)

	provider, err := NewProviderSystem(ctx, dstore, bstore, r)
	if err != nil {
		bserv.Close()
		return nil, errors.Wrap(err, "failed to create provider provider")
	}

	go func() {
		provider.Run()

		select {
		case <-ctx.Done():
			err := provider.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to close provider: %q\n", err)
			}

			err = bserv.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to close block service: %q\n", err)
			}

			err = pstore.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to close peer store: %q\n", err)
			}
		}
	}()

	dserv := merkledag.NewDAGService(bserv)
	return &Peer{
		host:     h,
		dserv:    dserv,
		provider: provider,
		routing:  r,
		bswap:    bswap,
		bserv:    bserv,
		bstore:   bstore,
		dstore:   dstore,
	}, nil
}

func (p *Peer) Host() host.Host {
	return p.host
}

func (p *Peer) DAGService() ipld.DAGService {
	return p.dserv
}

func (p *Peer) Add(ctx context.Context, r io.Reader) (ipld.Node, error) {
	prefix, err := merkledag.PrefixForCidVersion(1)
	if err != nil {
		return nil, errors.Wrap(err, "unrecognized CID version")
	}

	hashName := "sha2-256"
	hashFuncCode, ok := multihash.Names[hashName]
	if !ok {
		return nil, errors.Wrapf(err, "unrecognized hash function %q", hashName)
	}
	prefix.MhType = hashFuncCode

	buf := ipld.NewBufferedDAG(ctx, p.dserv)
	dbp := helpers.DagBuilderParams{
		Dagserv:    buf,
		RawLeaves:  false,
		Maxlinks:   helpers.DefaultLinksPerBlock,
		CidBuilder: &prefix,
	}

	chnk, err := chunker.FromString(r, "size-262144")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create chunker")
	}

	dbh, err := dbp.New(chnk)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create dag builder")
	}

	nd, err := balanced.Layout(dbh)
	if err != nil {
		return nil, err
	}

	return nd, buf.Commit()
}

func (p *Peer) Get(ctx context.Context, c cid.Cid) (files.Node, error) {
	nd, err := p.dserv.Get(ctx, c)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get file %q", c)
	}

	return unixfile.NewUnixfsFile(ctx, p.dserv, nd)
}

func NewBlockstore(ctx context.Context, ds datastore.Batching) (blockstore.Blockstore, error) {
	bs := blockstore.NewBlockstore(ds)
	bs = blockstore.NewIdStore(bs)

	var err error
	bs, err = blockstore.CachedBlockstore(ctx, bs, blockstore.DefaultCacheOpts())
	if err != nil {
		return nil, err
	}
	return bs, nil
}

func NewProviderSystem(ctx context.Context, ds datastore.Batching, bs blockstore.Blockstore, r routing.ContentRouting) (provider.System, error) {
	queue, err := queue.NewQueue(ctx, "repro", ds)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new queue")
	}

	prov := simple.NewProvider(ctx, queue, r)
	reprov := simple.NewReprovider(ctx, ReprovideInterval, r, simple.NewBlockstoreProvider(bs))
	system := provider.NewSystem(prov, reprov)
	return system, nil
}
