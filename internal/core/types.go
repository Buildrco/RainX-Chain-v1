package core

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	rcrypto "github.com/Buildrco/rainx-chain/internal/crypto"
)

const (
	ChainID                = "rainx-mainnet-1"
	Symbol                 = "RXC"
	Decimals        uint8  = 8
	Coin            uint64 = 100_000_000
	MaxSupply       uint64 = 21_000_000 * Coin
	InitialReward   uint64 = 50 * Coin
	BlockInterval   uint64 = 60
	HalvingInterval uint64 = 210_000
	DifficultyBits  uint8  = 18
	MaxBlockBytes          = 1_000_000
)

type OutPoint struct {
	TxID string `json:"txid"`
	Vout uint32 `json:"vout"`
}
type TxInput struct {
	PrevTxID  string `json:"prevTxid"`
	Vout      uint32 `json:"vout"`
	Signature string `json:"signature,omitempty"`
	PubKey    string `json:"pubkey,omitempty"`
}
type TxOutput struct {
	Value   uint64 `json:"value"`
	Address string `json:"address"`
}
type Transaction struct {
	Version   uint32     `json:"version"`
	Inputs    []TxInput  `json:"inputs"`
	Outputs   []TxOutput `json:"outputs"`
	Timestamp int64      `json:"timestamp"`
	Memo      string     `json:"memo,omitempty"`
}
type BlockHeader struct {
	Version    uint32 `json:"version"`
	ChainID    string `json:"chainId"`
	Height     uint64 `json:"height"`
	PrevHash   string `json:"prevHash"`
	MerkleRoot string `json:"merkleRoot"`
	Timestamp  int64  `json:"timestamp"`
	Bits       uint8  `json:"bits"`
	Nonce      uint64 `json:"nonce"`
}
type Block struct {
	Header       BlockHeader   `json:"header"`
	Transactions []Transaction `json:"transactions"`
}

func (tx Transaction) unsignedBytes() []byte {
	inputs := make([]TxInput, len(tx.Inputs))
	copy(inputs, tx.Inputs)
	for i := range inputs {
		inputs[i].Signature = ""
		inputs[i].PubKey = ""
	}
	b, _ := json.Marshal(struct {
		Version   uint32
		Inputs    []TxInput
		Outputs   []TxOutput
		Timestamp int64
		Memo      string
	}{tx.Version, inputs, tx.Outputs, tx.Timestamp, tx.Memo})
	return b
}
func (tx Transaction) ID() string { return rcrypto.HashHex(tx.unsignedBytes()) }
func (tx Transaction) SigningHash(inputIndex int) []byte {
	b := tx.unsignedBytes()
	h := rcrypto.Hash(b)
	return h[:]
}

func SignInput(tx *Transaction, i int, priv ed25519.PrivateKey) {
	tx.Inputs[i].Signature = hex.EncodeToString(ed25519.Sign(priv, tx.SigningHash(i)))
	tx.Inputs[i].PubKey = hex.EncodeToString(priv.Public().(ed25519.PublicKey))
}

func (tx Transaction) VerifyInput(i int) error {
	if i < 0 || i >= len(tx.Inputs) {
		return errors.New("input index out of range")
	}
	pub, err := hex.DecodeString(tx.Inputs[i].PubKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("bad public key")
	}
	sig, err := hex.DecodeString(tx.Inputs[i].Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("bad signature")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), tx.SigningHash(i), sig) {
		return errors.New("invalid signature")
	}
	return nil
}

func MerkleRoot(txs []Transaction) string {
	if len(txs) == 0 {
		return rcrypto.HashHex(nil)
	}
	hashes := make([][]byte, len(txs))
	for i, t := range txs {
		h := rcrypto.Hash([]byte(t.ID()))
		hashes[i] = h[:]
	}
	for len(hashes) > 1 {
		var next [][]byte
		for i := 0; i < len(hashes); i += 2 {
			right := hashes[i]
			if i+1 < len(hashes) {
				right = hashes[i+1]
			}
			x := append(append([]byte{}, hashes[i]...), right...)
			h := rcrypto.Hash(x)
			next = append(next, h[:])
		}
		hashes = next
	}
	return hex.EncodeToString(hashes[0])
}
func (b Block) Hash() string { raw, _ := json.Marshal(b.Header); return rcrypto.HashHex(raw) }
func (b Block) Work() uint64 { return uint64(1) << (64 - b.Header.Bits) }
func (b Block) ValidPoW() bool {
	h := rcrypto.Hash(mustJSON(b.Header))
	zeros := int(b.Header.Bits / 8)
	for i := 0; i < zeros; i++ {
		if h[i] != 0 {
			return false
		}
	}
	rem := int(b.Header.Bits % 8)
	if rem > 0 {
		if h[zeros]>>(8-rem) != 0 {
			return false
		}
	}
	return true
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func NewGenesis() Block {
	tx := Transaction{Version: 1, Timestamp: 1700000000, Memo: "RainX Network Genesis — independent native RXC"}
	return Block{Header: BlockHeader{Version: 1, ChainID: ChainID, Height: 0, PrevHash: "", MerkleRoot: MerkleRoot([]Transaction{tx}), Timestamp: 1700000000, Bits: DifficultyBits}, Transactions: []Transaction{tx}}
}
func BlockReward(height uint64) uint64 {
	if height == 0 {
		return 0
	}
	halvings := height / HalvingInterval
	if halvings >= 63 {
		return 0
	}
	reward := InitialReward >> halvings
	return reward
}

func NewCoinbase(height uint64, address string, reward uint64) Transaction {
	return Transaction{Version: 1, Timestamp: time.Now().Unix(), Inputs: nil, Outputs: []TxOutput{{Value: reward, Address: address}}, Memo: fmt.Sprintf("RainX block reward %d", height)}
}
func NewTransfer(outputs []TxOutput) Transaction {
	return Transaction{Version: 1, Timestamp: time.Now().Unix(), Outputs: outputs}
}

func ValidateBasicTransaction(tx Transaction) error {
	if tx.Version != 1 {
		return errors.New("unsupported transaction version")
	}
	if len(tx.Outputs) == 0 {
		return errors.New("transaction has no outputs")
	}
	var total uint64
	for _, o := range tx.Outputs {
		if o.Value == 0 {
			return errors.New("zero output")
		}
		if _, err := DecodeAddress(o.Address); err != nil {
			return err
		}
		if ^uint64(0)-total < o.Value {
			return errors.New("output overflow")
		}
		total += o.Value
	}
	if len(tx.Inputs) > 0 {
		seen := map[string]bool{}
		for _, in := range tx.Inputs {
			k := in.PrevTxID + fmt.Sprint(in.Vout)
			if seen[k] {
				return errors.New("duplicate input")
			}
			seen[k] = true
		}
	}
	return nil
}

func SortTransactions(txs []Transaction) {
	sort.Slice(txs, func(i, j int) bool { return txs[i].ID() < txs[j].ID() })
}
