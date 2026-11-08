//go:build integration

package content_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/content"
	"giki-sports-backend/internal/database"
)

// setupDisposableDB creates a disposable test database, executes all migrations,
// and returns an initialized *pgxpool.Pool alongside a teardown cleanup function.
func setupDisposableDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("integration test failed: unable to load configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	maintenanceDB, err := database.NewSQLDB(ctx, cfg.DB)
	if err != nil {
		t.Fatalf("integration test failed: unable to connect to PostgreSQL server: %v", err)
	}

	dbName := fmt.Sprintf("giki_sports_disposable_contentrepo_%d", time.Now().UnixNano())
	_, err = maintenanceDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s;", dbName))
	if err != nil {
		maintenanceDB.Close()
		t.Fatalf("failed to create disposable database %s: %v", dbName, err)
	}

	testDBCfg := cfg.DB
	testDBCfg.Name = dbName
	migSQLDB, err := database.NewSQLDB(ctx, testDBCfg)
	if err != nil {
		dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to connect to disposable database for migrations: %v", err)
	}

	migrationsDir := filepath.Join("..", "..", "migrations")
	runner, err := database.NewMigrationRunnerFromDir(migSQLDB, migrationsDir)
	if err != nil {
		migSQLDB.Close()
		dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to initialize migration runner: %v", err)
	}

	if _, err := runner.Up(ctx); err != nil {
		runner.Close()
		migSQLDB.Close()
		dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to apply migrations to disposable database: %v", err)
	}
	runner.Close()
	migSQLDB.Close()

	pool, err := database.NewPool(ctx, testDBCfg)
	if err != nil {
		dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
		t.Fatalf("failed to initialize pgx connection pool: %v", err)
	}

	cleanup := func() {
		pool.Close()
		dropDB(maintenanceDB, dbName)
		maintenanceDB.Close()
	}

	return pool, cleanup
}

func dropDB(db *sql.DB, dbName string) {
	if !strings.HasPrefix(dbName, "giki_sports_disposable_") {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = db.ExecContext(ctx, fmt.Sprintf(`
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = '%s' AND pid <> pg_backend_pid();`, dbName))
	_, _ = db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE);", dbName))
}

// insertTestAuthor inserts a test user to satisfy foreign key fk_content_items_creator.
func insertTestAuthor(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	authorID := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'author@example.test', 'Content Author', '$argon2id$v=19$dummy');
	`, authorID)
	if err != nil {
		t.Fatalf("failed to insert test author: %v", err)
	}
	return authorID
}

func TestPostgresRepository_ListPublishedByKind_EmptyTable(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()

	// Verify announcements on empty table
	announcements, err := repo.ListPublishedByKind(ctx, content.KindAnnouncement)
	if err != nil {
		t.Fatalf("expected nil error on empty table, got: %v", err)
	}
	if announcements == nil {
		t.Fatalf("expected non-nil empty slice, got nil")
	}
	if len(announcements) != 0 {
		t.Fatalf("expected 0 announcements, got %d", len(announcements))
	}

	// Verify achievements on empty table
	achievements, err := repo.ListPublishedByKind(ctx, content.KindAchievement)
	if err != nil {
		t.Fatalf("expected nil error on empty table, got: %v", err)
	}
	if achievements == nil {
		t.Fatalf("expected non-nil empty slice, got nil")
	}
	if len(achievements) != 0 {
		t.Fatalf("expected 0 achievements, got %d", len(achievements))
	}
}

func TestPostgresRepository_ListPublishedByKind_KindSeparation(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	annID := uuid.New()
	achID := uuid.New()

	// Insert 1 published announcement
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id, published_at)
		VALUES ($1, 'announcement', 'Gala Announcement', 'Details about the gala.', 'published', $2, now() - INTERVAL '10 minutes');
	`, annID, authorID)
	if err != nil {
		t.Fatalf("failed to insert published announcement: %v", err)
	}

	// Insert 1 published achievement
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id, published_at)
		VALUES ($1, 'achievement', 'Tennis Gold Medal', 'Details about the medal.', 'published', $2, now() - INTERVAL '5 minutes');
	`, achID, authorID)
	if err != nil {
		t.Fatalf("failed to insert published achievement: %v", err)
	}

	// Query announcements: must return only the announcement
	announcements, err := repo.ListPublishedByKind(ctx, content.KindAnnouncement)
	if err != nil {
		t.Fatalf("failed to list published announcements: %v", err)
	}
	if len(announcements) != 1 {
		t.Fatalf("expected 1 announcement, got %d", len(announcements))
	}
	if announcements[0].ID != annID {
		t.Errorf("expected announcement ID %s, got %s", annID, announcements[0].ID)
	}
	if announcements[0].Kind != content.KindAnnouncement {
		t.Errorf("expected kind %q, got %q", content.KindAnnouncement, announcements[0].Kind)
	}
	if announcements[0].Title != "Gala Announcement" {
		t.Errorf("expected title 'Gala Announcement', got %q", announcements[0].Title)
	}
	if announcements[0].Content != "Details about the gala." {
		t.Errorf("expected content 'Details about the gala.', got %q", announcements[0].Content)
	}

	// Query achievements: must return only the achievement
	achievements, err := repo.ListPublishedByKind(ctx, content.KindAchievement)
	if err != nil {
		t.Fatalf("failed to list published achievements: %v", err)
	}
	if len(achievements) != 1 {
		t.Fatalf("expected 1 achievement, got %d", len(achievements))
	}
	if achievements[0].ID != achID {
		t.Errorf("expected achievement ID %s, got %s", achID, achievements[0].ID)
	}
	if achievements[0].Kind != content.KindAchievement {
		t.Errorf("expected kind %q, got %q", content.KindAchievement, achievements[0].Kind)
	}
}

func TestPostgresRepository_ListPublishedByKind_HidesDrafts(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	// 1. Incomplete draft with NULL title, content, and published_at
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('announcement', NULL, NULL, 'draft', $1, NULL);
	`, authorID)
	if err != nil {
		t.Fatalf("failed to insert incomplete draft: %v", err)
	}

	// 2. Incomplete draft with empty title and empty content
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('achievement', '', '', 'draft', $1, NULL);
	`, authorID)
	if err != nil {
		t.Fatalf("failed to insert empty draft: %v", err)
	}

	// 3. Complete draft that has title, content, and a past published_at timestamp, but status is 'draft'
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (kind, title, content, status, creator_id, published_at)
		VALUES ('announcement', 'Draft Title', 'Draft Body Text', 'draft', $1, now() - INTERVAL '1 hour');
	`, authorID)
	if err != nil {
		t.Fatalf("failed to insert complete draft: %v", err)
	}

	// Verify announcements query returns 0 items
	announcements, err := repo.ListPublishedByKind(ctx, content.KindAnnouncement)
	if err != nil {
		t.Fatalf("unexpected error querying announcements: %v", err)
	}
	if len(announcements) != 0 {
		t.Fatalf("expected 0 published announcements, got %d (drafts leaked into public query!)", len(announcements))
	}

	// Verify achievements query returns 0 items
	achievements, err := repo.ListPublishedByKind(ctx, content.KindAchievement)
	if err != nil {
		t.Fatalf("unexpected error querying achievements: %v", err)
	}
	if len(achievements) != 0 {
		t.Fatalf("expected 0 published achievements, got %d (drafts leaked into public query!)", len(achievements))
	}
}

func TestPostgresRepository_ListPublishedByKind_ExcludesFutureDated(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	pastID := uuid.New()
	futureID := uuid.New()

	// Insert past published item (should appear)
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id, published_at)
		VALUES ($1, 'announcement', 'Active Notice', 'Currently visible notice.', 'published', $2, now() - INTERVAL '15 minutes');
	`, pastID, authorID)
	if err != nil {
		t.Fatalf("failed to insert past published notice: %v", err)
	}

	// Insert future-dated published item (must be excluded)
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id, published_at)
		VALUES ($1, 'announcement', 'Scheduled Future Notice', 'Should not be visible yet.', 'published', $2, now() + INTERVAL '2 hours');
	`, futureID, authorID)
	if err != nil {
		t.Fatalf("failed to insert future published notice: %v", err)
	}

	items, err := repo.ListPublishedByKind(ctx, content.KindAnnouncement)
	if err != nil {
		t.Fatalf("failed to list published announcements: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly 1 item, got %d", len(items))
	}
	if items[0].ID != pastID {
		t.Errorf("expected item %s, got %s", pastID, items[0].ID)
	}
}

func TestPostgresRepository_ListPublishedByKind_StableOrderingAndTieBreaker(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	// Fix exact timestamps
	baseTime := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	newerTime := baseTime.Add(30 * time.Minute)

	// Create two distinct UUIDs for the tie-breaker test where idA < idB
	id1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	id2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	idNewest := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	// Insert item with newer publication time
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id, published_at)
		VALUES ($1, 'achievement', 'Newest Victory', 'Newest body', 'published', $2, $3);
	`, idNewest, authorID, newerTime)
	if err != nil {
		t.Fatalf("failed to insert newest item: %v", err)
	}

	// Insert item with baseTime and higher UUID (inserted first to test SQL ORDER BY, not insert order)
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id, published_at)
		VALUES ($1, 'achievement', 'Tied Victory B', 'Body B', 'published', $2, $3);
	`, id2, authorID, baseTime)
	if err != nil {
		t.Fatalf("failed to insert item id2: %v", err)
	}

	// Insert item with baseTime and lower UUID
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id, published_at)
		VALUES ($1, 'achievement', 'Tied Victory A', 'Body A', 'published', $2, $3);
	`, id1, authorID, baseTime)
	if err != nil {
		t.Fatalf("failed to insert item id1: %v", err)
	}

	items, err := repo.ListPublishedByKind(ctx, content.KindAchievement)
	if err != nil {
		t.Fatalf("failed to list published achievements: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// 1st item: newest publication timestamp (idNewest)
	if items[0].ID != idNewest {
		t.Errorf("expected 1st item to be %s (newest), got %s", idNewest, items[0].ID)
	}

	// 2nd item: lower UUID tie-breaker (id1)
	if items[1].ID != id1 {
		t.Errorf("expected 2nd item to be %s (tie-breaker id ASC), got %s", id1, items[1].ID)
	}

	// 3rd item: higher UUID tie-breaker (id2)
	if items[2].ID != id2 {
		t.Errorf("expected 3rd item to be %s (tie-breaker id ASC), got %s", id2, items[2].ID)
	}
}

func TestPostgresRepository_ListPublishedByKind_InvalidKindAndCanceledContext(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)

	// 1. Invalid kind rejection
	_, err := repo.ListPublishedByKind(context.Background(), content.Kind("unsupported_kind"))
	if err == nil {
		t.Fatalf("expected error for invalid kind, got nil")
	}
	if !errors.Is(err, content.ErrInvalidKind) {
		t.Errorf("expected error wrapping ErrInvalidKind, got: %v", err)
	}

	// 2. Canceled context rejection
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = repo.ListPublishedByKind(canceledCtx, content.KindAnnouncement)
	if err == nil {
		t.Fatalf("expected error for canceled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected error wrapping context.Canceled, got: %v", err)
	}
}

func TestPostgresRepository_CreateDraft_IncompleteAndComplete(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	actorID := insertTestAuthor(t, pool)

	audit := content.AuditParams{
		ActorID:   actorID,
		IPAddress: "192.168.1.100",
		UserAgent: "TestAgent/1.0",
	}

	// 1. Incomplete draft: nil title and nil content
	draft1, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:    content.KindAnnouncement,
		Title:   nil,
		Content: nil,
	}, audit)
	if err != nil {
		t.Fatalf("failed to create incomplete draft: %v", err)
	}
	if draft1.ID == uuid.Nil {
		t.Errorf("expected generated UUID, got nil UUID")
	}
	if draft1.Kind != content.KindAnnouncement {
		t.Errorf("expected kind %s, got %s", content.KindAnnouncement, draft1.Kind)
	}
	if draft1.Title != nil {
		t.Errorf("expected nil title, got %v", *draft1.Title)
	}
	if draft1.Content != nil {
		t.Errorf("expected nil content, got %v", *draft1.Content)
	}
	if draft1.Status != "draft" {
		t.Errorf("expected status 'draft', got %s", draft1.Status)
	}
	if draft1.CreatorID != actorID {
		t.Errorf("expected creator ID %s, got %s", actorID, draft1.CreatorID)
	}
	if draft1.PublishedAt != nil {
		t.Errorf("expected nil publishedAt, got %v", draft1.PublishedAt)
	}

	// Verify audit log row for draft 1
	var auditAction, auditTargetType, auditDetails string
	var auditActorID, auditTargetID uuid.UUID
	err = pool.QueryRow(ctx, `
		SELECT actor_id, action, target_type, target_id, details::text
		FROM giki_sports.audit_logs
		WHERE target_id = $1;
	`, draft1.ID).Scan(&auditActorID, &auditAction, &auditTargetType, &auditTargetID, &auditDetails)
	if err != nil {
		t.Fatalf("failed to query audit log for draft 1: %v", err)
	}
	if auditActorID != actorID {
		t.Errorf("expected audit actor %s, got %s", actorID, auditActorID)
	}
	if auditAction != "content.draft.create" {
		t.Errorf("expected audit action 'content.draft.create', got %s", auditAction)
	}
	if auditTargetType != "content_item" {
		t.Errorf("expected audit target_type 'content_item', got %s", auditTargetType)
	}
	if auditTargetID != draft1.ID {
		t.Errorf("expected audit target ID %s, got %s", draft1.ID, auditTargetID)
	}
	var detailsMap1 map[string]interface{}
	if err := json.Unmarshal([]byte(auditDetails), &detailsMap1); err != nil {
		t.Fatalf("failed to unmarshal audit details for draft 1: %v", err)
	}
	if _, ok := detailsMap1["title"]; ok {
		t.Errorf("audit details must NOT contain unpublished title, got: %s", auditDetails)
	}
	if _, ok := detailsMap1["content"]; ok {
		t.Errorf("audit details must NOT contain unpublished content, got: %s", auditDetails)
	}
	if _, ok := detailsMap1["body"]; ok {
		t.Errorf("audit details must NOT contain unpublished body, got: %s", auditDetails)
	}

	// 2. Complete draft: populated title and content
	title := "Upcoming Inter-Hostel Badminton Cup"
	body := "Matches start next Monday at the Sports Complex Main Hall."
	draft2, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:    content.KindAchievement,
		Title:   &title,
		Content: &body,
	}, audit)
	if err != nil {
		t.Fatalf("failed to create complete draft: %v", err)
	}
	if draft2.Title == nil || *draft2.Title != title {
		t.Errorf("expected title %q, got %v", title, draft2.Title)
	}
	if draft2.Content == nil || *draft2.Content != body {
		t.Errorf("expected body %q, got %v", body, draft2.Content)
	}

	// Verify audit log for draft 2 does NOT leak title or body
	err = pool.QueryRow(ctx, `
		SELECT details::text
		FROM giki_sports.audit_logs
		WHERE target_id = $1;
	`, draft2.ID).Scan(&auditDetails)
	if err != nil {
		t.Fatalf("failed to query audit log for draft 2: %v", err)
	}
	var detailsMap2 map[string]interface{}
	if err := json.Unmarshal([]byte(auditDetails), &detailsMap2); err != nil {
		t.Fatalf("failed to unmarshal audit details for draft 2: %v", err)
	}
	if _, ok := detailsMap2["title"]; ok {
		t.Errorf("audit details must NOT contain unpublished title: %s", auditDetails)
	}
	if _, ok := detailsMap2["content"]; ok {
		t.Errorf("audit details must NOT contain unpublished content: %s", auditDetails)
	}
	if strings.Contains(auditDetails, title) || strings.Contains(auditDetails, body) {
		t.Errorf("audit details leaked unpublished title or body: %s", auditDetails)
	}
}

func TestPostgresRepository_GetDraftByID(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	actorID := insertTestAuthor(t, pool)

	audit := content.AuditParams{ActorID: actorID}

	// 1. Create a draft
	title := "Draft Notice"
	draft, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:  content.KindAnnouncement,
		Title: &title,
	}, audit)
	if err != nil {
		t.Fatalf("failed to create draft: %v", err)
	}

	// 2. Get existing draft by ID
	retrieved, err := repo.GetDraftByID(ctx, draft.ID)
	if err != nil {
		t.Fatalf("failed to get draft by ID: %v", err)
	}
	if retrieved.ID != draft.ID {
		t.Errorf("expected ID %s, got %s", draft.ID, retrieved.ID)
	}
	if retrieved.Title == nil || *retrieved.Title != title {
		t.Errorf("expected title %q, got %v", title, retrieved.Title)
	}
	if retrieved.Status != "draft" {
		t.Errorf("expected status 'draft', got %s", retrieved.Status)
	}

	// 3. Get non-existent draft ID
	_, err = repo.GetDraftByID(ctx, uuid.New())
	if err == nil {
		t.Fatalf("expected error for non-existent draft ID, got nil")
	}
	if !errors.Is(err, content.ErrDraftNotFound) {
		t.Errorf("expected ErrDraftNotFound, got: %v", err)
	}

	// 4. Get item that exists but is published (must return ErrNotADraft)
	pubID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id, published_at)
		VALUES ($1, 'announcement', 'Published Notice', 'Body', 'published', $2, now());
	`, pubID, actorID)
	if err != nil {
		t.Fatalf("failed to insert published item: %v", err)
	}

	_, err = repo.GetDraftByID(ctx, pubID)
	if err == nil {
		t.Fatalf("expected error when getting published item as draft, got nil")
	}
	if !errors.Is(err, content.ErrNotADraft) {
		t.Errorf("expected ErrNotADraft, got: %v", err)
	}
}

func TestPostgresRepository_UpdateDraft_PreservesIdentity(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	// Insert a second user to act as the updating actor
	updaterID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'updater@example.test', 'Draft Editor', '$argon2id$v=19$dummy');
	`, updaterID)
	if err != nil {
		t.Fatalf("failed to insert updater: %v", err)
	}

	// 1. Create initial draft
	origTitle := "Original Title"
	origBody := "Original Content"
	draft, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:    content.KindAchievement,
		Title:   &origTitle,
		Content: &origBody,
	}, content.AuditParams{ActorID: authorID})
	if err != nil {
		t.Fatalf("failed to create draft: %v", err)
	}

	// Sleep slightly to guarantee updated_at is distinct
	time.Sleep(10 * time.Millisecond)

	// 2. Update title and content with updater actor
	updatedTitle := "Final Tournament Victory"
	updatedBody := "GIKI won 1st position in Table Tennis."
	updatedDraft, err := repo.UpdateDraft(ctx, draft.ID, content.UpdateDraftInput{
		Title:   &updatedTitle,
		Content: &updatedBody,
	}, content.AuditParams{
		ActorID:   updaterID,
		IPAddress: "10.0.0.5",
		UserAgent: "EditorBrowser/2.0",
	})
	if err != nil {
		t.Fatalf("failed to update draft: %v", err)
	}

	// 3. Verify identity, kind, creator, and timestamps
	if updatedDraft.ID != draft.ID {
		t.Errorf("ID changed during update: expected %s, got %s", draft.ID, updatedDraft.ID)
	}
	if updatedDraft.Kind != draft.Kind {
		t.Errorf("Kind changed during update: expected %s, got %s", draft.Kind, updatedDraft.Kind)
	}
	if updatedDraft.CreatorID != authorID {
		t.Errorf("CreatorID changed during update: expected %s, got %s", authorID, updatedDraft.CreatorID)
	}
	if !updatedDraft.CreatedAt.Equal(draft.CreatedAt) {
		t.Errorf("CreatedAt changed: expected %v, got %v", draft.CreatedAt, updatedDraft.CreatedAt)
	}
	if !updatedDraft.UpdatedAt.After(draft.CreatedAt) && !updatedDraft.UpdatedAt.Equal(draft.CreatedAt) {
		t.Errorf("UpdatedAt %v should be >= CreatedAt %v", updatedDraft.UpdatedAt, draft.CreatedAt)
	}
	if updatedDraft.Title == nil || *updatedDraft.Title != updatedTitle {
		t.Errorf("expected updated title %q, got %v", updatedTitle, updatedDraft.Title)
	}
	if updatedDraft.Content == nil || *updatedDraft.Content != updatedBody {
		t.Errorf("expected updated content %q, got %v", updatedBody, updatedDraft.Content)
	}

	// 4. Verify audit log entry for update
	var auditAction string
	var auditActorID uuid.UUID
	var auditDetails string
	err = pool.QueryRow(ctx, `
		SELECT actor_id, action, details::text
		FROM giki_sports.audit_logs
		WHERE target_id = $1 AND action = 'content.draft.update';
	`, draft.ID).Scan(&auditActorID, &auditAction, &auditDetails)
	if err != nil {
		t.Fatalf("failed to query update audit log: %v", err)
	}
	if auditActorID != updaterID {
		t.Errorf("expected audit actor %s (updater), got %s", updaterID, auditActorID)
	}
	if auditAction != "content.draft.update" {
		t.Errorf("expected action 'content.draft.update', got %s", auditAction)
	}
	var updateDetailsMap map[string]interface{}
	if err := json.Unmarshal([]byte(auditDetails), &updateDetailsMap); err != nil {
		t.Fatalf("failed to unmarshal update audit details: %v", err)
	}
	if _, ok := updateDetailsMap["title"]; ok {
		t.Errorf("audit details must NOT contain unpublished title: %s", auditDetails)
	}
	if _, ok := updateDetailsMap["content"]; ok {
		t.Errorf("audit details must NOT contain unpublished content: %s", auditDetails)
	}
	if strings.Contains(auditDetails, updatedTitle) || strings.Contains(auditDetails, updatedBody) {
		t.Errorf("audit log must NOT contain unpublished title/body: %s", auditDetails)
	}
}

func TestPostgresRepository_UpdateDraft_RejectsNonDraftAndMissing(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	audit := content.AuditParams{ActorID: authorID}

	// 1. Update non-existent draft
	missingID := uuid.New()
	newTitle := "New Title"
	_, err := repo.UpdateDraft(ctx, missingID, content.UpdateDraftInput{
		Title: &newTitle,
	}, audit)
	if err == nil {
		t.Fatalf("expected error updating missing draft, got nil")
	}
	if !errors.Is(err, content.ErrDraftNotFound) {
		t.Errorf("expected ErrDraftNotFound, got: %v", err)
	}

	// 2. Insert published item and attempt to update it via UpdateDraft
	pubID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id, published_at)
		VALUES ($1, 'announcement', 'Original Published Title', 'Original Body', 'published', $2, now());
	`, pubID, authorID)
	if err != nil {
		t.Fatalf("failed to insert published item: %v", err)
	}

	_, err = repo.UpdateDraft(ctx, pubID, content.UpdateDraftInput{
		Title: &newTitle,
	}, audit)
	if err == nil {
		t.Fatalf("expected error updating published item as draft, got nil")
	}
	if !errors.Is(err, content.ErrNotADraft) {
		t.Errorf("expected ErrNotADraft, got: %v", err)
	}

	// Verify published item in DB was NOT modified
	var currentTitle string
	err = pool.QueryRow(ctx, "SELECT title FROM giki_sports.content_items WHERE id = $1;", pubID).Scan(&currentTitle)
	if err != nil || currentTitle != "Original Published Title" {
		t.Errorf("published item was modified: expected 'Original Published Title', got %q", currentTitle)
	}
}

func TestPostgresRepository_Validation_TitleLengthAndInvalidInputs(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	tooLongTitle := strings.Repeat("T", content.MaxTitleLength+1)

	// 1. CreateDraft title > 255 chars
	_, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:  content.KindAnnouncement,
		Title: &tooLongTitle,
	}, content.AuditParams{ActorID: authorID})
	if err == nil {
		t.Fatalf("expected validation error for title > 255 chars, got nil")
	}
	if !errors.Is(err, content.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got: %v", err)
	}

	// 2. CreateDraft invalid kind
	_, err = repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind: content.Kind("press_release"),
	}, content.AuditParams{ActorID: authorID})
	if err == nil {
		t.Fatalf("expected error for invalid kind, got nil")
	}
	if !errors.Is(err, content.ErrInvalidKind) {
		t.Errorf("expected ErrInvalidKind, got: %v", err)
	}

	// 3. CreateDraft nil actor ID
	_, err = repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind: content.KindAnnouncement,
	}, content.AuditParams{ActorID: uuid.Nil})
	if err == nil {
		t.Fatalf("expected error for nil actor ID, got nil")
	}
	if !errors.Is(err, content.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got: %v", err)
	}

	// 4. UpdateDraft title > 255 chars
	draft, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind: content.KindAnnouncement,
	}, content.AuditParams{ActorID: authorID})
	if err != nil {
		t.Fatalf("failed to create draft: %v", err)
	}

	_, err = repo.UpdateDraft(ctx, draft.ID, content.UpdateDraftInput{
		Title: &tooLongTitle,
	}, content.AuditParams{ActorID: authorID})
	if err == nil {
		t.Fatalf("expected validation error for update title > 255 chars, got nil")
	}
	if !errors.Is(err, content.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got: %v", err)
	}

	// 5. UpdateDraft nil ID
	_, err = repo.UpdateDraft(ctx, uuid.Nil, content.UpdateDraftInput{}, content.AuditParams{ActorID: authorID})
	if err == nil {
		t.Fatalf("expected error for nil draft ID, got nil")
	}
	if !errors.Is(err, content.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got: %v", err)
	}

	// 6. UpdateDraft nil actor ID
	_, err = repo.UpdateDraft(ctx, draft.ID, content.UpdateDraftInput{}, content.AuditParams{ActorID: uuid.Nil})
	if err == nil {
		t.Fatalf("expected error for nil actor ID on update, got nil")
	}
	if !errors.Is(err, content.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got: %v", err)
	}
}

func TestPostgresRepository_AuditRollback_CreateAndUpdate(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	// Actor UUID that does NOT exist in giki_sports.users table
	nonExistentActorID := uuid.MustParse("99999999-9999-9999-9999-999999999999")

	// 1. CreateDraft with non-existent actor ID -> fails on fk_content_items_creator or fk_audit_logs_actor
	title := "Rollback Title"
	_, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:  content.KindAnnouncement,
		Title: &title,
	}, content.AuditParams{ActorID: nonExistentActorID})
	if err == nil {
		t.Fatalf("expected error with non-existent actor ID, got nil")
	}

	// Verify NO draft was created in content_items
	var draftCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.content_items WHERE title = $1;", title).Scan(&draftCount)
	if err != nil || draftCount != 0 {
		t.Fatalf("expected 0 drafts created due to rollback, got %d", draftCount)
	}

	// 2. Create valid draft first
	validDraft, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:  content.KindAnnouncement,
		Title: &title,
	}, content.AuditParams{ActorID: authorID})
	if err != nil {
		t.Fatalf("failed to create valid draft: %v", err)
	}

	// Attempt UpdateDraft with non-existent actor ID (fails on audit log foreign key fk_audit_logs_actor)
	newTitle := "Should Rollback Update"
	_, err = repo.UpdateDraft(ctx, validDraft.ID, content.UpdateDraftInput{
		Title: &newTitle,
	}, content.AuditParams{ActorID: nonExistentActorID})
	if err == nil {
		t.Fatalf("expected error during update with non-existent actor ID, got nil")
	}

	// Verify draft title in DB remains the original title, NOT the aborted update title
	var currentTitle string
	err = pool.QueryRow(ctx, "SELECT title FROM giki_sports.content_items WHERE id = $1;", validDraft.ID).Scan(&currentTitle)
	if err != nil || currentTitle != title {
		t.Fatalf("expected draft title to remain %q after update rollback, got %q", title, currentTitle)
	}
}

func TestPostgresRepository_ListPublishedByKind_FiftyItemCap(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	// Insert 55 published achievements spaced by 1 minute
	baseTime := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 55; i++ {
		pubTime := baseTime.Add(time.Duration(i) * time.Minute)
		_, err := pool.Exec(ctx, `
			INSERT INTO giki_sports.content_items (id, kind, title, content, status, creator_id, published_at)
			VALUES ($1, 'achievement', $2, 'Body', 'published', $3, $4);
		`, uuid.New(), fmt.Sprintf("Achievement %02d", i), authorID, pubTime)
		if err != nil {
			t.Fatalf("failed to insert published item %d: %v", i, err)
		}
	}

	// Also insert 2 drafts that must remain completely hidden
	_, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind: content.KindAchievement,
	}, content.AuditParams{ActorID: authorID})
	if err != nil {
		t.Fatalf("failed to create draft: %v", err)
	}

	items, err := repo.ListPublishedByKind(ctx, content.KindAchievement)
	if err != nil {
		t.Fatalf("failed to list published achievements: %v", err)
	}

	// Verify exactly 50 items returned (the 50-item cap)
	if len(items) != content.MaxPublishedItemsPerKind {
		t.Fatalf("expected exactly %d items (50 cap), got %d", content.MaxPublishedItemsPerKind, len(items))
	}

	// Verify newest item is first ("Achievement 55")
	if items[0].Title != "Achievement 55" {
		t.Errorf("expected newest item first ('Achievement 55'), got %q", items[0].Title)
	}

	// Verify 50th item is "Achievement 06" (since items 01..05 were excluded by the 50 cap)
	if items[49].Title != "Achievement 06" {
		t.Errorf("expected 50th item to be 'Achievement 06', got %q", items[49].Title)
	}
}

func TestPostgresRepository_PublishDraft_CompleteAnnouncementAndAchievement(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	// Insert distinct publisher user
	publisherID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO giki_sports.users (id, email, display_name, password_hash)
		VALUES ($1, 'director@example.test', 'Sports Director', '$argon2id$v=19$dummy');
	`, publisherID)
	if err != nil {
		t.Fatalf("failed to insert publisher: %v", err)
	}

	publishAudit := content.AuditParams{
		ActorID:   publisherID,
		IPAddress: "172.16.0.1",
		UserAgent: "DirectorBrowser/1.0",
	}

	// 1. Create complete draft announcement
	annTitle := "Annual Sports Gala Dates Announced"
	annBody := "The sports gala will take place from Dec 1 to Dec 5, 2026."
	annDraft, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:    content.KindAnnouncement,
		Title:   &annTitle,
		Content: &annBody,
	}, content.AuditParams{ActorID: authorID})
	if err != nil {
		t.Fatalf("failed to create draft announcement: %v", err)
	}

	// 2. Create complete draft achievement
	achTitle := "Badminton Team Clinches National Gold"
	achBody := "GIKI Badminton Team won the championship finals."
	achDraft, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:    content.KindAchievement,
		Title:   &achTitle,
		Content: &achBody,
	}, content.AuditParams{ActorID: authorID})
	if err != nil {
		t.Fatalf("failed to create draft achievement: %v", err)
	}

	// Before publication: public list must be empty
	annListBefore, err := repo.ListPublishedByKind(ctx, content.KindAnnouncement)
	if err != nil || len(annListBefore) != 0 {
		t.Fatalf("expected 0 published announcements before publishing, got %d", len(annListBefore))
	}
	achListBefore, err := repo.ListPublishedByKind(ctx, content.KindAchievement)
	if err != nil || len(achListBefore) != 0 {
		t.Fatalf("expected 0 published achievements before publishing, got %d", len(achListBefore))
	}

	// 3. Publish announcement
	publishedAnn, err := repo.PublishDraft(ctx, annDraft.ID, publishAudit)
	if err != nil {
		t.Fatalf("failed to publish announcement: %v", err)
	}
	if publishedAnn.ID != annDraft.ID {
		t.Errorf("expected published ID %s, got %s", annDraft.ID, publishedAnn.ID)
	}
	if publishedAnn.Kind != content.KindAnnouncement {
		t.Errorf("expected kind %s, got %s", content.KindAnnouncement, publishedAnn.Kind)
	}
	if publishedAnn.Title != annTitle {
		t.Errorf("expected title %q, got %q", annTitle, publishedAnn.Title)
	}
	if publishedAnn.Content != annBody {
		t.Errorf("expected body %q, got %q", annBody, publishedAnn.Content)
	}
	if publishedAnn.PublishedAt.IsZero() {
		t.Errorf("expected non-zero publishedAt timestamp")
	}

	// Verify announcement is now visible in ListPublishedByKind
	annListAfter, err := repo.ListPublishedByKind(ctx, content.KindAnnouncement)
	if err != nil || len(annListAfter) != 1 {
		t.Fatalf("expected exactly 1 published announcement after publish, got %d", len(annListAfter))
	}
	if annListAfter[0].ID != annDraft.ID {
		t.Errorf("expected published announcement ID %s, got %s", annDraft.ID, annListAfter[0].ID)
	}

	// Verify achievement is STILL not visible
	achListMid, err := repo.ListPublishedByKind(ctx, content.KindAchievement)
	if err != nil || len(achListMid) != 0 {
		t.Fatalf("expected 0 published achievements, got %d", len(achListMid))
	}

	// 4. Publish achievement
	publishedAch, err := repo.PublishDraft(ctx, achDraft.ID, publishAudit)
	if err != nil {
		t.Fatalf("failed to publish achievement: %v", err)
	}
	if publishedAch.ID != achDraft.ID {
		t.Errorf("expected published ID %s, got %s", achDraft.ID, publishedAch.ID)
	}
	if publishedAch.Kind != content.KindAchievement {
		t.Errorf("expected kind %s, got %s", content.KindAchievement, publishedAch.Kind)
	}

	// Verify achievement is now visible in ListPublishedByKind
	achListAfter, err := repo.ListPublishedByKind(ctx, content.KindAchievement)
	if err != nil || len(achListAfter) != 1 {
		t.Fatalf("expected exactly 1 published achievement after publish, got %d", len(achListAfter))
	}

	// 5. Verify audit logs for both publications
	for _, pubID := range []uuid.UUID{annDraft.ID, achDraft.ID} {
		var auditAction, auditDetails string
		var auditActorID uuid.UUID
		err = pool.QueryRow(ctx, `
			SELECT actor_id, action, details::text
			FROM giki_sports.audit_logs
			WHERE target_id = $1 AND action = 'content.publish';
		`, pubID).Scan(&auditActorID, &auditAction, &auditDetails)
		if err != nil {
			t.Fatalf("failed to query audit log for %s: %v", pubID, err)
		}
		if auditActorID != publisherID {
			t.Errorf("expected audit actor %s, got %s", publisherID, auditActorID)
		}
		if auditAction != "content.publish" {
			t.Errorf("expected action 'content.publish', got %s", auditAction)
		}

		var detailsMap map[string]interface{}
		if err := json.Unmarshal([]byte(auditDetails), &detailsMap); err != nil {
			t.Fatalf("failed to unmarshal audit details: %v", err)
		}
		if _, ok := detailsMap["title"]; ok {
			t.Errorf("audit log must NOT contain title: %s", auditDetails)
		}
		if _, ok := detailsMap["content"]; ok {
			t.Errorf("audit log must NOT contain content: %s", auditDetails)
		}
	}
}

func TestPostgresRepository_PublishDraft_IncompleteAndWhitespaceRejected(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	audit := content.AuditParams{ActorID: authorID}
	validText := "Valid text content"

	tests := []struct {
		name    string
		title   *string
		content *string
	}{
		{
			name:    "nil title",
			title:   nil,
			content: &validText,
		},
		{
			name:    "spaces-only title",
			title:   content.StringPtr("   "),
			content: &validText,
		},
		{
			name:    "tabs and newlines title",
			title:   content.StringPtr("\t\n  \r\n"),
			content: &validText,
		},
		{
			name:    "nil content",
			title:   &validText,
			content: nil,
		},
		{
			name:    "spaces-only content",
			title:   &validText,
			content: content.StringPtr("    "),
		},
		{
			name:    "tabs and newlines content",
			title:   &validText,
			content: content.StringPtr(" \r\n\t "),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create draft with incomplete or whitespace-only inputs
			draft, err := repo.CreateDraft(ctx, content.CreateDraftInput{
				Kind:    content.KindAnnouncement,
				Title:   tt.title,
				Content: tt.content,
			}, audit)
			if err != nil {
				t.Fatalf("failed to create draft for test %q: %v", tt.name, err)
			}

			// Attempt to publish
			_, err = repo.PublishDraft(ctx, draft.ID, audit)
			if err == nil {
				t.Fatalf("expected publish to fail for %q, got nil", tt.name)
			}
			if !errors.Is(err, content.ErrIncompleteDraft) {
				t.Errorf("expected ErrIncompleteDraft, got: %v", err)
			}
			if !errors.Is(err, content.ErrInvalidInput) {
				t.Errorf("expected error to also match ErrInvalidInput, got: %v", err)
			}

			// Verify item remains draft in database
			var currentStatus string
			var publishedAt *time.Time
			err = pool.QueryRow(ctx, "SELECT status, published_at FROM giki_sports.content_items WHERE id = $1;", draft.ID).
				Scan(&currentStatus, &publishedAt)
			if err != nil {
				t.Fatalf("failed to query draft status: %v", err)
			}
			if currentStatus != "draft" {
				t.Errorf("expected item to remain draft, got status %q", currentStatus)
			}
			if publishedAt != nil {
				t.Errorf("expected published_at to remain nil, got %v", publishedAt)
			}

			// Verify item does NOT appear in public results
			items, err := repo.ListPublishedByKind(ctx, content.KindAnnouncement)
			if err != nil {
				t.Fatalf("failed to query public items: %v", err)
			}
			for _, item := range items {
				if item.ID == draft.ID {
					t.Fatalf("incomplete draft %s leaked into public feed!", draft.ID)
				}
			}
		})
	}
}

func TestPostgresRepository_PublishDraft_UnknownID(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	unknownID := uuid.New()
	_, err := repo.PublishDraft(ctx, unknownID, content.AuditParams{ActorID: authorID})
	if err == nil {
		t.Fatalf("expected error publishing unknown ID, got nil")
	}
	if !errors.Is(err, content.ErrDraftNotFound) {
		t.Errorf("expected ErrDraftNotFound, got: %v", err)
	}

	// Verify zero audit logs created
	var auditCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs WHERE target_id = $1;", unknownID).Scan(&auditCount)
	if err != nil || auditCount != 0 {
		t.Errorf("expected 0 audit logs for unknown ID, got %d", auditCount)
	}
}

func TestPostgresRepository_PublishDraft_AlreadyPublished_IdempotencyAndNoDuplicateAudit(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	audit := content.AuditParams{ActorID: authorID}

	title := "Championship Result"
	body := "Final victory achieved."
	draft, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:    content.KindAchievement,
		Title:   &title,
		Content: &body,
	}, audit)
	if err != nil {
		t.Fatalf("failed to create draft: %v", err)
	}

	// First publish call (succeeds)
	pubItem, err := repo.PublishDraft(ctx, draft.ID, audit)
	if err != nil {
		t.Fatalf("first publish failed: %v", err)
	}
	origPublishedAt := pubItem.PublishedAt

	// Sleep briefly
	time.Sleep(20 * time.Millisecond)

	// Second publish call on already published item (must fail with ErrNotADraft)
	_, err = repo.PublishDraft(ctx, draft.ID, audit)
	if err == nil {
		t.Fatalf("expected error on second publish call, got nil")
	}
	if !errors.Is(err, content.ErrNotADraft) {
		t.Errorf("expected ErrNotADraft, got: %v", err)
	}

	// Verify published_at timestamp in DB was NOT changed
	var currentPublishedAt time.Time
	err = pool.QueryRow(ctx, "SELECT published_at FROM giki_sports.content_items WHERE id = $1;", draft.ID).Scan(&currentPublishedAt)
	if err != nil {
		t.Fatalf("failed to query published_at: %v", err)
	}
	if !currentPublishedAt.Equal(origPublishedAt) {
		t.Errorf("published_at timestamp changed! original: %v, current: %v", origPublishedAt, currentPublishedAt)
	}

	// Verify exactly ONE content.publish audit log exists for this item
	var publishAuditCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs WHERE target_id = $1 AND action = 'content.publish';", draft.ID).
		Scan(&publishAuditCount)
	if err != nil || publishAuditCount != 1 {
		t.Errorf("expected exactly 1 publish audit log, got %d", publishAuditCount)
	}
}

func TestPostgresRepository_PublishDraft_ConcurrentAttempts(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	title := "Concurrent Announcement"
	body := "Testing concurrency locking."
	draft, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:    content.KindAnnouncement,
		Title:   &title,
		Content: &body,
	}, content.AuditParams{ActorID: authorID})
	if err != nil {
		t.Fatalf("failed to create draft: %v", err)
	}

	const concurrency = 2
	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	results := make([]*content.PublishedItem, concurrency)

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		workerIndex := i
		go func() {
			defer wg.Done()
			res, err := repo.PublishDraft(ctx, draft.ID, content.AuditParams{
				ActorID:   authorID,
				IPAddress: fmt.Sprintf("192.168.1.%d", 10+workerIndex),
			})
			results[workerIndex] = res
			errs[workerIndex] = err
		}()
	}
	wg.Wait()

	// Count successes and rejections
	successCount := 0
	notADraftCount := 0
	for _, err := range errs {
		if err == nil {
			successCount++
		} else if errors.Is(err, content.ErrNotADraft) {
			notADraftCount++
		} else {
			t.Errorf("unexpected error from concurrent worker: %v", err)
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 worker to succeed, got %d", successCount)
	}
	if notADraftCount != 1 {
		t.Fatalf("expected exactly 1 worker to receive ErrNotADraft, got %d", notADraftCount)
	}

	// Verify exactly ONE content.publish audit entry recorded
	var auditCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM giki_sports.audit_logs WHERE target_id = $1 AND action = 'content.publish';", draft.ID).
		Scan(&auditCount)
	if err != nil || auditCount != 1 {
		t.Errorf("expected exactly 1 audit entry after concurrent attempts, got %d", auditCount)
	}
}

func TestPostgresRepository_PublishDraft_ForcedAuditFailure_Rollback(t *testing.T) {
	pool, cleanup := setupDisposableDB(t)
	defer cleanup()

	repo := content.NewRepository(pool)
	ctx := context.Background()
	authorID := insertTestAuthor(t, pool)

	title := "Audit Failure Draft"
	body := "Draft that should remain draft on audit failure."
	draft, err := repo.CreateDraft(ctx, content.CreateDraftInput{
		Kind:    content.KindAchievement,
		Title:   &title,
		Content: &body,
	}, content.AuditParams{ActorID: authorID})
	if err != nil {
		t.Fatalf("failed to create draft: %v", err)
	}

	// Non-existent actor UUID to trigger foreign key violation on audit log
	nonExistentActorID := uuid.MustParse("88888888-8888-8888-8888-888888888888")

	_, err = repo.PublishDraft(ctx, draft.ID, content.AuditParams{ActorID: nonExistentActorID})
	if err == nil {
		t.Fatalf("expected PublishDraft to fail with non-existent actor ID, got nil")
	}

	// Verify the item remains a draft in the database with published_at = NULL
	var currentStatus string
	var publishedAt *time.Time
	err = pool.QueryRow(ctx, "SELECT status, published_at FROM giki_sports.content_items WHERE id = $1;", draft.ID).
		Scan(&currentStatus, &publishedAt)
	if err != nil {
		t.Fatalf("failed to inspect draft after rollback: %v", err)
	}
	if currentStatus != "draft" {
		t.Errorf("expected item to remain draft after rollback, got %q", currentStatus)
	}
	if publishedAt != nil {
		t.Errorf("expected published_at to remain nil, got %v", publishedAt)
	}

	// Verify item is NOT visible in public ListPublishedByKind
	items, err := repo.ListPublishedByKind(ctx, content.KindAchievement)
	if err != nil {
		t.Fatalf("failed to query public items: %v", err)
	}
	for _, item := range items {
		if item.ID == draft.ID {
			t.Fatalf("draft %s was found in public list after audit rollback!", draft.ID)
		}
	}
}



