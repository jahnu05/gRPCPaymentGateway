#!/bin/bash
# generate_certs.sh: Generate CA, server, and client certificates with SAN extensions.
# Usage:
#   ./generate_certs.sh [user1] [user2] ...
# If no users are provided, it defaults to "alice" and "bob".

set -e

# Directory for certificates
CERT_DIR="./certs"
mkdir -p "$CERT_DIR"

# Generate CA certificate and key if they do not exist
CA_KEY="$CERT_DIR/ca.key"
CA_CERT="$CERT_DIR/ca.crt"

if [ ! -f "$CA_KEY" ] || [ ! -f "$CA_CERT" ]; then
    echo "Generating CA key and certificate..."
    openssl genrsa -out "$CA_KEY" 4096
    # Using a proper SAN extension for the CA certificate instead of relying on CN
    cat > "$CERT_DIR/ca.cnf" <<EOF
[ req ]
default_bits       = 4096
prompt             = no
default_md         = sha256
distinguished_name = dn
x509_extensions    = v3_ca

[ dn ]
commonName         = MyRootCA

[ v3_ca ]
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer
basicConstraints = critical, CA:true
keyUsage = critical, digitalSignature, cRLSign, keyCertSign
EOF
    openssl req -new -x509 -nodes -key "$CA_KEY" -sha256 -days 365 -out "$CA_CERT" -config "$CERT_DIR/ca.cnf"
else
    echo "CA certificate already exists."
fi

# Create an OpenSSL configuration file for server and clients
OPENSSL_CNF="$CERT_DIR/openssl.cnf"
cat > "$OPENSSL_CNF" <<EOF
[ req ]
default_bits       = 2048
prompt             = no
default_md         = sha256
distinguished_name = dn
req_extensions     = req_ext

[ dn ]
commonName         = localhost

[ req_ext ]
subjectAltName = @alt_names

[ alt_names ]
DNS.1 = localhost
IP.1  = 127.0.0.1

[ v3_server ]
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[ v3_client ]
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth
subjectAltName = @alt_names
EOF

# Generate Server certificate and key with SAN
SERVER_KEY="$CERT_DIR/server.key"
SERVER_CSR="$CERT_DIR/server.csr"
SERVER_CERT="$CERT_DIR/server.crt"

if [ ! -f "$SERVER_KEY" ] || [ ! -f "$SERVER_CERT" ]; then
    echo "Generating server certificate with SAN..."
    openssl genrsa -out "$SERVER_KEY" 2048
    openssl req -new -key "$SERVER_KEY" -out "$SERVER_CSR" -config "$OPENSSL_CNF"
    openssl x509 -req -in "$SERVER_CSR" -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial \
      -out "$SERVER_CERT" -days 365 -sha256 -extfile "$OPENSSL_CNF" -extensions v3_server
else
    echo "Server certificate already exists."
fi

# Function to generate a client certificate for a given user with SAN.
generate_client_cert() {
    USERNAME=$1
    CLIENT_KEY="$CERT_DIR/${USERNAME}.key"
    CLIENT_CSR="$CERT_DIR/${USERNAME}.csr"
    CLIENT_CERT="$CERT_DIR/${USERNAME}.crt"
    CLIENT_CNF="$CERT_DIR/${USERNAME}.cnf"

    echo "Generating certificate for user: $USERNAME"
    
    # Create a custom config file for this client
    cat > "$CLIENT_CNF" <<EOF
[ req ]
default_bits       = 2048
prompt             = no
default_md         = sha256
distinguished_name = dn
req_extensions     = req_ext

[ dn ]
commonName         = ${USERNAME}

[ req_ext ]
subjectAltName = @alt_names

[ alt_names ]
DNS.1 = ${USERNAME}
EOF

    openssl genrsa -out "$CLIENT_KEY" 2048
    
    # Create CSR with both CN and SAN
    openssl req -new -key "$CLIENT_KEY" -out "$CLIENT_CSR" -config "$CLIENT_CNF"
    
    # Sign the certificate with the CA and include SAN
    openssl x509 -req -in "$CLIENT_CSR" -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial \
      -out "$CLIENT_CERT" -days 365 -sha256 -extfile "$OPENSSL_CNF" -extensions v3_client
      
    # Verify the key matches the certificate
    echo "Verifying that the private key matches the certificate for $USERNAME..."
    CERT_MODULUS=$(openssl x509 -noout -modulus -in "$CLIENT_CERT")
    KEY_MODULUS=$(openssl rsa -noout -modulus -in "$CLIENT_KEY")
    if [ "$CERT_MODULUS" = "$KEY_MODULUS" ]; then
        echo "Verification successful: Private key matches certificate for $USERNAME"
    else
        echo "ERROR: Private key does NOT match certificate for $USERNAME"
        exit 1
    fi
}

# Generate client certificates for the provided users or default to "alice" and "bob".
if [ "$#" -gt 0 ]; then
    USERS=("$@")
else
    USERS=("alice" "bob")
fi

for user in "${USERS[@]}"; do
    generate_client_cert "$user"
done

echo "All certificates generated in $CERT_DIR"
echo ""
echo "Important: These certificates properly use Subject Alternative Names (SAN) instead of"
echo "relying on the Common Name (CN) field, which is deprecated in modern TLS implementations."