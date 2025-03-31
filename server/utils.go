package main

import (
	"encoding/json"
	"io/ioutil"
)

// Account represents a user account.
type Account struct {
	Username string  `json:"username"`
	Password string  `json:"password"`
	Balance  float64 `json:"balance"`
}

// loadAccounts loads accounts from a JSON file.
func (s *BankServer) loadAccounts(filename string) error {
	s.filename = filename
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	var accs []Account
	if err := json.Unmarshal(data, &accs); err != nil {
		return err
	}
	s.accounts = make(map[string]*Account)
	for _, a := range accs {
		s.accounts[a.Username] = &Account{
			Username: a.Username,
			Password: a.Password,
			Balance:  a.Balance,
		}
	}
	return nil
}

// persistAccounts writes updated account data to the JSON file.
func (s *BankServer) persistAccounts() error {
	var accs []Account
	for _, a := range s.accounts {
		accs = append(accs, *a)
	}
	data, err := json.MarshalIndent(accs, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(s.filename, data, 0644)
}