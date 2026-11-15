//go:build integration

package padel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"giki-sports-backend/internal/wallet"
)

func TestCheckout_HappyPath_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	// 1. Seed facility
	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000101")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-arena-checkout', 'Checkout Test Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	// 2. Seed Alice
	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000101")
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice.checkout@example.test', 'Alice Checkout', 'hash', 'active');
	`, aliceID)
	if err != nil {
		t.Fatalf("failed to seed Alice: %v", err)
	}

	// 3. Create wallets & fund Alice with 500,000 paisa (PKR 5,000.00)
	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}

	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}

	revenueWalletID, err := walletSvc.FindOrCreateSystemWallet(ctx, "Sports Revenue", wallet.WalletTypeRevenue, "PKR")
	if err != nil {
		t.Fatalf("failed to find or create revenue wallet: %v", err)
	}

	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           500000,
		Type:             "CASHIER_TOPUP",
		ReferenceID:      "TOPUP-ALICE-CHECKOUT-1",
		Description:      "Alice initial topup",
	})
	if err != nil {
		t.Fatalf("failed to fund Alice wallet: %v", err)
	}

	// 4. Create slot & hold it
	startTime := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Hour)
	endTime := startTime.Add(60 * time.Minute)
	slotPrice := int64(100000) // PKR 1,000.00

	createdSlot, err := repo.CreateSlot(ctx, &Slot{
		FacilityID: facID,
		StartTime:  startTime,
		EndTime:    endTime,
		Status:     SlotStatusAvailable,
		PricePaisa: slotPrice,
	})
	if err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}
	slotID := createdSlot.ID

	holdResp, err := svc.HoldSlot(ctx, slotID, aliceID)
	if err != nil {
		t.Fatalf("failed to acquire hold: %v", err)
	}
	if holdResp.Status != "HELD" {
		t.Fatalf("expected status HELD, got %s", holdResp.Status)
	}

	// 5. Execute checkout (FR-03)
	idempotencyKey := "idemp-checkout-happy-1"
	confirmResp, err := svc.Checkout(ctx, CheckoutRequest{
		SlotID:         slotID.String(),
		IdempotencyKey: idempotencyKey,
	}, aliceID)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	// 6. Verify Confirmation DTO
	if confirmResp.AlreadyBooked {
		t.Errorf("expected AlreadyBooked to be false on first booking")
	}
	if confirmResp.SlotID != slotID.String() {
		t.Errorf("expected slot ID %s, got %s", slotID, confirmResp.SlotID)
	}
	if confirmResp.FacilityID != facID.String() {
		t.Errorf("expected facility ID %s, got %s", facID, confirmResp.FacilityID)
	}
	if confirmResp.FacilityName != "Checkout Test Arena" {
		t.Errorf("expected facility name 'Checkout Test Arena', got %s", confirmResp.FacilityName)
	}
	if confirmResp.Status != "CONFIRMED" {
		t.Errorf("expected status CONFIRMED, got %s", confirmResp.Status)
	}
	if confirmResp.FeePaisa != "100000" {
		t.Errorf("expected FeePaisa 100000, got %s", confirmResp.FeePaisa)
	}
	if len(confirmResp.CheckInCode) != 4 {
		t.Errorf("expected 4-digit check-in code, got %s", confirmResp.CheckInCode)
	}
	if confirmResp.DebitTransactionID == "" {
		t.Errorf("expected non-empty DebitTransactionID")
	}

	// 7. Verify Database State: padel_slots
	updatedSlot, err := repo.GetSlotByID(ctx, slotID)
	if err != nil {
		t.Fatalf("failed to get updated slot: %v", err)
	}
	if updatedSlot.Status != SlotStatusBooked {
		t.Errorf("expected slot status BOOKED, got %s", updatedSlot.Status)
	}
	if updatedSlot.HolderID != nil {
		t.Errorf("expected holder_id to be cleared, got %v", updatedSlot.HolderID)
	}
	if updatedSlot.HoldExpiresAt != nil {
		t.Errorf("expected hold_expires_at to be cleared, got %v", updatedSlot.HoldExpiresAt)
	}

	// 8. Verify Database State: padel_bookings
	bookingUUID := uuid.MustParse(confirmResp.BookingID)
	var dbBookingStatus string
	var dbFee int64
	var dbCheckInCode string
	var dbDebitTxID uuid.UUID
	err = pool.QueryRow(ctx, `
		SELECT status, fee_paisa, check_in_code, debit_transaction_id
		FROM giki_sports.padel_bookings
		WHERE id = $1;
	`, bookingUUID).Scan(&dbBookingStatus, &dbFee, &dbCheckInCode, &dbDebitTxID)
	if err != nil {
		t.Fatalf("failed to query booking row: %v", err)
	}
	if dbBookingStatus != "CONFIRMED" {
		t.Errorf("expected db booking status CONFIRMED, got %s", dbBookingStatus)
	}
	if dbFee != slotPrice {
		t.Errorf("expected db fee %d, got %d", slotPrice, dbFee)
	}
	if dbCheckInCode != confirmResp.CheckInCode {
		t.Errorf("expected db check-in code %s, got %s", confirmResp.CheckInCode, dbCheckInCode)
	}
	if dbDebitTxID.String() != confirmResp.DebitTransactionID {
		t.Errorf("expected debit tx %s, got %s", confirmResp.DebitTransactionID, dbDebitTxID)
	}

	// 9. Verify Ledger & Derived Balances
	aliceBal, err := walletSvc.GetUserWalletBalance(ctx, aliceID)
	if err != nil {
		t.Fatalf("failed to get Alice wallet balance: %v", err)
	}
	if aliceBal.Balance != 400000 {
		t.Errorf("expected Alice derived balance 400,000 paisa, got %d", aliceBal.Balance)
	}

	var revenueBalance int64
	err = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)
		FROM giki_sports.wallet_ledger_entries
		WHERE wallet_id = $1;
	`, revenueWalletID).Scan(&revenueBalance)
	if err != nil {
		t.Fatalf("failed to query revenue ledger: %v", err)
	}
	if revenueBalance != 100000 {
		t.Errorf("expected revenue balance 100,000 paisa, got %d", revenueBalance)
	}

	// 10. Verify Audit Log
	var auditCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM giki_sports.audit_logs
		WHERE actor_id = $1 AND action = 'booking.checkout' AND target_id = $2;
	`, aliceID, bookingUUID).Scan(&auditCount)
	if err != nil {
		t.Fatalf("failed to query audit log: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("expected 1 audit log entry, got %d", auditCount)
	}

	t.Log("Verified: Happy path checkout confirmed booking, debited ledger, and audited transaction atomically.")
}

func TestCheckout_Concurrency_50Contenders_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000201")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-arena-race', 'Race Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	// Seed 50 users, wallets, and funds
	const numContenders = 50
	userIDs := make([]uuid.UUID, numContenders)
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Race", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}

	for i := 0; i < numContenders; i++ {
		uID := uuid.MustParse(fmt.Sprintf("a0000000-0000-0000-0000-%012d", i+1))
		userIDs[i] = uID
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
			VALUES ($1, $2, $3, 'hash', 'active');
		`, uID, fmt.Sprintf("raceuser%d@example.test", i+1), fmt.Sprintf("Race User %d", i+1))
		if err != nil {
			t.Fatalf("failed to seed user %d: %v", i+1, err)
		}

		w, err := walletSvc.CreateWallet(ctx, &uID, fmt.Sprintf("Wallet User %d", i+1), wallet.WalletTypePersonal, "PKR")
		if err != nil {
			t.Fatalf("failed to create wallet for user %d: %v", i+1, err)
		}

		_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
			SenderWalletID:   clearingWallet.ID,
			ReceiverWalletID: w.ID,
			Amount:           200000,
			Type:             "TOPUP",
			ReferenceID:      fmt.Sprintf("TOPUP-RACE-%d", i+1),
			Description:      "Race topup",
		})
		if err != nil {
			t.Fatalf("failed to fund wallet for user %d: %v", i+1, err)
		}
	}

	// Create slot
	startTime := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Hour)
	endTime := startTime.Add(60 * time.Minute)
	createdSlot, err := repo.CreateSlot(ctx, &Slot{
		FacilityID: facID,
		StartTime:  startTime,
		EndTime:    endTime,
		Status:     SlotStatusAvailable,
		PricePaisa: 100000,
	})
	if err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}
	slotID := createdSlot.ID

	// User 0 acquires hold
	winnerID := userIDs[0]
	_, err = svc.HoldSlot(ctx, slotID, winnerID)
	if err != nil {
		t.Fatalf("failed to acquire initial hold: %v", err)
	}

	// 50 concurrent checkouts hit the slot simultaneously
	var successCount int64
	var conflictCount int64
	var otherErrCount int64

	startGate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(numContenders)

	for i := 0; i < numContenders; i++ {
		contenderID := userIDs[i]
		go func(uid uuid.UUID, idx int) {
			defer wg.Done()
			<-startGate

			req := CheckoutRequest{
				SlotID:         slotID.String(),
				IdempotencyKey: fmt.Sprintf("key-race-%d", idx),
			}
			_, err := svc.Checkout(context.Background(), req, uid)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else if errors.Is(err, ErrNotSlotHolder) || errors.Is(err, ErrSlotUnavailable) || errors.Is(err, ErrSlotConflict) {
				atomic.AddInt64(&conflictCount, 1)
			} else {
				atomic.AddInt64(&otherErrCount, 1)
			}
		}(contenderID, i)
	}

	close(startGate)
	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected EXACTLY 1 successful checkout, got %d (conflicts: %d, other errors: %d)",
			successCount, conflictCount, otherErrCount)
	}
	if conflictCount != int64(numContenders-1) {
		t.Errorf("expected %d conflicts, got %d", numContenders-1, conflictCount)
	}

	// Verify exactly 1 active booking in database
	var bookingCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM giki_sports.padel_bookings
		WHERE slot_id = $1 AND status IN ('CONFIRMED', 'CHECKED_IN');
	`, slotID).Scan(&bookingCount)
	if err != nil {
		t.Fatalf("failed to count bookings: %v", err)
	}
	if bookingCount != 1 {
		t.Errorf("expected exactly 1 confirmed booking row, got %d", bookingCount)
	}

	// Verify only the winner was charged (400,000 paisa across 50 users was 50 * 200,000 = 10,000,000 - 100,000 = 9,900,000)
	var totalLedgerDebit int64
	err = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)
		FROM giki_sports.wallet_ledger_entries
		WHERE transaction_id IN (
			SELECT debit_transaction_id FROM giki_sports.padel_bookings WHERE slot_id = $1
		) AND amount < 0;
	`, slotID).Scan(&totalLedgerDebit)
	if err != nil {
		t.Fatalf("failed to query ledger debits: %v", err)
	}
	if totalLedgerDebit != -100000 {
		t.Errorf("expected total debit of -100,000 paisa across all 50 users, got %d", totalLedgerDebit)
	}

	t.Logf("Verified QR-01: 50 concurrent checkout attempts produced exactly 1 booking and 49 conflict rejections.")
}

func TestCheckout_AtomicRollback_FailureInjection_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000301")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-arena-rollback', 'Rollback Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000301")
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice.rollback@example.test', 'Alice Rollback', 'hash', 'active');
	`, aliceID)
	if err != nil {
		t.Fatalf("failed to seed Alice: %v", err)
	}

	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Rollback", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           300000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-ALICE-ROLLBACK-1",
		Description:      "Rollback test topup",
	})
	if err != nil {
		t.Fatalf("failed to fund Alice wallet: %v", err)
	}

	startTime := time.Now().Add(4 * time.Hour).UTC().Truncate(time.Hour)
	endTime := startTime.Add(60 * time.Minute)
	createdSlot, err := repo.CreateSlot(ctx, &Slot{
		FacilityID: facID,
		StartTime:  startTime,
		EndTime:    endTime,
		Status:     SlotStatusAvailable,
		PricePaisa: 100000,
	})
	if err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}
	slotID := createdSlot.ID

	// Normal UoW for hold
	normalUoW := NewUnitOfWork(pool, walletSvc)
	svcNormal := NewService(repo, walletSvc, normalUoW)

	_, err = svcNormal.HoldSlot(ctx, slotID, aliceID)
	if err != nil {
		t.Fatalf("failed to acquire hold: %v", err)
	}

	// Now wrap UnitOfWork to inject a deliberate failure right after the business logic finishes
	injectedErr := errors.New("injected network crash before commit")
	failingUoW := &failingUnitOfWorkDecorator{
		realUoW:     normalUoW,
		injectedErr: injectedErr,
	}
	svcFailing := NewService(repo, walletSvc, failingUoW)

	// Execute checkout with failure injection
	_, err = svcFailing.Checkout(ctx, CheckoutRequest{
		SlotID:         slotID.String(),
		IdempotencyKey: "key-failure-injection",
	}, aliceID)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("expected injected error %v, got %v", injectedErr, err)
	}

	// QR-03 Verification:
	// 1. Zero bookings created
	var bookingsCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.padel_bookings WHERE slot_id = $1;`, slotID).Scan(&bookingsCount)
	if err != nil {
		t.Fatalf("failed to count bookings: %v", err)
	}
	if bookingsCount != 0 {
		t.Errorf("expected 0 bookings after rollback, found %d", bookingsCount)
	}

	// 2. Zero transactions created with PADEL_BOOKING type
	var txCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.wallet_transactions WHERE type = 'PADEL_BOOKING';`).Scan(&txCount)
	if err != nil {
		t.Fatalf("failed to count transactions: %v", err)
	}
	if txCount != 0 {
		t.Errorf("expected 0 wallet transactions after rollback, found %d", txCount)
	}

	// 3. Zero wallet ledger debits
	aliceBal, err := walletSvc.GetUserWalletBalance(ctx, aliceID)
	if err != nil {
		t.Fatalf("failed to get Alice wallet balance: %v", err)
	}
	if aliceBal.Balance != 300000 {
		t.Errorf("expected Alice derived balance to remain untouched at 300,000 paisa, got %d", aliceBal.Balance)
	}

	// 4. Slot status remains HELD with Alice's hold intact!
	slotAfter, err := repo.GetSlotByID(ctx, slotID)
	if err != nil {
		t.Fatalf("failed to get slot: %v", err)
	}
	if slotAfter.Status != SlotStatusHeld {
		t.Errorf("expected slot status to remain HELD, got %s", slotAfter.Status)
	}
	if slotAfter.HolderID == nil || *slotAfter.HolderID != aliceID {
		t.Errorf("expected Alice to remain the slot holder")
	}

	t.Log("Verified QR-03: Injected failure before commit rolled back booking and wallet debit completely.")
}

func TestCheckout_IdempotencyReplay_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000401")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-arena-idemp', 'Idempotency Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000401")
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice.idemp@example.test', 'Alice Idemp', 'hash', 'active');
	`, aliceID)
	if err != nil {
		t.Fatalf("failed to seed Alice: %v", err)
	}

	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Idemp", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           500000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-ALICE-IDEMP-1",
		Description:      "Idemp test topup",
	})
	if err != nil {
		t.Fatalf("failed to fund Alice wallet: %v", err)
	}

	startTime := time.Now().Add(5 * time.Hour).UTC().Truncate(time.Hour)
	endTime := startTime.Add(60 * time.Minute)
	createdSlot, err := repo.CreateSlot(ctx, &Slot{
		FacilityID: facID,
		StartTime:  startTime,
		EndTime:    endTime,
		Status:     SlotStatusAvailable,
		PricePaisa: 100000,
	})
	if err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}
	slotID := createdSlot.ID

	_, err = svc.HoldSlot(ctx, slotID, aliceID)
	if err != nil {
		t.Fatalf("failed to hold slot: %v", err)
	}

	idempotencyKey := "idemp-key-replay-20times"

	// Request 1: Initial checkout
	initialResp, err := svc.Checkout(ctx, CheckoutRequest{
		SlotID:         slotID.String(),
		IdempotencyKey: idempotencyKey,
	}, aliceID)
	if err != nil {
		t.Fatalf("initial checkout failed: %v", err)
	}
	if initialResp.AlreadyBooked {
		t.Errorf("expected AlreadyBooked to be false on first request")
	}

	// Requests 2..21: Replays
	for i := 1; i <= 20; i++ {
		replayResp, err := svc.Checkout(ctx, CheckoutRequest{
			SlotID:         slotID.String(),
			IdempotencyKey: idempotencyKey,
		}, aliceID)
		if err != nil {
			t.Fatalf("replay %d failed: %v", i, err)
		}
		if !replayResp.AlreadyBooked {
			t.Errorf("replay %d: expected AlreadyBooked to be true", i)
		}
		if replayResp.BookingID != initialResp.BookingID {
			t.Errorf("replay %d: expected BookingID %s, got %s", i, initialResp.BookingID, replayResp.BookingID)
		}
		if replayResp.CheckInCode != initialResp.CheckInCode {
			t.Errorf("replay %d: expected CheckInCode %s, got %s", i, initialResp.CheckInCode, replayResp.CheckInCode)
		}
		if replayResp.DebitTransactionID != initialResp.DebitTransactionID {
			t.Errorf("replay %d: expected DebitTransactionID %s, got %s", i, initialResp.DebitTransactionID, replayResp.DebitTransactionID)
		}
	}

	// Verify wallet balance: exactly 1 debit of 100,000 paisa occurred (500,000 -> 400,000)
	aliceBal, err := walletSvc.GetUserWalletBalance(ctx, aliceID)
	if err != nil {
		t.Fatalf("failed to get Alice wallet balance: %v", err)
	}
	if aliceBal.Balance != 400000 {
		t.Errorf("expected Alice derived balance 400,000 paisa across 21 requests, got %d", aliceBal.Balance)
	}

	// Verify exactly 1 booking in database
	var bookingCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.padel_bookings WHERE user_id = $1 AND idempotency_key = $2;`,
		aliceID, idempotencyKey).Scan(&bookingCount)
	if err != nil {
		t.Fatalf("failed to query bookings: %v", err)
	}
	if bookingCount != 1 {
		t.Errorf("expected 1 booking in database, got %d", bookingCount)
	}

	t.Log("Verified QR-04: 20 sequential replays returned identical booking confirmation with zero duplicate debits.")
}

func TestCheckout_IdempotencyConflict_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000501")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-arena-conflict', 'Conflict Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000501")
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice.conflict@example.test', 'Alice Conflict', 'hash', 'active');
	`, aliceID)
	if err != nil {
		t.Fatalf("failed to seed Alice: %v", err)
	}

	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Conflict", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           500000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-ALICE-CONFLICT-1",
		Description:      "Conflict test topup",
	})
	if err != nil {
		t.Fatalf("failed to fund Alice wallet: %v", err)
	}

	// Slot A
	startTimeA := time.Now().Add(6 * time.Hour).UTC().Truncate(time.Hour)
	createdSlotA, err := repo.CreateSlot(ctx, &Slot{
		FacilityID: facID,
		StartTime:  startTimeA,
		EndTime:    startTimeA.Add(60 * time.Minute),
		Status:     SlotStatusAvailable,
		PricePaisa: 100000,
	})
	if err != nil {
		t.Fatalf("failed to create slot A: %v", err)
	}
	slotA := createdSlotA.ID

	// Slot B
	startTimeB := startTimeA.Add(2 * time.Hour)
	createdSlotB, err := repo.CreateSlot(ctx, &Slot{
		FacilityID: facID,
		StartTime:  startTimeB,
		EndTime:    startTimeB.Add(60 * time.Minute),
		Status:     SlotStatusAvailable,
		PricePaisa: 100000,
	})
	if err != nil {
		t.Fatalf("failed to create slot B: %v", err)
	}
	slotB := createdSlotB.ID

	// Alice books Slot A with key "key-conflict-test"
	sharedKey := "key-conflict-test"
	_, err = svc.HoldSlot(ctx, slotA, aliceID)
	if err != nil {
		t.Fatalf("failed to hold slot A: %v", err)
	}
	_, err = svc.Checkout(ctx, CheckoutRequest{
		SlotID:         slotA.String(),
		IdempotencyKey: sharedKey,
	}, aliceID)
	if err != nil {
		t.Fatalf("checkout on slot A failed: %v", err)
	}

	// Alice holds Slot B and tries to checkout using the SAME sharedKey
	_, err = svc.HoldSlot(ctx, slotB, aliceID)
	if err != nil {
		t.Fatalf("failed to hold slot B: %v", err)
	}

	_, err = svc.Checkout(ctx, CheckoutRequest{
		SlotID:         slotB.String(),
		IdempotencyKey: sharedKey,
	}, aliceID)
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected ErrIdempotencyConflict, got %v", err)
	}

	t.Log("Verified: Reusing an idempotency key with mismatched slot parameters returns ErrIdempotencyConflict.")
}

func TestCheckout_ExpiredHold_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000601")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-arena-expired', 'Expired Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000601")
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice.expired@example.test', 'Alice Expired', 'hash', 'active');
	`, aliceID)
	if err != nil {
		t.Fatalf("failed to seed Alice: %v", err)
	}

	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Expired", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           500000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-ALICE-EXPIRED-1",
		Description:      "Expired test topup",
	})
	if err != nil {
		t.Fatalf("failed to fund Alice wallet: %v", err)
	}

	startTime := time.Now().Add(8 * time.Hour).UTC().Truncate(time.Hour)
	endTime := startTime.Add(60 * time.Minute)

	// Create slot with an already expired hold (expired 5 minutes ago)
	expiredAt := time.Now().Add(-5 * time.Minute).UTC()
	createdSlot, err := repo.CreateSlot(ctx, &Slot{
		FacilityID:    facID,
		StartTime:     startTime,
		EndTime:       endTime,
		Status:        SlotStatusHeld,
		HolderID:      &aliceID,
		HoldExpiresAt: &expiredAt,
		PricePaisa:    100000,
	})
	if err != nil {
		t.Fatalf("failed to create expired slot: %v", err)
	}
	slotID := createdSlot.ID

	svc := NewService(repo, walletSvc, uow)
	_, err = svc.Checkout(ctx, CheckoutRequest{
		SlotID:         slotID.String(),
		IdempotencyKey: "key-expired-test",
	}, aliceID)
	if !errors.Is(err, ErrHoldExpired) {
		t.Fatalf("expected ErrHoldExpired, got %v", err)
	}

	// Verify Alice was not debited
	aliceBal, err := walletSvc.GetUserWalletBalance(ctx, aliceID)
	if err != nil {
		t.Fatalf("failed to get Alice wallet balance: %v", err)
	}
	if aliceBal.Balance != 500000 {
		t.Errorf("expected Alice derived balance 500,000 paisa, got %d", aliceBal.Balance)
	}

	t.Log("Verified: Expired hold is rejected at checkout with ErrHoldExpired and zero debit.")
}

func TestCheckout_InsufficientFunds_Integration(t *testing.T) {
	repo, pool, cleanup := setupPadelIntegrationTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walletSvc := wallet.NewService(pool)
	uow := NewUnitOfWork(pool, walletSvc)
	svc := NewService(repo, walletSvc, uow)

	facID := uuid.MustParse("f0000000-0000-0000-0000-000000000701")
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.facilities (id, slug, name, status)
		VALUES ($1, 'padel-arena-poor', 'Poor Arena', 'active');
	`, facID)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}

	aliceID := uuid.MustParse("a0000000-0000-0000-0000-000000000701")
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash, status)
		VALUES ($1, 'alice.poor@example.test', 'Alice Poor', 'hash', 'active');
	`, aliceID)
	if err != nil {
		t.Fatalf("failed to seed Alice: %v", err)
	}

	// Alice has only 20,000 paisa (PKR 200.00), slot costs 100,000 paisa (PKR 1,000.00)
	aliceWallet, err := walletSvc.CreateWallet(ctx, &aliceID, "Alice Personal", wallet.WalletTypePersonal, "PKR")
	if err != nil {
		t.Fatalf("failed to create Alice wallet: %v", err)
	}
	clearingWallet, err := walletSvc.CreateWallet(ctx, nil, "Cashier Clearing Poor", wallet.WalletTypeLiability, "PKR")
	if err != nil {
		t.Fatalf("failed to create clearing wallet: %v", err)
	}
	_, err = walletSvc.PostTransaction(ctx, wallet.TransferParams{
		SenderWalletID:   clearingWallet.ID,
		ReceiverWalletID: aliceWallet.ID,
		Amount:           20000,
		Type:             "TOPUP",
		ReferenceID:      "TOPUP-ALICE-POOR-1",
		Description:      "Poor topup",
	})
	if err != nil {
		t.Fatalf("failed to fund Alice wallet: %v", err)
	}

	startTime := time.Now().Add(10 * time.Hour).UTC().Truncate(time.Hour)
	endTime := startTime.Add(60 * time.Minute)
	createdSlot, err := repo.CreateSlot(ctx, &Slot{
		FacilityID: facID,
		StartTime:  startTime,
		EndTime:    endTime,
		Status:     SlotStatusAvailable,
		PricePaisa: 100000,
	})
	if err != nil {
		t.Fatalf("failed to create slot: %v", err)
	}
	slotID := createdSlot.ID

	_, err = svc.HoldSlot(ctx, slotID, aliceID)
	if err != nil {
		t.Fatalf("failed to hold slot: %v", err)
	}

	_, err = svc.Checkout(ctx, CheckoutRequest{
		SlotID:         slotID.String(),
		IdempotencyKey: "key-poor-test",
	}, aliceID)
	if !errors.Is(err, wallet.ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	// Verify 0 bookings
	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM giki_sports.padel_bookings WHERE slot_id = $1;`, slotID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count bookings: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 bookings, got %d", count)
	}

	t.Log("Verified: Checkout rejected with ErrInsufficientFunds when user balance is lower than slot fee.")
}

// failingUnitOfWorkDecorator wraps a real UnitOfWork and deliberately injects an error
// right before commit to verify atomicity (QR-03).
type failingUnitOfWorkDecorator struct {
	realUoW     UnitOfWork
	injectedErr error
}

func (d *failingUnitOfWorkDecorator) Execute(ctx context.Context, fn func(factory TransactionRepositoryFactory) error) error {
	return d.realUoW.Execute(ctx, func(factory TransactionRepositoryFactory) error {
		if err := fn(factory); err != nil {
			return err
		}
		return d.injectedErr
	})
}
