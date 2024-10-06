package trie

import (
	"fmt"
	"io"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

var Indices = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e", "f", "[17]"}

type Node interface {
	Cache() (HashNode, bool)
	Encode(w rlp.EncoderBuffer)
	Fstring(string) string
}

type (
	FullNode struct {
		// TODO: Why the length of the array is 17? not 16?
		Children [17]Node // Actual trie node data to encode/decode (needs custom encoder)
		Flags    NodeFlag
	}
	ShortNode struct {
		Key   []byte
		Val   Node
		Flags NodeFlag
	}
	HashNode  []byte
	ValueNode []byte
)

// NilValueNode is used when collapsing internal trie nodes for hashing, since
// unset children need to serialize correctly.
var NilValueNode = ValueNode(nil)

// EncodeRLP encodes a full node into the consensus RLP format.
func (n *FullNode) EncodeRLP(w io.Writer) error {
	eb := rlp.NewEncoderBuffer(w)
	n.Encode(eb)
	return eb.Flush()
}
func (n *FullNode) Copy() *FullNode   { copy := *n; return &copy }
func (n *ShortNode) Copy() *ShortNode { copy := *n; return &copy }

// NodeFlag contains caching-related metadata about a node.
type NodeFlag struct {
	Hash  HashNode // cached hash of the node (may be nil)
	Dirty bool     // whether the node has changes that must be written to the database
}

func (n *FullNode) Cache() (HashNode, bool)  { return n.Flags.Hash, n.Flags.Dirty }
func (n *ShortNode) Cache() (HashNode, bool) { return n.Flags.Hash, n.Flags.Dirty }
func (n HashNode) Cache() (HashNode, bool)   { return nil, true }
func (n ValueNode) Cache() (HashNode, bool)  { return nil, true }

// Pretty printing.
func (n *FullNode) Fstring(ind string) string {
	resp := fmt.Sprintf("[\n%s  ", ind)
	for i, node := range &n.Children {
		if node == nil {
			resp += fmt.Sprintf("%s: <nil> ", Indices[i])
		} else {
			resp += fmt.Sprintf("%s: %v", Indices[i], node.Fstring(ind+"  "))
		}
	}
	return resp + fmt.Sprintf("\n%s] ", ind)
}
func (n *ShortNode) Fstring(ind string) string {
	return fmt.Sprintf("{%x: %v} ", n.Key, n.Val.Fstring(ind+"  "))
}
func (n HashNode) Fstring(ind string) string {
	return fmt.Sprintf("<%x> ", []byte(n))
}
func (n ValueNode) Fstring(ind string) string {
	return fmt.Sprintf("%x ", []byte(n))
}

// RawNode is a simple binary blob used to differentiate between collapsed trie
// nodes and already encoded RLP binary blobs (while at the same time store them
// in the same cache fields).
type RawNode []byte

func (n RawNode) Cache() (HashNode, bool)   { panic("this should never end up in a live trie") }
func (n RawNode) Fstring(ind string) string { panic("this should never end up in a live trie") }

func (n RawNode) EncodeRLP(w io.Writer) error {
	_, err := w.Write(n)
	return err
}

// MustDecodeNode is a wrapper of DecodeNode and panic if any error is encountered.
func MustDecodeNode(hash, buf []byte) Node {
	n, err := DecodeNode(hash, buf)
	if err != nil {
		panic(fmt.Sprintf("node %x: %v", hash, err))
	}
	return n
}

// MustDecodeNodeUnsafe is a wrapper of DecodeNodeUnsafe and panic if any error is
// encountered.
func MustDecodeNodeUnsafe(hash, buf []byte) Node {
	n, err := DecodeNodeUnsafe(hash, buf)
	if err != nil {
		panic(fmt.Sprintf("node %x: %v", hash, err))
	}
	return n
}

// DecodeNode parses the RLP encoding of a trie node. It will deep-copy the passed
// byte slice for decoding, so it's safe to modify the byte slice afterwards. The-
// decode performance of this function is not optimal, but it is suitable for most
// scenarios with low performance requirements and hard to determine whether the
// byte slice be modified or not.
func DecodeNode(hash, buf []byte) (Node, error) {
	return DecodeNodeUnsafe(hash, common.CopyBytes(buf))
}

// DecodeNodeUnsafe parses the RLP encoding of a trie node. The passed byte slice
// will be directly referenced by node without bytes deep copy, so the input MUST
// not be changed after.
func DecodeNodeUnsafe(hash, buf []byte) (Node, error) {
	if len(buf) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	elems, _, err := rlp.SplitList(buf)
	if err != nil {
		return nil, fmt.Errorf("decode error: %v", err)
	}
	switch c, _ := rlp.CountValues(elems); c {
	case 2:
		n, err := DecodeShort(hash, elems)
		return n, WrapError(err, "short")
	case 17:
		n, err := DecodeFull(hash, elems)
		return n, WrapError(err, "full")
	default:
		return nil, fmt.Errorf("invalid number of list elements: %v", c)
	}
}

func DecodeShort(hash, elems []byte) (Node, error) {
	kbuf, rest, err := rlp.SplitString(elems)
	if err != nil {
		return nil, err
	}
	flag := NodeFlag{Hash: hash}
	key := CompactToHex(kbuf)
	if HasTerm(key) {
		// value node
		val, _, err := rlp.SplitString(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid value node: %v", err)
		}
		return &ShortNode{key, ValueNode(val), flag}, nil
	}
	r, _, err := DecodeRef(rest)
	if err != nil {
		return nil, WrapError(err, "val")
	}
	return &ShortNode{key, r, flag}, nil
}

func DecodeFull(hash, elems []byte) (*FullNode, error) {
	n := &FullNode{Flags: NodeFlag{Hash: hash}}
	for i := 0; i < 16; i++ {
		cld, rest, err := DecodeRef(elems)
		if err != nil {
			return n, WrapError(err, fmt.Sprintf("[%d]", i))
		}
		n.Children[i], elems = cld, rest
	}
	val, _, err := rlp.SplitString(elems)
	if err != nil {
		return n, err
	}
	if len(val) > 0 {
		n.Children[16] = ValueNode(val)
	}
	return n, nil
}

const HashLen = len(common.Hash{})

func DecodeRef(buf []byte) (Node, []byte, error) {
	kind, val, rest, err := rlp.Split(buf)
	if err != nil {
		return nil, buf, err
	}
	switch {
	case kind == rlp.List:
		// 'embedded' node reference. The encoding must be smaller
		// than a hash in order to be valid.
		if size := len(buf) - len(rest); size > HashLen {
			err := fmt.Errorf("oversized embedded node (size is %d bytes, want size < %d)", size, HashLen)
			return nil, buf, err
		}
		n, err := DecodeNode(nil, buf)
		return n, rest, err
	case kind == rlp.String && len(val) == 0:
		// empty node
		return nil, rest, nil
	case kind == rlp.String && len(val) == 32:
		return HashNode(val), rest, nil
	default:
		return nil, nil, fmt.Errorf("invalid RLP string size %d (want 0 or 32)", len(val))
	}
}

// wraps a decoding error with information about the path to the
// invalid child node (for debugging encoding issues).
type DecodeError struct {
	what  error
	stack []string
}

func WrapError(err error, ctx string) error {
	if err == nil {
		return nil
	}
	if decErr, ok := err.(*DecodeError); ok {
		decErr.stack = append(decErr.stack, ctx)
		return decErr
	}
	return &DecodeError{err, []string{ctx}}
}

func (err *DecodeError) Error() string {
	return fmt.Sprintf("%v (decode path: %s)", err.what, strings.Join(err.stack, "<-"))
}
