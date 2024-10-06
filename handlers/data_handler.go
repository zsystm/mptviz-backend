package handlers

import (
	"encoding/hex"
	"math/rand"
	"time"

	"github.com/zsystm/mpt/data"
	"github.com/zsystm/mpt/types"
	"github.com/zsystm/mpt/utils"
)

// Examples returns a list of examples data for the MPT
func Examples(num int) []types.ExampleData {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	examples := make([]types.ExampleData, num)
	for i := 0; i < num; i++ {
		randomAcc := data.MustCreateRandomAccount(r)
		example := types.AccountToExampleData(*randomAcc)
		example.Key = hex.EncodeToString(utils.HashKey(randomAcc.Addr.Bytes()))
		example.Val = hex.EncodeToString(randomAcc.MustGetRlpEncoded())
		examples[i] = example
	}
	return examples
}
