package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Buildrco/rainx-chain/internal/core"
	"github.com/Buildrco/rainx-chain/internal/p2p"
	"github.com/Buildrco/rainx-chain/internal/rpc"
	"github.com/Buildrco/rainx-chain/internal/storage"
	"github.com/Buildrco/rainx-chain/internal/wallet"
)

func load(dir string) (*storage.Store, *core.Chain, []core.Transaction, error) {
	st, e := storage.Open(dir)
	if e != nil {
		return nil, nil, nil, e
	}
	bs, e := st.LoadBlocks()
	if e != nil {
		return nil, nil, nil, e
	}
	c := core.NewChain(bs)
	if len(bs) == 0 || bs[0].Header.ChainID != "" && bs[0].Header.ChainID != core.ChainID {
		return nil, nil, nil, errors.New("invalid chain database")
	}
	mp, e := st.LoadMempool()
	return st, c, mp, e
}

func mineBlock(c *core.Chain, mp []core.Transaction, address string) core.Block {
	txs := []core.Transaction{core.NewCoinbase(c.Height()+1, address, core.InitialReward)}
	txs = append(txs, mp...)
	b := core.Block{Header: core.BlockHeader{Version: 1, ChainID: core.ChainID, Height: c.Height() + 1, PrevHash: c.Tip().Hash(), MerkleRoot: core.MerkleRoot(txs), Timestamp: time.Now().Unix(), Bits: core.DifficultyBits}, Transactions: txs}
	for {
		if b.ValidPoW() {
			return b
		}
		b.Header.Nonce++
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "node":
		node(os.Args[2:])
	case "wallet":
		walletCmd(os.Args[2:])
	case "mine":
		mine(os.Args[2:])
	case "chain":
		chain(os.Args[2:])
	case "send":
		send(os.Args[2:])
	default:
		usage()
	}
}
func usage() {
	fmt.Println(`RainX Chain CLI

  rainxd node --data ./data
  rainxd wallet create --data ./data --name main --password PASSWORD
  rainxd wallet address --data ./data --name main --password PASSWORD
  rainxd mine --data ./data --rpc http://127.0.0.1:27778 --address RXC_ADDRESS
  rainxd chain --data ./data
  rainxd send --data ./data --name main --password PASSWORD --to RXC_ADDRESS --amount 1.25 --rpc http://127.0.0.1:27778`)
}
func node(args []string) {
	f := flag.NewFlagSet("node", flag.ExitOnError)
	data := f.String("data", "./data", "data directory")
	rpcAddr := f.String("rpc", "127.0.0.1:27778", "RPC address")
	p2pAddr := f.String("p2p", "0.0.0.0:27777", "P2P address")
	f.Parse(args)
	st, c, mp, e := load(*data)
	if e != nil {
		log.Fatal(e)
	}
	var mu = false
	_ = mu
	n := p2p.New(*p2pAddr, func(m p2p.Message) {
		if m.ChainID != core.ChainID || m.Block == nil {
			return
		}
		if c.ApplyBlock(*m.Block) == nil {
			_ = st.SaveBlocks(c.Blocks)
			_ = st.SaveMempool(mp)
		}
	})
	go func() {
		if e := n.Listen(); e != nil {
			log.Println(e)
		}
	}()
	srv := &rpc.Server{Chain: c, Store: st, Mempool: &mp, Mine: func(addr string) (core.Block, error) {
		b := mineBlock(c, mp, addr)
		if e := c.ApplyBlock(b); e != nil {
			return b, e
		}
		mp = nil
		if e := st.SaveBlocks(c.Blocks); e != nil {
			return b, e
		}
		_ = st.SaveMempool(mp)
		n.Broadcast(p2p.BlockMessage(b))
		return b, nil
	}}
	log.Printf("RainX node height=%d rpc=%s p2p=%s", c.Height(), *rpcAddr, *p2pAddr)
	log.Fatal(http.ListenAndServe(*rpcAddr, srv.Handler()))
}
func walletCmd(args []string) {
	if len(args) < 1 {
		usage()
		return
	}
	switch args[0] {
	case "create":
		f := flag.NewFlagSet("wallet create", flag.ExitOnError)
		dir := f.String("data", "./data", "data")
		name := f.String("name", "main", "name")
		pass := f.String("password", "", "password")
		f.Parse(args[1:])
		if *pass == "" {
			log.Fatal("password required")
		}
		w, e := wallet.NewAndSave(filepath.Join(*dir, "wallets"), *name, *pass)
		if e != nil {
			log.Fatal(e)
		}
		fmt.Println("created", w.Address())
	case "address":
		f := flag.NewFlagSet("wallet address", flag.ExitOnError)
		dir := f.String("data", "./data", "data")
		name := f.String("name", "main", "name")
		pass := f.String("password", "", "password")
		f.Parse(args[1:])
		w, e := wallet.Load(wallet.WalletPath(filepath.Join(*dir, "wallets"), *name), *pass)
		if e != nil {
			log.Fatal(e)
		}
		fmt.Println(w.Address())
	}
}
func mine(args []string) {
	f := flag.NewFlagSet("mine", flag.ExitOnError)
	data := f.String("data", "./data", "data")
	rpcURL := f.String("rpc", "http://127.0.0.1:27778", "rpc")
	addr := f.String("address", "", "address")
	f.Parse(args)
	if *addr == "" {
		log.Fatal("address required")
	}
	_, _, _, _ = load(*data)
	resp, e := http.Get(*rpcURL + "/api/mine?address=" + *addr)
	if e != nil {
		log.Fatal(e)
	}
	defer resp.Body.Close()
	var v map[string]any
	json.NewDecoder(resp.Body).Decode(&v)
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}
func send(args []string) {
	f := flag.NewFlagSet("send", flag.ExitOnError)
	data := f.String("data", "./data", "data")
	name := f.String("name", "main", "wallet name")
	pass := f.String("password", "", "wallet password")
	to := f.String("to", "", "destination")
	amount := f.String("amount", "", "RXC amount")
	rpcURL := f.String("rpc", "http://127.0.0.1:27778", "rpc")
	f.Parse(args)
	w, e := wallet.Load(wallet.WalletPath(filepath.Join(*data, "wallets"), *name), *pass)
	if e != nil {
		log.Fatal(e)
	}
	amt, e := wallet.ParseAmount(*amount)
	if e != nil {
		log.Fatal(e)
	}
	resp, e := http.Get(*rpcURL + "/api/status")
	if e != nil {
		log.Fatal(e)
	}
	defer resp.Body.Close()
	_, chain, _, e := load(*data)
	if e != nil {
		log.Fatal(e)
	}
	ops, total, e := core.SelectUTXOs(chain, w.Address(), amt)
	if e != nil {
		log.Fatal(e)
	}
	tx := core.NewTransfer([]core.TxOutput{{Value: amt, Address: *to}})
	if change := total - amt; change > 0 {
		tx.Outputs = append(tx.Outputs, core.TxOutput{Value: change, Address: w.Address()})
	}
	tx.Inputs = make([]core.TxInput, len(ops))
	for i, op := range ops {
		tx.Inputs[i] = core.TxInput{PrevTxID: op.TxID, Vout: op.Vout}
		core.SignInput(&tx, i, w.Private)
	}
	body, _ := json.Marshal(tx)
	req, _ := http.NewRequest(http.MethodPost, *rpcURL+"/api/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr, e := http.DefaultClient.Do(req)
	if e != nil {
		log.Fatal(e)
	}
	defer rr.Body.Close()
	var out map[string]any
	json.NewDecoder(rr.Body).Decode(&out)
	enc, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(enc))
}

func chain(args []string) {
	f := flag.NewFlagSet("chain", flag.ExitOnError)
	data := f.String("data", "./data", "data")
	f.Parse(args)
	_, c, mp, e := load(*data)
	if e != nil {
		log.Fatal(e)
	}
	fmt.Printf("RainX height=%d tip=%s work=%d mempool=%d\n", c.Height(), c.Tip().Hash(), c.TotalWork(), len(mp))
}
