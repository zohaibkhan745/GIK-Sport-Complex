package wallet

// WalletSummaryDTO summarizes the authenticated user's personal wallet.
type WalletSummaryDTO struct {
	Status       string `json:"status"`
	Currency     string `json:"currency"`
	BalancePaisa string `json:"balancePaisa"`
}

// WalletActivityDTO represents a single ledger transaction entry for the user.
// Counterparty details, system balances, and internal reference IDs are strictly excluded.
type WalletActivityDTO struct {
	EntryID       string `json:"entryId"`
	TransactionID string `json:"transactionId"`
	Type          string `json:"type"`
	AmountPaisa   string `json:"amountPaisa"`
	CreatedAt     string `json:"createdAt"`
}

// WalletMeResponse represents the unified response for GET /api/wallet/me.
// When the user has no wallet created yet, Wallet is nil ("wallet": null) and Activity is empty ("activity": []).
type WalletMeResponse struct {
	Wallet   *WalletSummaryDTO   `json:"wallet"`
	Activity []WalletActivityDTO `json:"activity"`
}

// ErrorResponse represents a standardized JSON error message.
type ErrorResponse struct {
	Error string `json:"error"`
}

