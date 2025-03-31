package main

import (
	"flag"
	"log"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	paymentpb "github.com/jahnu05/Assignment-2/P-3/protofiles"
)

// Global variable for abort timeout. Default to 5 seconds.
var abortTimeout = 5 * time.Second

func main() {
	abortTimeoutFlag := flag.Duration("abort_timeout", 5*time.Second, "Timeout duration for aborting transactions")
	flag.Parse()
	abortTimeout = *abortTimeoutFlag

	if len(os.Args) < 3 {
		log.Fatalf("Usage: bank_server [bankName] [accounts.json] [port(optional)]")
	}
	bankName := os.Args[1]
	accountsFile := os.Args[2]
	port := ":50052"
	if len(os.Args) >= 4 {
		port = os.Args[3]
	}
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", port, err)
	}
	grpcServer := grpc.NewServer()
	bankServer := &BankServer{bankName: bankName}
	if err := bankServer.loadAccounts(accountsFile); err != nil {
		log.Fatalf("Error loading accounts: %v", err)
	}
	paymentpb.RegisterBankServiceServer(grpcServer, bankServer)
	log.Printf("Bank server %s started on %s", bankName, port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
