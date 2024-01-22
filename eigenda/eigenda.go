package eigenda

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"time"

	ctypes "github.com/0xPolygon/cdk-data-availability/config/types"
	"github.com/Layr-Labs/eigenda/api/grpc/disperser"
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
}

type EigenDA interface {
	Get(ctx context.Context, ref *EigenDARef) ([]byte, error)
	Put(ctx context.Context, data []byte) (*EigenDARef, error)
}

type EigenDARef struct {
	BatchHeaderHash []byte `json:"batchHeaderHash"`
	BlobIndex       uint32 `json:"blobIndex"`
}

var _ EigenDA = (*eigenda)(nil)

type eigenda struct {
	cfg    Config
	client disperser.DisperserClient
}

func New(cfg Config) (*eigenda, error) {
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
	return &eigenda{
		client: disperser.NewDisperserClient(conn),
		cfg:    cfg,
	}, nil
}

func (e *eigenda) Get(ctx context.Context, ref *EigenDARef) ([]byte, error) {
	reply, err := e.client.RetrieveBlob(ctx, &disperser.RetrieveBlobRequest{
		BatchHeaderHash: ref.BatchHeaderHash,
		BlobIndex:       ref.BlobIndex,
	})
	if err != nil {
		return nil, err
	}
	return reply.Data, nil
}

func (e *eigenda) Put(ctx context.Context, data []byte) (*EigenDARef, error) {
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
		return nil, err
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
				continue
			}
			switch statusReply.GetStatus() {
			case disperser.BlobStatus_CONFIRMED, disperser.BlobStatus_FINALIZED:
				return &EigenDARef{
					BatchHeaderHash: statusReply.Info.BlobVerificationProof.BatchMetadata.BatchHeaderHash,
					BlobIndex:       statusReply.Info.BlobVerificationProof.BlobIndex,
				}, nil
			case disperser.BlobStatus_FAILED:
				return nil, fmt.Errorf("eigenda blob dispersal failed requestID %s", base64RequestID)
			default:
				continue
			}
		case <-c.Done():
			return nil, fmt.Errorf("eigenda blob dispersal timed out requestID: %s", base64RequestID)
		}
	}
}
