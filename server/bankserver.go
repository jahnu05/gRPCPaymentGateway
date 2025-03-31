package main

import (
	"context"
	"log"
	"sync"
	"time"

	paymentpb "github.com/jahnu05/Assignment-2/P-3/protofiles"
)

// BankServer implements the BankService.
type BankServer struct {
	paymentpb.UnimplementedBankServiceServer
	accounts map[string]*Account
	mu       sync.Mutex
	bankName string
	filename string // JSON file for persistence.
}

// PreparePayment checks that the account exists and (if sender) has sufficient funds.
func (s *BankServer) PreparePayment(ctx context.Context, req *paymentpb.PrepareRequest) (*paymentpb.PrepareResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	acc, ok := s.accounts[req.Account]
	if !ok {
		return &paymentpb.PrepareResponse{Vote: false, Message: "Account not found"}, nil
	}
	if acc.Balance < req.Amount {
		return &paymentpb.PrepareResponse{Vote: false, Message: "Insufficient funds"}, nil
	}
	log.Printf("Bank %s: Prepared transaction %s for account %s", s.bankName, req.TransactionId, req.Account)
	return &paymentpb.PrepareResponse{Vote: true, Message: "Prepared successfully"}, nil
}

// CommitPayment applies the transaction and persists updated balances.
func (s *BankServer) CommitPayment(ctx context.Context, req *paymentpb.CommitRequest) (*paymentpb.CommitResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	acc, ok := s.accounts[req.Account]
	if !ok {
		return &paymentpb.CommitResponse{Success: false, Message: "Account not found"}, nil
	}
	if req.IsSender {
		if acc.Balance < req.Amount {
			return &paymentpb.CommitResponse{Success: false, Message: "Insufficient funds on commit"}, nil
		}
		acc.Balance -= req.Amount
		log.Printf("Bank %s: Committed transaction %s. Account %s new balance: %.2f (deducted)", s.bankName, req.TransactionId, acc.Username, acc.Balance)
	} else {
		acc.Balance += req.Amount
		log.Printf("Bank %s: Committed transaction %s. Account %s new balance: %.2f (credited)", s.bankName, req.TransactionId, acc.Username, acc.Balance)
	}
	if err := s.persistAccounts(); err != nil {
		log.Printf("Bank %s: Error persisting accounts: %v", s.bankName, err)
	}
	return &paymentpb.CommitResponse{Success: true, Message: "Commit successful"}, nil
}

// AbortPayment handles a transaction abort with a configurable timeout.
func (s *BankServer) AbortPayment(ctx context.Context, req *paymentpb.AbortRequest) (*paymentpb.AbortResponse, error) {
	log.Printf("Bank %s: Initiating abort for transaction %s", s.bankName, req.TransactionId)
	// Wait for the configured timeout duration before completing the abort.
	// This timeout can be adjusted via the command-line flag.
	select {
	case <-time.After(abortTimeout):
		log.Printf("Bank %s: Aborted transaction %s after timeout of %v", s.bankName, req.TransactionId, abortTimeout)
		return &paymentpb.AbortResponse{Success: true, Message: "Abort processed after timeout"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetBalance returns the current balance for the specified account.
func (s *BankServer) GetBalance(ctx context.Context, req *paymentpb.GetBalanceRequest) (*paymentpb.GetBalanceResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	acc, ok := s.accounts[req.Username]
	if !ok {
		return nil, fmt.Errorf("account not found")
	}
	log.Printf("Bank %s: Returning balance for account %s: %.2f", s.bankName, req.Username, acc.Balance)
	return &paymentpb.GetBalanceResponse{Balance: acc.Balance}, nil
}