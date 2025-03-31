package main

import (
    "flag"
    "log"

    "cpath/to/client/commands"
)

func main() {
    flag.Parse()

    // Load pending transactions from file at startup.
    commands.LoadOfflineQueue("pending_transactions.json")

    creds, err := commands.GetTLSCredentials()
    if err != nil {
        log.Fatalf("Failed to get TLS credentials: %v", err)
    }

    // Parse and execute the command.
    args := flag.Args()
    if len(args) < 1 {
        commands.PrintUsage()
        return
    }

    mode := args[0]
    switch mode {
    case "register":
        commands.RegisterUser(args, creds)
    case "pay":
        commands.MakePayment(args, creds)
    case "getbalance":
        commands.GetBalance(args, creds)
    case "gethistory":
        commands.GetTransactionHistory(args, creds)
    case "unregister":
        commands.UnregisterUser(args, creds)
    default:
        commands.PrintUsage()
    }
}
