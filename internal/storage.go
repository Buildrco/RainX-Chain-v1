package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/Buildrco/rainx-chain/internal/core"
)

type Store struct {
	dir string
	mu  sync.RWMutex
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}
func (s *Store) blocksPath() string  { return filepath.Join(s.dir, "blocks.json") }
func (s *Store) mempoolPath() string { return filepath.Join(s.dir, "mempool.json") }
func (s *Store) LoadBlocks() ([]core.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := os.ReadFile(s.blocksPath())
	if os.IsNotExist(err) {
		return []core.Block{core.NewGenesis()}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []core.Block
	if err = json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return []core.Block{core.NewGenesis()}, nil
	}
	return out, nil
}
func (s *Store) SaveBlocks(bs []core.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(bs, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.blocksPath() + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.blocksPath())
}
func (s *Store) LoadMempool() ([]core.Transaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := os.ReadFile(s.mempoolPath())
	if os.IsNotExist(err) {
		return []core.Transaction{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []core.Transaction
	if err = json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Store) SaveMempool(tx []core.Transaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.MarshalIndent(tx, "", "  ")
	return os.WriteFile(s.mempoolPath(), b, 0600)
}

var ErrNotFound = errors.New("not found")
