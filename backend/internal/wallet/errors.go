package wallet

import "errors"

var (
	// ErrSelfTransfer is returned when sender and receiver wallet IDs are identical.
	ErrSelfTransfer = errors.New("cannot transfer to the same wallet")

	// ErrInvalidAmount is returned when amount is zero or negative.
	ErrInvalidAmount = errors.New("transaction amount must be a positive integer in paisa")

	// ErrEmptyType is returned when the transaction type string is blank.
	ErrEmptyType = errors.New("transaction type cannot be empty")

	// ErrEmptyReference is returned when the transaction reference ID is blank.
	ErrEmptyReference = errors.New("transaction reference cannot be empty")

	// ErrWalletNotFound is returned when a sender or receiver wallet does not exist.
	ErrWalletNotFound = errors.New("wallet not found")

	// ErrWalletInactive is returned when a sender or receiver wallet is not in ACTIVE status.
	ErrWalletInactive = errors.New("wallet is not active")

	// ErrCurrencyMismatch is returned when sender and receiver wallets use different currencies.
	ErrCurrencyMismatch = errors.New("wallet currency mismatch")

	// ErrInsufficientFunds is returned when a personal sender wallet does not have enough derived balance to cover the debit.
	ErrInsufficientFunds = errors.New("insufficient funds in personal wallet")

	// ErrTransactionConflict is returned when a transaction with the same (type, reference_id) already exists but with conflicting transfer parameters (different sender, receiver, or amount).
	ErrTransactionConflict = errors.New("transaction reference exists with conflicting transfer parameters")
)

