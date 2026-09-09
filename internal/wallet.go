package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Buildrco/rainx-chain/internal/core"
)

type File struct {
	Name       string `json:"name"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type Wallet struct {
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
}

func New() (*Wallet, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Wallet{Private: priv, Public: pub}, nil
}
func (w *Wallet) Address() string { return core.AddressFromPubKey(w.Public) }

func pbkdf2(password, salt []byte, iters, keyLen int) []byte {
	var out []byte
	for block := 1; len(out) < keyLen; block++ {
		u := hmacSHA256(password, append(append([]byte{}, salt...), byte(block>>24), byte(block>>16), byte(block>>8), byte(block)))
		t := append([]byte{}, u...)
		for i := 1; i < iters; i++ {
			u = hmacSHA256(password, u)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
func hmacSHA256(key, data []byte) []byte {
	// HMAC-SHA256 without external dependencies.
	const block = 64
	k := append([]byte{}, key...)
	if len(k) > block {
		h := sha256.Sum256(k)
		k = h[:]
	}
	if len(k) < block {
		k = append(k, make([]byte, block-len(k))...)
	}
	ipad := make([]byte, block)
	opad := make([]byte, block)
	for i := 0; i < block; i++ {
		ipad[i] = k[i] ^ 0x36
		opad[i] = k[i] ^ 0x5c
	}
	hi := sha256.Sum256(append(ipad, data...))
	ho := sha256.Sum256(append(opad, hi[:]...))
	return ho[:]
}

func Save(path, name, password string, w *Wallet) error {
	salt := make([]byte, 16)
	nonce := make([]byte, 12)
	if _, e := io.ReadFull(rand.Reader, salt); e != nil {
		return e
	}
	if _, e := io.ReadFull(rand.Reader, nonce); e != nil {
		return e
	}
	key := pbkdf2([]byte(password), salt, 120000, 32)
	block, e := aes.NewCipher(key)
	if e != nil {
		return e
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		return e
	}
	ct := gcm.Seal(nil, nonce, w.Private, nil)
	f := File{Name: name, Salt: base64.StdEncoding.EncodeToString(salt), Nonce: base64.StdEncoding.EncodeToString(nonce), Ciphertext: base64.StdEncoding.EncodeToString(ct)}
	b, _ := json.MarshalIndent(f, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}
func Load(path, password string) (*Wallet, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var f File
	if e = json.Unmarshal(b, &f); e != nil {
		return nil, e
	}
	salt, _ := base64.StdEncoding.DecodeString(f.Salt)
	nonce, _ := base64.StdEncoding.DecodeString(f.Nonce)
	ct, _ := base64.StdEncoding.DecodeString(f.Ciphertext)
	key := pbkdf2([]byte(password), salt, 120000, 32)
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	priv, e := gcm.Open(nil, nonce, ct, nil)
	if e != nil {
		return nil, errors.New("wrong wallet password or corrupt wallet")
	}
	if len(priv) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid private key")
	}
	p := ed25519.PrivateKey(priv)
	return &Wallet{Private: p, Public: p.Public().(ed25519.PublicKey)}, nil
}
func NewAndSave(dir, name, password string) (*Wallet, error) {
	w, e := New()
	if e != nil {
		return nil, e
	}
	return w, Save(filepath.Join(dir, name+".json"), name, password, w)
}
func WalletPath(dir, name string) string { return filepath.Join(dir, name+".json") }
func FormatAmount(v uint64) string       { return fmt.Sprintf("%d.%08d", v/core.Coin, v%core.Coin) }
func ParseAmount(s string) (uint64, error) {
	var whole, frac uint64
	var err error
	if _, e := fmt.Sscanf(s, "%d.%d", &whole, &frac); e != nil {
		return 0, e
	}
	if frac >= core.Coin {
		return 0, errors.New("too many decimals")
	}
	return whole*core.Coin + frac, err
}
func HexPub(w *Wallet) string { return hex.EncodeToString(w.Public) }
