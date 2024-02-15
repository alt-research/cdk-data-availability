package types

import "github.com/ethereum/go-ethereum/common"

type EigenDARef struct {
	Key             common.Hash
	BatchHeaderHash []byte
	BlobIndex       uint32
}
