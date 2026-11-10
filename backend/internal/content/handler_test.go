package content_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/authentication"
	"giki-sports-backend/internal/content"
	"giki-sports-backend/internal/user"
)

type fakeContentRepo struct {
	items            map[content.Kind][]content.PublishedItem
	drafts           map[uuid.UUID]*content.Draft
	listErr          error
	listDraftsErr    error
	createDraftErr   error
	getDraftErr      error
	updateDraftErr   error
	publishDraftErr  error
	observedDeadline time.Time
	hasDeadline      bool
	capturedAudit    content.AuditParams
}

func (f *fakeContentRepo) ListPublishedByKind(ctx context.Context, kind content.Kind) ([]content.PublishedItem, error) {
	dl, ok := ctx.Deadline()
	f.hasDeadline = ok
	f.observedDeadline = dl

	if f.listErr != nil {
		return nil, f.listErr
	}

	res, ok := f.items[kind]
	if !ok || res == nil {
		return make([]content.PublishedItem, 0), nil
	}
	return res, nil
}

func (f *fakeContentRepo) ListDrafts(ctx context.Context, kind content.Kind) ([]content.DraftSummary, error) {
	dl, ok := ctx.Deadline()
	f.hasDeadline = ok
	f.observedDeadline = dl

	if f.listDraftsErr != nil {
		return nil, f.listDraftsErr
	}

	if kind != "" {
		if err := content.ValidateKind(kind); err != nil {
			return nil, err
		}
	}

	res := make([]content.DraftSummary, 0)
	for _, d := range f.drafts {
		if d.Status != "draft" {
			continue
		}
		if kind != "" && d.Kind != kind {
			continue
		}
		res = append(res, content.DraftSummary{
			ID:        d.ID,
			Kind:      d.Kind,
			Title:     d.Title,
			UpdatedAt: d.UpdatedAt,
		})
	}

	sort.Slice(res, func(i, j int) bool {
		if res[i].UpdatedAt.Equal(res[j].UpdatedAt) {
			return res[i].ID.String() < res[j].ID.String()
		}
		return res[i].UpdatedAt.After(res[j].UpdatedAt)
	})

	if len(res) > 50 {
		res = res[:50]
	}
	return res, nil
}

func (f *fakeContentRepo) CreateDraft(ctx context.Context, input content.CreateDraftInput, audit content.AuditParams) (*content.Draft, error) {
	dl, ok := ctx.Deadline()
	f.hasDeadline = ok
	f.observedDeadline = dl
	f.capturedAudit = audit

	if f.createDraftErr != nil {
		return nil, f.createDraftErr
	}

	id := uuid.New()
	d := &content.Draft{
		ID:        id,
		Kind:      input.Kind,
		Title:     input.Title,
		Content:   input.Content,
		Status:    "draft",
		CreatorID: audit.ActorID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if f.drafts == nil {
		f.drafts = make(map[uuid.UUID]*content.Draft)
	}
	f.drafts[id] = d
	return d, nil
}

func (f *fakeContentRepo) GetDraftByID(ctx context.Context, id uuid.UUID) (*content.Draft, error) {
	dl, ok := ctx.Deadline()
	f.hasDeadline = ok
	f.observedDeadline = dl

	if f.getDraftErr != nil {
		return nil, f.getDraftErr
	}

	d, ok := f.drafts[id]
	if !ok {
		return nil, &content.DraftNotFoundError{ID: id}
	}
	if d.Status != "draft" {
		return nil, &content.NotADraftError{ID: id, Status: d.Status}
	}
	return d, nil
}

func (f *fakeContentRepo) UpdateDraft(ctx context.Context, id uuid.UUID, input content.UpdateDraftInput, audit content.AuditParams) (*content.Draft, error) {
	dl, ok := ctx.Deadline()
	f.hasDeadline = ok
	f.observedDeadline = dl
	f.capturedAudit = audit

	if f.updateDraftErr != nil {
		return nil, f.updateDraftErr
	}

	d, ok := f.drafts[id]
	if !ok {
		return nil, &content.DraftNotFoundError{ID: id}
	}
	if d.Status != "draft" {
		return nil, &content.NotADraftError{ID: id, Status: d.Status}
	}

	d.Title = input.Title
	d.Content = input.Content
	d.UpdatedAt = time.Now().UTC()
	return d, nil
}

func (f *fakeContentRepo) PublishDraft(ctx context.Context, id uuid.UUID, audit content.AuditParams) (*content.PublishedItem, error) {
	dl, ok := ctx.Deadline()
	f.hasDeadline = ok
	f.observedDeadline = dl
	f.capturedAudit = audit

	if f.publishDraftErr != nil {
		return nil, f.publishDraftErr
	}

	d, ok := f.drafts[id]
	if !ok {
		return nil, &content.DraftNotFoundError{ID: id}
	}
	if d.Status != "draft" {
		return nil, &content.NotADraftError{ID: id, Status: d.Status}
	}
	if d.Title == nil || strings.TrimSpace(*d.Title) == "" || d.Content == nil || strings.TrimSpace(*d.Content) == "" {
		return nil, &content.IncompleteDraftError{ID: id, Reason: "missing title or content"}
	}

	d.Status = "published"
	now := time.Now().UTC()
	d.PublishedAt = &now
	d.UpdatedAt = now

	return &content.PublishedItem{
		ID:          d.ID,
		Kind:        d.Kind,
		Title:       *d.Title,
		Content:     *d.Content,
		PublishedAt: now,
	}, nil
}

func setupTestApp(repo content.Repository, timeout time.Duration) *fiber.App {
	app := fiber.New()
	api := app.Group("/api")
	h := content.NewHandler(repo, timeout)
	h.RegisterPublicRoutes(api)
	return app
}

func samplePrincipal(role user.Role) *authentication.AuthenticatedPrincipal {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	return &authentication.AuthenticatedPrincipal{
		User: &user.User{
			ID:          userID,
			Email:       "director@example.test",
			DisplayName: "Sports Director",
			Status:      user.StatusActive,
			Roles:       []user.Role{role},
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	}
}

func setupDraftTestApp(
	fake *fakeContentRepo,
	timeout time.Duration,
	principal *authentication.AuthenticatedPrincipal,
) *fiber.App {
	app := fiber.New()
	api := app.Group("/api")

	if principal != nil {
		api.Use(func(c *fiber.Ctx) error {
			authentication.SetPrincipalForTest(c, principal)
			return c.Next()
		})
	}

	// Passthrough dummy middlewares for test harness
	dummyMiddleware := func(c *fiber.Ctx) error {
		return c.Next()
	}

	h := content.NewHandler(fake, timeout)
	h.RegisterPublicRoutes(api)
	h.RegisterDraftRoutes(api, dummyMiddleware, dummyMiddleware, dummyMiddleware)
	return app
}

func strPtr(s string) *string {
	return &s
}

// -----------------------------------------------------------------------------
// Public Endpoints Tests
// -----------------------------------------------------------------------------

func TestHandler_ListAnnouncements_Success(t *testing.T) {
	pubTime := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	id1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	id2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	fake := &fakeContentRepo{
		items: map[content.Kind][]content.PublishedItem{
			content.KindAnnouncement: {
				{
					ID:          id1,
					Kind:        content.KindAnnouncement,
					Title:       "Court Maintenance Notice",
					Content:     "Courts 1 and 2 closed tomorrow for resurfacing.",
					PublishedAt: pubTime,
				},
				{
					ID:          id2,
					Kind:        content.KindAnnouncement,
					Title:       "Badminton Trials",
					Content:     "Trials begin Monday at 5 PM.",
					PublishedAt: pubTime.Add(-2 * time.Hour),
				},
			},
		},
	}

	app := setupTestApp(fake, 3*time.Second)
	req := httptest.NewRequest("GET", "/api/content/announcements", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var items []content.PublicContentResponse
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 announcements, got %d", len(items))
	}
	if items[0].ID != id1 || items[0].Kind != "announcement" || items[0].Title != "Court Maintenance Notice" {
		t.Errorf("unexpected first item: %+v", items[0])
	}
	if items[1].ID != id2 || items[1].Kind != "announcement" || items[1].Title != "Badminton Trials" {
		t.Errorf("unexpected second item: %+v", items[1])
	}
	if !fake.hasDeadline {
		t.Error("expected bounded context deadline to be set on repository call")
	}
}

func TestHandler_ListAchievements_Success(t *testing.T) {
	pubTime := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	id := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	fake := &fakeContentRepo{
		items: map[content.Kind][]content.PublishedItem{
			content.KindAchievement: {
				{
					ID:          id,
					Kind:        content.KindAchievement,
					Title:       "Inter-varsity Table Tennis Gold",
					Content:     "GIKI Table Tennis team secured Gold at LUMS Sports Fest.",
					PublishedAt: pubTime,
				},
			},
		},
	}

	app := setupTestApp(fake, 3*time.Second)
	req := httptest.NewRequest("GET", "/api/content/achievements", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var items []content.PublicContentResponse
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 achievement, got %d", len(items))
	}
	if items[0].ID != id || items[0].Kind != "achievement" || items[0].Title != "Inter-varsity Table Tennis Gold" {
		t.Errorf("unexpected item: %+v", items[0])
	}
}

func TestHandler_EmptyFeed_ReturnsEmptySlice(t *testing.T) {
	fake := &fakeContentRepo{
		items: map[content.Kind][]content.PublishedItem{},
	}

	app := setupTestApp(fake, 3*time.Second)

	for _, path := range []string{"/api/content/announcements", "/api/content/achievements"} {
		req := httptest.NewRequest("GET", path, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected app.Test error for %s: %v", path, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("path %s: expected HTTP 200, got %d", path, resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("path %s: failed to read response body: %v", path, err)
		}

		trimmed := strings.TrimSpace(string(body))
		if trimmed != "[]" {
			t.Errorf("path %s: expected empty JSON array '[]', got %q", path, trimmed)
		}
	}
}

func TestHandler_DeadlineExceeded_Returns503(t *testing.T) {
	fake := &fakeContentRepo{
		listErr: context.DeadlineExceeded,
	}

	app := setupTestApp(fake, 1*time.Second)
	req := httptest.NewRequest("GET", "/api/content/announcements", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("expected HTTP 503, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	var errResp content.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}
	if errResp.Error != "service unavailable: request timed out" {
		t.Errorf("expected timeout message, got %q", errResp.Error)
	}
}

func TestHandler_UnexpectedError_Returns500(t *testing.T) {
	fake := &fakeContentRepo{
		listErr: errors.New("database connection pool exhausted"),
	}

	app := setupTestApp(fake, 1*time.Second)
	req := httptest.NewRequest("GET", "/api/content/achievements", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("expected HTTP 500, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	var errResp content.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}
	if errResp.Error != "internal server error" {
		t.Errorf("expected generic internal error, got %q", errResp.Error)
	}
	if strings.Contains(string(body), "exhausted") {
		t.Error("error response leaked internal database failure details")
	}
}

func TestHandler_NoInternalFieldsExposed(t *testing.T) {
	id := uuid.New()
	pubTime := time.Now().UTC()

	fake := &fakeContentRepo{
		items: map[content.Kind][]content.PublishedItem{
			content.KindAnnouncement: {
				{
					ID:          id,
					Kind:        content.KindAnnouncement,
					Title:       "Test Announcement",
					Content:     "Public content details.",
					PublishedAt: pubTime,
				},
			},
		},
	}

	app := setupTestApp(fake, 3*time.Second)
	req := httptest.NewRequest("GET", "/api/content/announcements", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var rawList []map[string]interface{}
	if err := json.Unmarshal(body, &rawList); err != nil {
		t.Fatalf("failed to unmarshal into generic map: %v", err)
	}

	if len(rawList) != 1 {
		t.Fatalf("expected 1 element, got %d", len(rawList))
	}

	rawMap := rawList[0]

	allowedFields := map[string]bool{
		"id":          true,
		"kind":        true,
		"title":       true,
		"content":     true,
		"publishedAt": true,
	}

	for k := range rawMap {
		if !allowedFields[k] {
			t.Errorf("unexpected field %q found in public content response", k)
		}
	}

	forbiddenFields := []string{
		"creator_id", "creatorId", "creator",
		"status",
		"created_at", "createdAt",
		"updated_at", "updatedAt",
		"audit", "audit_logs",
	}

	for _, field := range forbiddenFields {
		if _, exists := rawMap[field]; exists {
			t.Errorf("forbidden internal field %q was exposed in public response", field)
		}
	}
}

func TestHandler_SignedOutRequestsAllowed(t *testing.T) {
	fake := &fakeContentRepo{
		items: map[content.Kind][]content.PublishedItem{
			content.KindAnnouncement: {},
			content.KindAchievement:  {},
		},
	}

	app := setupTestApp(fake, 3*time.Second)

	for _, path := range []string{"/api/content/announcements", "/api/content/achievements"} {
		req := httptest.NewRequest("GET", path, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("signed-out request to %s: expected HTTP 200, got %d", path, resp.StatusCode)
		}
	}
}

// -----------------------------------------------------------------------------
// Protected Draft Endpoints Tests (Step 10F)
// -----------------------------------------------------------------------------

func TestHandler_CreateDraft_Success(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeContentRepo{}
	app := setupDraftTestApp(fake, 3*time.Second, principal)

	payload := `{"kind":"announcement","title":"Annual Sports Gala","content":"Gala begins next month."}`
	req := httptest.NewRequest("POST", "/api/admin/content/drafts", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP 201 Created, got %d: %s", resp.StatusCode, string(body))
	}

	// Verify Cache-Control: no-store
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %q", cc)
	}

	body, _ := io.ReadAll(resp.Body)
	var created content.DraftResponse
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("failed to unmarshal DraftResponse: %v", err)
	}

	if created.ID == uuid.Nil {
		t.Error("expected non-nil database generated draft UUID")
	}
	if created.Kind != "announcement" {
		t.Errorf("expected kind 'announcement', got %q", created.Kind)
	}
	if created.Title == nil || *created.Title != "Annual Sports Gala" {
		t.Errorf("unexpected title: %v", created.Title)
	}
	if created.Status != "draft" {
		t.Errorf("expected status 'draft', got %q", created.Status)
	}
	if created.CreatorID != principal.User.ID {
		t.Errorf("expected creatorId %s, got %s", principal.User.ID, created.CreatorID)
	}
	if fake.capturedAudit.ActorID != principal.User.ID {
		t.Errorf("expected audit actor %s, got %s", principal.User.ID, fake.capturedAudit.ActorID)
	}
}

func TestHandler_CreateDraft_IncompleteAllowed(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeContentRepo{}
	app := setupDraftTestApp(fake, 3*time.Second, principal)

	// Omit title and content entirely
	payload := `{"kind":"achievement"}`
	req := httptest.NewRequest("POST", "/api/admin/content/drafts", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP 201 Created for incomplete draft, got %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	var created content.DraftResponse
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if created.Kind != "achievement" {
		t.Errorf("expected kind 'achievement', got %q", created.Kind)
	}
	if created.Title != nil {
		t.Errorf("expected nil title for incomplete draft, got %q", *created.Title)
	}
	if created.Content != nil {
		t.Errorf("expected nil content for incomplete draft, got %q", *created.Content)
	}
}

func TestHandler_CreateDraft_Rejections(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	fake := &fakeContentRepo{}
	app := setupDraftTestApp(fake, 3*time.Second, principal)

	tests := []struct {
		name         string
		contentType  string
		payload      string
		expectedCode int
	}{
		{
			name:         "non-json content-type",
			contentType:  "text/plain",
			payload:      `{"kind":"announcement"}`,
			expectedCode: fiber.StatusUnsupportedMediaType,
		},
		{
			name:         "empty body",
			contentType:  "application/json",
			payload:      ``,
			expectedCode: fiber.StatusBadRequest,
		},
		{
			name:         "oversized body > 64KB",
			contentType:  "application/json",
			payload:      `{"kind":"announcement","content":"` + strings.Repeat("A", 65*1024) + `"}`,
			expectedCode: fiber.StatusRequestEntityTooLarge,
		},
		{
			name:         "unknown field actor_id rejected",
			contentType:  "application/json",
			payload:      `{"kind":"announcement","actor_id":"` + uuid.New().String() + `"}`,
			expectedCode: fiber.StatusBadRequest,
		},
		{
			name:         "unknown field status rejected",
			contentType:  "application/json",
			payload:      `{"kind":"announcement","status":"published"}`,
			expectedCode: fiber.StatusBadRequest,
		},
		{
			name:         "unknown field published_at rejected",
			contentType:  "application/json",
			payload:      `{"kind":"announcement","published_at":"2026-11-08T00:00:00Z"}`,
			expectedCode: fiber.StatusBadRequest,
		},
		{
			name:         "invalid kind rejected",
			contentType:  "application/json",
			payload:      `{"kind":"unknown_type"}`,
			expectedCode: fiber.StatusBadRequest,
		},
		{
			name:         "title exceeding 255 chars rejected",
			contentType:  "application/json",
			payload:      `{"kind":"announcement","title":"` + strings.Repeat("A", 256) + `"}`,
			expectedCode: fiber.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/admin/content/drafts", strings.NewReader(tc.payload))
			req.Header.Set("Content-Type", tc.contentType)

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("unexpected app.Test error: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedCode {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("expected HTTP %d, got %d: %s", tc.expectedCode, resp.StatusCode, string(body))
			}
		})
	}
}

func TestHandler_GetDraftByID_Success(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	draftID := uuid.New()
	title := "Reopened Draft Title"
	contentBody := "Draft content to be reopened."

	fake := &fakeContentRepo{
		drafts: map[uuid.UUID]*content.Draft{
			draftID: {
				ID:        draftID,
				Kind:      content.KindAnnouncement,
				Title:     &title,
				Content:   &contentBody,
				Status:    "draft",
				CreatorID: principal.User.ID,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
		},
	}

	app := setupDraftTestApp(fake, 3*time.Second, principal)
	req := httptest.NewRequest("GET", "/api/admin/content/drafts/"+draftID.String(), nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP 200, got %d: %s", resp.StatusCode, string(body))
	}

	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %q", cc)
	}

	body, _ := io.ReadAll(resp.Body)
	var got content.DraftResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("failed to unmarshal DraftResponse: %v", err)
	}

	if got.ID != draftID {
		t.Errorf("expected ID %s, got %s", draftID, got.ID)
	}
	if got.Title == nil || *got.Title != title {
		t.Errorf("expected title %q, got %v", title, got.Title)
	}
	if got.Status != "draft" {
		t.Errorf("expected status 'draft', got %q", got.Status)
	}
}

func TestHandler_GetDraftByID_Errors(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	publishedID := uuid.New()
	missingID := uuid.New()

	fake := &fakeContentRepo{
		drafts: map[uuid.UUID]*content.Draft{
			publishedID: {
				ID:     publishedID,
				Kind:   content.KindAnnouncement,
				Status: "published",
			},
		},
	}

	app := setupDraftTestApp(fake, 3*time.Second, principal)

	// Case 1: Invalid UUID format -> 400
	reqInvalid := httptest.NewRequest("GET", "/api/admin/content/drafts/not-a-valid-uuid", nil)
	respInvalid, _ := app.Test(reqInvalid)
	if respInvalid.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", respInvalid.StatusCode)
	}

	// Case 2: Missing draft -> 404
	reqMissing := httptest.NewRequest("GET", "/api/admin/content/drafts/"+missingID.String(), nil)
	respMissing, _ := app.Test(reqMissing)
	if respMissing.StatusCode != fiber.StatusNotFound {
		t.Errorf("expected 404 for missing draft, got %d", respMissing.StatusCode)
	}

	// Case 3: Published item (not a draft) -> 409
	reqPublished := httptest.NewRequest("GET", "/api/admin/content/drafts/"+publishedID.String(), nil)
	respPublished, _ := app.Test(reqPublished)
	if respPublished.StatusCode != fiber.StatusConflict {
		t.Errorf("expected 409 for non-draft item, got %d", respPublished.StatusCode)
	}
}

func TestHandler_UpdateDraft_Success(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	draftID := uuid.New()
	initialTitle := "Initial Title"
	initialContent := "Initial Content"

	fake := &fakeContentRepo{
		drafts: map[uuid.UUID]*content.Draft{
			draftID: {
				ID:        draftID,
				Kind:      content.KindAnnouncement,
				Title:     &initialTitle,
				Content:   &initialContent,
				Status:    "draft",
				CreatorID: principal.User.ID,
				CreatedAt: time.Now().Add(-1 * time.Hour).UTC(),
				UpdatedAt: time.Now().Add(-1 * time.Hour).UTC(),
			},
		},
	}

	app := setupDraftTestApp(fake, 3*time.Second, principal)

	payload := `{"title":"Edited Title","content":"Edited Content Body"}`
	req := httptest.NewRequest("PUT", "/api/admin/content/drafts/"+draftID.String(), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP 200, got %d: %s", resp.StatusCode, string(body))
	}

	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %q", cc)
	}

	body, _ := io.ReadAll(resp.Body)
	var updated content.DraftResponse
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatalf("failed to unmarshal DraftResponse: %v", err)
	}

	if updated.Title == nil || *updated.Title != "Edited Title" {
		t.Errorf("expected title 'Edited Title', got %v", updated.Title)
	}
	if updated.Content == nil || *updated.Content != "Edited Content Body" {
		t.Errorf("expected content 'Edited Content Body', got %v", updated.Content)
	}
	if updated.Kind != "announcement" {
		t.Errorf("expected kind to be preserved as 'announcement', got %q", updated.Kind)
	}
	if updated.CreatorID != principal.User.ID {
		t.Errorf("expected creatorId to be preserved as %s, got %s", principal.User.ID, updated.CreatorID)
	}
}

func TestHandler_UpdateDraft_ClearsOmittedOrNullFields(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	draftID := uuid.New()
	initialTitle := "Old Title"
	initialContent := "Old Content"

	fake := &fakeContentRepo{
		drafts: map[uuid.UUID]*content.Draft{
			draftID: {
				ID:        draftID,
				Kind:      content.KindAchievement,
				Title:     &initialTitle,
				Content:   &initialContent,
				Status:    "draft",
				CreatorID: principal.User.ID,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
		},
	}

	app := setupDraftTestApp(fake, 3*time.Second, principal)

	// Sending empty JSON object {} replaces both fields with nil/null
	payload := `{}`
	req := httptest.NewRequest("PUT", "/api/admin/content/drafts/"+draftID.String(), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP 200, got %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	var cleared content.DraftResponse
	if err := json.Unmarshal(body, &cleared); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if cleared.Title != nil {
		t.Errorf("expected title to be cleared (nil), got %v", cleared.Title)
	}
	if cleared.Content != nil {
		t.Errorf("expected content to be cleared (nil), got %v", cleared.Content)
	}
}

func TestHandler_UpdateDraft_Rejections(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	draftID := uuid.New()
	publishedID := uuid.New()
	missingID := uuid.New()

	initialTitle := "Some Draft"
	fake := &fakeContentRepo{
		drafts: map[uuid.UUID]*content.Draft{
			draftID: {
				ID:     draftID,
				Kind:   content.KindAnnouncement,
				Title:  &initialTitle,
				Status: "draft",
			},
			publishedID: {
				ID:     publishedID,
				Kind:   content.KindAnnouncement,
				Status: "published",
			},
		},
	}

	app := setupDraftTestApp(fake, 3*time.Second, principal)

	tests := []struct {
		name         string
		targetID     string
		contentType  string
		payload      string
		expectedCode int
	}{
		{
			name:         "invalid UUID param",
			targetID:     "not-a-uuid",
			contentType:  "application/json",
			payload:      `{"title":"Updated"}`,
			expectedCode: fiber.StatusBadRequest,
		},
		{
			name:         "non-json media type",
			targetID:     draftID.String(),
			contentType:  "text/plain",
			payload:      `{"title":"Updated"}`,
			expectedCode: fiber.StatusUnsupportedMediaType,
		},
		{
			name:         "empty body",
			targetID:     draftID.String(),
			contentType:  "application/json",
			payload:      ``,
			expectedCode: fiber.StatusBadRequest,
		},
		{
			name:         "oversized body > 64KB",
			targetID:     draftID.String(),
			contentType:  "application/json",
			payload:      `{"content":"` + strings.Repeat("A", 65*1024) + `"}`,
			expectedCode: fiber.StatusRequestEntityTooLarge,
		},
		{
			name:         "unknown field kind rejected",
			targetID:     draftID.String(),
			contentType:  "application/json",
			payload:      `{"kind":"achievement","title":"New Title"}`,
			expectedCode: fiber.StatusBadRequest,
		},
		{
			name:         "unknown field actor_id rejected",
			targetID:     draftID.String(),
			contentType:  "application/json",
			payload:      `{"actor_id":"` + uuid.New().String() + `"}`,
			expectedCode: fiber.StatusBadRequest,
		},
		{
			name:         "title too long > 255 chars",
			targetID:     draftID.String(),
			contentType:  "application/json",
			payload:      `{"title":"` + strings.Repeat("T", 256) + `"}`,
			expectedCode: fiber.StatusBadRequest,
		},
		{
			name:         "missing draft -> 404",
			targetID:     missingID.String(),
			contentType:  "application/json",
			payload:      `{"title":"New Title"}`,
			expectedCode: fiber.StatusNotFound,
		},
		{
			name:         "published item (not a draft) -> 409",
			targetID:     publishedID.String(),
			contentType:  "application/json",
			payload:      `{"title":"New Title"}`,
			expectedCode: fiber.StatusConflict,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "/api/admin/content/drafts/"+tc.targetID, strings.NewReader(tc.payload))
			req.Header.Set("Content-Type", tc.contentType)

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("unexpected app.Test error: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedCode {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("expected HTTP %d, got %d: %s", tc.expectedCode, resp.StatusCode, string(body))
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Protected Publish Draft Endpoints Tests (Step 10G)
// -----------------------------------------------------------------------------

func TestHandler_PublishDraft_Success(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	draftID := uuid.New()
	title := "Badminton Tournament Final"
	contentBody := "Final matches will be played on Sunday at 4 PM."

	fake := &fakeContentRepo{
		drafts: map[uuid.UUID]*content.Draft{
			draftID: {
				ID:        draftID,
				Kind:      content.KindAnnouncement,
				Title:     &title,
				Content:   &contentBody,
				Status:    "draft",
				CreatorID: principal.User.ID,
				CreatedAt: time.Now().Add(-2 * time.Hour).UTC(),
				UpdatedAt: time.Now().Add(-1 * time.Hour).UTC(),
			},
		},
	}

	app := setupDraftTestApp(fake, 3*time.Second, principal)

	// Valid POST to /publish with NO body
	req := httptest.NewRequest("POST", "/api/admin/content/drafts/"+draftID.String()+"/publish", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP 200 OK, got %d: %s", resp.StatusCode, string(body))
	}

	// Verify Cache-Control: no-store
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %q", cc)
	}

	// Verify response matches PublicContentResponse
	body, _ := io.ReadAll(resp.Body)
	var pubItem content.PublicContentResponse
	if err := json.Unmarshal(body, &pubItem); err != nil {
		t.Fatalf("failed to unmarshal PublicContentResponse: %v", err)
	}

	if pubItem.ID != draftID {
		t.Errorf("expected ID %s, got %s", draftID, pubItem.ID)
	}
	if pubItem.Kind != "announcement" {
		t.Errorf("expected kind 'announcement', got %q", pubItem.Kind)
	}
	if pubItem.Title != title {
		t.Errorf("expected title %q, got %q", title, pubItem.Title)
	}
	if pubItem.Content != contentBody {
		t.Errorf("expected content %q, got %q", contentBody, pubItem.Content)
	}
	if pubItem.PublishedAt.IsZero() {
		t.Error("expected non-zero publishedAt timestamp")
	}

	// Verify audit params captured principal actor
	if fake.capturedAudit.ActorID != principal.User.ID {
		t.Errorf("expected audit actor %s, got %s", principal.User.ID, fake.capturedAudit.ActorID)
	}
}

func TestHandler_PublishDraft_RejectionsAndErrors(t *testing.T) {
	principal := samplePrincipal(user.RoleSportsDirector)
	validDraftID := uuid.New()
	incompleteDraftID := uuid.New()
	publishedID := uuid.New()
	missingID := uuid.New()

	validTitle := "Valid Title"
	validContent := "Valid Content"

	fake := &fakeContentRepo{
		drafts: map[uuid.UUID]*content.Draft{
			validDraftID: {
				ID:        validDraftID,
				Kind:      content.KindAnnouncement,
				Title:     &validTitle,
				Content:   &validContent,
				Status:    "draft",
				CreatorID: principal.User.ID,
			},
			incompleteDraftID: {
				ID:        incompleteDraftID,
				Kind:      content.KindAnnouncement,
				Title:     nil, // missing title
				Content:   &validContent,
				Status:    "draft",
				CreatorID: principal.User.ID,
			},
			publishedID: {
				ID:        publishedID,
				Kind:      content.KindAnnouncement,
				Title:     &validTitle,
				Content:   &validContent,
				Status:    "published",
				CreatorID: principal.User.ID,
			},
		},
	}

	app := setupDraftTestApp(fake, 3*time.Second, principal)

	// 1. Unexpected request body -> 400
	reqWithBody := httptest.NewRequest("POST", "/api/admin/content/drafts/"+validDraftID.String()+"/publish", strings.NewReader(`{"title":"New Title"}`))
	reqWithBody.Header.Set("Content-Type", "application/json")
	respWithBody, _ := app.Test(reqWithBody)
	if respWithBody.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for unexpected body, got %d", respWithBody.StatusCode)
	}

	// 2. Invalid UUID parameter -> 400
	reqInvalidUUID := httptest.NewRequest("POST", "/api/admin/content/drafts/not-a-valid-uuid/publish", nil)
	respInvalidUUID, _ := app.Test(reqInvalidUUID)
	if respInvalidUUID.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", respInvalidUUID.StatusCode)
	}

	// 3. Draft not found -> 404
	reqMissing := httptest.NewRequest("POST", "/api/admin/content/drafts/"+missingID.String()+"/publish", nil)
	respMissing, _ := app.Test(reqMissing)
	if respMissing.StatusCode != fiber.StatusNotFound {
		t.Errorf("expected 404 for missing draft, got %d", respMissing.StatusCode)
	}

	// 4. Already published item -> 409
	reqAlreadyPublished := httptest.NewRequest("POST", "/api/admin/content/drafts/"+publishedID.String()+"/publish", nil)
	respAlreadyPublished, _ := app.Test(reqAlreadyPublished)
	if respAlreadyPublished.StatusCode != fiber.StatusConflict {
		t.Errorf("expected 409 for already published item, got %d", respAlreadyPublished.StatusCode)
	}

	// 5. Incomplete draft -> 422 Unprocessable Entity
	reqIncomplete := httptest.NewRequest("POST", "/api/admin/content/drafts/"+incompleteDraftID.String()+"/publish", nil)
	respIncomplete, _ := app.Test(reqIncomplete)
	if respIncomplete.StatusCode != fiber.StatusUnprocessableEntity {
		body, _ := io.ReadAll(respIncomplete.Body)
		t.Errorf("expected 422 for incomplete draft, got %d: %s", respIncomplete.StatusCode, string(body))
	}

	// 6. Timeout -> 503
	fake.publishDraftErr = context.DeadlineExceeded
	reqTimeout := httptest.NewRequest("POST", "/api/admin/content/drafts/"+validDraftID.String()+"/publish", nil)
	respTimeout, _ := app.Test(reqTimeout)
	if respTimeout.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("expected 503 on timeout, got %d", respTimeout.StatusCode)
	}

	// 7. Unexpected error -> 500
	fake.publishDraftErr = errors.New("database connection broken")
	reqErr := httptest.NewRequest("POST", "/api/admin/content/drafts/"+validDraftID.String()+"/publish", nil)
	respErr, _ := app.Test(reqErr)
	if respErr.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("expected 500 on unexpected error, got %d", respErr.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// Step 10H: GET /api/admin/content/drafts Unit Tests
// -----------------------------------------------------------------------------

func TestHandler_ListDrafts_Success(t *testing.T) {
	now := time.Date(2026, 11, 8, 14, 0, 0, 0, time.UTC)
	id1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	id2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	fake := &fakeContentRepo{
		drafts: map[uuid.UUID]*content.Draft{
			id1: {
				ID:        id1,
				Kind:      content.KindAnnouncement,
				Title:     content.StringPtr("Inter-batch Table Tennis"),
				Content:   content.StringPtr("Registration opens tomorrow."),
				Status:    "draft",
				CreatorID: uuid.New(),
				CreatedAt: now.Add(-2 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
			id2: {
				ID:        id2,
				Kind:      content.KindAchievement,
				Title:     nil, // incomplete draft
				Content:   nil,
				Status:    "draft",
				CreatorID: uuid.New(),
				CreatedAt: now.Add(-3 * time.Hour),
				UpdatedAt: now, // most recently updated
			},
		},
	}

	app := setupDraftTestApp(fake, 3*time.Second, samplePrincipal(user.RoleSportsDirector))

	req := httptest.NewRequest("GET", "/api/admin/content/drafts", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %q", cc)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var summaries []content.DraftSummaryResponse
	if err := json.Unmarshal(body, &summaries); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v\nBody: %s", err, string(body))
	}

	if len(summaries) != 2 {
		t.Fatalf("expected 2 draft summaries, got %d", len(summaries))
	}

	// id2 was updated most recently (now), so it should be first
	if summaries[0].ID != id2 {
		t.Errorf("expected first item to be %s (newest updated), got %s", id2, summaries[0].ID)
	}
	if summaries[0].Title != nil {
		t.Errorf("expected first item (incomplete draft) to have null title, got %v", *summaries[0].Title)
	}
	if summaries[0].Kind != "achievement" {
		t.Errorf("expected kind 'achievement', got %s", summaries[0].Kind)
	}

	if summaries[1].ID != id1 {
		t.Errorf("expected second item to be %s, got %s", id1, summaries[1].ID)
	}
	if summaries[1].Title == nil || *summaries[1].Title != "Inter-batch Table Tennis" {
		t.Errorf("expected title 'Inter-batch Table Tennis', got %v", summaries[1].Title)
	}
	if summaries[1].Kind != "announcement" {
		t.Errorf("expected kind 'announcement', got %s", summaries[1].Kind)
	}

	// Verify that the body is NOT exposed in the raw JSON
	var rawMaps []map[string]interface{}
	_ = json.Unmarshal(body, &rawMaps)
	for i, m := range rawMaps {
		if _, exists := m["content"]; exists {
			t.Errorf("summary %d exposed internal content body!", i)
		}
		if _, exists := m["creatorId"]; exists {
			t.Errorf("summary %d exposed creatorId!", i)
		}
		if _, exists := m["status"]; exists {
			t.Errorf("summary %d exposed status!", i)
		}
	}
}

func TestHandler_ListDrafts_EmptyArray(t *testing.T) {
	fake := &fakeContentRepo{
		drafts: make(map[uuid.UUID]*content.Draft),
	}

	app := setupDraftTestApp(fake, 3*time.Second, samplePrincipal(user.RoleSportsDirector))

	req := httptest.NewRequest("GET", "/api/admin/content/drafts", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "[]" {
		t.Fatalf("expected strict empty JSON array '[]', got %q", string(body))
	}
}

func TestHandler_ListDrafts_KindFilter(t *testing.T) {
	idAnnounce := uuid.New()
	idAchieve := uuid.New()

	fake := &fakeContentRepo{
		drafts: map[uuid.UUID]*content.Draft{
			idAnnounce: {
				ID:        idAnnounce,
				Kind:      content.KindAnnouncement,
				Title:     content.StringPtr("Announcement Draft"),
				Status:    "draft",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
			idAchieve: {
				ID:        idAchieve,
				Kind:      content.KindAchievement,
				Title:     content.StringPtr("Achievement Draft"),
				Status:    "draft",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
		},
	}

	app := setupDraftTestApp(fake, 3*time.Second, samplePrincipal(user.RoleSportsDirector))

	// Filter by announcement
	reqAnnounce := httptest.NewRequest("GET", "/api/admin/content/drafts?kind=announcement", nil)
	respAnnounce, _ := app.Test(reqAnnounce)
	if respAnnounce.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 for ?kind=announcement, got %d", respAnnounce.StatusCode)
	}
	var announceSummaries []content.DraftSummaryResponse
	bodyAnnounce, _ := io.ReadAll(respAnnounce.Body)
	_ = json.Unmarshal(bodyAnnounce, &announceSummaries)
	if len(announceSummaries) != 1 || announceSummaries[0].ID != idAnnounce {
		t.Errorf("expected only announcement draft, got: %v", announceSummaries)
	}

	// Filter by achievement
	reqAchieve := httptest.NewRequest("GET", "/api/admin/content/drafts?kind=achievement", nil)
	respAchieve, _ := app.Test(reqAchieve)
	if respAchieve.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 for ?kind=achievement, got %d", respAchieve.StatusCode)
	}
	var achieveSummaries []content.DraftSummaryResponse
	bodyAchieve, _ := io.ReadAll(respAchieve.Body)
	_ = json.Unmarshal(bodyAchieve, &achieveSummaries)
	if len(achieveSummaries) != 1 || achieveSummaries[0].ID != idAchieve {
		t.Errorf("expected only achievement draft, got: %v", achieveSummaries)
	}
}

func TestHandler_ListDrafts_InvalidKind_Returns400(t *testing.T) {
	fake := &fakeContentRepo{}
	app := setupDraftTestApp(fake, 3*time.Second, samplePrincipal(user.RoleSportsDirector))

	for _, invalidKind := range []string{"invalid", "tournament", "ANNOUNCEMENT", "achievement%20"} {
		req := httptest.NewRequest("GET", "/api/admin/content/drafts?kind="+invalidKind, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected app.Test error: %v", err)
		}
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("expected HTTP 400 for invalid kind %q, got %d", invalidKind, resp.StatusCode)
		}
	}
}

func TestHandler_ListDrafts_Errors(t *testing.T) {
	fake := &fakeContentRepo{
		listDraftsErr: context.DeadlineExceeded,
	}

	app := setupDraftTestApp(fake, 1*time.Second, samplePrincipal(user.RoleSportsDirector))

	// 1. Timeout -> 503
	reqTimeout := httptest.NewRequest("GET", "/api/admin/content/drafts", nil)
	respTimeout, _ := app.Test(reqTimeout)
	if respTimeout.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("expected 503 on timeout, got %d", respTimeout.StatusCode)
	}

	// 2. Unexpected error -> 500
	fake.listDraftsErr = errors.New("database connection pool exhausted")
	reqErr := httptest.NewRequest("GET", "/api/admin/content/drafts", nil)
	respErr, _ := app.Test(reqErr)
	if respErr.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("expected 500 on unexpected error, got %d", respErr.StatusCode)
	}
}

func TestHandler_PublicContent_RejectsDirectWrites(t *testing.T) {
	fake := &fakeContentRepo{
		items: map[content.Kind][]content.PublishedItem{},
	}
	app := setupTestApp(fake, 3*time.Second)

	for _, writeReq := range []struct {
		method string
		path   string
	}{
		{"POST", "/api/content/announcements"},
		{"POST", "/api/content/achievements"},
		{"PUT", "/api/content/announcements"},
		{"DELETE", "/api/content/announcements"},
		{"DELETE", "/api/content/achievements"},
	} {
		req := httptest.NewRequest(writeReq.method, writeReq.path, strings.NewReader(`{"title":"hack"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected app.Test error for %s %s: %v", writeReq.method, writeReq.path, err)
		}
		resp.Body.Close()

		if resp.StatusCode != fiber.StatusMethodNotAllowed && resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("expected 404 or 405 for direct unauthenticated write %s %s, got %d",
				writeReq.method, writeReq.path, resp.StatusCode)
		}
	}
}
