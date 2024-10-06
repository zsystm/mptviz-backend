package trie

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// ErrCommitted is returned when an already committed trie is requested for usage.
// The potential usages can be `Get`, `Update`, `Delete`, `NodeIterator`, `Prove`
// and so on.
var ErrCommitted = errors.New("trie is already committed")

// MissingNodeError is returned by the trie functions (Get, Update, Delete)
// in the case where a trie node is not present in the local database. It contains
// information necessary for retrieving the missing node.
type MissingNodeError struct {
	Owner    common.Hash // owner of the trie if it's 2-layered trie
	NodeHash common.Hash // hash of the missing node
	Path     []byte      // hex-encoded path to the missing node
	Err      error       // concrete error for missing trie node
}

// Unwrap returns the concrete error for missing trie node which
// allows us for further analysis outside.
func (err *MissingNodeError) Unwrap() error {
	return err.Err
}

func (err *MissingNodeError) Error() string {
	if err.Owner == (common.Hash{}) {
		return fmt.Sprintf("missing trie node %x (path %x) %v", err.NodeHash, err.Path, err.Err)
	}
	return fmt.Sprintf("missing trie node %x (owner %x) (path %x) %v", err.NodeHash, err.Owner, err.Path, err.Err)
}
