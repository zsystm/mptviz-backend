package utils

import "github.com/ethereum/go-ethereum/crypto"

func HashKey(key []byte) []byte {
	return crypto.Keccak256Hash(key).Bytes()
}
