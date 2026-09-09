package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestAddressRoundTrip(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	a := AddressFromPubKey(pub)
	if _, e := DecodeAddress(a); e != nil {
		t.Fatal(e)
	}
}
func TestTransactionSignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	_ = pub
	tx := NewTransfer([]TxOutput{{Value: 100, Address: AddressFromPubKey(pub)}})
	tx.Inputs = []TxInput{{PrevTxID: "abc", Vout: 0}}
	SignInput(&tx, 0, priv)
	if e := tx.VerifyInput(0); e != nil {
		t.Fatal(e)
	}
}
func TestPoW(t *testing.T) {
	g := NewGenesis()
	tx := NewCoinbase(1, AddressFromPubKey(make([]byte, 32)), InitialReward)
	b := Block{Header: BlockHeader{Version: 1, ChainID: ChainID, Height: 1, PrevHash: g.Hash(), MerkleRoot: MerkleRoot([]Transaction{tx}), Timestamp: g.Header.Timestamp + 1, Bits: 8}, Transactions: []Transaction{tx}}
	for !b.ValidPoW() {
		b.Header.Nonce++
	}
	if !b.ValidPoW() {
		t.Fatal("pow")
	}
}

func TestBlockRewardHalving(t *testing.T) {
	if got := BlockReward(1); got != InitialReward {
		t.Fatalf("height 1 reward = %d", got)
	}
	if got := BlockReward(HalvingInterval); got != InitialReward/2 {
		t.Fatalf("first halving = %d", got)
	}
	if got := BlockReward(HalvingInterval * 2); got != InitialReward/4 {
		t.Fatalf("second halving = %d", got)
	}
}
