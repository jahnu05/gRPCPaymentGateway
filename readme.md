# Payment Gateway System

This project implements a secure and modular payment gateway system with support for registration, authentication, authorization, idempotency, offline payments, and persistent transaction history. It uses gRPC for communication between the client, gateway, and bank servers.

## Features

- **TLS Encryption**: Secure communication using TLS certificates.
- **gRPC Services**:
  - `PaymentGateway` for client interactions.
  - `BankService` for bank server operations.
- **Two-Phase Commit**: Ensures atomicity of transactions.
- **Offline Payments**: Queues transactions when the receiver's bank is offline.
- **Persistent Transaction History**: Stores transaction records in a JSON file.
- **Idempotency**: Prevents duplicate transactions using unique keys.

## Project Structure

```
.
├── client/
│   ├── main.go                # Client entry point
│   ├── commands.go            # Client command logic
├── gateway/
│   ├── main.go                # Entry point for the Payment Gateway server
│   ├── server.go              # Core server setup and initialization
│   ├── auth.go                # Authentication and authorization interceptors
│   ├── transaction.go         # Transaction processing logic
│   ├── user_management.go     # User registration and unregistration logic
│   ├── history.go             # Transaction history management
├── server/
│   ├── accounts.go            # Bank account logic
├── protofiles/
│   ├── payment.proto          # Protocol Buffers definition
│   ├── payment.pb.go          # Generated Go code from .proto
│   └── payment_grpc.pb.go     # Generated gRPC code
├── cert.sh                    # Script to generate TLS certificates
├── test.sh                    # Test script for the system
├── transaction_history.json   # Persistent transaction history
├── Makefile                   # Build and clean commands
├── accounts_bank_a.json       # Bank A account data
├── accounts_bank_b.json       # Bank B account data
└── README.md                  # Project documentation
```

## Prerequisites

- Go 1.18 or later
- Protocol Buffers Compiler (`protoc`)
- OpenSSL (for generating certificates)

## Setup

1. **Generate Protocol Buffers Code**:
   ```bash
   make proto
   ```

2. **Build the Project**:
   ```bash
   make build
   ```

3. **Generate TLS Certificates**:
   ```bash
   ./cert.sh alice bob
   ```

4. **Run the System**:
   - Start the Payment Gateway:
     ```bash
     ./payment_gateway
     ```
   - Start Bank Servers:
     ```bash
     ./bank_server BankA accounts_bank_a.json :50052
     ./bank_server BankB accounts_bank_b.json :50053
     ```
   - Use the client to interact with the system.

5. **Run Tests**:
   ```bash
   ./test.sh
   ```

## Usage

### Client Commands

1. **Register a User**:
   ```bash
   ./client_file --cert=certs/alice.crt --key=certs/alice.key --ca=certs/ca.crt register localhost:50051 alice secretalice localhost:50052
   ```

2. **Make a Payment**:
   ```bash
   ./client_file --senderPass=secretalice pay localhost:50051 localhost:50052 localhost:50053 alice bob 50
   ```

3. **Unregister a User**:
   ```bash
   ./client_file --senderPass=secretalice unregister localhost:50051 alice
   ```

4. **Get Transaction History**:
   ```bash
   ./client_file --senderPass=secretalice gethistory localhost:50051 alice
   ```

5. **Get Balance**:
   ```bash
   ./client_file --senderPass=secretalice getbalance localhost:50051 alice
   ```
