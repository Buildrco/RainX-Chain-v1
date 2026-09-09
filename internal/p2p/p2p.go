package p2p

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/Buildrco/rainx-chain/internal/core"
)

type Message struct {
	Type    string            `json:"type"`
	ChainID string            `json:"chainId"`
	Block   *core.Block       `json:"block,omitempty"`
	Tx      *core.Transaction `json:"tx,omitempty"`
	Height  uint64            `json:"height,omitempty"`
}
type Peer struct {
	Conn net.Conn
	Enc  *json.Encoder
	Mu   sync.Mutex
}
type Node struct {
	Addr      string
	Peers     map[string]*Peer
	Mu        sync.RWMutex
	OnMessage func(Message)
}

func New(addr string, on func(Message)) *Node {
	return &Node{Addr: addr, Peers: map[string]*Peer{}, OnMessage: on}
}
func (n *Node) Listen() error {
	ln, e := net.Listen("tcp", n.Addr)
	if e != nil {
		return e
	}
	for {
		c, e := ln.Accept()
		if e != nil {
			continue
		}
		n.add(c)
	}
}
func (n *Node) add(c net.Conn) {
	p := &Peer{Conn: c, Enc: json.NewEncoder(c)}
	n.Mu.Lock()
	n.Peers[c.RemoteAddr().String()] = p
	n.Mu.Unlock()
	go func() {
		s := bufio.NewScanner(c)
		for s.Scan() {
			var m Message
			if json.Unmarshal(s.Bytes(), &m) == nil && n.OnMessage != nil {
				n.OnMessage(m)
			}
		}
		c.Close()
		n.Mu.Lock()
		delete(n.Peers, c.RemoteAddr().String())
		n.Mu.Unlock()
	}()
}
func (n *Node) Connect(addr string) error {
	c, e := net.Dial("tcp", addr)
	if e != nil {
		return e
	}
	n.add(c)
	return nil
}
func (n *Node) Broadcast(m Message) {
	n.Mu.RLock()
	defer n.Mu.RUnlock()
	for _, p := range n.Peers {
		p.Mu.Lock()
		_ = p.Enc.Encode(m)
		p.Mu.Unlock()
	}
}
func Hello(height uint64) Message {
	return Message{Type: "hello", ChainID: core.ChainID, Height: height}
}
func BlockMessage(b core.Block) Message {
	return Message{Type: "block", ChainID: core.ChainID, Block: &b, Height: b.Header.Height}
}
func TxMessage(t core.Transaction) Message { return Message{Type: "tx", ChainID: core.ChainID, Tx: &t} }
func (n *Node) String() string {
	n.Mu.RLock()
	defer n.Mu.RUnlock()
	return fmt.Sprintf("%s peers=%d", n.Addr, len(n.Peers))
}
