package rpc

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/Buildrco/rainx-chain/internal/core"
	"github.com/Buildrco/rainx-chain/internal/storage"
)

type Server struct {
	Chain        *core.Chain
	Store        *storage.Store
	Mempool      *[]core.Transaction
	MinerAddress func() string
	Mine         func(string) (core.Block, error)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.home)
	mux.HandleFunc("/api/status", s.status)
	mux.HandleFunc("/api/blocks", s.blocks)
	mux.HandleFunc("/api/balance", s.balance)
	mux.HandleFunc("/api/utxos", s.utxos)
	mux.HandleFunc("/api/history", s.history)
	mux.HandleFunc("/api/block", s.block)
	mux.HandleFunc("/api/tx", s.tx)
	mux.HandleFunc("/api/submit", s.submit)
	mux.HandleFunc("/api/mine", s.mine)
	return mux
}
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(indexHTML))
}
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	s.cors(w)
	s.json(w, map[string]any{"name": "RainX Network", "symbol": core.Symbol, "chainId": core.ChainID, "height": s.Chain.Height(), "hash": s.Chain.Tip().Hash(), "work": s.Chain.TotalWork(), "mempool": len(*s.Mempool), "time": time.Now().UTC()})
}
func (s *Server) blocks(w http.ResponseWriter, r *http.Request) {
	n := 10
	if x := r.URL.Query().Get("limit"); x != "" {
		fmt.Sscanf(x, "%d", &n)
	}
	if n > 50 {
		n = 50
	}
	bs := s.Chain.Blocks
	start := 0
	if len(bs) > n {
		start = len(bs) - n
	}
	s.json(w, bs[start:])
}
func (s *Server) balance(w http.ResponseWriter, r *http.Request) {
	addr := r.URL.Query().Get("address")
	s.json(w, map[string]any{"address": addr, "balance": s.Chain.Balance(addr)})
}

func (s *Server) utxos(w http.ResponseWriter, r *http.Request) {
	s.cors(w)
	addr := strings.TrimSpace(r.URL.Query().Get("address"))
	utxos := s.Chain.UTXOs()
	type row struct {
		TxID    string `json:"txid"`
		Vout    uint32 `json:"vout"`
		Value   uint64 `json:"value"`
		Address string `json:"address"`
	}
	out := make([]row, 0)
	for key, output := range utxos {
		if output.Address != addr {
			continue
		}
		var txid string
		var vout uint32
		fmt.Sscanf(key, "%s:%d", &txid, &vout)
		out = append(out, row{TxID: txid, Vout: vout, Value: output.Value, Address: output.Address})
	}
	s.json(w, out)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	s.cors(w)
	addr := strings.TrimSpace(r.URL.Query().Get("address"))
	outputIndex := map[string]core.TxOutput{}
	for _, b := range s.Chain.Blocks {
		for _, tx := range b.Transactions {
			id := tx.ID()
			for i, o := range tx.Outputs {
				outputIndex[fmt.Sprintf("%s:%d", id, i)] = o
			}
		}
	}
	seen := map[string]bool{}
	out := make([]core.Transaction, 0)
	for i := len(s.Chain.Blocks) - 1; i >= 0; i-- {
		b := s.Chain.Blocks[i]
		for j := len(b.Transactions) - 1; j >= 0; j-- {
			tx := b.Transactions[j]
			match := false
			for _, o := range tx.Outputs {
				if o.Address == addr {
					match = true
					break
				}
			}
			if !match {
				for _, in := range tx.Inputs {
					if o, ok := outputIndex[fmt.Sprintf("%s:%d", in.PrevTxID, in.Vout)]; ok && o.Address == addr {
						match = true
						break
					}
				}
			}
			id := tx.ID()
			if match && !seen[id] {
				out = append(out, tx)
				seen[id] = true
			}
		}
	}
	s.json(w, out)
}

func (s *Server) block(w http.ResponseWriter, r *http.Request) {
	s.cors(w)
	var height uint64
	if _, err := fmt.Sscanf(r.URL.Query().Get("height"), "%d", &height); err != nil {
		http.Error(w, "invalid height", 400)
		return
	}
	for _, b := range s.Chain.Blocks {
		if b.Header.Height == height {
			s.json(w, b)
			return
		}
	}
	http.NotFound(w, r)
}
func (s *Server) tx(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	for _, b := range s.Chain.Blocks {
		for _, t := range b.Transactions {
			if t.ID() == id {
				s.json(w, t)
				return
			}
		}
	}
	http.NotFound(w, r)
}
func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	s.cors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return
	}
	var tx core.Transaction
	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.Chain.ValidateMempoolTx(tx); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	for _, existing := range *s.Mempool {
		if existing.ID() == tx.ID() {
			s.json(w, map[string]any{"accepted": true, "txid": tx.ID(), "duplicate": true})
			return
		}
	}
	*s.Mempool = append(*s.Mempool, tx)
	if err := s.Store.SaveMempool(*s.Mempool); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.json(w, map[string]any{"accepted": true, "txid": tx.ID()})
}

func (s *Server) mine(w http.ResponseWriter, r *http.Request) {
	s.cors(w)
	if s.Mine == nil {
		http.Error(w, "mining disabled", 503)
		return
	}
	addr := r.URL.Query().Get("address")
	if addr == "" {
		addr = s.MinerAddress()
	}
	b, e := s.Mine(addr)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	s.json(w, b)
}
func (s *Server) cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
}

func (s *Server) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

var _ = template.HTMLEscapeString

const indexHTML = `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>RainX Network</title><style>:root{font-family:-apple-system,BlinkMacSystemFont,"SF Pro Display","SF Pro Text",Inter,system-ui,sans-serif;color:#f5f5f7;background:#070707}*{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at 20% 0%,#2b210b 0,#0b0b0c 34%,#050505 100%);min-height:100vh}main{max-width:1180px;margin:auto;padding:28px 20px 60px}.nav{display:flex;align-items:center;justify-content:space-between;padding:10px 2px 34px}.brand{display:flex;align-items:center;gap:12px;font-weight:700;font-size:21px;letter-spacing:-.03em}.coin{width:38px;height:38px;border-radius:12px;background:linear-gradient(145deg,#ffe9a0,#c99627);display:grid;place-items:center;color:#15110a;font-weight:900;box-shadow:0 10px 40px #c9982d33}.pill{border:1px solid #ffffff16;background:#ffffff08;border-radius:999px;padding:8px 12px;color:#aaa;font-size:13px}.hero{padding:24px 0 30px}.eyebrow{color:#d9b95c;font-weight:650;font-size:14px}.title{font-size:clamp(42px,7vw,78px);line-height:.98;letter-spacing:-.065em;margin:10px 0 16px;max-width:820px}.sub{color:#a6a6ab;max-width:680px;font-size:17px;line-height:1.55}.grid{display:grid;grid-template-columns:repeat(4,1fr);gap:12px}.card{background:#101011dd;border:1px solid #ffffff10;border-radius:24px;padding:20px;box-shadow:0 20px 70px #0008}.label{color:#85858a;font-size:13px}.value{font-size:25px;font-weight:700;letter-spacing:-.035em;margin-top:9px}.accent{color:#e4bd57}.wide{grid-column:span 4}.section{margin-top:28px}.section h2{font-size:24px;letter-spacing:-.04em;margin:0 0 12px}.table{width:100%;border-collapse:collapse}.table th,.table td{text-align:left;padding:13px 8px;border-bottom:1px solid #ffffff0d;font-size:14px}.table th{color:#777}.mono{font-family:"SF Mono",ui-monospace,monospace;font-size:12px;color:#a9a9ad}.status{display:inline-flex;gap:7px;align-items:center}.dot{width:8px;height:8px;background:#65d38b;border-radius:50%;box-shadow:0 0 14px #65d38b}.chart{height:190px;position:relative;overflow:hidden}.chart svg{width:100%;height:100%}.foot{color:#69696f;font-size:12px;margin-top:25px}@media(max-width:760px){main{padding:20px 16px 45px}.grid{grid-template-columns:1fr 1fr}.wide{grid-column:span 2}.card{border-radius:19px;padding:17px}.title{font-size:48px}.sub{font-size:16px}}</style></head><body><main><div class="nav"><div class="brand"><div class="coin">R</div>RainX Network</div><div class="pill status"><span class="dot"></span> Node online</div></div><section class="hero"><div class="eyebrow">NATIVE BLOCKCHAIN · RXC</div><div class="title">RainX Coin has its own chain.</div><div class="sub">A native proof-of-work network with independent blocks, wallets, signatures and peer-to-peer validation. This explorer reads the node directly.</div></section><section class="grid"><div class="card"><div class="label">Height</div><div id="height" class="value">—</div></div><div class="card"><div class="label">Native asset</div><div class="value accent">RXC</div></div><div class="card"><div class="label">Consensus</div><div class="value">Proof of Work</div></div><div class="card"><div class="label">Block target</div><div class="value">60 sec</div></div><div class="card wide"><div class="label">Network activity</div><div class="chart"><svg viewBox="0 0 900 190" preserveAspectRatio="none"><defs><linearGradient id="g" x1="0" x2="0" y1="0" y2="1"><stop offset="0" stop-color="#e5bf5b" stop-opacity=".42"/><stop offset="1" stop-color="#e5bf5b" stop-opacity="0"/></linearGradient></defs><path d="M0 165 C80 145 90 155 160 120 S245 145 320 96 S405 132 480 78 S560 112 640 62 S720 92 790 48 S850 67 900 30 L900 190 L0 190Z" fill="url(#g)"/><path d="M0 165 C80 145 90 155 160 120 S245 145 320 96 S405 132 480 78 S560 112 640 62 S720 92 790 48 S850 67 900 30" fill="none" stroke="#e5bf5b" stroke-width="3" stroke-linecap="round"/></svg></div></div></section><section class="section"><h2>Latest blocks</h2><div class="card"><table class="table"><thead><tr><th>Height</th><th>Hash</th><th>Transactions</th><th>Time</th></tr></thead><tbody id="blocks"></tbody></table></div></section><div class="foot">RainX Chain v1 · Protocol values are versioned. Do not use a development node for real funds.</div></main><script>async function load(){let s=await fetch('/api/status').then(r=>r.json());document.querySelector('#height').textContent=s.height;let bs=await fetch('/api/blocks?limit=10').then(r=>r.json());document.querySelector('#blocks').innerHTML=bs.slice().reverse().map(b=>'<tr><td>'+b.header.height+'</td><td class="mono">'+b.header.prevHash.slice(0,12)+'…</td><td>'+b.transactions.length+'</td><td>'+new Date(b.header.timestamp*1000).toLocaleString()+'</td></tr>').join('')}load();setInterval(load,5000)</script></body></html>`
