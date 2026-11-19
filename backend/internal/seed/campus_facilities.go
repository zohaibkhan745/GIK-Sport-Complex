package seed

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CampusFacilityFixtures returns the complete catalog of authentic GIKI Sports Complex facilities
// and their recurring weekly operating schedules.
func CampusFacilityFixtures() []FacilityFixture {
	return []FacilityFixture{
		{
			ID:          uuid.MustParse("c0000001-0000-4000-8000-000000000001"),
			Slug:        "glass-padel-courts",
			Name:        "GIKI Glass Padel Courts",
			Description: strPtr("Panoramic tempered glass, synthetic turf, and 800 Lux LED tournament floodlights across two championship-grade courts. Online slot booking available."),
			Location:    strPtr("Sports Complex, Outdoor Courts North"),
			Status:      "active",
			Schedules: []ScheduleFixture{
				{
					ID:        uuid.MustParse("c0000001-0000-4000-8000-000000000011"),
					DayOfWeek: 1, // Mon
					StartTime: "06:00:00",
					EndTime:   "11:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000001-0000-4000-8000-000000000012"),
					DayOfWeek: 3, // Wed
					StartTime: "06:00:00",
					EndTime:   "11:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000001-0000-4000-8000-000000000013"),
					DayOfWeek: 5, // Fri
					StartTime: "06:00:00",
					EndTime:   "11:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000001-0000-4000-8000-000000000014"),
					DayOfWeek: 2, // Tue
					StartTime: "16:00:00",
					EndTime:   "18:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000001-0000-4000-8000-000000000015"),
					DayOfWeek: 4, // Thu
					StartTime: "16:00:00",
					EndTime:   "18:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000001-0000-4000-8000-000000000016"),
					DayOfWeek: 6, // Sat
					StartTime: "08:00:00",
					EndTime:   "13:00:00",
					Audience:  "general",
				},
				{
					ID:        uuid.MustParse("c0000001-0000-4000-8000-000000000017"),
					DayOfWeek: 7, // Sun
					StartTime: "14:00:00",
					EndTime:   "17:00:00",
					Audience:  "women_only",
				},
			},
		},
		{
			ID:          uuid.MustParse("c0000002-0000-4000-8000-000000000002"),
			Slug:        "central-fitness-gym",
			Name:        "GIKI Central Fitness & Strength Gym",
			Description: strPtr("Comprehensive campus fitness facility housing both the New Gym (premium Olympic lifting and cardio suite; 30-day membership pass required) and Old Gym (open student conditioning room; free walk-in access for all students)."),
			Location:    strPtr("Sports Complex, Ground Floor East"),
			Status:      "active",
			Schedules: []ScheduleFixture{
				{
					ID:        uuid.MustParse("c0000002-0000-4000-8000-000000000021"),
					DayOfWeek: 1, // Mon
					StartTime: "06:00:00",
					EndTime:   "11:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000002-0000-4000-8000-000000000022"),
					DayOfWeek: 2, // Tue
					StartTime: "06:00:00",
					EndTime:   "11:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000002-0000-4000-8000-000000000023"),
					DayOfWeek: 3, // Wed
					StartTime: "06:00:00",
					EndTime:   "11:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000002-0000-4000-8000-000000000024"),
					DayOfWeek: 4, // Thu
					StartTime: "06:00:00",
					EndTime:   "11:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000002-0000-4000-8000-000000000025"),
					DayOfWeek: 5, // Fri
					StartTime: "06:00:00",
					EndTime:   "11:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000002-0000-4000-8000-000000000026"),
					DayOfWeek: 6, // Sat
					StartTime: "06:00:00",
					EndTime:   "11:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000002-0000-4000-8000-000000000027"),
					DayOfWeek: 2, // Tue
					StartTime: "11:00:00",
					EndTime:   "14:00:00",
					Audience:  "faculty",
				},
				{
					ID:        uuid.MustParse("c0000002-0000-4000-8000-000000000028"),
					DayOfWeek: 4, // Thu
					StartTime: "11:00:00",
					EndTime:   "14:00:00",
					Audience:  "faculty",
				},
				{
					ID:        uuid.MustParse("c0000002-0000-4000-8000-000000000029"),
					DayOfWeek: 1, // Mon
					StartTime: "14:00:00",
					EndTime:   "16:00:00",
					Audience:  "women_only",
				},
				{
					ID:        uuid.MustParse("c0000002-0000-4000-8000-000000000030"),
					DayOfWeek: 3, // Wed
					StartTime: "14:00:00",
					EndTime:   "16:00:00",
					Audience:  "women_only",
				},
				{
					ID:        uuid.MustParse("c0000002-0000-4000-8000-000000000031"),
					DayOfWeek: 5, // Fri
					StartTime: "14:00:00",
					EndTime:   "16:00:00",
					Audience:  "women_only",
				},
			},
		},
		{
			ID:          uuid.MustParse("c0000003-0000-4000-8000-000000000003"),
			Slug:        "badminton-championship-hall",
			Name:        "Indoor Badminton Championship Hall",
			Description: strPtr("Four BWF-certified teakwood sprung courts with climate cooling, built to host championship-standard play year-round."),
			Location:    strPtr("Indoor Sports Hall, Level 1"),
			Status:      "active",
			Schedules: []ScheduleFixture{
				{
					ID:        uuid.MustParse("c0000003-0000-4000-8000-000000000031"),
					DayOfWeek: 1, // Mon
					StartTime: "06:00:00",
					EndTime:   "12:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000003-0000-4000-8000-000000000032"),
					DayOfWeek: 3, // Wed
					StartTime: "06:00:00",
					EndTime:   "12:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000003-0000-4000-8000-000000000033"),
					DayOfWeek: 5, // Fri
					StartTime: "06:00:00",
					EndTime:   "12:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000003-0000-4000-8000-000000000034"),
					DayOfWeek: 2, // Tue
					StartTime: "14:00:00",
					EndTime:   "16:00:00",
					Audience:  "women_only",
				},
				{
					ID:        uuid.MustParse("c0000003-0000-4000-8000-000000000035"),
					DayOfWeek: 4, // Thu
					StartTime: "14:00:00",
					EndTime:   "16:00:00",
					Audience:  "women_only",
				},
				{
					ID:        uuid.MustParse("c0000003-0000-4000-8000-000000000036"),
					DayOfWeek: 6, // Sat
					StartTime: "08:00:00",
					EndTime:   "14:00:00",
					Audience:  "general",
				},
			},
		},
		{
			ID:          uuid.MustParse("c0000004-0000-4000-8000-000000000004"),
			Slug:        "glass-back-squash-complex",
			Name:        "Glass-Back Squash Complex",
			Description: strPtr("Three glass-back competition squash courts with Junckers beech flooring and electronic scoring monitors."),
			Location:    strPtr("Sports Complex, West Wing Ground Floor"),
			Status:      "active",
			Schedules: []ScheduleFixture{
				{
					ID:        uuid.MustParse("c0000004-0000-4000-8000-000000000041"),
					DayOfWeek: 1, // Mon
					StartTime: "07:00:00",
					EndTime:   "12:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000004-0000-4000-8000-000000000042"),
					DayOfWeek: 3, // Wed
					StartTime: "07:00:00",
					EndTime:   "12:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000004-0000-4000-8000-000000000043"),
					DayOfWeek: 5, // Fri
					StartTime: "07:00:00",
					EndTime:   "12:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000004-0000-4000-8000-000000000044"),
					DayOfWeek: 2, // Tue
					StartTime: "17:00:00",
					EndTime:   "20:00:00",
					Audience:  "faculty",
				},
				{
					ID:        uuid.MustParse("c0000004-0000-4000-8000-000000000045"),
					DayOfWeek: 6, // Sat
					StartTime: "09:00:00",
					EndTime:   "14:00:00",
					Audience:  "general",
				},
			},
		},
		{
			ID:          uuid.MustParse("c0000005-0000-4000-8000-000000000005"),
			Slug:        "hard-tennis-courts",
			Name:        "Hard Tennis Courts",
			Description: strPtr("Two US Open-grade acrylic cushioned hard courts, resurfaced for consistent bounce and year-round play."),
			Location:    strPtr("Campus South Perimeter Sports Ground"),
			Status:      "active",
			Schedules: []ScheduleFixture{
				{
					ID:        uuid.MustParse("c0000005-0000-4000-8000-000000000051"),
					DayOfWeek: 1, // Mon
					StartTime: "06:00:00",
					EndTime:   "10:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000005-0000-4000-8000-000000000052"),
					DayOfWeek: 3, // Wed
					StartTime: "06:00:00",
					EndTime:   "10:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000005-0000-4000-8000-000000000053"),
					DayOfWeek: 5, // Fri
					StartTime: "06:00:00",
					EndTime:   "10:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000005-0000-4000-8000-000000000054"),
					DayOfWeek: 6, // Sat
					StartTime: "07:00:00",
					EndTime:   "11:00:00",
					Audience:  "general",
				},
				{
					ID:        uuid.MustParse("c0000005-0000-4000-8000-000000000055"),
					DayOfWeek: 7, // Sun
					StartTime: "07:00:00",
					EndTime:   "11:00:00",
					Audience:  "general",
				},
			},
		},
		{
			ID:          uuid.MustParse("c0000006-0000-4000-8000-000000000006"),
			Slug:        "floodlit-futsal-court",
			Name:        "GIKI Floodlit Futsal Court",
			Description: strPtr("A floodlit, FIFA Quality artificial turf pitch with rebound netting, built for fast, physical five-a-side football."),
			Location:    strPtr("Sports Complex Central Courtyard"),
			Status:      "active",
			Schedules: []ScheduleFixture{
				{
					ID:        uuid.MustParse("c0000006-0000-4000-8000-000000000061"),
					DayOfWeek: 1, // Mon
					StartTime: "16:00:00",
					EndTime:   "20:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000006-0000-4000-8000-000000000062"),
					DayOfWeek: 3, // Wed
					StartTime: "16:00:00",
					EndTime:   "20:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000006-0000-4000-8000-000000000063"),
					DayOfWeek: 5, // Fri
					StartTime: "16:00:00",
					EndTime:   "20:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000006-0000-4000-8000-000000000064"),
					DayOfWeek: 6, // Sat
					StartTime: "17:00:00",
					EndTime:   "22:00:00",
					Audience:  "general",
				},
			},
		},
		{
			ID:          uuid.MustParse("c0000007-0000-4000-8000-000000000007"),
			Slug:        "semi-olympic-swimming-pool",
			Name:        "Semi-Olympic Swimming Pool",
			Description: strPtr("A 25-meter, 6-lane temperature-controlled pool with ozone filtration, used for open swim, training, and galas."),
			Location:    strPtr("Aquatic Pavilion, Sports Complex"),
			Status:      "active",
			Schedules: []ScheduleFixture{
				{
					ID:        uuid.MustParse("c0000007-0000-4000-8000-000000000071"),
					DayOfWeek: 1, // Mon
					StartTime: "06:00:00",
					EndTime:   "10:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000007-0000-4000-8000-000000000072"),
					DayOfWeek: 3, // Wed
					StartTime: "06:00:00",
					EndTime:   "10:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000007-0000-4000-8000-000000000073"),
					DayOfWeek: 5, // Fri
					StartTime: "06:00:00",
					EndTime:   "10:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000007-0000-4000-8000-000000000074"),
					DayOfWeek: 1, // Mon
					StartTime: "14:00:00",
					EndTime:   "16:00:00",
					Audience:  "women_only",
				},
				{
					ID:        uuid.MustParse("c0000007-0000-4000-8000-000000000075"),
					DayOfWeek: 3, // Wed
					StartTime: "14:00:00",
					EndTime:   "16:00:00",
					Audience:  "women_only",
				},
				{
					ID:        uuid.MustParse("c0000007-0000-4000-8000-000000000076"),
					DayOfWeek: 6, // Sat
					StartTime: "09:00:00",
					EndTime:   "13:00:00",
					Audience:  "faculty",
				},
			},
		},
		{
			ID:          uuid.MustParse("c0000008-0000-4000-8000-000000000008"),
			Slug:        "table-tennis-hall",
			Name:        "Table Tennis Hall",
			Description: strPtr("Six ITTF-approved Butterfly tournament tables in a dedicated hall, open for casual play and club training alike."),
			Location:    strPtr("Indoor Complex, Mezzanine Hall"),
			Status:      "active",
			Schedules: []ScheduleFixture{
				{
					ID:        uuid.MustParse("c0000008-0000-4000-8000-000000000081"),
					DayOfWeek: 1, // Mon
					StartTime: "08:00:00",
					EndTime:   "17:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000008-0000-4000-8000-000000000082"),
					DayOfWeek: 2, // Tue
					StartTime: "08:00:00",
					EndTime:   "17:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000008-0000-4000-8000-000000000083"),
					DayOfWeek: 3, // Wed
					StartTime: "08:00:00",
					EndTime:   "17:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000008-0000-4000-8000-000000000084"),
					DayOfWeek: 4, // Thu
					StartTime: "08:00:00",
					EndTime:   "17:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000008-0000-4000-8000-000000000085"),
					DayOfWeek: 5, // Fri
					StartTime: "08:00:00",
					EndTime:   "17:00:00",
					Audience:  "student",
				},
				{
					ID:        uuid.MustParse("c0000008-0000-4000-8000-000000000086"),
					DayOfWeek: 6, // Sat
					StartTime: "08:00:00",
					EndTime:   "22:00:00",
					Audience:  "general",
				},
			},
		},
	}
}

// ApplyCampus populates all authentic GIKI Sports Complex facilities and schedules
// in an idempotent, atomic transaction.
func ApplyCampus(ctx context.Context, pool *pgxpool.Pool) (*SeedSummary, error) {
	return ApplyFixtures(ctx, pool, CampusFacilityFixtures())
}
