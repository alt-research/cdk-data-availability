package datacom

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/0xPolygon/cdk-data-availability/db"
	"github.com/0xPolygon/cdk-data-availability/eigenda"
	"github.com/0xPolygon/cdk-data-availability/log"
	"github.com/0xPolygon/cdk-data-availability/rpc"
	"github.com/0xPolygon/cdk-data-availability/sequencer"
	"github.com/0xPolygon/cdk-data-availability/types"
	"github.com/ethereum/go-ethereum/common"

	"golang.org/x/sync/errgroup"
)

// APIDATACOM is the namespace of the datacom service
const APIDATACOM = "datacom"

// DataComEndpoints contains implementations for the "datacom" RPC endpoints
type DataComEndpoints struct {
	eigenda          eigenda.EigenDA
	db               db.DB
	txMan            rpc.DBTxManager
	privateKey       *ecdsa.PrivateKey
	sequencerTracker *sequencer.Tracker
}

// NewDataComEndpoints returns DataComEndpoints
func NewDataComEndpoints(
	db db.DB, privateKey *ecdsa.PrivateKey, sequencerTracker *sequencer.Tracker, eigenda eigenda.EigenDA,
) *DataComEndpoints {
	return &DataComEndpoints{
		eigenda:          eigenda,
		db:               db,
		privateKey:       privateKey,
		sequencerTracker: sequencerTracker,
	}
}

// SignSequence generates the accumulated input hash aka accInputHash of the sequence and sign it.
// After storing the data that will be sent hashed to the contract, it returns the signature.
// This endpoint is only accessible to the sequencer
func (d *DataComEndpoints) SignSequence(signedSequence types.SignedSequence) (interface{}, rpc.Error) {
	// Verify that the request comes from the sequencer
	sender, err := signedSequence.Signer()
	if err != nil {
		return "0x0", rpc.NewRPCError(rpc.DefaultErrorCode, "failed to verify sender")
	}
	if sender != d.sequencerTracker.GetAddr() {
		return "0x0", rpc.NewRPCError(rpc.DefaultErrorCode, "unauthorized")
	}

	var (
		offChainData = signedSequence.Sequence.OffChainData()
		refs         = make(map[common.Hash]*eigenda.EigenDARef, len(offChainData))
		mu           sync.Mutex
		eg, ctx      = errgroup.WithContext(context.Background())
	)

	for _, data := range offChainData {
		data := data
		eg.Go(func() error {
			ref, err := d.eigenda.Put(ctx, data.Value)
			if err != nil {
				return err
			}
			mu.Lock()
			refs[data.Key] = ref
			mu.Unlock()
			return nil
		})
	}
	if err = eg.Wait(); err != nil {
		log.Errorf("failed to store data in eigenda: %v", err)
		return "0x0", rpc.NewRPCError(rpc.DefaultErrorCode, "failed to store data in eigenda")
	}

	var eigendaRefs []types.OffChainData
	for key, ref := range refs {
		b, _ := json.Marshal(ref)
		eigendaRefs = append(eigendaRefs, types.OffChainData{
			Key:   key,
			Value: b,
		})
	}

	// Store off-chain data by hash (hash(L2Data): L2Data)
	_, err = d.txMan.NewDbTxScope(d.db, func(ctx context.Context, dbTx db.Tx) (interface{}, rpc.Error) {
		err := d.db.StoreOffChainData(ctx, eigendaRefs, dbTx)
		if err != nil {
			return "0x0", rpc.NewRPCError(rpc.DefaultErrorCode, fmt.Errorf("failed to store offchain data. Error: %w", err).Error())
		}

		return nil, nil
	})
	if err != nil {
		return "0x0", rpc.NewRPCError(rpc.DefaultErrorCode, err.Error())
	}
	// Sign
	signedSequenceByMe, err := signedSequence.Sequence.Sign(d.privateKey)
	if err != nil {
		return "0x0", rpc.NewRPCError(rpc.DefaultErrorCode, fmt.Errorf("failed to sign. Error: %w", err).Error())
	}
	// Return signature
	return signedSequenceByMe.Signature, nil
}
