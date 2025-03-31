package config

import "os"

// Paths for certificates and transaction history
var (
    CertDir              = "./certs"
    TransactionHistory   = "./transaction_history.json"
    AccountsBankA        = "./accounts_bank_a.json"
    AccountsBankB        = "./accounts_bank_b.json"
    DefaultServerAddress = ":50051"
)

// GetEnv fetches environment variables with a fallback default value
func GetEnv(key, fallback string) string {
    if value, exists := os.LookupEnv(key); exists {
        return value
    }
    return fallback
}
