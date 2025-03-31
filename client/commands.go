package commands

import (
    "context"
    "crypto/tls"
    "crypto/x509"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "strconv"
    "sync"
    "time"

    "github.com/google/uuid"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
    "google.golang.org/grpc/metadata"
    "google.golang.org/grpc/status"

    paymentpb "path/to/protofiles"
)

var (
    offlineQueue []PaymentTransaction
    queueMutex   sync.Mutex
)

// PaymentTransaction wraps a TransactionRequest.
type PaymentTransaction struct {
    req *paymentpb.TransactionRequest
}

// OfflineTransaction is used for persisting offline transactions.
type OfflineTransaction struct {
    TransactionId    string  `json:"transactionId"`
    SenderUsername   string  `json:"senderUsername"`
    ReceiverUsername string  `json:"receiverUsername"`
    Amount           float64 `json:"amount"`
    SenderBank       string  `json:"senderBank"`
    ReceiverBank     string  `json:"receiverBank"`
    IdempotencyKey   string  `json:"idempotencyKey"`
}

// LoadOfflineQueue reads the offline transactions from the JSON file and loads them into offlineQueue.
// saveOfflineQueue writes the current offline queue to a JSON file.
func saveOfflineQueue() {
	queueMutex.Lock()
	defer queueMutex.Unlock()

	var offlineList []OfflineTransaction
	for _, tx := range offlineQueue {
		offlineList = append(offlineList, OfflineTransaction{
			TransactionId:    tx.req.TransactionId,
			SenderUsername:   tx.req.SenderUsername,
			ReceiverUsername: tx.req.ReceiverUsername,
			Amount:           tx.req.Amount,
			SenderBank:       tx.req.SenderBank,
			ReceiverBank:     tx.req.ReceiverBank,
			IdempotencyKey:   tx.req.IdempotencyKey,
		})
	}
	data, err := json.MarshalIndent(offlineList, "", "  ")
	if err != nil {
		log.Printf("Error marshalling offline transactions: %v", err)
		return
	}
	if err := ioutil.WriteFile("pending_transactions.json", data, 0644); err != nil {
		log.Printf("Error writing offline transactions to file: %v", err)
	} else {
		log.Printf("Saved %d pending transactions to pending_transactions.json", len(offlineList))
	}
}

// loadOfflineQueue reads the offline transactions from the JSON file and loads them into offlineQueue.
func loadOfflineQueue(filename string) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("No pending transactions file found: %v", err)
		return
	}
	var offlineList []OfflineTransaction
	if err := json.Unmarshal(data, &offlineList); err != nil {
		log.Printf("Error unmarshalling pending transactions: %v", err)
		return
	}
	queueMutex.Lock()
	defer queueMutex.Unlock()
	offlineQueue = nil
	for _, offTx := range offlineList {
		tx := PaymentTransaction{
			req: &paymentpb.TransactionRequest{
				TransactionId:    offTx.TransactionId,
				SenderUsername:   offTx.SenderUsername,
				ReceiverUsername: offTx.ReceiverUsername,
				Amount:           offTx.Amount,
				SenderBank:       offTx.SenderBank,
				ReceiverBank:     offTx.ReceiverBank,
				IdempotencyKey:   offTx.IdempotencyKey,
			},
		}
		offlineQueue = append(offlineQueue, tx)
	}
	log.Printf("Loaded %d pending transactions from %s", len(offlineList), filename)
}

// getTLSCredentials loads the client's TLS credentials.
func getTLSCredentials() (credentials.TransportCredentials, error) {
	clientCert, err := tls.LoadX509KeyPair(*clientCertFile, *clientKeyFile)
	if err != nil {
		return nil, fmt.Errorf("cannot load client certificate: %w", err)
	}
	caCert, err := ioutil.ReadFile(*caCertFile)
	if err != nil {
		return nil, fmt.Errorf("cannot load CA certificate: %w", err)
	}
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("failed to append CA certificate")
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      certPool,
	}
	return credentials.NewTLS(tlsConfig), nil
}

// PrintUsage prints the usage instructions for the client.
func PrintUsage() {
    fmt.Println(`Usage:
  client register [gateway_address] [username] [password] [bankName]
  client pay [gateway_address] [sender_bank_address] [receiver_bank_address] [sender_username] [receiver_username] [amount]
  client getbalance [gateway_address] [username]
  client gethistory [gateway_address] [username]
  client unregister [gateway_address] [username]`)
}

// RegisterUser handles the registration command.
func RegisterUser(args []string, creds credentials.TransportCredentials) {
	if len(args) != 5 {
		fmt.Println("Usage: client register [gateway_address] [username] [password] [bankName]")
		return
	}
	gatewayAddr := args[1]
	username := args[2]
	password := args[3]
	bankName := args[4]

	conn, err := grpc.Dial(gatewayAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("Failed to connect to Payment Gateway: %v", err)
	}
	defer conn.Close()

	client := paymentpb.NewPaymentGatewayClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Register(ctx, &paymentpb.RegisterRequest{
		Username: username,
		Password: password,
		BankName: bankName,
	})
	if err != nil {
		log.Fatalf("Error during registration: %v", err)
	}
	log.Printf("Registration response: %s", resp.Message)
}

func MakePayment(args []string, creds credentials.TransportCredentials) {
	if len(args) != 7 {
		fmt.Println("Usage: client pay [gateway_address] [sender_bank_address] [receiver_bank_address] [sender_username] [receiver_username] [amount]")
		return
	}
	gatewayAddr := args[1]
	senderBank := args[2]
	receiverBank := args[3]
	senderUsername := args[4]
	receiverUsername := args[5]
	var amt float64
	if parsed, err := strconv.ParseFloat(args[6], 64); err != nil {
		log.Fatalf("Invalid amount: %v", err)
	} else {
		amt = parsed
	}

	transactionID := fmt.Sprintf("%d", time.Now().UnixNano())
	idempotencyKey := uuid.New().String()

	txReq := &paymentpb.TransactionRequest{
		TransactionId:    transactionID,
		SenderUsername:   senderUsername,
		ReceiverUsername: receiverUsername,
		Amount:           amt,
		SenderBank:       senderBank,
		ReceiverBank:     receiverBank,
		IdempotencyKey:   idempotencyKey,
	}

	loadOfflineQueue("pending_transactions.json")
	go tryProcessQueue(gatewayAddr, creds)

	err := sendPayment(gatewayAddr, txReq, creds)
	if err != nil {
		log.Printf("Payment failed; added to offline queue: %v", err)
		queueMutex.Lock()
		offlineQueue = append(offlineQueue, PaymentTransaction{req: txReq})
		queueMutex.Unlock()
		saveOfflineQueue()
		log.Printf("Transaction %s queued for retry.", transactionID)
	}
}

func GetBalance(args []string, creds credentials.TransportCredentials) {
	if len(args) != 3 {
		fmt.Println("Usage: client getbalance [gateway_address] [username]")
		return
	}
	gatewayAddr := args[1]
	username := args[2]

	conn, err := grpc.Dial(gatewayAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("Failed to connect to Payment Gateway: %v", err)
	}
	defer conn.Close()

	client := paymentpb.NewPaymentGatewayClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	md := metadata.Pairs("username", username, "password", "secret")
	ctx = metadata.NewOutgoingContext(ctx, md)

	resp, err := client.GetBalance(ctx, &paymentpb.BalanceRequest{Username: username})
	if err != nil {
		log.Fatalf("Error getting balance: %v", err)
	}
	log.Printf("Balance for user %s: %.2f", username, resp.Balance)
}

func GetTransactionHistory(args []string, creds credentials.TransportCredentials) {
	if len(args) != 3 {
		fmt.Println("Usage: client gethistory [gateway_address] [username]")
		return
	}
	gatewayAddr := args[1]
	username := args[2]

	conn, err := grpc.Dial(gatewayAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("Failed to connect to Payment Gateway: %v", err)
	}
	defer conn.Close()

	client := paymentpb.NewPaymentGatewayClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	md := metadata.Pairs("username", username, "password", "secret")
	ctx = metadata.NewOutgoingContext(ctx, md)

	resp, err := client.GetTransactionHistory(ctx, &paymentpb.HistoryRequest{Username: username})
	if err != nil {
		log.Fatalf("Error getting transaction history: %v", err)
	}
	log.Printf("Transaction history for user %s:", username)
	for _, rec := range resp.Records {
		log.Printf("ID: %s, Sender: %s, Receiver: %s, Amount: %.2f, Time: %s, Msg: %s",
			rec.TransactionId, rec.Sender, rec.Receiver, rec.Amount, rec.Timestamp, rec.Message)
	}
}

func UnregisterUser(args []string, creds credentials.TransportCredentials) {
	if len(args) != 3 {
		fmt.Println("Usage: client unregister [gateway_address] [username]")
		return
	}
	gatewayAddr := args[1]
	username := args[2]

	conn, err := grpc.Dial(gatewayAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("Failed to connect to Payment Gateway: %v", err)
	}
	defer conn.Close()

	client := paymentpb.NewPaymentGatewayClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	md := metadata.Pairs("username", username, "password", "secret")
	ctx = metadata.NewOutgoingContext(ctx, md)

	resp, err := client.Unregister(ctx, &paymentpb.UnregisterRequest{Username: username})
	if err != nil {
		log.Fatalf("Error during unregister: %v", err)
	}
	log.Printf("Unregister response for user %s: %s", username, resp.Message)
}