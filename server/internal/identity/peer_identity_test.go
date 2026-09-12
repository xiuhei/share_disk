package identity

import (
	"crypto/rand"
	"testing"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestValidatePeerPublicKey(t *testing.T) {
	privateKey, publicKey, err := libp2pcrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := peer.IDFromPrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := libp2pcrypto.MarshalPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePeerPublicKey(peerID.String(), encoded); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
	if err := validatePeerPublicKey("not-the-derived-peer", encoded); err == nil {
		t.Fatal("mismatched peer id was accepted")
	}
	if err := validatePeerPublicKey(peerID.String(), []byte("not-a-key")); err == nil {
		t.Fatal("invalid public key was accepted")
	}
}
