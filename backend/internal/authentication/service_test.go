package authentication_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"giki-sports-backend/internal/auth"
	"giki-sports-backend/internal/authentication"
	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
)

type mockUserRepo struct {
	getCredentialsByEmailFn func(ctx context.Context, email string) (*user.UserCredentials, error)
	getByIDFn               func(ctx context.Context, id uuid.UUID) (*user.User, error)
}

func (m *mockUserRepo) GetCredentialsByEmail(ctx context.Context, email string) (*user.UserCredentials, error) {
	if m.getCredentialsByEmailFn != nil {
		return m.getCredentialsByEmailFn(ctx, email)
	}
	return nil, user.ErrNotFound
}

func (m *mockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, user.ErrNotFound
}

type mockSessionStore struct {
	createFn           func(ctx context.Context, userID uuid.UUID, tokenHash string) (*session.Session, error)
	validateAndTouchFn func(ctx context.Context, tokenHash string) (*session.Session, error)
	revokeFn           func(ctx context.Context, tokenHash string) error
}

func (m *mockSessionStore) Create(ctx context.Context, userID uuid.UUID, tokenHash string) (*session.Session, error) {
	if m.createFn != nil {
		return m.createFn(ctx, userID, tokenHash)
	}
	return nil, errors.New("not implemented")
}

func (m *mockSessionStore) ValidateAndTouch(ctx context.Context, tokenHash string) (*session.Session, error) {
	if m.validateAndTouchFn != nil {
		return m.validateAndTouchFn(ctx, tokenHash)
	}
	return nil, session.ErrInvalidSession
}

func (m *mockSessionStore) Revoke(ctx context.Context, tokenHash string) error {
	if m.revokeFn != nil {
		return m.revokeFn(ctx, tokenHash)
	}
	return nil
}

func TestService_Login_Success(t *testing.T) {
	ctx := context.Background()
	testUserID := uuid.New()
	const rawTokenValue = "v1-valid-test-token-43-chars-padded-no-padding"
	tokenHash := strings.Repeat("a", 64)

	userRepo := &mockUserRepo{
		getCredentialsByEmailFn: func(ctx context.Context, email string) (*user.UserCredentials, error) {
			if email != "student@giki.edu.pk" {
				return nil, user.ErrNotFound
			}
			return &user.UserCredentials{
				User: user.User{
					ID:          testUserID,
					Email:       "student@giki.edu.pk",
					DisplayName: "Test Student",
					Status:      user.StatusActive,
					Roles:       []user.Role{user.RoleStudent},
				},
				PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2g",
			}, nil
		},
	}

	sessionCreated := false
	sessionRepo := &mockSessionStore{
		createFn: func(ctx context.Context, userID uuid.UUID, hash string) (*session.Session, error) {
			if userID != testUserID || hash != tokenHash {
				return nil, errors.New("mismatched create params")
			}
			sessionCreated = true
			now := time.Now().UTC()
			return &session.Session{
				ID:                uuid.New(),
				UserID:            userID,
				CreatedAt:         now,
				LastActivityAt:    now,
				AbsoluteExpiresAt: now.Add(8 * time.Hour),
			}, nil
		},
	}

	tokenGenCalled := false
	tokenGen := func() (string, string, error) {
		tokenGenCalled = true
		return rawTokenValue, tokenHash, nil
	}

	verifyFn := func(password, encodedHash string) (bool, error) {
		return password == "test-correct-pass", nil
	}

	svc := authentication.NewServiceWithParams(userRepo, sessionRepo, "dummy-hash", tokenGen, verifyFn)

	result, err := svc.Login(ctx, "Student@GIKI.edu.pk  ", "test-correct-pass")
	if err != nil {
		t.Fatalf("expected successful login, got %v", err)
	}

	if !tokenGenCalled {
		t.Error("expected token generator to be called on valid login")
	}
	if !sessionCreated {
		t.Error("expected session repository Create to be called")
	}
	if result.RawToken != rawTokenValue {
		t.Errorf("expected raw token %q, got %q", rawTokenValue, result.RawToken)
	}
	if result.User == nil || result.User.ID != testUserID {
		t.Errorf("expected user ID %s, got %v", testUserID, result.User)
	}
	if result.Session == nil || result.Session.UserID != testUserID {
		t.Errorf("expected session for user %s, got %v", testUserID, result.Session)
	}
}

func TestService_Login_Rejections_And_DummyVerification(t *testing.T) {
	ctx := context.Background()
	activeUserID := uuid.New()
	inactiveUserID := uuid.New()

	userRepo := &mockUserRepo{
		getCredentialsByEmailFn: func(ctx context.Context, email string) (*user.UserCredentials, error) {
			switch email {
			case "active@giki.edu.pk":
				return &user.UserCredentials{
					User: user.User{
						ID:     activeUserID,
						Email:  "active@giki.edu.pk",
						Status: user.StatusActive,
					},
					PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$aGFzaDE",
				}, nil
			case "inactive@giki.edu.pk":
				return &user.UserCredentials{
					User: user.User{
						ID:     inactiveUserID,
						Email:  "inactive@giki.edu.pk",
						Status: user.StatusInactive,
					},
					PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$aGFzaDI",
				}, nil
			default:
				return nil, user.ErrNotFound
			}
		},
	}

	sessionRepo := &mockSessionStore{}

	tokenGenCalled := false
	tokenGen := func() (string, string, error) {
		tokenGenCalled = true
		return "token", "hash", nil
	}

	dummyVerifyCount := 0
	verifyFn := func(password, encodedHash string) (bool, error) {
		if encodedHash == "precomputed-dummy-hash" {
			dummyVerifyCount++
			return false, nil
		}
		if password == "CorrectPassword!" {
			return true, nil
		}
		return false, nil
	}

	svc := authentication.NewServiceWithParams(userRepo, sessionRepo, "precomputed-dummy-hash", tokenGen, verifyFn)

	tests := []struct {
		name               string
		email              string
		password           string
		expectDummyVerify  bool
	}{
		{
			name:              "Unknown user",
			email:             "unknown@giki.edu.pk",
			password:          "test-any-pass",
			expectDummyVerify: true,
		},
		{
			name:              "Inactive account",
			email:             "inactive@giki.edu.pk",
			password:          "test-any-pass",
			expectDummyVerify: true,
		},
		{
			name:              "Wrong password",
			email:             "active@giki.edu.pk",
			password:          "test-wrong-pass",
			expectDummyVerify: false,
		},
		{
			name:              "Empty email",
			email:             "",
			password:          "test-any-pass",
			expectDummyVerify: true,
		},
		{
			name:              "Empty password",
			email:             "active@giki.edu.pk",
			password:          "",
			expectDummyVerify: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokenGenCalled = false
			initialDummyCount := dummyVerifyCount

			res, err := svc.Login(ctx, tc.email, tc.password)
			if !errors.Is(err, authentication.ErrInvalidCredentials) {
				t.Fatalf("expected ErrInvalidCredentials, got %v", err)
			}
			if res != nil {
				t.Fatalf("expected nil LoginResult on rejection, got %+v", res)
			}
			if tokenGenCalled {
				t.Errorf("token generation must NOT occur for rejected login attempt: %s", tc.name)
			}

			if tc.expectDummyVerify && dummyVerifyCount <= initialDummyCount {
				t.Errorf("expected dummy password verification to be invoked for %s", tc.name)
			}
		})
	}
}

func TestService_Login_Failures_And_MalformedHash(t *testing.T) {
	ctx := context.Background()
	testUserID := uuid.New()

	t.Run("EntropyFailureReturnsNoToken", func(t *testing.T) {
		userRepo := &mockUserRepo{
			getCredentialsByEmailFn: func(ctx context.Context, email string) (*user.UserCredentials, error) {
				return &user.UserCredentials{
					User:         user.User{ID: testUserID, Status: user.StatusActive},
					PasswordHash: "valid-hash",
				}, nil
			},
		}
		sessionRepo := &mockSessionStore{}
		tokenGen := func() (string, string, error) {
			return "", "", errors.New("entropy pool failure")
		}
		verifyFn := func(password, hash string) (bool, error) {
			return true, nil
		}

		svc := authentication.NewServiceWithParams(userRepo, sessionRepo, "dummy", tokenGen, verifyFn)
		res, err := svc.Login(ctx, "user@giki.edu.pk", "password")
		if err == nil {
			t.Fatal("expected error on token generation failure")
		}
		if res != nil {
			t.Fatalf("expected nil result on token generation failure, got %+v", res)
		}
	})

	t.Run("SessionPersistenceFailureReturnsNoToken", func(t *testing.T) {
		userRepo := &mockUserRepo{
			getCredentialsByEmailFn: func(ctx context.Context, email string) (*user.UserCredentials, error) {
				return &user.UserCredentials{
					User:         user.User{ID: testUserID, Status: user.StatusActive},
					PasswordHash: "valid-hash",
				}, nil
			},
		}
		sessionRepo := &mockSessionStore{
			createFn: func(ctx context.Context, userID uuid.UUID, tokenHash string) (*session.Session, error) {
				return nil, errors.New("database connection reset")
			},
		}
		svc := authentication.NewServiceWithParams(userRepo, sessionRepo, "dummy", nil, func(p, h string) (bool, error) { return true, nil })
		res, err := svc.Login(ctx, "user@giki.edu.pk", "password")
		if err == nil {
			t.Fatal("expected error on session persistence failure")
		}
		if res != nil {
			t.Fatalf("expected nil result on persistence failure, got %+v", res)
		}
	})

	t.Run("ConcurrentAccountDeactivationDuringCreationMapsToRejection", func(t *testing.T) {
		userRepo := &mockUserRepo{
			getCredentialsByEmailFn: func(ctx context.Context, email string) (*user.UserCredentials, error) {
				return &user.UserCredentials{
					User:         user.User{ID: testUserID, Status: user.StatusActive},
					PasswordHash: "valid-hash",
				}, nil
			},
		}
		sessionRepo := &mockSessionStore{
			createFn: func(ctx context.Context, userID uuid.UUID, tokenHash string) (*session.Session, error) {
				return nil, session.ErrUserNotFoundOrInactive
			},
		}
		svc := authentication.NewServiceWithParams(userRepo, sessionRepo, "dummy", nil, func(p, h string) (bool, error) { return true, nil })
		res, err := svc.Login(ctx, "user@giki.edu.pk", "password")
		if !errors.Is(err, authentication.ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials for concurrent deactivation, got %v", err)
		}
		if res != nil {
			t.Fatalf("expected nil result on rejection, got %+v", res)
		}
	})

	t.Run("MalformedStoredHashReturnsInternalError", func(t *testing.T) {
		userRepo := &mockUserRepo{
			getCredentialsByEmailFn: func(ctx context.Context, email string) (*user.UserCredentials, error) {
				return &user.UserCredentials{
					User:         user.User{ID: testUserID, Status: user.StatusActive},
					PasswordHash: "malformed-unparseable-hash",
				}, nil
			},
		}
		sessionRepo := &mockSessionStore{}
		verifyFn := func(password, hash string) (bool, error) {
			return false, auth.ErrInvalidHashFormat
		}

		svc := authentication.NewServiceWithParams(userRepo, sessionRepo, "dummy", nil, verifyFn)
		res, err := svc.Login(ctx, "user@giki.edu.pk", "password")
		if !errors.Is(err, authentication.ErrInternal) {
			t.Fatalf("expected ErrInternal for malformed stored hash, got %v", err)
		}
		if errors.Is(err, authentication.ErrInvalidCredentials) {
			t.Error("malformed hash must NOT be reported as generic ErrInvalidCredentials")
		}
		if res != nil {
			t.Fatalf("expected nil result on internal error, got %+v", res)
		}
	})
}

func TestService_ValidateSession(t *testing.T) {
	ctx := context.Background()
	testUserID := uuid.New()
	rawToken, tokenHash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("failed to generate valid session token: %v", err)
	}

	t.Run("SuccessfulValidationLoadsCurrentRoles", func(t *testing.T) {
		userRepo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*user.User, error) {
				if id != testUserID {
					return nil, user.ErrNotFound
				}
				return &user.User{
					ID:          testUserID,
					Email:       "director@giki.edu.pk",
					DisplayName: "Sports Director",
					Status:      user.StatusActive,
					Roles:       []user.Role{user.RoleFaculty, user.RoleSportsDirector},
				}, nil
			},
		}

		sessionRepo := &mockSessionStore{
			validateAndTouchFn: func(ctx context.Context, hash string) (*session.Session, error) {
				if hash != tokenHash {
					return nil, session.ErrInvalidSession
				}
				now := time.Now().UTC()
				return &session.Session{
					ID:                uuid.New(),
					UserID:            testUserID,
					CreatedAt:         now.Add(-10 * time.Minute),
					LastActivityAt:    now,
					AbsoluteExpiresAt: now.Add(8 * time.Hour),
				}, nil
			},
		}

		svc := authentication.NewServiceWithParams(userRepo, sessionRepo, "dummy", nil, nil)
		principal, err := svc.ValidateSession(ctx, rawToken)
		if err != nil {
			t.Fatalf("expected valid session, got %v", err)
		}
		if principal.User == nil || principal.User.ID != testUserID {
			t.Errorf("expected principal user ID %s, got %v", testUserID, principal.User)
		}
		if len(principal.User.Roles) != 2 || principal.User.Roles[1] != user.RoleSportsDirector {
			t.Errorf("expected updated roles [faculty, sports_director], got %v", principal.User.Roles)
		}
	})

	t.Run("InvalidTokenFormatDoesNotQuerySessionRepo", func(t *testing.T) {
		sessionQueried := false
		sessionRepo := &mockSessionStore{
			validateAndTouchFn: func(ctx context.Context, hash string) (*session.Session, error) {
				sessionQueried = true
				return nil, nil
			},
		}
		userRepo := &mockUserRepo{}

		svc := authentication.NewServiceWithParams(userRepo, sessionRepo, "dummy", nil, nil)

		badTokens := []string{
			"",
			"short",
			"too-long-token-with-more-than-forty-three-chars-123456",
			"bad-padding================================",
		}

		for _, bad := range badTokens {
			principal, err := svc.ValidateSession(ctx, bad)
			if !errors.Is(err, authentication.ErrInvalidSession) {
				t.Errorf("expected ErrInvalidSession for token %q, got %v", bad, err)
			}
			if principal != nil {
				t.Errorf("expected nil principal for %q, got %+v", bad, principal)
			}
		}

		if sessionQueried {
			t.Error("session repository should NOT be queried for malformed token strings")
		}
	})

	t.Run("ExpiredOrRevokedSessionReturnsErrInvalidSession", func(t *testing.T) {
		userRepo := &mockUserRepo{}
		sessionRepo := &mockSessionStore{
			validateAndTouchFn: func(ctx context.Context, hash string) (*session.Session, error) {
				return nil, session.ErrInvalidSession
			},
		}
		svc := authentication.NewServiceWithParams(userRepo, sessionRepo, "dummy", nil, nil)

		principal, err := svc.ValidateSession(ctx, rawToken)
		if !errors.Is(err, authentication.ErrInvalidSession) {
			t.Fatalf("expected ErrInvalidSession, got %v", err)
		}
		if principal != nil {
			t.Fatalf("expected nil principal, got %+v", principal)
		}
	})

	t.Run("InactiveUserAfterSessionValidationIsRejected", func(t *testing.T) {
		userRepo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*user.User, error) {
				return &user.User{
					ID:     testUserID,
					Status: user.StatusInactive,
				}, nil
			},
		}
		sessionRepo := &mockSessionStore{
			validateAndTouchFn: func(ctx context.Context, hash string) (*session.Session, error) {
				return &session.Session{UserID: testUserID}, nil
			},
		}
		svc := authentication.NewServiceWithParams(userRepo, sessionRepo, "dummy", nil, nil)

		principal, err := svc.ValidateSession(ctx, rawToken)
		if !errors.Is(err, authentication.ErrInvalidSession) {
			t.Fatalf("expected ErrInvalidSession for inactive user, got %v", err)
		}
		if principal != nil {
			t.Fatalf("expected nil principal, got %+v", principal)
		}
	})
}

func TestService_Logout(t *testing.T) {
	ctx := context.Background()
	rawToken, tokenHash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	t.Run("LogoutRevokesSessionIdempotently", func(t *testing.T) {
		revokedHashes := make(map[string]bool)
		sessionRepo := &mockSessionStore{
			revokeFn: func(ctx context.Context, hash string) error {
				if hash != tokenHash {
					return session.ErrInvalidSession
				}
				revokedHashes[hash] = true
				return nil
			},
		}
		svc := authentication.NewServiceWithParams(&mockUserRepo{}, sessionRepo, "dummy", nil, nil)

		// First logout: success
		if err := svc.Logout(ctx, rawToken); err != nil {
			t.Fatalf("first logout failed: %v", err)
		}
		if !revokedHashes[tokenHash] {
			t.Error("expected token hash to be revoked in session store")
		}

		// Repeated logout on already revoked / non-existent session
		sessionRepo.revokeFn = func(ctx context.Context, hash string) error {
			return session.ErrInvalidSession // returns not found or already invalid
		}
		if err := svc.Logout(ctx, rawToken); err != nil {
			t.Errorf("repeated logout must be harmless idempotent success, got %v", err)
		}

		// Malformed token: harmless
		if err := svc.Logout(ctx, "invalid-short-token"); err != nil {
			t.Errorf("malformed token logout must be harmless, got %v", err)
		}
	})

	t.Run("LogoutPropagatesInfrastructureFailures", func(t *testing.T) {
		infraErr := errors.New("database connection refused")
		sessionRepo := &mockSessionStore{
			revokeFn: func(ctx context.Context, hash string) error {
				return infraErr
			},
		}
		svc := authentication.NewServiceWithParams(&mockUserRepo{}, sessionRepo, "dummy", nil, nil)

		err := svc.Logout(ctx, rawToken)
		if !errors.Is(err, infraErr) {
			t.Fatalf("expected infrastructure error to be propagated, got %v", err)
		}
	})
}

func TestLoginResult_RedactionAndJSONOmission(t *testing.T) {
	testToken := "raw-sensitive-session-token-43-chars-value1"
	res := authentication.LoginResult{
		RawToken: testToken,
		User: &user.User{
			ID:          uuid.New(),
			Email:       "user@giki.edu.pk",
			DisplayName: "Normal User",
			Status:      user.StatusActive,
		},
		Session: &session.Session{
			ID:                uuid.New(),
			UserID:            uuid.New(),
			CreatedAt:         time.Now().UTC(),
			LastActivityAt:    time.Now().UTC(),
			AbsoluteExpiresAt: time.Now().UTC().Add(8 * time.Hour),
		},
	}

	// 1. JSON marshaling must omit raw token
	jsonData, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("failed to marshal LoginResult: %v", err)
	}
	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, testToken) || strings.Contains(jsonStr, "rawToken") || strings.Contains(jsonStr, "RawToken") {
		t.Errorf("JSON serialization leaked raw token: %s", jsonStr)
	}

	// 2. fmt.Stringer (%s, %v) must redact raw token
	strRep := res.String()
	if strings.Contains(strRep, testToken) {
		t.Errorf("String() leaked raw token: %s", strRep)
	}
	if !strings.Contains(strRep, "[REDACTED]") {
		t.Errorf("String() expected [REDACTED], got: %s", strRep)
	}

	// 3. GoStringer (%#v) must redact raw token
	goStrRep := fmt.Sprintf("%#v", res)
	if strings.Contains(goStrRep, testToken) {
		t.Errorf("GoString() leaked raw token: %s", goStrRep)
	}
	if !strings.Contains(goStrRep, "[REDACTED]") {
		t.Errorf("GoString() expected [REDACTED], got: %s", goStrRep)
	}
}
