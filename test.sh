#!/bin/bash
# test.sh: Test script for registration, authentication, authorization,
# idempotency, offline payments, and persistent transaction history.

echo "=== Cleaning and building project ==="
make clean
make proto
make build

echo "=== Generating TLS certificates (if not present) ==="
./cert.sh alice bob

echo "=== Starting Payment Gateway ==="
./payment_gateway &
PG_PID=$!
sleep 3

echo "=== Starting Bank Servers: BankA and BankB ==="
./bank_server BankA accounts_bank_a.json :50052 &
BANKA_PID=$!
./bank_server BankB accounts_bank_b.json :50053 &
BANKB_PID=$!
sleep 3

echo "=== Registration: Register user 'alice' for BankA and 'bob' for BankB ==="
./client_file --cert=certs/alice.crt --key=certs/alice.key --ca=certs/ca.crt register localhost:50051 alice secretalice localhost:50052
sleep 2
./client_file --cert=certs/bob.crt --key=certs/bob.key --ca=certs/ca.crt register localhost:50051 bob secretbob localhost:50053
sleep 2
echo "=== Payment Test: Correct credentials (alice pays bob 100.50) ==="
./client_file --senderPass=secretalice pay localhost:50051 localhost:50052 localhost:50053 alice bob 50
# sleep 3
# echo "=== Payment Test: Incorrect credentials (should be rejected and NOT queued) ==="
# ./client_file --cert=certs/alice.crt --key=certs/alice.key --ca=certs/ca.crt --senderPass=wrongpass pay localhost:50051 localhost:50052 localhost:50053 alice bob 20.00
# sleep 3


echo "=== Unregister Alice ==="
echo  \n
./client_file --senderPass=secretalice unregister localhost:50051 alice
# sleep 3

echo "=== Make Payment(To be added to queue) ==="

./client_file --senderPass=secretalice pay localhost:50051 localhost:50052 localhost:50053 alice bob 30

# sleep 3
echo "=== register Alice ==="

./client_file --cert=certs/alice.crt --key=certs/alice.key --ca=certs/ca.crt register localhost:50051 alice secretalice localhost:50052
echo "=== Make Payment(To be executed) ==="

./client_file --senderPass=secretalice pay localhost:50051 localhost:50052 localhost:50053 alice bob 15

# # sleep 10
# sleep 3
echo "=== Unregister Bob ==="
echo  \n
./client_file --senderPass=secretalice unregister localhost:50051 bob

# sleep 3
echo "=== Make Payment(To be added to queue) ==="

./client_file --senderPass=secretalice pay localhost:50051 localhost:50052 localhost:50053 alice bob 20


echo "=== Make register bob ==="

./client_file --cert=certs/bob.crt --key=certs/bob.key --ca=certs/ca.crt register localhost:50051 bob secretbob localhost:50053
# sleep 3
echo "=== Make Payment(To be added to queue) ==="

./client_file --senderPass=secretalice pay localhost:50051 localhost:50052 localhost:50053 alice bob 10



# echo "=== Testing GetTransactionHistory (authorization): Alice querying her history ==="
# ./client_file --senderPass=secretalice gethistory localhost:50051 alice
# sleep 3

# echo "=== Testing Offline Payments: Stopping BankB (receiver bank) ==="
# kill $PG_PID
# sleep 3

# echo "Attempting payment while BankB is offline (transaction will be queued)..."
# ./client_file --senderPass=secretalice pay localhost:50051 localhost:50052 localhost:50053 alice bob 50
# OFFLINE_CLIENT_PID=$!
# sleep 5

# echo "Restarting gateway..."
# ./payment_gateway &
# NEW_BANKB_PID=$!
# sleep 15

# echo "=== Final Account Balances ==="
# echo "BankA (sender) accounts:"
# cat accounts_bank_a.json
# echo "BankB (receiver) accounts:"
# cat accounts_bank_b.json

# echo "=== Final Transaction History ==="
# cat transaction_history.json

echo "=== Cleaning up ==="
kill $PG_PID $BANKA_PID $NEW_BANKB_PID $OFFLINE_CLIENT_PID
echo "Test complete."
