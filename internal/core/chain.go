package core

import (
	"errors"
	"fmt"
	"sort"
)

type UTXO struct {
	OutPoint OutPoint
	Output   TxOutput
}
type Chain struct{ Blocks []Block }

func NewChain(blocks []Block) *Chain { return &Chain{Blocks: blocks} }
func (c *Chain) Tip() Block          { return c.Blocks[len(c.Blocks)-1] }
func (c *Chain) Height() uint64      { return c.Tip().Header.Height }
func (c *Chain) TotalWork() uint64 {
	var w uint64
	for _, b := range c.Blocks {
		w += b.Work()
	}
	return w
}
func (c *Chain) UTXOs() map[string]TxOutput {
	u := map[string]TxOutput{}
	for _, b := range c.Blocks {
		for _, tx := range b.Transactions {
			for _, in := range tx.Inputs {
				delete(u, fmt.Sprintf("%s:%d", in.PrevTxID, in.Vout))
			}
			id := tx.ID()
			for i, o := range tx.Outputs {
				u[fmt.Sprintf("%s:%d", id, i)] = o
			}
		}
	}
	return u
}
func (c *Chain) Balance(addr string) uint64 {
	var n uint64
	for _, o := range c.UTXOs() {
		if o.Address == addr {
			n += o.Value
		}
	}
	return n
}
func (c *Chain) ValidateBlock(b Block) error {
	if b.Header.ChainID != ChainID {
		return errors.New("wrong chain id")
	}
	if b.Header.Height == 0 {
		return errors.New("genesis cannot be appended")
	}
	if len(b.Transactions) == 0 {
		return errors.New("empty block")
	}
	if b.Header.Height != c.Height()+1 {
		return errors.New("wrong height")
	}
	if b.Header.PrevHash != c.Tip().Hash() {
		return errors.New("wrong previous hash")
	}
	if b.Header.MerkleRoot != MerkleRoot(b.Transactions) {
		return errors.New("bad merkle root")
	}
	if !b.ValidPoW() {
		return errors.New("invalid proof of work")
	}
	if b.Header.Timestamp < c.Tip().Header.Timestamp {
		return errors.New("timestamp before parent")
	}
	if len(b.Transactions[0].Inputs) != 0 {
		return errors.New("coinbase has inputs")
	}
	var coinbase uint64
	for _, o := range b.Transactions[0].Outputs {
		coinbase += o.Value
	}
	if coinbase > BlockReward(b.Header.Height) {
		return errors.New("coinbase exceeds block reward")
	}
	return nil
}
func (c *Chain) ApplyBlock(b Block) error {
	if err := c.ValidateBlock(b); err != nil {
		return err
	}
	utxo := c.UTXOs()
	spent := map[string]bool{}
	for ti, tx := range b.Transactions {
		if err := ValidateBasicTransaction(tx); err != nil {
			return err
		}
		if ti == 0 {
			continue
		}
		var inSum uint64
		for i, in := range tx.Inputs {
			k := fmt.Sprintf("%s:%d", in.PrevTxID, in.Vout)
			if spent[k] {
				return errors.New("double spend in block")
			}
			o, ok := utxo[k]
			if !ok {
				return errors.New("missing input")
			}
			if err := tx.VerifyInput(i); err != nil {
				return err
			}
			pubAddr := AddressFromPubKey(mustDecode(tx.Inputs[i].PubKey))
			if pubAddr != o.Address {
				return errors.New("input signer does not own output")
			}
			spent[k] = true
			inSum += o.Value
		}
		var outSum uint64
		for _, o := range tx.Outputs {
			outSum += o.Value
		}
		if outSum > inSum {
			return errors.New("outputs exceed inputs")
		}
	}
	c.Blocks = append(c.Blocks, b)
	return nil
}
func mustDecode(s string) []byte { b, _ := hexDecode(s); return b }
func hexDecode(s string) ([]byte, error) {
	var out []byte
	for i := 0; i < len(s); i += 2 {
		var v byte
		_, err := fmt.Sscanf(s[i:i+2], "%02x", &v)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (c *Chain) ValidateMempoolTx(tx Transaction) error {
	if len(tx.Inputs) == 0 {
		return errors.New("non-coinbase transaction requires inputs")
	}
	if err := ValidateBasicTransaction(tx); err != nil {
		return err
	}
	utxo := c.UTXOs()
	var inSum, outSum uint64
	seen := map[string]bool{}
	for i, in := range tx.Inputs {
		k := fmt.Sprintf("%s:%d", in.PrevTxID, in.Vout)
		if seen[k] {
			return errors.New("duplicate input")
		}
		seen[k] = true
		o, ok := utxo[k]
		if !ok {
			return errors.New("input is not unspent")
		}
		if err := tx.VerifyInput(i); err != nil {
			return err
		}
		pub, err := hexDecode(tx.Inputs[i].PubKey)
		if err != nil {
			return err
		}
		if AddressFromPubKey(pub) != o.Address {
			return errors.New("input signer does not own output")
		}
		inSum += o.Value
	}
	for _, o := range tx.Outputs {
		outSum += o.Value
	}
	if outSum > inSum {
		return errors.New("outputs exceed inputs")
	}
	return nil
}

func SelectUTXOs(c *Chain, addr string, need uint64) ([]OutPoint, uint64, error) {
	type pair struct {
		k string
		o TxOutput
	}
	var p []pair
	for k, o := range c.UTXOs() {
		if o.Address == addr {
			p = append(p, pair{k, o})
		}
	}
	sort.Slice(p, func(i, j int) bool { return p[i].o.Value < p[j].o.Value })
	var total uint64
	var ops []OutPoint
	for _, x := range p {
		var txid string
		var v uint32
		fmt.Sscanf(x.k, "%s:%d", &txid, &v)
		ops = append(ops, OutPoint{TxID: txid, Vout: v})
		total += x.o.Value
		if total >= need {
			return ops, total, nil
		}
	}
	return nil, 0, errors.New("insufficient balance")
}
