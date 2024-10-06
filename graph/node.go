package graph

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zsystm/mpt/trie"
)

type Attribute struct {
	Key []byte `json:"key"`
	Val []byte `json:"val"`
}

// NodeData represents the node in the trie.
type NodeData struct {
	Name       string       `json:"name"`
	Attributes []*Attribute `json:"attributes"`
	Children   []*NodeData  `json:"children"`
}

// MarshalJSON customizes the JSON serialization for NodeData.
func (n *NodeData) MarshalJSON() ([]byte, error) {
	// Create a temporary structure to hold the serialized data.
	type Alias NodeData // Create an alias to avoid infinite recursion.
	attributesMap := make(map[string]string)

	// Convert the Attributes slice into a map[string]string
	for _, attr := range n.Attributes {
		var key, val string
		if attr.Key == nil {
			key = ""
		} else {
			//key = hex.EncodeToString(attr.Key)
			//key = fmt.Sprintf("%s(%d)", key, len(attr.Key))
			key = hex.EncodeToString(attr.Key)
			key = fmt.Sprintf("%s(%d)", CleanHex(key), len(attr.Key))
		}
		if attr.Val == nil {
			val = ""
		} else {
			val = hex.EncodeToString(attr.Val)
		}
		attributesMap[key] = val
	}

	// Create an anonymous struct that combines the original NodeData fields
	// with the new Attributes map[string]string
	return json.Marshal(&struct {
		Name       string            `json:"name"`
		Attributes map[string]string `json:"attributes"`
		Children   []*NodeData       `json:"children"`
	}{
		Name:       n.Name,
		Attributes: attributesMap,
		Children:   n.Children,
	})
}

func NewNodeData(n trie.Node) *NodeData {
	if n == nil {
		return &NodeData{
			Name: "Empty",
		}
	}

	return &NodeData{
		Name:       nodeType(n),
		Attributes: make([]*Attribute, 0),
		Children:   make([]*NodeData, 0),
	}
}

// CleanHex removes leading '0's from every pair of hex digits.
func CleanHex(hexStr string) string {
	var result strings.Builder

	// Iterate over the string in steps of 2 (hex pairs)
	for i := 0; i < len(hexStr)-1; i += 2 {
		// Take two characters at a time
		pair := hexStr[i : i+2]

		// If the first character is '0', use only the second character
		if pair[0] == '0' {
			result.WriteByte(pair[1])
		} else {
			// Otherwise, add both characters
			result.WriteString(pair)
		}
	}

	return result.String()
}
