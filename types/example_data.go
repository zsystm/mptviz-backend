package types

import (
	"github.com/zsystm/mpt/data"
)

type ExampleData struct {
	Key     string `json:"key"`
	Val     string `json:"val"`
	Address string `json:"address"`
	Nonce   uint64 `json:"nonce"`
	Balance string `json:"balance"`
	// TODO: Handle smart contract state
}

func AccountToExampleData(acc data.Account) ExampleData {
	return ExampleData{
		Address: acc.Addr.String(),
		Nonce:   acc.State.Nonce,
		Balance: acc.State.Balance.String(),
	}
}
