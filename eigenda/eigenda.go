package eigenda

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	ctypes "github.com/0xPolygon/cdk-data-availability/config/types"
	"github.com/0xPolygon/cdk-data-availability/db"
	"github.com/0xPolygon/cdk-data-availability/log"
	"github.com/0xPolygon/cdk-data-availability/types"
	"github.com/Layr-Labs/eigenda/api/grpc/disperser"
	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Config is a struct that defines EigenDA settings
type Config struct {
	EigenDARpc                      string          `mapstructure:"EigenDARpc"`
	EigenDARpcInsecure              bool            `mapstructure:"EigenDARpcInsecure"`
	EigenDAQuorumID                 uint32          `mapstructure:"EigenDAQuorumID"`
	EigenDAAdversaryThreshold       uint32          `mapstructure:"EigenDAAdversaryThreshold"`
	EigenDAQuorumThreshold          uint32          `mapstructure:"EigenDAQuorumThreshold"`
	EigenDAStatusQueryRetryInterval ctypes.Duration `mapstructure:"EigenDAStatusQueryRetryInterval"`
	EigenDAStatusQueryTimeout       ctypes.Duration `mapstructure:"EigenDAStatusQueryTimeout"`
	NumWorker                       uint32          `mapstructure:"EigenDANumWorker"`
}

type EigenDA interface {
	Start()
	Stop()
	// Process handle store data into eigenda, it may fail, but we can always recover it from postgres
	// in the future, we may want to add retry and queue mechanism to make sure all data stored in eigenda
	Process(key common.Hash)
}

type eigenda struct {
	cfg    Config
	client disperser.DisperserClient
	db     *db.DB
	ch     chan common.Hash
	wg     sync.WaitGroup
}

var _ EigenDA = (*eigenda)(nil)

func New(db *db.DB, cfg Config) (*eigenda, error) {
	var opts []grpc.DialOption
	if cfg.EigenDARpcInsecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		creds := credentials.NewTLS(&tls.Config{})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}
	conn, err := grpc.Dial(cfg.EigenDARpc, opts...)
	if err != nil {
		return nil, err
	}
	d := &eigenda{
		client: disperser.NewDisperserClient(conn),
		cfg:    cfg,
		db:     db,
		ch:     make(chan common.Hash),
	}
	return d, nil
}

func (e *eigenda) Process(key common.Hash) {
	e.ch <- key
}

func (e *eigenda) Start() {
	for i := uint32(0); i < e.cfg.NumWorker; i++ {
		e.wg.Add(1)
		i := i
		go func() {
			defer e.wg.Done()
			for key := range e.ch {
				// when calling the Stop method, it will send `NumWorker` empty hash to the channel
				// so we need to check if the key is empty, then we return and close this goroutine
				if key.Hex() == (common.Hash{}).Hex() {
					log.Infof("[eigenda] worker stopped", i)
					return
				}
				log.Infof("[eigenda] worker %d processing key %s", i, key.Hex())
				tx, err := e.db.BeginStateTransaction(context.Background())
				if err != nil {
					log.Errorf("[eigenda] failed to begin state transaction: %v", err)
					continue
				}
				data, err := e.db.GetOffChainData(context.Background(), key, tx)
				if err != nil {
					log.Errorf("[eigenda] failed to get offchain data from db: %v", err)
					continue
				}
				batchHeaderHash, blobIndex, err := e.put(context.Background(), data)
				if err != nil {
					log.Errorf("[eigenda] failed to put data to eigenda: %v", err)
					continue
				}
				if err = e.db.StoreEigenDARef(context.Background(),
					types.EigenDARef{
						Key:             key,
						BatchHeaderHash: batchHeaderHash,
						BlobIndex:       blobIndex,
					}, tx,
				); err != nil {
					log.Errorf("[eigenda] failed to store eigenDA ref to db: %v", err)
					continue
				}
				log.Infof("[eigenda] worker %d stored key %s with batchHeaderHash %s and blobIndex %d", i, key.Hex(), common.Bytes2Hex(batchHeaderHash), blobIndex)
			}
		}()
	}
}

func (e *eigenda) Stop() {
	for i := uint32(0); i < e.cfg.NumWorker; i++ {
		e.ch <- common.Hash{}
	}
	log.Info("[eigenda] waiting for workers to stop")
	e.wg.Wait()
	log.Info("[eigenda] all workers stopped")
}

func (e *eigenda) put(ctx context.Context, data []byte) (batchHeaderHash []byte, blobIndex uint32, err error) {
	disperseReq := &disperser.DisperseBlobRequest{
		Data: data,
		SecurityParams: []*disperser.SecurityParams{
			{
				QuorumId:           e.cfg.EigenDAQuorumID,
				AdversaryThreshold: e.cfg.EigenDAAdversaryThreshold,
				QuorumThreshold:    e.cfg.EigenDAQuorumThreshold,
			},
		},
	}
	disperseRes, err := e.client.DisperseBlob(ctx, disperseReq)
	if err != nil {
		return nil, 0, err
	}
	base64RequestID := base64.StdEncoding.EncodeToString(disperseRes.RequestId)
	ticker := time.NewTicker(e.cfg.EigenDAStatusQueryRetryInterval.Duration)
	defer ticker.Stop()
	c, cancel := context.WithTimeout(ctx, e.cfg.EigenDAStatusQueryTimeout.Duration)
	defer cancel()
	for {
		select {
		case <-ticker.C:
			statusReply, err := e.client.GetBlobStatus(ctx, &disperser.BlobStatusRequest{
				RequestId: disperseRes.RequestId,
			})
			if err != nil {
				log.Warnf("[eigenda] failed to get blob status requestID %s: %v, will retry", base64RequestID, err)
				continue
			}
			switch statusReply.GetStatus() {
			case disperser.BlobStatus_CONFIRMED, disperser.BlobStatus_FINALIZED:
				return statusReply.Info.BlobVerificationProof.BatchMetadata.BatchHeaderHash,
					statusReply.Info.BlobVerificationProof.BlobIndex,
					nil
			case disperser.BlobStatus_FAILED:
				return nil, 0, fmt.Errorf("blob dispersal failed requestID %s", base64RequestID)
			default:
				continue
			}
		case <-c.Done():
			return nil, 0, fmt.Errorf("blob dispersal timed out requestID: %s", base64RequestID)
		}
	}
}
