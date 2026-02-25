package graph

import (
	"encoding/hex"
	"fmt"
	"sync/atomic"

	"github.com/zsystm/mpt/trie"
)

var nodeCounter atomic.Int64

func resetNodeCounter() {
	nodeCounter.Store(0)
}

func nextNodeID() string {
	return fmt.Sprintf("node-%d", nodeCounter.Add(1))
}

// BuildTrieGraph traverses the trie and builds the visualization graph with metadata
func BuildTrieGraph(root trie.Node) (*NodeData, *TrieMetadata) {
	resetNodeCounter()
	meta := &TrieMetadata{}
	data := dfs(nil, root, nil, -1, meta, 0)
	return data, meta
}

// dfs traverses the trie depth-first, building NodeData for each node
// path: accumulated nibble path from root
// n: current node
// parent: parent node (for context)
// childIdx: which child slot of parent branch (-1 if root or non-branch)
// meta: accumulates statistics
// depth: current depth
func dfs(path []byte, n trie.Node, parent trie.Node, childIdx int, meta *TrieMetadata, depth int) *NodeData {
	if n == nil {
		return nil
	}

	meta.NodeCount++
	if depth > meta.Depth {
		meta.Depth = depth
	}

	switch node := n.(type) {
	case *trie.ShortNode:
		data := &NodeData{
			ID:         nextNodeID(),
			Path:       NibblesToString(path),
			ChildIndex: childIdx,
			Children:   make([]*NodeData, 0),
		}

		// Determine if this is a leaf or extension based on terminator
		if trie.HasTerm(node.Key) {
			data.NodeType = "leaf"
			data.NibbleKey = NibblesToString(node.Key[:len(node.Key)-1]) // exclude terminator
			data.HasTerm = true
			meta.LeafCount++

			// Try to decode the value
			if vn, ok := node.Val.(trie.ValueNode); ok {
				data.Value = hex.EncodeToString([]byte(vn))
				data.DecodedValue = DecodeStateAccount([]byte(vn))
			}
		} else {
			data.NodeType = "extension"
			data.NibbleKey = NibblesToString(node.Key)
			data.HasTerm = false
			meta.ExtensionCount++

			// Recurse into the child
			childPath := append(append([]byte{}, path...), node.Key...)
			childData := dfs(childPath, node.Val, n, -1, meta, depth+1)
			if childData != nil {
				data.Children = append(data.Children, childData)
			}
		}

		// Add hash if available
		if hash, dirty := node.Cache(); hash != nil && !dirty {
			data.Hash = hex.EncodeToString(hash)
		}

		return data

	case *trie.FullNode:
		data := &NodeData{
			ID:          nextNodeID(),
			NodeType:    "branch",
			Path:        NibblesToString(path),
			ChildIndex:  childIdx,
			Children:    make([]*NodeData, 0),
			ActiveSlots: make([]int, 0),
		}
		meta.BranchCount++

		for i := 0; i < 16; i++ {
			child := node.Children[i]
			if child == nil {
				continue
			}
			data.ActiveSlots = append(data.ActiveSlots, i)
			childPath := append(append([]byte{}, path...), byte(i))
			childData := dfs(childPath, child, n, i, meta, depth+1)
			if childData != nil {
				data.Children = append(data.Children, childData)
			}
		}

		// Check slot 16 (inline value)
		if node.Children[16] != nil {
			if vn, ok := node.Children[16].(trie.ValueNode); ok {
				valData := &NodeData{
					ID:         nextNodeID(),
					NodeType:   "value",
					Path:       NibblesToString(path),
					Value:      hex.EncodeToString([]byte(vn)),
					ChildIndex: 16,
					Children:   make([]*NodeData, 0),
				}
				valData.DecodedValue = DecodeStateAccount([]byte(vn))
				data.Children = append(data.Children, valData)
				meta.NodeCount++
			}
		}

		// Add hash if available
		if hash, dirty := node.Cache(); hash != nil && !dirty {
			data.Hash = hex.EncodeToString(hash)
		}

		return data

	case trie.HashNode:
		meta.HashNodeCount++
		return &NodeData{
			ID:         nextNodeID(),
			NodeType:   "hash",
			Path:       NibblesToString(path),
			Hash:       hex.EncodeToString([]byte(node)),
			ChildIndex: childIdx,
			Children:   make([]*NodeData, 0),
		}

	case trie.ValueNode:
		return &NodeData{
			ID:           nextNodeID(),
			NodeType:     "value",
			Path:         NibblesToString(path),
			Value:        hex.EncodeToString([]byte(node)),
			DecodedValue: DecodeStateAccount([]byte(node)),
			ChildIndex:   childIdx,
			Children:     make([]*NodeData, 0),
		}
	}

	return nil
}
