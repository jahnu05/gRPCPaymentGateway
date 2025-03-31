package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/jahnu05/Assignment-2/P-3/gateway"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/codes"

	paymentpb "github.com/jahnu05/Assignment-2/P-3/protofiles"
)

// PaymentGatewayServer implements the PaymentGateway service.
type PaymentGatewayServer struct {
	paymentpb.UnimplementedPaymentGatewayServer
	processedTxs sync.Map // stores outcome keyed by idempotency key
	users        sync.Map // stores registeredUser keyed by username

	// Mutex for transaction history file.
	historyMu sync.Mutex
	// Path to the transaction history JSON file.
	historyFile string
}

type registeredUser struct {
	password string
	bank     string
}

// TransactionRecord defines the structure for a transaction history record.
type TransactionRecord struct {
	TransactionId string  `json:"transactionId"`
	Sender        string  `json:"sender"`
	Receiver      string  `json:"receiver"`
	Amount        float64 `json:"amount"`
	Timestamp     string  `json:"timestamp"`
	Message       string  `json:"message"`
}

// Global pointer to the active gateway instance.
var gatewayInstance *PaymentGatewayServer

// // computeFingerprint creates an idempotency key from critical transaction parameters.
// func computeFingerprint(req *paymentpb.TransactionRequest) string {
// 	data := req.SenderUsername +
// 		req.ReceiverUsername +
// 		fmt.Sprintf("%.2f", req.Amount) +
// 		req.SenderBank +
// 		req.ReceiverBank
// 	hash := sha256.Sum256([]byte(data))
// 	return hex.EncodeToString(hash[:])
// }

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

// Register registers a new user.
func (s *PaymentGatewayServer) Register(ctx context.Context, req *paymentpb.RegisterRequest) (*paymentpb.RegisterResponse, error) {
	log.Printf("Registering user: %s for bank: %s", req.Username, req.BankName)
	s.users.Store(req.Username, registeredUser{password: req.Password, bank: req.BankName})
	return &paymentpb.RegisterResponse{Success: true, Message: "User registered successfully"}, nil
}
// Unregister removes a user from the gateway’s registry.
func (s *PaymentGatewayServer) Unregister(ctx context.Context, req *paymentpb.UnregisterRequest) (*paymentpb.UnregisterResponse, error) {
	// Authentication is handled by interceptors.
	_, exists := s.users.Load(req.Username)
	if !exists {
		return &paymentpb.UnregisterResponse{Success: false, Message: "User not registered"}, nil
	}
	s.users.Delete(req.Username)
	log.Printf("User %s unregistered", req.Username)
	return &paymentpb.UnregisterResponse{Success: true, Message: "User unregistered successfully"}, nil
}
// ProcessPayment implements idempotency and two-phase commit.
// ProcessPayment implements idempotency and two-phase commit.
// It uses the provided IdempotencyKey (expected to be a UUID) to check for duplicate transactions.
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

// GetBalance queries the registered user's bank server for the updated balance.
func (s *PaymentGatewayServer) GetBalance(ctx context.Context, req *paymentpb.BalanceRequest) (*paymentpb.BalanceResponse, error) {
	log.Printf("GetBalance called for user: %s", req.Username)
	// Lookup the user to get the bank address.
	val, exists := s.users.Load(req.Username)
	if !exists {
		return nil, fmt.Errorf("user not registered")
	}
	regUser := val.(registeredUser)
	// Dial the bank server (assumed insecure for internal communication).
	conn, err := grpc.Dial(regUser.bank, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to bank server: %v", err)
	}
	defer conn.Close()
	bankClient := paymentpb.NewBankServiceClient(conn)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Call the bank server's GetBalance RPC.
	bResp, err := bankClient.GetBalance(ctx2, &paymentpb.GetBalanceRequest{Username: req.Username})
	if err != nil {
		return nil, fmt.Errorf("error from bank server: %v", err)
	}
	return &paymentpb.BalanceResponse{Balance: bResp.Balance}, nil
}

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

	// Convert []TransactionRecord to []*paymentpb.TransactionRecord.
	var recordsProto []*paymentpb.TransactionRecord
	for _, rec := range filtered {
		r := rec // create a new variable for correct pointer reference
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


func loadTLSCredentials() credentials.TransportCredentials {
	// Load CA certificate.
	caCert, err := ioutil.ReadFile("certs/ca.crt")
	if err != nil {
		log.Fatalf("Could not read CA certificate: %v", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		log.Fatalf("Failed to append CA certificate")
	}

	// Load server certificate and key.
	serverCert, err := tls.LoadX509KeyPair("certs/server.crt", "certs/server.key")
	if err != nil {
		log.Fatalf("Could not load server certificate and key: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
	return credentials.NewTLS(tlsConfig)
}

func createGRPCServer(pgServer *gateway.PaymentGatewayServer, creds credentials.TransportCredentials) *grpc.Server {
	return grpc.NewServer(
		grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(
			gateway.AuthInterceptor,
			gateway.AuthorizationInterceptor,
			gateway.LoggingInterceptor,
		),
	)
}

func main() {
	historyFilePath := "transaction_history.json"

	// Load TLS credentials.
	creds := loadTLSCredentials()

	// Initialize the Payment Gateway server.
	pgServer := &gateway.PaymentGatewayServer{HistoryFile: historyFilePath}
	gatewayInstance = pgServer

	// Create and configure the gRPC server.
	grpcServer := createGRPCServer(pgServer, creds)

	// Register the Payment Gateway service.
	paymentpb.RegisterPaymentGatewayServer(grpcServer, pgServer)

	// Start listening on the specified port.
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen on :50051: %v", err)
	}

	log.Println("Secure Payment Gateway server started on :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
