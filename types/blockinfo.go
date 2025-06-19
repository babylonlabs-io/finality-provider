package types

import sdk "github.com/cosmos/cosmos-sdk/types"

var _ BlockDescription = (*BlockInfo)(nil)

type BlockInfo struct {
	Height    uint64
	Hash      []byte
	Finalized bool
}

func (b BlockInfo) GetHeight() uint64 {
	return b.Height
}

func (b BlockInfo) GetHash() []byte {
	return b.Hash
}

func (b BlockInfo) IsFinalized() bool {
	return b.Finalized
}

func (b BlockInfo) MsgToSign() []byte {
	return append(sdk.Uint64ToBigEndian(b.Height), b.Hash...)
}
