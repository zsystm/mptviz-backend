package data

import (
	"math/rand"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

type Account struct {
	Addr  common.Address
	State types.StateAccount
}

func (a *Account) MustGetRlpEncoded() []byte {
	val, err := rlp.EncodeToBytes(&a.State)
	if err != nil {
		panic(err)
	}
	return val
}

func MustCreateRandomAccount(r *rand.Rand) *Account {
	// create random address
	randAddr := make([]byte, 20)
	_, err := r.Read(randAddr)
	if err != nil {
		panic(err)
	}
	return &Account{
		Addr: common.BytesToAddress(randAddr),
		State: types.StateAccount{
			Nonce:   r.Uint64(),
			Balance: uint256.NewInt(uint64(r.Int63())),
		},
	}
}
