package seed

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FacilityFixture defines a deterministic demonstration facility.
type FacilityFixture struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Description *string
	Location    *string
	Status      string
	Schedules   []ScheduleFixture
}

// ScheduleFixture defines a deterministic demonstration weekly operating window.
type ScheduleFixture struct {
	ID        uuid.UUID
	DayOfWeek int
	StartTime string
	EndTime   string
	Audience  string
}

// SeedSummary records the exact count of inserted and skipped records.
type SeedSummary struct {
	FacilitiesInserted int
	FacilitiesSkipped  int
	SchedulesInserted  int
	SchedulesSkipped   int
}

func strPtr(s string) *string {
	return &s
}

// DefaultDemoFixtures returns deterministic, clearly synthetic demonstration records.
// All names, locations, and hours are explicitly marked as demonstration data and
// do not represent approved institutional facility information.
func DefaultDemoFixtures() []FacilityFixture {
	return []FacilityFixture{
		{
			ID:          uuid.MustParse("d0000001-0000-4000-8000-000000000001"),
			Slug:        "demo-badminton-court",
			Name:        "[Demo] Campus Badminton Court",
			Description: strPtr("Synthetic demonstration court for local testing and timetable preview"),
			Location:    strPtr("Demo Sports Hall Wing B"),
			Status:      "active",
			Schedules: []ScheduleFixture{
				{
					ID:        uuid.MustParse("d0000001-0000-4000-8000-000000000011"),
					DayOfWeek: 1, // Monday
					StartTime: "09:00:00",
					EndTime:   "12:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("d0000001-0000-4000-8000-000000000012"),
					DayOfWeek: 3, // Wednesday
					StartTime: "16:00:00",
					EndTime:   "18:00:00",
					Audience:  "faculty",
				},
				{
					ID:        uuid.MustParse("d0000001-0000-4000-8000-000000000013"),
					DayOfWeek: 5, // Friday
					StartTime: "14:00:00",
					EndTime:   "16:00:00",
					Audience:  "women_only",
				},
			},
		},
		{
			ID:          uuid.MustParse("d0000002-0000-4000-8000-000000000002"),
			Slug:        "demo-table-tennis",
			Name:        "[Demo] Table Tennis Arena",
			Description: strPtr("Synthetic demonstration table tennis arena for local testing"),
			Location:    strPtr("Demo Activity Center 1st Floor"),
			Status:      "active",
			Schedules: []ScheduleFixture{
				{
					ID:        uuid.MustParse("d0000002-0000-4000-8000-000000000021"),
					DayOfWeek: 2, // Tuesday
					StartTime: "10:00:00",
					EndTime:   "13:00:00",
					Audience:  "general",
				},
				{
					ID:        uuid.MustParse("d0000002-0000-4000-8000-000000000022"),
					DayOfWeek: 6, // Saturday
					StartTime: "18:00:00",
					EndTime:   "21:00:00",
					Audience:  "student",
				},
			},
		},
	}
}

// ApplyDemo applies the default demonstration fixtures within an atomic database transaction.
func ApplyDemo(ctx context.Context, pool *pgxpool.Pool) (*SeedSummary, error) {
	return ApplyFixtures(ctx, pool, DefaultDemoFixtures())
}

// ApplyFixtures applies the specified facility fixtures within a single atomic database transaction.
//
// Idempotency & Conflict Strategy:
// 1. Checks that required migration tables exist before proceeding.
// 2. For each fixture facility:
//    - Searches giki_sports.facilities for existing records matching ID OR Slug.
//    - If no match: inserts facility and its schedules (inserted).
//    - If matching BOTH ID and Slug: skips facility and skips schedules (skipped).
//      Preserves administrator edits on facility fields and schedules without overwrite.
//    - If matching ID with different Slug, or Slug with different ID: reports an actionable conflict error
//      and aborts the entire transaction without partial inserts.
// 3. Any error triggers transaction rollback.
func ApplyFixtures(ctx context.Context, pool *pgxpool.Pool, fixtures []FacilityFixture) (*SeedSummary, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin seed transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Verify required tables exist
	if err := VerifyRequiredSchema(ctx, tx); err != nil {
		return nil, err
	}

	summary := SeedSummary{}

	for _, fac := range fixtures {
		// Check for existing facility matching either ID or Slug
		const findExistingSQL = `
			SELECT id, slug 
			FROM giki_sports.facilities 
			WHERE id = $1 OR slug = $2;
		`

		rows, err := tx.Query(ctx, findExistingSQL, fac.ID, fac.Slug)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing facility (slug %q): %w", fac.Slug, err)
		}

		type matchRecord struct {
			id   uuid.UUID
			slug string
		}
		var matches []matchRecord

		for rows.Next() {
			var m matchRecord
			if err := rows.Scan(&m.id, &m.slug); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan facility match: %w", err)
			}
			matches = append(matches, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error reading facility matches: %w", err)
		}

		switch len(matches) {
		case 0:
			// New fixture: insert facility
			const insertFacSQL = `
				INSERT INTO giki_sports.facilities (id, slug, name, description, location, status)
				VALUES ($1, $2, $3, $4, $5, $6);
			`
			_, err = tx.Exec(ctx, insertFacSQL, fac.ID, fac.Slug, fac.Name, fac.Description, fac.Location, fac.Status)
			if err != nil {
				return nil, fmt.Errorf("failed to insert demo facility %q: %w", fac.Slug, err)
			}
			summary.FacilitiesInserted++

			// Insert fixture schedules for the newly created facility
			for _, s := range fac.Schedules {
				const insertSchedSQL = `
					INSERT INTO giki_sports.schedules (id, facility_id, day_of_week, start_time, end_time, audience)
					VALUES ($1, $2, $3, $4::time, $5::time, $6);
				`
				_, err = tx.Exec(ctx, insertSchedSQL, s.ID, fac.ID, s.DayOfWeek, s.StartTime, s.EndTime, s.Audience)
				if err != nil {
					return nil, fmt.Errorf("failed to insert schedule for demo facility %q: %w", fac.Slug, err)
				}
				summary.SchedulesInserted++
			}

		case 1:
			m := matches[0]
			if m.id == fac.ID && m.slug == fac.Slug {
				// Existing fixture facility: preserve as-is without overwriting facility fields
				// or recreating/modifying schedules that may have been edited or deleted by an administrator.
				summary.FacilitiesSkipped++
				summary.SchedulesSkipped += len(fac.Schedules)
			} else if m.id == fac.ID && m.slug != fac.Slug {
				return nil, fmt.Errorf("fixture conflict: facility with ID %s already exists with slug %q (expected %q)", fac.ID, m.slug, fac.Slug)
			} else {
				return nil, fmt.Errorf("fixture conflict: facility with slug %q already exists with ID %s (expected %s)", fac.Slug, m.id, fac.ID)
			}

		default:
			return nil, fmt.Errorf("fixture conflict: multiple existing facilities collide with fixture ID %s and slug %q", fac.ID, fac.Slug)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit seed transaction: %w", err)
	}

	return &summary, nil
}

// VerifyRequiredSchema checks whether the facilities and schedules tables exist in the giki_sports schema.
func VerifyRequiredSchema(ctx context.Context, tx pgx.Tx) error {
	const checkSQL = `
		SELECT 
			to_regclass('giki_sports.facilities') IS NOT NULL AND 
			to_regclass('giki_sports.schedules') IS NOT NULL;
	`
	var tablesExist bool
	if err := tx.QueryRow(ctx, checkSQL).Scan(&tablesExist); err != nil {
		return fmt.Errorf("failed to verify required schema: %w", err)
	}
	if !tablesExist {
		return errors.New("required schema migrations have not been applied: tables 'giki_sports.facilities' and 'giki_sports.schedules' do not exist (run migrations first via 'go run ./cmd/migrate up')")
	}
	return nil
}
