package ipcs

import (
	"context"
	"crypto/rand"
	"fmt"

	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/ipfs/interface-go-ipfs-core/options"
)

type keystoreServer struct {
	p *Peer
}

func (p *Peer) Keystore() KeystoreServer {
	return &keystoreServer{p}
}

func (k *keystoreServer) Add(ctx context.Context, req *AddRequest) (*AddResponse, error) {
	return &AddResponse{}, nil
}

func (k *keystoreServer) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	name := req.Name
	_, err := k.p.kstore.Get(name)
	if err == nil {
		return nil, fmt.Errorf("key with name '%s' already exists", name)
	}

	var sk crypto.PrivKey
	var pk crypto.PubKey

	switch req.Type {
	case KeyType_RSA:
		size := int(req.Size_)
		if size == 0 {
			size = options.DefaultRSALen
		}

		priv, pub, err := crypto.GenerateKeyPairWithReader(crypto.RSA, size, rand.Reader)
		if err != nil {
			return nil, err
		}

		sk = priv
		pk = pub
	case KeyType_Ed25519:
		priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, err
		}

		sk = priv
		pk = pub
	default:
		return nil, fmt.Errorf("unrecognized key type: %s", req.Type)
	}

	err = k.p.kstore.Put(name, sk)
	if err != nil {
		return nil, err
	}

	data, err := crypto.MarshalPublicKey(pk)
	if err != nil {
		return nil, err
	}

	return &GenerateResponse{
		Key: Key{
			Name: name,
			PublicKey: PublicKey{
				Type: req.Type,
				Data: data,
			},
		},
	}, nil
}

func (k *keystoreServer) List(ctx context.Context, req *ListRequest) (*ListResponse, error) {
	names, err := k.p.kstore.List()
	if err != nil {
		return nil, err
	}

	var keys []*Key
	for _, name := range names {
		sk, err := k.p.kstore.Get(name)
		if err != nil {
			return nil, err
		}

		pk := sk.GetPublic()
		data, err := crypto.MarshalPublicKey(pk)
		if err != nil {
			return nil, err
		}

		keys = append(keys, &Key{
			Name: name,
			PublicKey: PublicKey{
				Type: KeyType(pk.Type()),
				Data: data,
			},
		})
	}

	return &ListResponse{Keys: keys}, nil
}

func (k *keystoreServer) Remove(ctx context.Context, req *RemoveRequest) (*RemoveResponse, error) {
	var deleted []string
	for _, name := range req.Names {
		err := k.p.kstore.Delete(name)
		if err != nil {
			return &RemoveResponse{Names: deleted}, err
		}
		deleted = append(deleted, name)
	}
	return &RemoveResponse{Names: deleted}, nil
}

func (k *keystoreServer) Rename(ctx context.Context, req *RenameRequest) (*RenameResponse, error) {
	oldKey, err := k.p.kstore.Get(req.OldName)
	if err != nil {
		return nil, fmt.Errorf("no key named %s was found", req.OldName)
	}

	if req.NewName == req.OldName {
		return &RenameResponse{Name: req.NewName}, nil
	}

	err = k.p.kstore.Put(req.NewName, oldKey)
	if err != nil {
		return nil, err
	}

	return &RenameResponse{Name: req.NewName}, k.p.kstore.Delete(req.OldName)
}
