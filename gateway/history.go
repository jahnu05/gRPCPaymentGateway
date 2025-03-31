package gateway

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	paymentpb "github.com/jahnu05/Assignment-2/P-3/protofiles"
)

// GetTransactionHistory returns all transaction records involving the given user.
func (s *PaymentGatewayServer) GetTransactionHistory(ctx context.Context, req *paymentpb.HistoryRequest) (*paymentpb.HistoryResponse, error) {
	log.Printf("GetTransactionHistory called for user: %s", req.Username)
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(16, "missing metadata")
	}
	usernames := md["username"]
	if len(usernames) == 0 || usernames[0] != req.Username {
		return nil, status.Errorf(7, "unauthorized access")
	}

	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	var records []TransactionRecord
	data, err := ioutil.ReadFile(s.historyFile)
	if err == nil {
		json.Unmarshal(data, &records)
	}
	var filtered []TransactionRecord
	for _, rec := range records {
		if rec.Sender == req.Username || rec.Receiver == req.Username {
			filtered = append(filtered, rec)
		}
	}

	var recordsProto []*paymentpb.TransactionRecord
	for _, rec := range filtered {
		r := rec
		recordProto := &paymentpb.TransactionRecord{
			TransactionId: r.TransactionId,
			Sender:        r.Sender,
			Receiver:      r.Receiver,
			Amount:        r.Amount,
			Timestamp:     r.Timestamp,
			Message:       r.Message,
		}
		recordsProto = append(recordsProto, recordProto)
	}

	return &paymentpb.HistoryResponse{Records: recordsProto}, nil
}
