package wallet

import (
	"time"

	"github.com/google/uuid"
)

// WalletType represents the account classification in the double-entry ledger.
type WalletType string

const (
	WalletTypePersonal  WalletType = "PERSONAL"
	WalletTypeRevenue   WalletType = "SYS_REVENUE"
	WalletTypeLiability WalletType = "SYS_LIABILITY"
)

// WalletStatus represents the lifecycle state of a wallet.
type WalletStatus string

const (
	WalletStatusActive    WalletStatus = "ACTIVE"
	WalletStatusFrozen    WalletStatus = "FROZEN"
	WalletStatusSuspended WalletStatus = "SUSPENDED"
	WalletStatusClosed    WalletStatus = "CLOSED"
)

// Wallet models a record in giki_sports.wallets.
type Wallet struct {
	ID        uuid.UUID
	UserID    *uuid.UUID
	Name      string
	Type      WalletType
	Status    WalletStatus
	Currency  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TransferParams specifies the input attributes for posting a two-entry double-entry transfer.
type TransferParams struct {
	SenderWalletID   uuid.UUID
	ReceiverWalletID uuid.UUID
	Amount           int64 // Stored in integer paisa (1 PKR = 100 paisa)
	Type             string
	ReferenceID      string
	Description      string
}

// PostResult summarizes the result of posting a double-entry transaction.
type PostResult struct {
	TransactionID uuid.UUID
	AlreadyPosted bool
}

// LedgerEntry models a record in giki_sports.wallet_ledger_entries.
type LedgerEntry struct {
	ID            uuid.UUID
	WalletID      uuid.UUID
	TransactionID uuid.UUID
	Amount        int64
	CreatedAt     time.Time
}

// UserWalletBalance encapsulates the current financial status and derived balance of a user's personal wallet.
type UserWalletBalance struct {
	WalletID uuid.UUID    `json:"wallet_id"`
	UserID   uuid.UUID    `json:"user_id"`
	Status   WalletStatus `json:"status"`
	Currency string       `json:"currency"`
	Balance  int64        `json:"balance"` // In integer paisa, derived from SUM(amount) across all historical ledger entries
}

// WalletActivityItem models a single entry in the owner-scoped recent activity feed.
type WalletActivityItem struct {
	EntryID       uuid.UUID `json:"entry_id"`
	TransactionID uuid.UUID `json:"transaction_id"`
	Type          string    `json:"type"`
	Amount        int64     `json:"amount"` // Signed integer paisa (negative for debits, positive for credits)
	CreatedAt     time.Time `json:"created_at"`
}

