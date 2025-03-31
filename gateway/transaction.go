package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentpb "github.com/jahnu05/Assignment-2/P-3/protofiles"
)

// TransactionRecord defines the structure for a transaction history record.
type TransactionRecord struct {
	TransactionId string  `json:"transactionId"`
	Sender        string  `json:"sender"`
	Receiver      string  `json:"receiver"`
	Amount        float64 `json:"amount"`
	Timestamp     string  `json:"timestamp"`
	Message       string  `json:"message"`
}

// storeTransactionRecord appends a transaction record to the persistent JSON file.
func (s *PaymentGatewayServer) storeTransactionRecord(record TransactionRecord) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()

	var records []TransactionRecord
	if _, err := os.Stat(s.historyFile); err == nil {
		data, err := ioutil.ReadFile(s.historyFile)
		if err == nil {
			json.Unmarshal(data, &records)
		}
	}
	records = append(records, record)
	newData, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		log.Printf("Error marshalling transaction history: %v", err)
		return
	}
	if err := ioutil.WriteFile(s.historyFile, newData, 0644); err != nil {
		log.Printf("Error writing transaction history: %v", err)
	}
}

// ProcessPayment implements idempotency and two-phase commit.
func (s *PaymentGatewayServer) ProcessPayment(ctx context.Context, req *paymentpb.TransactionRequest) (*paymentpb.TransactionResponse, error) {
	// First, verify that both sender and receiver are registered.
	_, senderRegistered := s.users.Load(req.SenderUsername)
	_, receiverRegistered := s.users.Load(req.ReceiverUsername)
	if !senderRegistered || !receiverRegistered {
		return nil, status.Errorf(codes.FailedPrecondition, "One or both users are not registered")
	}

	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" {
		return nil, status.Errorf(codes.InvalidArgument, "IdempotencyKey must be provided")
	}
	// Check if the transaction has already been processed.
	if result, exists := s.processedTxs.Load(idempotencyKey); exists {
		msg := fmt.Sprintf("Transaction already processed: %v", result)
		log.Println(msg)
		return &paymentpb.TransactionResponse{Success: true, Message: msg}, nil
	}
	s.processedTxs.Store(idempotencyKey, false)
	log.Printf("Processing transaction with idempotency key: %s", idempotencyKey)

	// Connect to bank servers (using insecure connections for internal communication).
	senderConn, err := grpc.Dial(req.SenderBank, grpc.WithInsecure())
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "Error connecting to sender bank: %v", err)
	}
	defer senderConn.Close()
	senderClient := paymentpb.NewBankServiceClient(senderConn)

	receiverConn, err := grpc.Dial(req.ReceiverBank, grpc.WithInsecure())
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "Error connecting to receiver bank: %v", err)
	}
	defer receiverConn.Close()
	receiverClient := paymentpb.NewBankServiceClient(receiverConn)

	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Phase 1: Prepare on sender.
	senderPrep, err := senderClient.PreparePayment(ctx2, &paymentpb.PrepareRequest{
		TransactionId: req.TransactionId,
		Account:       req.SenderUsername,
		Amount:        req.Amount,
	})
	if err != nil || !senderPrep.Vote {
		s.processedTxs.Store(idempotencyKey, false)
		return nil, status.Errorf(codes.Aborted, "Sender bank aborted the transaction")
	}

	// Phase 1: Prepare on receiver.
	receiverPrep, err := receiverClient.PreparePayment(ctx2, &paymentpb.PrepareRequest{
		TransactionId: req.TransactionId,
		Account:       req.ReceiverUsername,
		Amount:        req.Amount,
	})
	if err != nil || !receiverPrep.Vote {
		s.processedTxs.Store(idempotencyKey, false)
		// Abort on sender side.
		senderClient.AbortPayment(ctx2, &paymentpb.AbortRequest{TransactionId: req.TransactionId})
		return nil, status.Errorf(codes.Aborted, "Receiver bank aborted the transaction")
	}

	// Phase 2: Commit on sender.
	senderCommit, err := senderClient.CommitPayment(ctx2, &paymentpb.CommitRequest{
		TransactionId: req.TransactionId,
		Account:       req.SenderUsername,
		Amount:        req.Amount,
		IsSender:      true,
	})
	if err != nil || !senderCommit.Success {
		s.processedTxs.Store(idempotencyKey, false)
		return nil, status.Errorf(codes.Aborted, "Sender bank commit failed")
	}

	// Phase 2: Commit on receiver.
	receiverCommit, err := receiverClient.CommitPayment(ctx2, &paymentpb.CommitRequest{
		TransactionId: req.TransactionId,
		Account:       req.ReceiverUsername,
		Amount:        req.Amount,
		IsSender:      false,
	})
	if err != nil || !receiverCommit.Success {
		s.processedTxs.Store(idempotencyKey, false)
		return nil, status.Errorf(codes.Aborted, "Receiver bank commit failed")
	}

	s.processedTxs.Store(idempotencyKey, true)
	record := TransactionRecord{
		TransactionId: req.TransactionId,
		Sender:        req.SenderUsername,
		Receiver:      req.ReceiverUsername,
		Amount:        req.Amount,
		Timestamp:     time.Now().Format(time.RFC3339),
		Message:       "Transaction committed successfully",
	}
	s.storeTransactionRecord(record)

	return &paymentpb.TransactionResponse{Success: true, Message: "Transaction committed successfully"}, nil
}
