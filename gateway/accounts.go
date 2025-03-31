package gateway

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	paymentpb "github.com/jahnu05/Assignment-2/P-3/protofiles"
)

type registeredUser struct {
	password string
	bank     string
}

// Register registers a new user.
func (s *PaymentGatewayServer) Register(ctx context.Context, req *paymentpb.RegisterRequest) (*paymentpb.RegisterResponse, error) {
	log.Printf("Registering user: %s for bank: %s", req.Username, req.BankName)
	s.users.Store(req.Username, registeredUser{password: req.Password, bank: req.BankName})
	return &paymentpb.RegisterResponse{Success: true, Message: "User registered successfully"}, nil
}

// Unregister removes a user from the gateway’s registry.
func (s *PaymentGatewayServer) Unregister(ctx context.Context, req *paymentpb.UnregisterRequest) (*paymentpb.UnregisterResponse, error) {
	_, exists := s.users.Load(req.Username)
	if !exists {
		return &paymentpb.UnregisterResponse{Success: false, Message: "User not registered"}, nil
	}
	s.users.Delete(req.Username)
	log.Printf("User %s unregistered", req.Username)
	return &paymentpb.UnregisterResponse{Success: true, Message: "User unregistered successfully"}, nil
}

// GetBalance queries the registered user's bank server for the updated balance.
func (s *PaymentGatewayServer) GetBalance(ctx context.Context, req *paymentpb.BalanceRequest) (*paymentpb.BalanceResponse, error) {
	log.Printf("GetBalance called for user: %s", req.Username)
	val, exists := s.users.Load(req.Username)
	if !exists {
		return nil, fmt.Errorf("user not registered")
	}
	regUser := val.(registeredUser)
	conn, err := grpc.Dial(regUser.bank, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to bank server: %v", err)
	}
	defer conn.Close()
	bankClient := paymentpb.NewBankServiceClient(conn)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	bResp, err := bankClient.GetBalance(ctx2, &paymentpb.GetBalanceRequest{Username: req.Username})
	if err != nil {
		return nil, fmt.Errorf("error from bank server: %v", err)
	}
	return &paymentpb.BalanceResponse{Balance: bResp.Balance}, nil
}
