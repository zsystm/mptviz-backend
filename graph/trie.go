package graph

import (
	"encoding/hex"
	"fmt"

	"github.com/zsystm/mpt/trie"
)

// Dfs traverses the trie in depth-first order and returns the node data for each node.
func Dfs(key []byte, n trie.Node, parent trie.Node) *NodeData {
	if n == nil {
		return nil
	}
	switch node := n.(type) {
	case *trie.ShortNode:
		child := node.Val
		data := NewNodeData(n)
		if parent != nil && isFullNode(parent) {
			posKey := key[len(key)-1]
			data.Name += fmt.Sprintf("[%s]", CleanHex(hex.EncodeToString([]byte{posKey})))
		}
		data.Attributes = append(data.Attributes, &Attribute{
			Key: node.Key,
		})
		nodeData := Dfs(node.Key, child, n)
		data.Children = append(data.Children, nodeData)
		return data
	case *trie.FullNode:
		data := NewNodeData(n)
		if parent != nil && isFullNode(parent) {
			posKey := key[len(key)-1]
			data.Name += fmt.Sprintf("[%s]", CleanHex(hex.EncodeToString([]byte{posKey})))
		}
		for i, child := range node.Children {
			var nodeData *NodeData
			nodeData = Dfs(append(key, byte(i)), child, n)
			if nodeData == nil {
				continue
				nodeData = NewNodeData(nil)
				nodeData.Name += fmt.Sprintf("[%s]", CleanHex(hex.EncodeToString([]byte{byte(i)})))
			}
			data.Children = append(data.Children, nodeData)
		}
		return data
	case trie.HashNode:
		data := NewNodeData(n)
		data.Attributes = append(data.Attributes, &Attribute{
			Val: node,
		})
		return data
	case trie.ValueNode:
		data := NewNodeData(n)
		data.Attributes = append(data.Attributes, &Attribute{
			Val: node,
		})
		return data
	case trie.RawNode:
		data := NewNodeData(n)
		data.Attributes = append(data.Attributes, &Attribute{
			Val: node,
		})
		return data
	}

	return nil
}

func nodeType(n trie.Node) string {
	switch n.(type) {
	case *trie.ShortNode:
		return "ShortNode"
	case *trie.FullNode:
		return "FullNode"
	case trie.HashNode:
		return "HashNode"
	case trie.ValueNode:
		return "ValueNode"
	case *trie.RawNode:
		return "RawNode"
	}
	return "Unknown"
}

func isFullNode(n trie.Node) bool {
	_, ok := n.(*trie.FullNode)
	return ok
}
