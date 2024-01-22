package sync

import (
	"context"
	"encoding/json"

	"github.com/0xPolygon/cdk-data-availability/db"
	"github.com/0xPolygon/cdk-data-availability/eigenda"
	"github.com/0xPolygon/cdk-data-availability/log"
	"github.com/0xPolygon/cdk-data-availability/rpc"
	"github.com/0xPolygon/cdk-data-availability/types"
)

// APISYNC  is the namespace of the sync service
const APISYNC = "sync"

// SyncEndpoints contains implementations for the "zkevm" RPC endpoints
type SyncEndpoints struct {
	eigenda eigenda.EigenDA
	db      db.DB
	txMan   rpc.DBTxManager
}

// NewSyncEndpoints returns ZKEVMEndpoints
func NewSyncEndpoints(db db.DB, eigenda eigenda.EigenDA) *SyncEndpoints {
	return &SyncEndpoints{
		db:      db,
		eigenda: eigenda,
	}
}

// GetOffChainData returns the image of the given hash
func (z *SyncEndpoints) GetOffChainData(hash types.ArgHash) (interface{}, rpc.Error) {
	if refData, err := z.txMan.NewDbTxScope(z.db, func(ctx context.Context, dbTx db.Tx) (interface{}, rpc.Error) {
		data, err := z.db.GetOffChainData(ctx, hash.Hash(), dbTx)
		if err != nil {
			log.Errorf("failed to get the offchain requested data from the DB: %v", err)
			return "0x0", rpc.NewRPCError(rpc.DefaultErrorCode, "failed to get the requested data")
		}

		return data, nil
	}); err != nil {
		return "0x0", err
	} else {
		var ref eigenda.EigenDARef
		if err := json.Unmarshal(refData.(types.ArgBytes), &ref); err != nil {
			return "0x0", rpc.NewRPCError(rpc.DefaultErrorCode, "failed to unmarshal eigenda ref")
		}

		data, err := z.eigenda.Get(context.Background(), &ref)
		if err != nil {
			log.Errorf("failed to get data from eigenda: %v", err)
			return "0x0", rpc.NewRPCError(rpc.DefaultErrorCode, "failed to get data from eigenda")
		}

		return types.ArgBytes(data), nil
	}
}
