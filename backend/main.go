package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
)

type Facility struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Category             string   `json:"category"` // Racquet, Indoor, Outdoor, Fitness, Aquatics
	Description          string   `json:"description"`
	Location             string   `json:"location"`
	MaxPlayers           int      `json:"maxPlayers"`
	StudentTimings       string   `json:"studentTimings"`
	FacultyTimings       string   `json:"facultyTimings"`
	Slots                []string `json:"slots"`
	Rules                []string `json:"rules"`
	EquipmentAvailable   []string `json:"equipmentAvailable"`
	Image                string   `json:"image"`
	Badge                string   `json:"badge"`
	IsMembershipRequired bool     `json:"isMembershipRequired"`
}

type Booking struct {
	ID               string    `json:"id"`
	FacilityID       string    `json:"facilityId"`
	FacilityName     string    `json:"facilityName"`
	Date             string    `json:"date"` // YYYY-MM-DD
	TimeSlot         string    `json:"timeSlot"`
	BookerType       string    `json:"bookerType"` // student or faculty
	BookerName       string    `json:"bookerName"`
	BookerID         string    `json:"bookerId"` // e.g. 2023-CS-104 or FAC-EE-42
	BookerEmail      string    `json:"bookerEmail"`
	BookerPhone      string    `json:"bookerPhone"`
	EquipmentRentals []string  `json:"equipmentRentals"`
	Status           string    `json:"status"` // confirmed, cancelled, completed
	CheckInCode      string    `json:"checkInCode"`
	CreatedAt        time.Time `json:"createdAt"`
}

type Achievement struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Sport        string    `json:"sport"`
	Date         string    `json:"date"`
	Category     string    `json:"category"` // National Gold, Intervarsity Champion, Runner Up, Special Mention
	Description  string    `json:"description"`
	TeamMembers  string    `json:"teamMembers"`
	Image        string    `json:"image"`
	DirectorNote string    `json:"directorNote"`
	Featured     bool      `json:"featured"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Announcement struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Date      string    `json:"date"`
	Priority  string    `json:"priority"` // urgent, info, maintenance
	CreatedAt time.Time `json:"createdAt"`
}

type Membership struct {
	ID            string    `json:"id"`
	ApplicantName string    `json:"applicantName"`
	ApplicantID   string    `json:"applicantId"`
	ApplicantType string    `json:"applicantType"`
	Plan          string    `json:"plan"`
	Status        string    `json:"status"` // active, pending, approved
	ValidUntil    string    `json:"validUntil"`
	CreatedAt     time.Time `json:"createdAt"`
}

type StoreData struct {
	Facilities    []Facility     `json:"facilities"`
	Bookings      []Booking      `json:"bookings"`
	Achievements  []Achievement  `json:"achievements"`
	Announcements []Announcement `json:"announcements"`
	Memberships   []Membership   `json:"memberships"`
}

type Store struct {
	mu       sync.RWMutex
	filePath string
	Data     StoreData
}

func NewStore(filePath string) *Store {
	s := &Store{
		filePath: filePath,
	}
	s.load()
	return s
}

func (s *Store) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.filePath)
	_ = os.MkdirAll(dir, 0755)

	if data, err := os.ReadFile(s.filePath); err == nil {
		if err := json.Unmarshal(data, &s.Data); err == nil && len(s.Data.Facilities) > 0 {
			log.Println("Loaded existing database from disk")
			return
		}
	}

	// Seed with authentic GIKI Sports Complex data
	log.Println("Seeding initial GIKI Sports Complex data...")
	s.seedDefaultData()
	s.saveUnsafe()
}

func (s *Store) saveUnsafe() error {
	data, err := json.MarshalIndent(s.Data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnsafe()
}

func (s *Store) seedDefaultData() {
	slots := []string{
		"06:00 - 07:00",
		"07:00 - 08:00",
		"08:00 - 09:00",
		"09:00 - 10:00",
		"10:00 - 11:00",
		"15:00 - 16:00",
		"16:00 - 17:00",
		"17:00 - 18:00",
		"18:00 - 19:00",
		"19:00 - 20:00",
		"20:00 - 21:00",
		"21:00 - 22:00",
		"22:00 - 23:00",
	}

	s.Data.Facilities = []Facility{
		{
			ID:                   "padel-court-1",
			Name:                 "GIKI Glass Padel Arena (Court 1)",
			Category:             "Racquet",
			Description:          "State-of-the-art panoramic tempered glass court with high-grade synthetic turf and anti-glare LED tournament lighting.",
			Location:             "Sports Complex, Outdoor Arena North",
			MaxPlayers:           4,
			StudentTimings:       "06:00 AM - 11:00 AM & 04:00 PM - 11:00 PM",
			FacultyTimings:       "05:00 PM - 07:00 PM (Reserved Priority Slot)",
			Slots:                slots,
			Rules:                []string{"Non-marking turf shoes mandatory", "Max 4 players per match", "Racquet safety leash required"},
			EquipmentAvailable:   []string{"Babolat Padel Racket", "Pro Padel Balls (Can of 3)", "Wrist Wraps"},
			Image:                "https://images.unsplash.com/photo-1554068865-24cecd4e34b8?auto=format&fit=crop&w=1200&q=80",
			Badge:                "Most Popular",
			IsMembershipRequired: false,
		},
		{
			ID:                   "padel-court-2",
			Name:                 "GIKI Glass Padel Arena (Court 2)",
			Category:             "Racquet",
			Description:          "Tournament standard court configured for doubles & singles match play with spectator seating deck.",
			Location:             "Sports Complex, Outdoor Arena North",
			MaxPlayers:           4,
			StudentTimings:       "06:00 AM - 11:00 AM & 04:00 PM - 11:00 PM",
			FacultyTimings:       "06:00 PM - 08:00 PM (Reserved Priority Slot)",
			Slots:                slots,
			Rules:                []string{"No outdoor spiked footwear", "Strict 60-min slot adherence"},
			EquipmentAvailable:   []string{"Wilson Carbon Racket", "Dunlop Pro Padel Balls"},
			Image:                "https://images.unsplash.com/photo-1622279457486-62dcc4a431d6?auto=format&fit=crop&w=1200&q=80",
			Badge:                "Floodlit",
			IsMembershipRequired: false,
		},
		{
			ID:                   "badminton-indoor",
			Name:                 "Indoor Badminton Championship Hall",
			Category:             "Indoor",
			Description:          "Four BWF-certified teakwood sprung courts with shock absorption, climate cooling, and spectator gallery.",
			Location:             "Indoor Sports Hall, Level 1",
			MaxPlayers:           4,
			StudentTimings:       "06:00 AM - 12:00 PM & 03:00 PM - 11:00 PM",
			FacultyTimings:       "04:00 PM - 06:00 PM (Court 1 & 2 Dedicated)",
			Slots:                slots,
			Rules:                []string{"Strict non-marking indoor court shoes only", "No food or sugary drinks on wood surface"},
			EquipmentAvailable:   []string{"Yonex Nanoray Racket", "Aerosensa Feather Shuttles", "Nylon Shuttles"},
			Image:                "https://images.unsplash.com/photo-1626224583764-f87db24ac4ea?auto=format&fit=crop&w=1200&q=80",
			Badge:                "BWF Certified",
			IsMembershipRequired: false,
		},
		{
			ID:                   "squash-courts",
			Name:                 "Glass-Back Squash Complex (Courts 1-3)",
			Category:             "Racquet",
			Description:          "Three glass-back competition squash courts with Junckers Beech flooring and electronic scoring monitors.",
			Location:             "Sports Complex, West Wing Ground Floor",
			MaxPlayers:           2,
			StudentTimings:       "06:00 AM - 11:00 AM & 04:00 PM - 10:30 PM",
			FacultyTimings:       "05:00 PM - 07:00 PM (Court 3 Reserved)",
			Slots:                slots,
			Rules:                []string{"Goggles/Eye protection recommended", "White/Gum sole shoes only"},
			EquipmentAvailable:   []string{"Head Squash Rackets", "Dunlop Pro Double Yellow Dot Balls"},
			Image:                "https://images.unsplash.com/photo-1574629810360-7efbbe195018?auto=format&fit=crop&w=1200&q=80",
			Badge:                "Glass Back",
			IsMembershipRequired: false,
		},
		{
			ID:                   "tennis-synthetic",
			Name:                 "Hard Tennis Courts (Courts 1 & 2)",
			Category:             "Outdoor",
			Description:          "Two US Open-grade acrylic cushioned hard courts with high-mast LED night lighting.",
			Location:             "Campus South Perimeter Sports Ground",
			MaxPlayers:           4,
			StudentTimings:       "06:00 AM - 10:00 AM & 04:30 PM - 10:30 PM",
			FacultyTimings:       "05:00 PM - 07:00 PM",
			Slots:                slots,
			Rules:                []string{"Tennis shoes with smooth soles required", "Turn off floodlights if last to leave"},
			EquipmentAvailable:   []string{"Wilson Clash Rackets", "Penn Championship Tennis Balls"},
			Image:                "https://images.unsplash.com/photo-1595435934249-5df7ed86e1c0?auto=format&fit=crop&w=1200&q=80",
			Badge:                "Tournament Grade",
			IsMembershipRequired: false,
		},
		{
			ID:                   "futsal-arena",
			Name:                 "GIKI Floodlit Futsal Arena",
			Category:             "Outdoor",
			Description:          "FIFA Quality certified artificial turf pitch with side rebound netting and digital score display.",
			Location:             "Sports Complex Central Courtyard",
			MaxPlayers:           12,
			StudentTimings:       "05:00 PM - 11:30 PM",
			FacultyTimings:       "08:00 PM - 09:30 PM (Wednesdays & Fridays)",
			Slots:                slots,
			Rules:                []string{"Turf shoes only (strictly no metal studs)", "Team captain must present student ID"},
			EquipmentAvailable:   []string{"Match Futsal Balls (Low Bounce)", "Numbered Bibs (Red/Neon)", "Goalkeeper Gloves"},
			Image:                "https://images.unsplash.com/photo-1529900245534-47fbf024a16d?auto=format&fit=crop&w=1200&q=80",
			Badge:                "Night Arena",
			IsMembershipRequired: false,
		},
		{
			ID:                   "table-tennis",
			Name:                 "Table Tennis Arena (6 Tables)",
			Category:             "Indoor",
			Description:          "Equipped with 6 ITTF approved Butterfly Centrefold 25 tournament tables with tournament lighting.",
			Location:             "Indoor Complex, Mezzanine Hall",
			MaxPlayers:           4,
			StudentTimings:       "08:00 AM - 11:00 PM",
			FacultyTimings:       "All-day access (Table 1 dedicated)",
			Slots:                slots,
			Rules:                []string{"Return rackets and 3-star balls to rack after use"},
			EquipmentAvailable:   []string{"Stiga Professional Paddles", "Nittaku 3-Star Poly Balls"},
			Image:                "https://images.unsplash.com/photo-1534158914592-062992fbe900?auto=format&fit=crop&w=1200&q=80",
			Badge:                "ITTF Standard",
			IsMembershipRequired: false,
		},
		{
			ID:                   "gymnasium",
			Name:                 "GIKI Central Fitness & Strength Gym",
			Category:             "Fitness",
			Description:          "Comprehensive weightlifting zone, Olympic squat racks, cardiovascular treadmills, ellipticals, and functional trainer rigs.",
			Location:             "Sports Complex, Ground Floor East",
			MaxPlayers:           35,
			StudentTimings:       "06:30 AM - 10:30 AM (Male/Female Slots) & 04:00 PM - 10:30 PM",
			FacultyTimings:       "05:30 PM - 07:30 PM (Dedicated Faculty Area)",
			Slots:                slots,
			Rules:                []string{"Clean workout towel mandatory", "Re-rack all weights and dumbbells after use", "Indoor trainers required"},
			EquipmentAvailable:   []string{"Lifting Belts", "Resistance Bands", "Foam Rollers"},
			Image:                "https://images.unsplash.com/photo-1534438327276-14e5300c3a48?auto=format&fit=crop&w=1200&q=80",
			Badge:                "Strength & Cardio",
			IsMembershipRequired: true,
		},
		{
			ID:                   "swimming-pool",
			Name:                 "GIKI Semi-Olympic Swimming Pool",
			Category:             "Aquatics",
			Description:          "25-meter 6-lane temperature-controlled pool with ozone filtration and certified lifeguards on duty.",
			Location:             "Aquatic Center, Sports Complex",
			MaxPlayers:           25,
			StudentTimings:       "07:00 AM - 10:00 AM & 04:00 PM - 08:30 PM (Designated male/female sessions)",
			FacultyTimings:       "05:00 PM - 06:30 PM",
			Slots:                slots,
			Rules:                []string{"Shower before entering pool", "Silicone swim cap mandatory", "No diving in shallow end"},
			EquipmentAvailable:   []string{"Kickboards", "Pull Buoys", "Swim Goggles"},
			Image:                "https://images.unsplash.com/photo-1530549387789-4c1017266635?auto=format&fit=crop&w=1200&q=80",
			Badge:                "Heated 25m",
			IsMembershipRequired: true,
		},
	}

	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	s.Data.Bookings = []Booking{
		{
			ID:               "BK-10492",
			FacilityID:       "padel-court-1",
			FacilityName:     "GIKI Glass Padel Arena (Court 1)",
			Date:             today,
			TimeSlot:         "18:00 - 19:00",
			BookerType:       "student",
			BookerName:       "Hamza Tariq",
			BookerID:         "2022-CS-089",
			BookerEmail:      "u2022089@giki.edu.pk",
			BookerPhone:      "+92 301 5551234",
			EquipmentRentals: []string{"Babolat Padel Racket", "Pro Padel Balls (Can of 3)"},
			Status:           "confirmed",
			CheckInCode:      "4821",
			CreatedAt:        time.Now().Add(-2 * time.Hour),
		},
		{
			ID:               "BK-10493",
			FacilityID:       "padel-court-1",
			FacilityName:     "GIKI Glass Padel Arena (Court 1)",
			Date:             today,
			TimeSlot:         "19:00 - 20:00",
			BookerType:       "faculty",
			BookerName:       "Dr. Zia Ur Rehman",
			BookerID:         "FAC-FES-014",
			BookerEmail:      "zia.rehman@giki.edu.pk",
			BookerPhone:      "+92 333 4448899",
			EquipmentRentals: []string{},
			Status:           "confirmed",
			CheckInCode:      "9183",
			CreatedAt:        time.Now().Add(-5 * time.Hour),
		},
		{
			ID:               "BK-10494",
			FacilityID:       "badminton-indoor",
			FacilityName:     "Indoor Badminton Championship Hall",
			Date:             today,
			TimeSlot:         "17:00 - 18:00",
			BookerType:       "student",
			BookerName:       "Ayesha Farooq",
			BookerID:         "2023-EE-045",
			BookerEmail:      "u2023045@giki.edu.pk",
			BookerPhone:      "+92 312 9988776",
			EquipmentRentals: []string{"Aerosensa Feather Shuttles"},
			Status:           "confirmed",
			CheckInCode:      "2049",
			CreatedAt:        time.Now().Add(-8 * time.Hour),
		},
		{
			ID:               "BK-10495",
			FacilityID:       "squash-courts",
			FacilityName:     "Glass-Back Squash Complex (Courts 1-3)",
			Date:             tomorrow,
			TimeSlot:         "18:00 - 19:00",
			BookerType:       "student",
			BookerName:       "Bilal Saeed",
			BookerID:         "2021-ME-112",
			BookerEmail:      "u2021112@giki.edu.pk",
			BookerPhone:      "+92 345 7766554",
			EquipmentRentals: []string{"Head Squash Rackets"},
			Status:           "confirmed",
			CheckInCode:      "7731",
			CreatedAt:        time.Now().Add(-1 * time.Hour),
		},
	}

	s.Data.Achievements = []Achievement{
		{
			ID:           "ach-001",
			Title:        "GIKI Padel Titans Crowned Champions at HEC Intervarsity Tournament 2026",
			Sport:        "Padel Tennis",
			Date:         "September 2026",
			Category:     "National Gold",
			Description:  "The GIKI Varsity Padel squad displayed masterclass tactical precision and endurance, overcoming IBA Karachi in a thrilling 3-set finale (6-4, 4-6, 7-5) to hoist the national trophy.",
			TeamMembers:  "Hamza Tariq (Captain), Daniyal Munir, Saad Ali, Rayyan Shah",
			Image:        "https://images.unsplash.com/photo-1554068865-24cecd4e34b8?auto=format&fit=crop&w=1200&q=80",
			DirectorNote: "Historic victory for GIKI in the rapidly growing paddle arena! Our investments in the glass courts are already generating national championships.",
			Featured:     true,
			CreatedAt:    time.Now().AddDate(0, 0, -10),
		},
		{
			ID:           "ach-002",
			Title:        "All-Pakistan Inter-University Badminton Championship: Silver Medal for GIKI",
			Sport:        "Badminton",
			Date:         "August 2026",
			Category:     "Intervarsity Champion",
			Description:  "GIKI's men doubles pair clinched the Silver Medal after an intense 55-minute clash at the Pakistan Sports Complex Islamabad, defeating LUMS and NUST in preceding rounds.",
			TeamMembers:  "Shahzaib Khan & Waleed Ahmed",
			Image:        "https://images.unsplash.com/photo-1626224583764-f87db24ac4ea?auto=format&fit=crop&w=1200&q=80",
			DirectorNote: "Superb resilience and court coverage. Both student-athletes balanced rigorous engineering exams while training under floodlights.",
			Featured:     true,
			CreatedAt:    time.Now().AddDate(0, -1, 0),
		},
		{
			ID:           "ach-003",
			Title:        "KPK Squash Open: GIKI Squash Ace Reaches Semifinals",
			Sport:        "Squash",
			Date:         "July 2026",
			Category:     "Special Mention",
			Description:  "Representing GIKI Sports Complex, sophomore Hassan Raza defeated 2 seeded provincial players before reaching the semifinal stage of the prestigious KPK Open.",
			TeamMembers:  "Hassan Raza (Mechanical Engg.)",
			Image:        "https://images.unsplash.com/photo-1574629810360-7efbbe195018?auto=format&fit=crop&w=1200&q=80",
			DirectorNote: "Hassan's dedication sets a benchmark for all incoming freshers at the sports orientation.",
			Featured:     false,
			CreatedAt:    time.Now().AddDate(0, -2, 0),
		},
		{
			ID:           "ach-004",
			Title:        "GIKI Futsal League 2026: Mechanical Hawks Hoist President's Cup",
			Sport:        "Futsal",
			Date:         "May 2026",
			Category:     "Campus Championship",
			Description:  "Over 24 departmental teams battled through 3 weeks under the lights. Mechanical Engineering emerged victorious in sudden-death penalty shootout against Computer Science.",
			TeamMembers:  "Hawks Squad (Capt: Zeeshan Malik, Golden Boot: Zain Ul Abideen)",
			Image:        "https://images.unsplash.com/photo-1529900245534-47fbf024a16d?auto=format&fit=crop&w=1200&q=80",
			DirectorNote: "Unprecedented campus engagement with over 800 students attending the night final.",
			Featured:     true,
			CreatedAt:    time.Now().AddDate(0, -4, 0),
		},
	}

	s.Data.Announcements = []Announcement{
		{
			ID:        "ann-01",
			Title:     "New Glass Padel Court #2 Floodlights Upgraded to 800 Lux LED",
			Content:   "To support evening tournament play and student night sessions, court #2 lighting has been upgraded. Extra night slots are now bookable up to 11:00 PM.",
			Date:      today,
			Priority:  "info",
			CreatedAt: time.Now().Add(-12 * time.Hour),
		},
		{
			ID:        "ann-02",
			Title:     "GIKI Annual Sports Gala 2026 Registration Opens Next Monday",
			Content:   "Trial registrations for Racquet Sports, Futsal, Table Tennis, and Swimming will commence through this portal. Keep your GIKI ID validated.",
			Date:      today,
			Priority:  "urgent",
			CreatedAt: time.Now().Add(-24 * time.Hour),
		},
		{
			ID:        "ann-03",
			Title:     "Squash Court #2 Floor Buffing & Maintenance Schedule",
			Content:   "Squash Court 2 will remain offline on Sunday morning (08:00 - 14:00) for standard polyurethane varnish refinishing. Court 1 & 3 remain operational.",
			Date:      today,
			Priority:  "maintenance",
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
	}

	s.Data.Memberships = []Membership{
		{
			ID:            "MBR-8801",
			ApplicantName: "Saad Farooq",
			ApplicantID:   "2023-CS-302",
			ApplicantType: "student",
			Plan:          "Padel Court Priority Pass (Semester)",
			Status:        "active",
			ValidUntil:    "2026-12-31",
			CreatedAt:     time.Now().AddDate(0, -1, 0),
		},
		{
			ID:            "MBR-8802",
			ApplicantName: "Dr. Tariq Jadoon",
			ApplicantID:   "FAC-FCSE-003",
			ApplicantType: "faculty",
			Plan:          "Faculty All-Sports VIP Card",
			Status:        "active",
			ValidUntil:    "2027-06-30",
			CreatedAt:     time.Now().AddDate(0, -2, 0),
		},
	}
}

func main() {
	storePath := filepath.Join(".", "data", "store.json")
	store := NewStore(storePath)

	app := fiber.New(fiber.Config{
		AppName:      "GIKI Sports Complex API v1.0 (Ultra Fast)",
		ServerHeader: "GoFiber/GIKI-Sports",
	})

	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${latency} ${method} ${path}\n",
	}))
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Director-Key",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	// Static & Health
	api := app.Group("/api")

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":    "online",
			"engine":    "Go Fiber v2 / fasthttp",
			"time":      time.Now().Format(time.RFC3339),
			"timestamp": time.Now().UnixMilli(),
		})
	})

	// Facilities & Timings
	api.Get("/facilities", func(c *fiber.Ctx) error {
		store.mu.RLock()
		defer store.mu.RUnlock()
		return c.JSON(store.Data.Facilities)
	})

	api.Get("/facilities/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		store.mu.RLock()
		defer store.mu.RUnlock()

		for _, f := range store.Data.Facilities {
			if f.ID == id {
				return c.JSON(f)
			}
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Facility not found"})
	})

	// Bookings
	api.Get("/bookings", func(c *fiber.Ctx) error {
		date := c.Query("date")
		facilityID := c.Query("facilityId")

		store.mu.RLock()
		defer store.mu.RUnlock()

		var results []Booking
		for _, b := range store.Data.Bookings {
			if b.Status == "cancelled" {
				continue
			}
			if date != "" && b.Date != date {
				continue
			}
			if facilityID != "" && b.FacilityID != facilityID {
				continue
			}
			results = append(results, b)
		}
		return c.JSON(results)
	})

	api.Post("/bookings", func(c *fiber.Ctx) error {
		var req struct {
			FacilityID       string   `json:"facilityId"`
			Date             string   `json:"date"`
			TimeSlot         string   `json:"timeSlot"`
			BookerType       string   `json:"bookerType"`
			BookerName       string   `json:"bookerName"`
			BookerID         string   `json:"bookerId"`
			BookerEmail      string   `json:"bookerEmail"`
			BookerPhone      string   `json:"bookerPhone"`
			EquipmentRentals []string `json:"equipmentRentals"`
		}

		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request payload"})
		}

		if req.FacilityID == "" || req.Date == "" || req.TimeSlot == "" || req.BookerName == "" || req.BookerID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Missing mandatory booking fields"})
		}

		store.mu.Lock()
		defer store.mu.Unlock()

		// Verify facility exists
		var facilityName string
		facilityExists := false
		for _, f := range store.Data.Facilities {
			if f.ID == req.FacilityID {
				facilityExists = true
				facilityName = f.Name
				break
			}
		}
		if !facilityExists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid facility specified"})
		}

		// Check for conflict (concurrency protection)
		for _, b := range store.Data.Bookings {
			if b.FacilityID == req.FacilityID && b.Date == req.Date && b.TimeSlot == req.TimeSlot && b.Status != "cancelled" {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error": fmt.Sprintf("Time slot %s on %s is already reserved by %s (%s)", req.TimeSlot, req.Date, b.BookerName, b.BookerType),
				})
			}
		}

		// Generate random 4-digit check-in code
		code := fmt.Sprintf("%04d", rand.Intn(9000)+1000)
		bookingID := fmt.Sprintf("BK-%05d", (time.Now().Unix()%100000)+int64(rand.Intn(1000)))

		newBooking := Booking{
			ID:               bookingID,
			FacilityID:       req.FacilityID,
			FacilityName:     facilityName,
			Date:             req.Date,
			TimeSlot:         req.TimeSlot,
			BookerType:       req.BookerType,
			BookerName:       req.BookerName,
			BookerID:         req.BookerID,
			BookerEmail:      req.BookerEmail,
			BookerPhone:      req.BookerPhone,
			EquipmentRentals: req.EquipmentRentals,
			Status:           "confirmed",
			CheckInCode:      code,
			CreatedAt:        time.Now(),
		}

		store.Data.Bookings = append([]Booking{newBooking}, store.Data.Bookings...)
		_ = store.saveUnsafe()

		return c.Status(fiber.StatusCreated).JSON(newBooking)
	})

	api.Delete("/bookings/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		store.mu.Lock()
		defer store.mu.Unlock()

		found := false
		for i, b := range store.Data.Bookings {
			if b.ID == id {
				store.Data.Bookings[i].Status = "cancelled"
				found = true
				break
			}
		}

		if !found {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Booking not found"})
		}

		_ = store.saveUnsafe()
		return c.JSON(fiber.Map{"success": true, "message": "Booking cancelled successfully"})
	})

	// Achievements (Director of Sports Showcase)
	api.Get("/achievements", func(c *fiber.Ctx) error {
		store.mu.RLock()
		defer store.mu.RUnlock()
		return c.JSON(store.Data.Achievements)
	})

	api.Post("/achievements", func(c *fiber.Ctx) error {
		var ach Achievement
		if err := c.BodyParser(&ach); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid achievement payload"})
		}

		if ach.Title == "" || ach.Sport == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Title and Sport are required"})
		}

		if ach.Image == "" {
			ach.Image = "https://images.unsplash.com/photo-1554068865-24cecd4e34b8?auto=format&fit=crop&w=1200&q=80"
		}
		if ach.Date == "" {
			ach.Date = time.Now().Format("January 2006")
		}

		ach.ID = "ach-" + uuid.New().String()[:8]
		ach.CreatedAt = time.Now()

		store.mu.Lock()
		store.Data.Achievements = append([]Achievement{ach}, store.Data.Achievements...)
		_ = store.saveUnsafe()
		store.mu.Unlock()

		return c.Status(fiber.StatusCreated).JSON(ach)
	})

	api.Delete("/achievements/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		store.mu.Lock()
		defer store.mu.Unlock()

		var updated []Achievement
		found := false
		for _, a := range store.Data.Achievements {
			if a.ID == id {
				found = true
			} else {
				updated = append(updated, a)
			}
		}

		if !found {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Achievement not found"})
		}

		store.Data.Achievements = updated
		_ = store.saveUnsafe()
		return c.JSON(fiber.Map{"success": true})
	})

	// Announcements
	api.Get("/announcements", func(c *fiber.Ctx) error {
		store.mu.RLock()
		defer store.mu.RUnlock()
		return c.JSON(store.Data.Announcements)
	})

	api.Post("/announcements", func(c *fiber.Ctx) error {
		var ann Announcement
		if err := c.BodyParser(&ann); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid payload"})
		}
		if ann.Title == "" || ann.Content == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Title and content required"})
		}

		ann.ID = "ann-" + uuid.New().String()[:8]
		ann.Date = time.Now().Format("2006-01-02")
		ann.CreatedAt = time.Now()

		store.mu.Lock()
		store.Data.Announcements = append([]Announcement{ann}, store.Data.Announcements...)
		_ = store.saveUnsafe()
		store.mu.Unlock()

		return c.Status(fiber.StatusCreated).JSON(ann)
	})

	// Memberships
	api.Get("/memberships", func(c *fiber.Ctx) error {
		store.mu.RLock()
		defer store.mu.RUnlock()
		return c.JSON(store.Data.Memberships)
	})

	api.Post("/memberships", func(c *fiber.Ctx) error {
		var req struct {
			ApplicantName string `json:"applicantName"`
			ApplicantID   string `json:"applicantId"`
			ApplicantType string `json:"applicantType"`
			Plan          string `json:"plan"`
		}

		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid payload"})
		}

		if req.ApplicantName == "" || req.ApplicantID == "" || req.Plan == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Missing applicant details or plan"})
		}

		mbr := Membership{
			ID:            fmt.Sprintf("MBR-%04d", rand.Intn(9000)+1000),
			ApplicantName: req.ApplicantName,
			ApplicantID:   strings.ToUpper(req.ApplicantID),
			ApplicantType: req.ApplicantType,
			Plan:          req.Plan,
			Status:        "active",
			ValidUntil:    time.Now().AddDate(0, 6, 0).Format("2006-01-02"),
			CreatedAt:     time.Now(),
		}

		store.mu.Lock()
		store.Data.Memberships = append([]Membership{mbr}, store.Data.Memberships...)
		_ = store.saveUnsafe()
		store.mu.Unlock()

		return c.Status(fiber.StatusCreated).JSON(mbr)
	})

	// Stats endpoint for quick dashboard
	api.Get("/stats", func(c *fiber.Ctx) error {
		store.mu.RLock()
		defer store.mu.RUnlock()

		today := time.Now().Format("2006-01-02")
		todayBookingsCount := 0
		for _, b := range store.Data.Bookings {
			if b.Date == today && b.Status != "cancelled" {
				todayBookingsCount++
			}
		}

		return c.JSON(fiber.Map{
			"totalFacilities":   len(store.Data.Facilities),
			"activeBookings":    len(store.Data.Bookings),
			"todayBookings":     todayBookingsCount,
			"achievementsCount": len(store.Data.Achievements),
			"membershipsCount":  len(store.Data.Memberships),
			"systemStatus":      "Optimal - All Courts Lit & Operational",
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("⚡ GIKI Sports Complex Ultra-Fast Go Backend listening on port :%s\n", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
