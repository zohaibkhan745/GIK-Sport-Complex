//go:build integration

package content_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/auth"
	"giki-sports-backend/internal/authentication"
	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/content"
	"giki-sports-backend/internal/session"
	"giki-sports-backend/internal/user"
)

// TestContentHTTP_Integration exercises Fiber public and protected content endpoints with the real
// PostgreSQL repository and real session middleware against an isolated disposable database.
func TestContentHTTP_Integration(t *testing.T) {
	pool, teardown := setupDisposableDB(t)
	defer teardown()

	ctx := context.Background()

	// Step 1: Wire real authentication and user services for protected middleware
	userRepo := user.NewRepository(pool)
	sessionRepo := session.NewRepository(pool)

	authService, err := authentication.NewService(userRepo, sessionRepo)
	if err != nil {
		t.Fatalf("failed to initialize auth service: %v", err)
	}

	authCfg := config.AuthConfig{
		CookieName:          "giki_session",
		CookieSecure:        false,
		CookiePath:          "/api",
		CookieSameSite:      "Lax",
		TrustedOrigins:      []string{"http://127.0.0.1:5173", "http://localhost:5173"},
		LoginMaxConcurrent:  4,
		RequestTimeout:      5 * time.Second,
		LoginRateLimit:      50,
		LoginRateWindow:     time.Minute,
	}

	authHandler := authentication.NewHandler(authService, authCfg)

	// Step 2: Seed test users (director and student)
	passHash, err := auth.Hash("SecurePass123!")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	directorID := uuid.New()
	studentID := uuid.New()

	for _, u := range []struct {
		id    uuid.UUID
		email string
		name  string
		role  user.Role
	}{
		{id: directorID, email: "director@giki.test", name: "Sports Director", role: user.RoleSportsDirector},
		{id: studentID, email: "student@giki.test", name: "Student User", role: user.RoleStudent},
	} {
		_, err := pool.Exec(ctx, `
			INSERT INTO giki_sports.users (id, email, display_name, password_hash)
			VALUES ($1, $2, $3, $4);
		`, u.id, u.email, u.name, passHash)
		if err != nil {
			t.Fatalf("failed to seed user %s: %v", u.email, err)
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.user_roles (user_id, role)
			VALUES ($1, $2);
		`, u.id, u.role)
		if err != nil {
			t.Fatalf("failed to seed role for %s: %v", u.email, err)
		}
	}

	// Step 3: Wire content repository and handlers
	repo := content.NewRepository(pool)
	handler := content.NewHandler(repo, 5*time.Second)

	app := fiber.New()
	api := app.Group("/api")

	authHandler.RegisterRoutes(api)
	handler.RegisterPublicRoutes(api)
	handler.RegisterDraftRoutes(api,
		authHandler.RequireTrustedOrigin(),
		authHandler.RequireAuth(),
		authentication.RequireAnyRole(user.RoleSportsDirector, user.RoleSuperAdmin),
	)

	// Helper to login and acquire session cookie
	loginUser := func(email string) string {
		payload := fmt.Sprintf(`{"email":%q,"password":"SecurePass123!"}`, email)
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:5173")

		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatalf("login request failed for %s: %v", email, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("login failed for %s (status %d): %s", email, resp.StatusCode, string(body))
		}

		for _, c := range resp.Cookies() {
			if c.Name == "giki_session" {
				return c.Value
			}
		}
		t.Fatalf("session cookie not found for %s", email)
		return ""
	}

	directorCookie := loginUser("director@giki.test")
	studentCookie := loginUser("student@giki.test")

	// Step 4: Verify empty public feed returns HTTP 200 with JSON []
	t.Run("EmptyFeed_SignedOut_ReturnsEmptyArray", func(t *testing.T) {
		for _, path := range []string{"/api/content/announcements", "/api/content/achievements"} {
			req := httptest.NewRequest("GET", path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("GET %s failed: %v", path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("GET %s: expected HTTP 200, got %d", path, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			trimmed := strings.TrimSpace(string(body))
			if trimmed != "[]" {
				t.Errorf("GET %s: expected '[]', got %q", path, trimmed)
			}
		}
	})

	// Step 5: Insert synthetic test fixtures into disposable database
	annOlderID := uuid.New()
	annNewerID := uuid.New()
	achOlderID := uuid.New()
	annDraftID := uuid.New()
	achDraftID := uuid.New()
	annIncompleteDraftID := uuid.New()
	annFutureID := uuid.New()
	achFutureID := uuid.New()

	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id, published_at, created_at, updated_at)
		VALUES 
			-- Published Announcements (past dates)
			($1, 'announcement', 'Older Announcement', 'Body of older announcement', 'published', $9, now() - interval '2 days', now() - interval '2 days', now() - interval '2 days'),
			($2, 'announcement', 'Newer Announcement', 'Body of newer announcement', 'published', $9, now() - interval '1 day', now() - interval '1 day', now() - interval '1 day'),
			-- Published Achievement (past date)
			($3, 'achievement', 'Tennis Champions', 'Team won tournament', 'published', $9, now() - interval '3 days', now() - interval '3 days', now() - interval '3 days'),
			-- Complete Draft Announcement (complete text, but status = draft)
			($4, 'announcement', 'Complete Draft Announcement', 'Draft announcement body ready to publish', 'draft', $9, NULL, now(), now()),
			-- Complete Draft Achievement (status = draft)
			($5, 'achievement', 'Complete Draft Achievement', 'Draft achievement body', 'draft', $9, NULL, now(), now()),
			-- Incomplete Draft Announcement (status = draft, content is null)
			($6, 'announcement', 'Incomplete Draft Notice', NULL, 'draft', $9, NULL, now(), now()),
			-- Future-dated Announcement (status = published, published_at in future)
			($7, 'announcement', 'Future Announcement', 'Future notice', 'published', $9, now() + interval '1 day', now(), now()),
			-- Future-dated Achievement (status = published, published_at in future)
			($8, 'achievement', 'Future Achievement', 'Future trophy', 'published', $9, now() + interval '1 day', now(), now());
	`, annOlderID, annNewerID, achOlderID, annDraftID, achDraftID, annIncompleteDraftID, annFutureID, achFutureID, directorID)
	if err != nil {
		t.Fatalf("failed to insert test fixtures: %v", err)
	}

	// Step 6: Verify public feed filtering and sanitization
	t.Run("GetAnnouncements_SignedOut_FiltersAndSanitizes", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/content/announcements", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET /api/content/announcements failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		var items []content.PublicContentResponse
		if err := json.Unmarshal(body, &items); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		if len(items) != 2 {
			t.Fatalf("expected 2 published announcements, got %d", len(items))
		}

		if items[0].ID != annNewerID || items[1].ID != annOlderID {
			t.Errorf("ordering mismatch: expected %s then %s", annNewerID, annOlderID)
		}

		for _, it := range items {
			if it.ID == annDraftID || it.ID == annIncompleteDraftID || it.ID == annFutureID {
				t.Errorf("draft or future item %s unexpectedly returned in public feed", it.ID)
			}
		}
	})

	// Step 7: Protected Draft Endpoints Tests (Step 10F)
	t.Run("DraftEndpoints_AuthenticationAndRoleGuards", func(t *testing.T) {
		dummyDraftID := annDraftID.String()

		// 1. Signed-out requests are rejected with 401
		routes := []struct {
			method string
			path   string
			body   string
		}{
			{"POST", "/api/admin/content/drafts", `{"kind":"announcement"}`},
			{"GET", "/api/admin/content/drafts/" + dummyDraftID, ""},
			{"PUT", "/api/admin/content/drafts/" + dummyDraftID, `{"title":"T"}`},
			{"POST", "/api/admin/content/drafts/" + dummyDraftID + "/publish", ""},
		}

		for _, r := range routes {
			var bodyReader io.Reader
			if r.body != "" {
				bodyReader = strings.NewReader(r.body)
			}
			req := httptest.NewRequest(r.method, r.path, bodyReader)
			if r.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Origin", "http://127.0.0.1:5173")

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("signed-out request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Errorf("signed-out %s %s: expected 401 Unauthorized, got %d", r.method, r.path, resp.StatusCode)
			}
		}

		// 2. Student role requests are rejected with 403 Forbidden
		for _, r := range routes {
			var bodyReader io.Reader
			if r.body != "" {
				bodyReader = strings.NewReader(r.body)
			}
			req := httptest.NewRequest(r.method, r.path, bodyReader)
			if r.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Origin", "http://127.0.0.1:5173")
			req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", studentCookie))

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("student request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusForbidden {
				t.Errorf("student %s %s: expected 403 Forbidden, got %d", r.method, r.path, resp.StatusCode)
			}
		}

		// 3. Untrusted Origin is rejected with 403 for POST and PUT
		writeRoutes := []struct {
			method string
			path   string
			body   string
		}{
			{"POST", "/api/admin/content/drafts", `{"kind":"announcement"}`},
			{"PUT", "/api/admin/content/drafts/" + dummyDraftID, `{"title":"T"}`},
			{"POST", "/api/admin/content/drafts/" + dummyDraftID + "/publish", ""},
		}
		for _, r := range writeRoutes {
			var bodyReader io.Reader
			if r.body != "" {
				bodyReader = strings.NewReader(r.body)
			}
			req := httptest.NewRequest(r.method, r.path, bodyReader)
			if r.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Origin", "http://untrusted-origin.attacker.test")
			req.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("untrusted origin request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusForbidden {
				t.Errorf("untrusted origin %s %s: expected 403 Forbidden, got %d", r.method, r.path, resp.StatusCode)
			}
		}

		// 4. GET does NOT require Origin check
		reqGet := httptest.NewRequest("GET", "/api/admin/content/drafts/"+dummyDraftID, nil)
		reqGet.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))
		respGet, err := app.Test(reqGet)
		if err != nil {
			t.Fatalf("GET request failed: %v", err)
		}
		defer respGet.Body.Close()

		if respGet.StatusCode != fiber.StatusOK {
			t.Errorf("director GET without Origin header: expected 200 OK, got %d", respGet.StatusCode)
		}
	})

	// Step 8: Full lifecycle: create incomplete draft, reopen it, and edit it
	t.Run("Director_CreateIncompleteDraft_Reopen_And_Edit", func(t *testing.T) {
		// 1. Create incomplete draft with kind only
		createPayload := `{"kind":"announcement"}`
		reqCreate := httptest.NewRequest("POST", "/api/admin/content/drafts", strings.NewReader(createPayload))
		reqCreate.Header.Set("Content-Type", "application/json")
		reqCreate.Header.Set("Origin", "http://127.0.0.1:5173")
		reqCreate.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		respCreate, err := app.Test(reqCreate)
		if err != nil {
			t.Fatalf("POST /api/admin/content/drafts failed: %v", err)
		}
		defer respCreate.Body.Close()

		if respCreate.StatusCode != fiber.StatusCreated {
			body, _ := io.ReadAll(respCreate.Body)
			t.Fatalf("expected 201 Created, got %d: %s", respCreate.StatusCode, string(body))
		}

		// Verify Cache-Control: no-store
		if cc := respCreate.Header.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("expected Cache-Control: no-store on draft create, got %q", cc)
		}

		bodyCreate, _ := io.ReadAll(respCreate.Body)
		var createdDraft content.DraftResponse
		if err := json.Unmarshal(bodyCreate, &createdDraft); err != nil {
			t.Fatalf("failed to unmarshal created draft: %v", err)
		}

		if createdDraft.ID == uuid.Nil {
			t.Fatal("expected non-nil draft ID")
		}
		if createdDraft.Kind != "announcement" {
			t.Errorf("expected kind 'announcement', got %q", createdDraft.Kind)
		}
		if createdDraft.Title != nil || createdDraft.Content != nil {
			t.Error("expected nil title and content for incomplete draft")
		}
		if createdDraft.CreatorID != directorID {
			t.Errorf("expected creatorId %s, got %s", directorID, createdDraft.CreatorID)
		}

		newDraftID := createdDraft.ID

		// 2. Reopen draft via GET
		reqGet := httptest.NewRequest("GET", "/api/admin/content/drafts/"+newDraftID.String(), nil)
		reqGet.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		respGet, err := app.Test(reqGet)
		if err != nil {
			t.Fatalf("GET /api/admin/content/drafts/:id failed: %v", err)
		}
		defer respGet.Body.Close()

		if respGet.StatusCode != fiber.StatusOK {
			t.Fatalf("expected 200 OK reopening draft, got %d", respGet.StatusCode)
		}

		if cc := respGet.Header.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("expected Cache-Control: no-store on draft get, got %q", cc)
		}

		bodyGet, _ := io.ReadAll(respGet.Body)
		var reopenedDraft content.DraftResponse
		if err := json.Unmarshal(bodyGet, &reopenedDraft); err != nil {
			t.Fatalf("failed to unmarshal reopened draft: %v", err)
		}
		if reopenedDraft.ID != newDraftID {
			t.Errorf("expected draft ID %s, got %s", newDraftID, reopenedDraft.ID)
		}

		// 3. Edit draft via PUT
		updatePayload := `{"title":"Badminton Tournament Schedule","content":"Matches start at 9 AM in Main Hall."}`
		reqUpdate := httptest.NewRequest("PUT", "/api/admin/content/drafts/"+newDraftID.String(), strings.NewReader(updatePayload))
		reqUpdate.Header.Set("Content-Type", "application/json")
		reqUpdate.Header.Set("Origin", "http://127.0.0.1:5173")
		reqUpdate.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		respUpdate, err := app.Test(reqUpdate)
		if err != nil {
			t.Fatalf("PUT /api/admin/content/drafts/:id failed: %v", err)
		}
		defer respUpdate.Body.Close()

		if respUpdate.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(respUpdate.Body)
			t.Fatalf("expected 200 OK updating draft, got %d: %s", respUpdate.StatusCode, string(body))
		}

		if cc := respUpdate.Header.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("expected Cache-Control: no-store on draft update, got %q", cc)
		}

		bodyUpdate, _ := io.ReadAll(respUpdate.Body)
		var updatedDraft content.DraftResponse
		if err := json.Unmarshal(bodyUpdate, &updatedDraft); err != nil {
			t.Fatalf("failed to unmarshal updated draft: %v", err)
		}

		if updatedDraft.Title == nil || *updatedDraft.Title != "Badminton Tournament Schedule" {
			t.Errorf("expected updated title, got %v", updatedDraft.Title)
		}
		if updatedDraft.Content == nil || *updatedDraft.Content != "Matches start at 9 AM in Main Hall." {
			t.Errorf("expected updated content, got %v", updatedDraft.Content)
		}
		if updatedDraft.Kind != "announcement" {
			t.Errorf("expected kind to be preserved as 'announcement', got %q", updatedDraft.Kind)
		}
		if updatedDraft.CreatorID != directorID {
			t.Errorf("expected creatorId to be preserved as %s, got %s", directorID, updatedDraft.CreatorID)
		}

		// 4. Verify audit entries in giki_sports.audit_logs
		var createCount, updateCount int
		err = pool.QueryRow(ctx, `
			SELECT count(*) FROM giki_sports.audit_logs 
			WHERE action = 'content.draft.create' AND target_id = $1 AND actor_id = $2;
		`, newDraftID, directorID).Scan(&createCount)
		if err != nil {
			t.Fatalf("failed to query create audit log: %v", err)
		}
		if createCount != 1 {
			t.Errorf("expected 1 content.draft.create audit log, got %d", createCount)
		}

		err = pool.QueryRow(ctx, `
			SELECT count(*) FROM giki_sports.audit_logs 
			WHERE action = 'content.draft.update' AND target_id = $1 AND actor_id = $2;
		`, newDraftID, directorID).Scan(&updateCount)
		if err != nil {
			t.Fatalf("failed to query update audit log: %v", err)
		}
		if updateCount != 1 {
			t.Errorf("expected 1 content.draft.update audit log, got %d", updateCount)
		}

		// Verify audit details do not leak title or content
		var auditDetails string
		err = pool.QueryRow(ctx, `
			SELECT details FROM giki_sports.audit_logs
			WHERE action = 'content.draft.update' AND target_id = $1;
		`, newDraftID).Scan(&auditDetails)
		if err != nil {
			t.Fatalf("failed to inspect audit details: %v", err)
		}
		if strings.Contains(auditDetails, "Badminton Tournament") || strings.Contains(auditDetails, "Matches start") {
			t.Errorf("audit log leaked draft title or body: %s", auditDetails)
		}

		// 5. Confirm draft never appears on either public feed
		for _, publicPath := range []string{"/api/content/announcements", "/api/content/achievements"} {
			reqPub := httptest.NewRequest("GET", publicPath, nil)
			respPub, err := app.Test(reqPub)
			if err != nil {
				t.Fatalf("GET %s failed: %v", publicPath, err)
			}
			defer respPub.Body.Close()

			bodyPub, _ := io.ReadAll(respPub.Body)
			var pubItems []content.PublicContentResponse
			_ = json.Unmarshal(bodyPub, &pubItems)

			for _, it := range pubItems {
				if it.ID == newDraftID {
					t.Fatalf("draft %s was found in public feed %s!", newDraftID, publicPath)
				}
			}
		}

		// 6. Verify that a published item CANNOT be edited through PUT (returns 409 Conflict)
		reqPubEdit := httptest.NewRequest("PUT", "/api/admin/content/drafts/"+annOlderID.String(), strings.NewReader(`{"title":"Illegal Mutation"}`))
		reqPubEdit.Header.Set("Content-Type", "application/json")
		reqPubEdit.Header.Set("Origin", "http://127.0.0.1:5173")
		reqPubEdit.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		respPubEdit, err := app.Test(reqPubEdit)
		if err != nil {
			t.Fatalf("PUT on published item failed: %v", err)
		}
		defer respPubEdit.Body.Close()

		if respPubEdit.StatusCode != fiber.StatusConflict {
			body, _ := io.ReadAll(respPubEdit.Body)
			t.Errorf("expected 409 Conflict editing published item, got %d: %s", respPubEdit.StatusCode, string(body))
		}
	})

	// Step 9: Audited Publication Lifecycle & Guards (Step 10G)
	t.Run("Director_PublishDraft_AuditedLifecycle_And_Guards", func(t *testing.T) {
		// 1. Director publishes complete draft (annDraftID)
		reqPub := httptest.NewRequest("POST", "/api/admin/content/drafts/"+annDraftID.String()+"/publish", nil)
		reqPub.Header.Set("Origin", "http://127.0.0.1:5173")
		reqPub.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		respPub, err := app.Test(reqPub)
		if err != nil {
			t.Fatalf("POST /publish failed: %v", err)
		}
		defer respPub.Body.Close()

		if respPub.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(respPub.Body)
			t.Fatalf("expected HTTP 200 OK publishing complete draft, got %d: %s", respPub.StatusCode, string(body))
		}

		// Verify Cache-Control: no-store
		if cc := respPub.Header.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("expected Cache-Control: no-store on publish response, got %q", cc)
		}

		bodyPub, _ := io.ReadAll(respPub.Body)
		var pubResp content.PublicContentResponse
		if err := json.Unmarshal(bodyPub, &pubResp); err != nil {
			t.Fatalf("failed to unmarshal publish response: %v", err)
		}

		if pubResp.ID != annDraftID {
			t.Errorf("expected published ID %s, got %s", annDraftID, pubResp.ID)
		}
		if pubResp.Kind != "announcement" {
			t.Errorf("expected kind 'announcement', got %q", pubResp.Kind)
		}
		if pubResp.Title != "Complete Draft Announcement" {
			t.Errorf("expected title 'Complete Draft Announcement', got %q", pubResp.Title)
		}
		if pubResp.PublishedAt.IsZero() {
			t.Error("expected non-zero publication timestamp")
		}

		// 2. Signed-out GET returns published item in correct feed
		reqFeed := httptest.NewRequest("GET", "/api/content/announcements", nil)
		respFeed, err := app.Test(reqFeed)
		if err != nil {
			t.Fatalf("GET /api/content/announcements failed: %v", err)
		}
		defer respFeed.Body.Close()

		bodyFeed, _ := io.ReadAll(respFeed.Body)
		var feedItems []content.PublicContentResponse
		_ = json.Unmarshal(bodyFeed, &feedItems)

		foundInFeed := false
		for _, item := range feedItems {
			if item.ID == annDraftID {
				foundInFeed = true
				break
			}
		}
		if !foundInFeed {
			t.Errorf("published draft %s not found in public announcements feed", annDraftID)
		}

		// Verify achievements feed does not contain it (kind separation)
		reqAch := httptest.NewRequest("GET", "/api/content/achievements", nil)
		respAch, _ := app.Test(reqAch)
		defer respAch.Body.Close()
		bodyAch, _ := io.ReadAll(respAch.Body)
		var achItems []content.PublicContentResponse
		_ = json.Unmarshal(bodyAch, &achItems)
		for _, item := range achItems {
			if item.ID == annDraftID {
				t.Errorf("announcement %s leaked into achievements feed", annDraftID)
			}
		}

		// 3. Check audit entry identifies authenticated actor
		var publishAuditCount int
		err = pool.QueryRow(ctx, `
			SELECT count(*) FROM giki_sports.audit_logs
			WHERE action = 'content.publish' AND target_id = $1 AND actor_id = $2;
		`, annDraftID, directorID).Scan(&publishAuditCount)
		if err != nil {
			t.Fatalf("failed to query publish audit log: %v", err)
		}
		if publishAuditCount != 1 {
			t.Errorf("expected 1 content.publish audit entry, got %d", publishAuditCount)
		}

		// 4. Incomplete draft stays hidden and cannot be published (returns 422)
		reqIncPub := httptest.NewRequest("POST", "/api/admin/content/drafts/"+annIncompleteDraftID.String()+"/publish", nil)
		reqIncPub.Header.Set("Origin", "http://127.0.0.1:5173")
		reqIncPub.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		respIncPub, err := app.Test(reqIncPub)
		if err != nil {
			t.Fatalf("POST /publish on incomplete draft failed: %v", err)
		}
		defer respIncPub.Body.Close()

		if respIncPub.StatusCode != fiber.StatusUnprocessableEntity {
			body, _ := io.ReadAll(respIncPub.Body)
			t.Errorf("expected 422 Unprocessable Entity for incomplete draft, got %d: %s", respIncPub.StatusCode, string(body))
		}

		// Confirm incomplete draft is still absent from public feed
		reqFeedCheck := httptest.NewRequest("GET", "/api/content/announcements", nil)
		respFeedCheck, _ := app.Test(reqFeedCheck)
		defer respFeedCheck.Body.Close()
		bodyFeedCheck, _ := io.ReadAll(respFeedCheck.Body)
		var checkItems []content.PublicContentResponse
		_ = json.Unmarshal(bodyFeedCheck, &checkItems)
		for _, item := range checkItems {
			if item.ID == annIncompleteDraftID {
				t.Fatalf("incomplete draft %s unexpectedly appeared in public feed!", annIncompleteDraftID)
			}
		}

		// 5. Repeat publication adds no audit entry and returns 409 Conflict
		reqRepeat := httptest.NewRequest("POST", "/api/admin/content/drafts/"+annDraftID.String()+"/publish", nil)
		reqRepeat.Header.Set("Origin", "http://127.0.0.1:5173")
		reqRepeat.Header.Set("Cookie", fmt.Sprintf("giki_session=%s", directorCookie))

		respRepeat, err := app.Test(reqRepeat)
		if err != nil {
			t.Fatalf("repeat publish failed: %v", err)
		}
		defer respRepeat.Body.Close()

		if respRepeat.StatusCode != fiber.StatusConflict {
			body, _ := io.ReadAll(respRepeat.Body)
			t.Errorf("expected 409 Conflict for repeat publish, got %d: %s", respRepeat.StatusCode, string(body))
		}

		// Re-verify audit log count is still exactly 1
		err = pool.QueryRow(ctx, `
			SELECT count(*) FROM giki_sports.audit_logs
			WHERE action = 'content.publish' AND target_id = $1;
		`, annDraftID).Scan(&publishAuditCount)
		if err != nil {
			t.Fatalf("failed to check audit count after repeat: %v", err)
		}
		if publishAuditCount != 1 {
			t.Errorf("repeat publication created duplicate audit log; expected 1, got %d", publishAuditCount)
		}

		// 6. Forced audit failure leaves item a draft
		// Seed a complete draft
		forcedDraftID := uuid.New()
		_, err = pool.Exec(ctx, `
			INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id)
			VALUES ($1, 'announcement', 'Forced Failure Notice', 'Body text', 'draft', $2);
		`, forcedDraftID, directorID)
		if err != nil {
			t.Fatalf("failed to seed forced audit draft: %v", err)
		}

		// Build a test sub-app with non-existent actor in session to force foreign key violation on audit_logs
		badActorID := uuid.MustParse("99999999-9999-9999-9999-999999999999")
		badPrincipal := &authentication.AuthenticatedPrincipal{
			User: &user.User{
				ID:          badActorID,
				Email:       "ghost@giki.test",
				DisplayName: "Ghost User",
				Roles:       []user.Role{user.RoleSportsDirector},
			},
		}

		subApp := fiber.New()
		subAPI := subApp.Group("/api")
		subAPI.Use(func(c *fiber.Ctx) error {
			authentication.SetPrincipalForTest(c, badPrincipal)
			return c.Next()
		})
		passthrough := func(c *fiber.Ctx) error { return c.Next() }
		handler.RegisterDraftRoutes(subAPI, passthrough, passthrough, passthrough)

		reqForced := httptest.NewRequest("POST", "/api/admin/content/drafts/"+forcedDraftID.String()+"/publish", nil)
		respForced, err := subApp.Test(reqForced)
		if err != nil {
			t.Fatalf("forced audit test failed: %v", err)
		}
		defer respForced.Body.Close()

		if respForced.StatusCode != fiber.StatusInternalServerError {
			t.Errorf("expected 500 Internal Server Error on forced audit failure, got %d", respForced.StatusCode)
		}

		// Verify database state: row must remain in 'draft' status with published_at NULL
		var currentStatus string
		var currentPubAt *time.Time
		err = pool.QueryRow(ctx, `
			SELECT status, published_at FROM giki_sports.content_items WHERE id = $1;
		`, forcedDraftID).Scan(&currentStatus, &currentPubAt)
		if err != nil {
			t.Fatalf("failed to inspect status of forced failure item: %v", err)
		}
		if currentStatus != "draft" {
			t.Errorf("expected status to remain 'draft' after audit rollback, got %q", currentStatus)
		}
		if currentPubAt != nil {
			t.Errorf("expected published_at to remain nil after audit rollback, got %v", currentPubAt)
		}
	})

	// =========================================================================
	// STEP 10H: Protected Content Drafts Listing and Lifecycle
	// =========================================================================
	t.Run("Step10H_ListDrafts_Auth_Role_Filter_And_PublishLifecycle", func(t *testing.T) {
		// 1. Signed-out users are denied (401)
		reqSignedOut := httptest.NewRequest("GET", "/api/admin/content/drafts", nil)
		respSignedOut, err := app.Test(reqSignedOut)
		if err != nil {
			t.Fatalf("signed-out request failed: %v", err)
		}
		defer respSignedOut.Body.Close()
		if respSignedOut.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for signed-out user, got %d", respSignedOut.StatusCode)
		}

		// 2. Students are denied (403)
		reqStudent := httptest.NewRequest("GET", "/api/admin/content/drafts", nil)
		reqStudent.AddCookie(&http.Cookie{Name: "giki_session", Value: studentCookie})
		respStudent, err := app.Test(reqStudent)
		if err != nil {
			t.Fatalf("student request failed: %v", err)
		}
		defer respStudent.Body.Close()
		if respStudent.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for student user, got %d", respStudent.StatusCode)
		}

		// 3. Sports Director is authorized (200) without requiring an Origin header
		reqDirector := httptest.NewRequest("GET", "/api/admin/content/drafts", nil)
		reqDirector.AddCookie(&http.Cookie{Name: "giki_session", Value: directorCookie})
		respDirector, err := app.Test(reqDirector)
		if err != nil {
			t.Fatalf("director request failed: %v", err)
		}
		defer respDirector.Body.Close()
		if respDirector.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for sports director, got %d", respDirector.StatusCode)
		}
		if cc := respDirector.Header.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("expected Cache-Control: no-store, got %q", cc)
		}

		// 4. Create drafts with distinct content kinds
		// A: Incomplete announcement draft (no title, no content)
		bodyIncomplete := `{"kind":"announcement"}`
		reqInc := httptest.NewRequest("POST", "/api/admin/content/drafts", strings.NewReader(bodyIncomplete))
		reqInc.Header.Set("Content-Type", "application/json")
		reqInc.Header.Set("Origin", "http://127.0.0.1:5173")
		reqInc.AddCookie(&http.Cookie{Name: "giki_session", Value: directorCookie})
		respInc, _ := app.Test(reqInc)
		if respInc.StatusCode != http.StatusCreated {
			t.Fatalf("failed to create incomplete draft: status %d", respInc.StatusCode)
		}
		var incDraft content.DraftResponse
		_ = json.NewDecoder(respInc.Body).Decode(&incDraft)
		respInc.Body.Close()

		// B: Complete achievement draft
		bodyAchieve := `{"kind":"achievement","title":"Squash Championship","content":"GIKI won university squash finals."}`
		reqAch := httptest.NewRequest("POST", "/api/admin/content/drafts", strings.NewReader(bodyAchieve))
		reqAch.Header.Set("Content-Type", "application/json")
		reqAch.Header.Set("Origin", "http://127.0.0.1:5173")
		reqAch.AddCookie(&http.Cookie{Name: "giki_session", Value: directorCookie})
		respAch, _ := app.Test(reqAch)
		if respAch.StatusCode != http.StatusCreated {
			t.Fatalf("failed to create achievement draft: status %d", respAch.StatusCode)
		}
		var achDraft content.DraftResponse
		_ = json.NewDecoder(respAch.Body).Decode(&achDraft)
		respAch.Body.Close()

		// C: Complete announcement draft to be published
		bodyPub := `{"kind":"announcement","title":"Track Repair Scheduled","content":"Running track will be resurfaced this week."}`
		reqPubDraft := httptest.NewRequest("POST", "/api/admin/content/drafts", strings.NewReader(bodyPub))
		reqPubDraft.Header.Set("Content-Type", "application/json")
		reqPubDraft.Header.Set("Origin", "http://127.0.0.1:5173")
		reqPubDraft.AddCookie(&http.Cookie{Name: "giki_session", Value: directorCookie})
		respPubDraft, _ := app.Test(reqPubDraft)
		if respPubDraft.StatusCode != http.StatusCreated {
			t.Fatalf("failed to create announcement draft: status %d", respPubDraft.StatusCode)
		}
		var pubDraft content.DraftResponse
		_ = json.NewDecoder(respPubDraft.Body).Decode(&pubDraft)
		respPubDraft.Body.Close()

		// 5. Query all drafts -> must include incomplete draft, achievement draft, and announcement draft
		reqListAll := httptest.NewRequest("GET", "/api/admin/content/drafts", nil)
		reqListAll.AddCookie(&http.Cookie{Name: "giki_session", Value: directorCookie})
		respListAll, _ := app.Test(reqListAll)
		if respListAll.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 from ListDrafts, got %d", respListAll.StatusCode)
		}
		var allSummaries []content.DraftSummaryResponse
		_ = json.NewDecoder(respListAll.Body).Decode(&allSummaries)
		respListAll.Body.Close()

		// Verify that all 3 created drafts are present
		foundInc := false
		foundAch := false
		foundPub := false
		for _, s := range allSummaries {
			if s.ID == incDraft.ID {
				foundInc = true
				if s.Title != nil {
					t.Errorf("expected incomplete draft title to be null, got %v", *s.Title)
				}
				if s.Kind != "announcement" {
					t.Errorf("expected incomplete draft kind announcement, got %s", s.Kind)
				}
			}
			if s.ID == achDraft.ID {
				foundAch = true
				if s.Title == nil || *s.Title != "Squash Championship" {
					t.Errorf("expected achievement title 'Squash Championship', got %v", s.Title)
				}
			}
			if s.ID == pubDraft.ID {
				foundPub = true
			}
		}
		if !foundInc || !foundAch || !foundPub {
			t.Errorf("missing drafts from list: inc=%v, ach=%v, pub=%v", foundInc, foundAch, foundPub)
		}

		// 6. Test kind filtering: ?kind=announcement
		reqFilterAnn := httptest.NewRequest("GET", "/api/admin/content/drafts?kind=announcement", nil)
		reqFilterAnn.AddCookie(&http.Cookie{Name: "giki_session", Value: directorCookie})
		respFilterAnn, _ := app.Test(reqFilterAnn)
		if respFilterAnn.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for ?kind=announcement, got %d", respFilterAnn.StatusCode)
		}
		var annSummaries []content.DraftSummaryResponse
		_ = json.NewDecoder(respFilterAnn.Body).Decode(&annSummaries)
		respFilterAnn.Body.Close()

		for _, s := range annSummaries {
			if s.Kind != "announcement" {
				t.Errorf("unexpected non-announcement item in filtered list: %v", s)
			}
			if s.ID == achDraft.ID {
				t.Errorf("achievement draft appeared in announcement filter!")
			}
		}

		// 7. Test kind filtering: ?kind=achievement
		reqFilterAch := httptest.NewRequest("GET", "/api/admin/content/drafts?kind=achievement", nil)
		reqFilterAch.AddCookie(&http.Cookie{Name: "giki_session", Value: directorCookie})
		respFilterAch, _ := app.Test(reqFilterAch)
		if respFilterAch.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for ?kind=achievement, got %d", respFilterAch.StatusCode)
		}
		var achSummaries []content.DraftSummaryResponse
		_ = json.NewDecoder(respFilterAch.Body).Decode(&achSummaries)
		respFilterAch.Body.Close()

		for _, s := range achSummaries {
			if s.Kind != "achievement" {
				t.Errorf("unexpected non-achievement item in filtered list: %v", s)
			}
			if s.ID == incDraft.ID || s.ID == pubDraft.ID {
				t.Errorf("announcement draft appeared in achievement filter!")
			}
		}

		// 8. Test invalid kind filter returns 400 Bad Request
		reqInvalidKind := httptest.NewRequest("GET", "/api/admin/content/drafts?kind=invalid_filter", nil)
		reqInvalidKind.AddCookie(&http.Cookie{Name: "giki_session", Value: directorCookie})
		respInvalidKind, _ := app.Test(reqInvalidKind)
		if respInvalidKind.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for invalid kind query param, got %d", respInvalidKind.StatusCode)
		}
		respInvalidKind.Body.Close()

		// 9. Publish pubDraft -> it must drop out of draft list and appear in public announcements feed
		reqPublish := httptest.NewRequest("POST", "/api/admin/content/drafts/"+pubDraft.ID.String()+"/publish", nil)
		reqPublish.Header.Set("Origin", "http://127.0.0.1:5173")
		reqPublish.AddCookie(&http.Cookie{Name: "giki_session", Value: directorCookie})
		respPublish, err := app.Test(reqPublish)
		if err != nil {
			t.Fatalf("publish request failed: %v", err)
		}
		defer respPublish.Body.Close()
		if respPublish.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(respPublish.Body)
			t.Fatalf("publish failed (status %d): %s", respPublish.StatusCode, string(body))
		}

		// 10. Verify pubDraft has dropped out of GET /api/admin/content/drafts
		reqListAfterPub := httptest.NewRequest("GET", "/api/admin/content/drafts", nil)
		reqListAfterPub.AddCookie(&http.Cookie{Name: "giki_session", Value: directorCookie})
		respListAfterPub, _ := app.Test(reqListAfterPub)
		var summariesAfterPub []content.DraftSummaryResponse
		_ = json.NewDecoder(respListAfterPub.Body).Decode(&summariesAfterPub)
		respListAfterPub.Body.Close()

		for _, s := range summariesAfterPub {
			if s.ID == pubDraft.ID {
				t.Fatalf("published item %s still appeared in draft list after publication!", pubDraft.ID)
			}
		}

		// 11. Verify pubDraft appears in public feed GET /api/content/announcements
		reqPublicAnn := httptest.NewRequest("GET", "/api/content/announcements", nil)
		respPublicAnn, _ := app.Test(reqPublicAnn)
		if respPublicAnn.StatusCode != http.StatusOK {
			t.Fatalf("public announcements request failed: status %d", respPublicAnn.StatusCode)
		}
		var publicAnnouncements []content.PublicContentResponse
		_ = json.NewDecoder(respPublicAnn.Body).Decode(&publicAnnouncements)
		respPublicAnn.Body.Close()

		foundInPublic := false
		for _, pubItem := range publicAnnouncements {
			if pubItem.ID == pubDraft.ID {
				foundInPublic = true
				if pubItem.Title != "Track Repair Scheduled" {
					t.Errorf("expected public title 'Track Repair Scheduled', got %q", pubItem.Title)
				}
				if pubItem.Content != "Running track will be resurfaced this week." {
					t.Errorf("expected public content 'Running track will be resurfaced this week.', got %q", pubItem.Content)
				}
				break
			}
		}
		if !foundInPublic {
			t.Errorf("published announcement %s was not found in public announcements feed!", pubDraft.ID)
		}
	})
}
