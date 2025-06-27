package types

import sdk "github.com/cosmos/cosmos-sdk/types"

var _ BlockDescription = (*BlockInfo)(nil)

type BlockInfo struct {
	height    uint64
	Hash      []byte
	Finalized bool
}

func NewBlockInfo(height uint64, hash []byte, finalized bool) *BlockInfo {
	return &BlockInfo{
		height:    height,
		Hash:      hash,
		Finalized: finalized,
	}
}

func (b BlockInfo) GetHeight() uint64 {
	return b.height
}

func (b BlockInfo) GetHash() []byte {
	return b.Hash
}

func (b BlockInfo) IsFinalized() bool {
	return b.Finalized
}

func (b BlockInfo) MsgToSign() []byte {
	return append(sdk.Uint64ToBigEndian(b.height), b.Hash...)
}
