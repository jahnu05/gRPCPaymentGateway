PROTO_DIR = protofiles
CLIENT_DIR = client
SERVER_DIR = server
GATEWAY_DIR = gateway
PROTO_FILE_GREET = $(PROTO_DIR)/payment.proto
PROTO_OUT_DIR = .

GO_FLAGS = --go_out=$(PROTO_OUT_DIR) --go_opt=paths=source_relative \
           --go-grpc_out=$(PROTO_OUT_DIR) --go-grpc_opt=paths=source_relative

SERVER_PORT = 50051

.PHONY: proto build clean

proto:
	protoc $(GO_FLAGS) $(PROTO_FILE_GREET)


build:
	go build -o payment_gateway ${GATEWAY_DIR}/main.go
	go build -o bank_server ${SERVER_DIR}/main.go
	go build -o client_file ${CLIENT_DIR}/main.go

clean:
	rm -f payment_gateway bank_server client_file
	rm -rf $(PROTO_DEST)
