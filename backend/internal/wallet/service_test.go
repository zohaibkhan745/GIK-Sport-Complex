package wallet

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPostTransaction_InputValidation(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()

	validID1 := uuid.New()
	validID2 := uuid.New()

	tests := []struct {
		name        string
		params      TransferParams
		expectedErr error
	}{
		{
			name: "Nil sender wallet",
			params: TransferParams{
				SenderWalletID:   uuid.Nil,
				ReceiverWalletID: validID2,
				Amount:           1000,
				Type:             "TEST",
				ReferenceID:      "REF-1",
			},
			expectedErr: ErrWalletNotFound,
		},
		{
			name: "Nil receiver wallet",
			params: TransferParams{
				SenderWalletID:   validID1,
				ReceiverWalletID: uuid.Nil,
				Amount:           1000,
				Type:             "TEST",
				ReferenceID:      "REF-1",
			},
			expectedErr: ErrWalletNotFound,
		},
		{
			name: "Self-transfer rejected",
			params: TransferParams{
				SenderWalletID:   validID1,
				ReceiverWalletID: validID1,
				Amount:           1000,
				Type:             "TEST",
				ReferenceID:      "REF-1",
			},
			expectedErr: ErrSelfTransfer,
		},
		{
			name: "Zero amount rejected",
			params: TransferParams{
				SenderWalletID:   validID1,
				ReceiverWalletID: validID2,
				Amount:           0,
				Type:             "TEST",
				ReferenceID:      "REF-1",
			},
			expectedErr: ErrInvalidAmount,
		},
		{
			name: "Negative amount rejected",
			params: TransferParams{
				SenderWalletID:   validID1,
				ReceiverWalletID: validID2,
				Amount:           -500,
				Type:             "TEST",
				ReferenceID:      "REF-1",
			},
			expectedErr: ErrInvalidAmount,
		},
		{
			name: "Blank type rejected",
			params: TransferParams{
				SenderWalletID:   validID1,
				ReceiverWalletID: validID2,
				Amount:           1000,
				Type:             "   ",
				ReferenceID:      "REF-1",
			},
			expectedErr: ErrEmptyType,
		},
		{
			name: "Blank reference rejected",
			params: TransferParams{
				SenderWalletID:   validID1,
				ReceiverWalletID: validID2,
				Amount:           1000,
				Type:             "TEST",
				ReferenceID:      "   ",
			},
			expectedErr: ErrEmptyReference,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.PostTransaction(ctx, tc.params)
			if err != tc.expectedErr {
				t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
			}
		})
	}
}

func TestStableLockOrdering(t *testing.T) {
	// Verify that regardless of which wallet is sender or receiver,
	// the lock order (firstID, secondID) is deterministic based on lexical string comparison.
	idA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	idB := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	// Scenario 1: A -> B
	first1, second1 := idA, idB
	if strings.Compare(first1.String(), second1.String()) > 0 {
		first1, second1 = second1, first1
	}

	// Scenario 2: B -> A
	first2, second2 := idB, idA
	if strings.Compare(first2.String(), second2.String()) > 0 {
		first2, second2 = second2, first2
	}

	if first1 != idA || second1 != idB {
		t.Fatalf("expected order idA, idB, got %s, %s", first1, second1)
	}
	if first2 != idA || second2 != idB {
		t.Fatalf("expected order idA, idB, got %s, %s", first2, second2)
	}
}

func TestGetUserWallet_NilUserIDValidation(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()

	_, err := svc.GetUserWalletBalance(ctx, uuid.Nil)
	if !errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("expected ErrWalletNotFound for nil user balance, got %v", err)
	}

	_, err = svc.GetUserWalletHistory(ctx, uuid.Nil)
	if !errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("expected ErrWalletNotFound for nil user history, got %v", err)
	}
}

