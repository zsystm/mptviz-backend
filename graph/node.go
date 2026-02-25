package graph

import (
	"encoding/hex"
	"encoding/json"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// DecodedAccount represents a decoded Ethereum state account for display
type DecodedAccount struct {
	Nonce    uint64 `json:"nonce"`
	Balance  string `json:"balance"`
	Root     string `json:"root"`
	CodeHash string `json:"codeHash"`
}

// OperationRecord tracks a single insert/delete operation
type OperationRecord struct {
	Action      string `json:"action"`
	OriginalKey string `json:"originalKey"`
	HashedKey   string `json:"hashedKey"`
	Value       string `json:"value,omitempty"`
	Step        int    `json:"step"`
}

// TrieMetadata contains statistics about the trie
type TrieMetadata struct {
	NodeCount      int    `json:"nodeCount"`
	BranchCount    int    `json:"branchCount"`
	ExtensionCount int    `json:"extensionCount"`
	LeafCount      int    `json:"leafCount"`
	HashNodeCount  int    `json:"hashNodeCount"`
	Depth          int    `json:"depth"`
	RootHash       string `json:"rootHash,omitempty"`
}

// TrieResponse is the top-level API response
type TrieResponse struct {
	Root       *NodeData          `json:"root"`
	Operations []*OperationRecord `json:"operations"`
	Metadata   *TrieMetadata      `json:"metadata"`
}

// NodeData represents a node in the trie for visualization
type NodeData struct {
	ID           string          `json:"id"`
	NodeType     string          `json:"nodeType"`               // "branch"|"extension"|"leaf"|"hash"|"value"
	Path         string          `json:"path"`                   // full nibble path from root
	NibbleKey    string          `json:"nibbleKey,omitempty"`    // nibble path segment for extension/leaf
	HasTerm      bool            `json:"hasTerm"`
	Value        string          `json:"value,omitempty"`        // hex-encoded value
	DecodedValue *DecodedAccount `json:"decodedValue,omitempty"` // decoded account
	Hash         string          `json:"hash,omitempty"`         // node hash
	ChildIndex   int             `json:"childIndex"`             // parent branch slot (-1 if root or non-branch child)
	ActiveSlots  []int           `json:"activeSlots,omitempty"`  // for branch nodes: which slots have children
	Children     []*NodeData     `json:"children"`
}

// NibblesToString converts a nibble byte slice to a hex string where each nibble is one char
func NibblesToString(nibbles []byte) string {
	result := make([]byte, len(nibbles))
	for i, n := range nibbles {
		if n < 10 {
			result[i] = '0' + n
		} else {
			result[i] = 'a' + n - 10
		}
	}
	return string(result)
}

// DecodeStateAccount attempts to RLP-decode a value as a StateAccount
func DecodeStateAccount(value []byte) *DecodedAccount {
	var account types.StateAccount
	if err := rlp.DecodeBytes(value, &account); err != nil {
		return nil
	}
	return &DecodedAccount{
		Nonce:    account.Nonce,
		Balance:  account.Balance.String(),
		Root:     hex.EncodeToString(account.Root[:]),
		CodeHash: hex.EncodeToString(account.CodeHash),
	}
}

// MarshalJSON customizes the JSON serialization for NodeData.
// This is kept for backward-compatible behavior if needed but the new struct
// uses json tags directly. Implementing custom marshal to ensure Children is
// always an array (never null).
func (n *NodeData) MarshalJSON() ([]byte, error) {
	type Alias NodeData
	children := n.Children
	if children == nil {
		children = make([]*NodeData, 0)
	}
	return json.Marshal(&struct {
		*Alias
		Children []*NodeData `json:"children"`
	}{
		Alias:    (*Alias)(n),
		Children: children,
	})
}
