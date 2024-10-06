package trie

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/trie/trienode"
)

// Committer is the tool used for the trie Commit operation. The committer will
// capture all dirty nodes during the commit process and keep them cached in
// insertion order.
type Committer struct {
	Nodes       *trienode.NodeSet
	Tracer      *Tracer
	CollectLeaf bool
}

// CewCommitter creates a new Committer or picks one from the pool.
func NewCommitter(nodeset *trienode.NodeSet, tracer *Tracer, collectLeaf bool) *Committer {
	return &Committer{
		Nodes:       nodeset,
		Tracer:      tracer,
		CollectLeaf: collectLeaf,
	}
}

// Commit collapses a node down into a hash node.
func (c *Committer) Commit(n Node) HashNode {
	return c.DoCommit(nil, n).(HashNode)
}

// DoCommit collapses a node down into a hash node and returns it.
func (c *Committer) DoCommit(path []byte, n Node) Node {
	// if this path is clean, use available cached data
	hash, dirty := n.Cache()
	if hash != nil && !dirty {
		return hash
	}
	// Commit children, then parent, and remove the dirty flag.
	switch cn := n.(type) {
	case *ShortNode:
		// Commit child
		collapsed := cn.Copy()

		// If the child is fullNode, recursively commit,
		// otherwise it can only be hashNode or valueNode.
		if _, ok := cn.Val.(*FullNode); ok {
			collapsed.Val = c.DoCommit(append(path, cn.Key...), cn.Val)
		}
		// The key needs to be copied, since we're adding it to the
		// modified nodeset.
		collapsed.Key = HexToCompact(cn.Key)
		hashedNode := c.Store(path, collapsed)
		if hn, ok := hashedNode.(HashNode); ok {
			return hn
		}
		return collapsed
	case *FullNode:
		hashedKids := c.CommitChildren(path, cn)
		collapsed := cn.Copy()
		collapsed.Children = hashedKids

		hashedNode := c.Store(path, collapsed)
		if hn, ok := hashedNode.(HashNode); ok {
			return hn
		}
		return collapsed
	case HashNode:
		return cn
	default:
		// nil, valuenode shouldn't be committed
		panic(fmt.Sprintf("%T: invalid node: %v", n, n))
	}
}

// CommitChildren commits the children of the given fullnode
func (c *Committer) CommitChildren(path []byte, n *FullNode) [17]Node {
	var children [17]Node
	for i := 0; i < 16; i++ {
		child := n.Children[i]
		if child == nil {
			continue
		}
		// If it's the hashed child, save the hash value directly.
		// Note: it's impossible that the child in range [0, 15]
		// is a valueNode.
		if hn, ok := child.(HashNode); ok {
			children[i] = hn
			continue
		}
		// Commit the child recursively and store the "hashed" value.
		// Note the returned node can be some embedded nodes, so it's
		// possible the type is not hashNode.
		children[i] = c.DoCommit(append(path, byte(i)), child)
	}
	// For the 17th child, it's possible the type is valuenode.
	if n.Children[16] != nil {
		children[16] = n.Children[16]
	}
	return children
}

// Store hashes the node n and adds it to the modified nodeset. If leaf collection
// is enabled, leaf nodes will be tracked in the modified nodeset as well.
func (c *Committer) Store(path []byte, n Node) Node {
	// Larger nodes are replaced by their hash and stored in the database.
	var hash, _ = n.Cache()

	// This was not generated - must be a small node stored in the parent.
	// In theory, we should check if the node is leaf here (embedded node
	// usually is leaf node). But small value (less than 32bytes) is not
	// our target (leaves in account trie only).
	if hash == nil {
		// The node is embedded in its parent, in other words, this node
		// will not be stored in the database independently, mark it as
		// deleted only if the node was existent in database before.
		_, ok := c.Tracer.AccessList[string(path)]
		if ok {
			c.Nodes.AddNode(path, trienode.NewDeleted())
		}
		return n
	}
	// Collect the dirty node to nodeset for return.
	nhash := common.BytesToHash(hash)
	c.Nodes.AddNode(path, trienode.New(nhash, nodeToBytes(n)))

	// Collect the corresponding leaf node if it's required. We don't check
	// full node since it's impossible to store value in fullNode. The key
	// length of leaves should be exactly same.
	if c.CollectLeaf {
		if sn, ok := n.(*ShortNode); ok {
			if val, ok := sn.Val.(ValueNode); ok {
				c.Nodes.AddLeaf(nhash, val)
			}
		}
	}
	return hash
}

// ForGatherChildren decodes the provided node and traverses the children inside.
func ForGatherChildren(node []byte, onChild func(common.Hash)) {
	DoForGatherChildren(MustDecodeNodeUnsafe(nil, node), onChild)
}

// DoForGatherChildren traverses the node hierarchy and invokes the callback
// for all the hashnode children.
func DoForGatherChildren(n Node, onChild func(hash common.Hash)) {
	switch n := n.(type) {
	case *ShortNode:
		DoForGatherChildren(n.Val, onChild)
	case *FullNode:
		for i := 0; i < 16; i++ {
			DoForGatherChildren(n.Children[i], onChild)
		}
	case HashNode:
		onChild(common.BytesToHash(n))
	case ValueNode, nil:
	default:
		panic(fmt.Sprintf("unknown node type: %T", n))
	}
}
