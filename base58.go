package core

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"math/big"
)

const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func b58Encode(input []byte) string {
	x := new(big.Int).SetBytes(input)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	var out []byte
	for x.Cmp(zero) > 0 {
		mod := new(big.Int)
		x.DivMod(x, base, mod)
		out = append(out, alphabet[mod.Int64()])
	}
	for _, b := range input {
		if b == 0 {
			out = append(out, '1')
		} else {
			break
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func b58Decode(s string) ([]byte, error) {
	x := big.NewInt(0)
	base := big.NewInt(58)
	for _, c := range []byte(s) {
		i := bytes.IndexByte([]byte(alphabet), c)
		if i < 0 {
			return nil, errors.New("invalid base58")
		}
		x.Mul(x, base)
		x.Add(x, big.NewInt(int64(i)))
	}
	raw := x.Bytes()
	zeros := 0
	for zeros < len(s) && s[zeros] == '1' {
		zeros++
	}
	return append(bytes.Repeat([]byte{0}, zeros), raw...), nil
}

func checksum(b []byte) []byte { a := sha256.Sum256(b); b2 := sha256.Sum256(a[:]); return b2[:4] }

func AddressFromPubKey(pub []byte) string {
	h := sha256.Sum256(pub)
	payload := append([]byte{0x52}, h[:20]...)
	payload = append(payload, checksum(payload)...)
	return b58Encode(payload)
}

func DecodeAddress(addr string) ([]byte, error) {
	raw, err := b58Decode(addr)
	if err != nil || len(raw) != 25 {
		return nil, errors.New("invalid RXC address")
	}
	if raw[0] != 0x52 {
		return nil, errors.New("wrong RXC address version")
	}
	if !bytes.Equal(raw[21:], checksum(raw[:21])) {
		return nil, errors.New("bad RXC checksum")
	}
	return raw[1:21], nil
}
