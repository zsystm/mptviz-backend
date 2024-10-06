package trie

import "github.com/ethereum/go-ethereum/rlp"

func nodeToBytes(n Node) []byte {
	w := rlp.NewEncoderBuffer(nil)
	n.Encode(w)
	result := w.ToBytes()
	w.Flush()
	return result
}

func (n *FullNode) Encode(w rlp.EncoderBuffer) {
	offset := w.List()
	for _, c := range n.Children {
		if c != nil {
			c.Encode(w)
		} else {
			w.Write(rlp.EmptyString)
		}
	}
	w.ListEnd(offset)
}

func (n *ShortNode) Encode(w rlp.EncoderBuffer) {
	offset := w.List()
	w.WriteBytes(n.Key)
	if n.Val != nil {
		n.Val.Encode(w)
	} else {
		w.Write(rlp.EmptyString)
	}
	w.ListEnd(offset)
}

func (n HashNode) Encode(w rlp.EncoderBuffer) {
	w.WriteBytes(n)
}

func (n ValueNode) Encode(w rlp.EncoderBuffer) {
	w.WriteBytes(n)
}

func (n RawNode) Encode(w rlp.EncoderBuffer) {
	w.Write(n)
}
