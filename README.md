# ⚡ GIKI Sports Complex — Web Application

Official sports facility booking, timings directory, and Director of Sports achievements portal for **Ghulam Ishaq Khan Institute of Engineering Sciences and Technology (GIKI)**.

---

## 🌟 Overview

The **GIKI Sports Complex Portal** is designed to provide students, faculty, and administrative staff with a seamless digital platform for:
1. **Sports & Court Timings**: View all 9 sports facilities with distinct student hours and faculty-dedicated priority slots.
2. **Court Slot Reservation**: Instant, real-time booking for high-demand courts (Padel, Badminton, Squash, Tennis, Futsal, and more).
3. **Director of Sports Showcase**: A dedicated portal for the Director of Sports to publish tournament victories, HEC Intervarsity championship trophies, medals, and official campus sports notices.
4. **Passes & Memberships**: Manage semester racquet club passes, gym/pool memberships, and priority court passes.

---

## 🚀 Technology Stack

### Backend (Ultra-Fast)
- **Language & Runtime**: [Go 1.26](https://go.dev/)
- **Web Framework**: [Go Fiber v2](https://gofiber.io/) — powered by `fasthttp`, the fastest HTTP engine in Go, delivering sub-millisecond responses.
- **Concurrency & Safety**: Thread-safe mutex locking (`sync.RWMutex`) to guarantee zero double-booking race conditions during high-demand slot reservation rushes.
- **Data Persistence**: Atomic JSON storage (`backend/data/store.json`) with automatic seeding of realistic GIKI sports complex facilities, achievements, notices, and test bookings.

### Frontend
- **Framework**: [React 19](https://react.dev/) + [TypeScript](https://www.typescriptlang.org/)
- **Bundler & Dev Server**: [Vite](https://vite.dev/) with configured `/api` proxy to the Go backend.
- **Styling**: [Tailwind CSS v4](https://tailwindcss.com/) with `@tailwindcss/vite` Oxide engine.
- **API Client & Types**: Ready-to-use typed client in [`frontend/src/api.ts`](frontend/src/api.ts) and TypeScript models in [`frontend/src/types.ts`](frontend/src/types.ts).

---

## 📁 Repository Structure

```text
giki sports complex/
├── README.md                     # Project documentation & API guide
├── backend/
│   ├── data/
│   │   └── store.json            # Persistent database (facilities, bookings, achievements)
│   ├── giki-backend.exe          # Compiled ultra-fast Go binary
│   ├── go.mod                    # Go module definition
│   ├── go.sum                    # Dependency checksums
│   └── main.go                   # Go Fiber REST API implementation
└── frontend/
    ├── package.json              # React dependencies (Vite, Tailwind v4, Lucide)
    ├── vite.config.ts            # Vite config with Tailwind v4 & backend proxy
    ├── index.html                # Entry HTML with athletic typography
    └── src/
        ├── api.ts                # Full REST API client for Go backend
        ├── types.ts              # TypeScript interfaces for facilities, slots, bookings
        ├── index.css             # Tailwind CSS v4 base & design tokens
        ├── App.tsx               # Main React entry component
        └── main.tsx              # React DOM bootstrap
```

---

## ⚡ Getting Started

### 1. Run the Ultra-Fast Go Backend

Navigate to the `backend` folder and run the server:

```powershell
cd backend
go run .
```

*Or run the compiled executable directly:*
```powershell
cd backend
.\giki-backend.exe
```

The Go API server will start on:
👉 **`http://localhost:8080`**

To test backend health:
```powershell
curl http://localhost:8080/api/health
```

---

### 2. Run the React Frontend

Open a new terminal window, navigate to the `frontend` folder, and start Vite:

```powershell
cd frontend
npm run dev
```

The frontend dev server will launch on:
👉 **`http://localhost:5173`** (with automatic `/api` proxy to port 8080).

---

## 📡 REST API Reference

All endpoints return JSON and provide sub-millisecond response latency.

### 🟢 System & Health
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/health` | Returns server health status and runtime engine (`Go Fiber v2 / fasthttp`). |
| `GET` | `/api/stats` | Aggregated dashboard metrics (total facilities, today's active bookings, trophies, memberships). |

---

### 🏸 Facilities & Timings
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/facilities` | List all sports facilities with operating hours, student timings, faculty priority slots, status, and schedules. |
| `GET` | `/api/facilities/:slug` | Retrieve specific facility details by URL slug (e.g., `gymnasium`, `table-tennis`, `padel`). |
| `POST` | `/api/facilities` | **Director / Super Admin**: Create a new sports facility with auto/custom slug and audit logging. |
| `PATCH` | `/api/facilities/:slug` | **Director / Super Admin**: Edit existing facility attributes (name, status, description) with audit trail. |
| `POST` | `/api/facilities/:slug/schedules` | **Director / Super Admin**: Add a weekly operating window for a facility with audit logging. |
| `PUT` | `/api/facilities/:slug/schedules/:id` | **Director / Super Admin**: Modify an existing operating schedule window with audit logging. |
| `DELETE` | `/api/facilities/:slug/schedules/:id` | **Director / Super Admin**: Delete a weekly operating window with audit logging. |

#### Example Create Facility Request:
```http
POST /api/facilities
Content-Type: application/json
Authorization: Session-based cookie auth (sports_director or super_admin)

{
  "name": "Squash Complex",
  "slug": "squash-complex",
  "status": "active",
  "description": "Three ASB glass-backed squash courts with Junckers sprung beechwood flooring and electronic scoring."
}
```

#### Example Update Facility Request:
```http
PATCH /api/facilities/squash-complex
Content-Type: application/json
Authorization: Session-based cookie auth (sports_director or super_admin)

{
  "name": "Main Squash Complex",
  "status": "maintenance",
  "description": "Under scheduled glass polishing and annual floor recoating until Monday."
}
```

---

### 📅 Court Slot Bookings
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/bookings` | Query bookings. Optional query parameters: `?date=YYYY-MM-DD` and `?facilityId=padel-court-1`. |
| `POST` | `/api/bookings` | Reserve a court time slot. Concurrency-safe; prevents double booking. |
| `DELETE` | `/api/bookings/:id` | Cancel a booking reservation. |

#### Example Booking Request:
```http
POST /api/bookings
Content-Type: application/json

{
  "facilityId": "padel-court-1",
  "date": "2026-09-26",
  "timeSlot": "19:00 - 20:00",
  "bookerType": "student",
  "bookerName": "Hamza Tariq",
  "bookerId": "2023-CS-104",
  "bookerEmail": "u2023104@giki.edu.pk",
  "bookerPhone": "+92 300 1234567",
  "equipmentRentals": ["Babolat Padel Racket", "Pro Padel Balls (Can of 3)"]
}
```

---

### 🏆 Director of Sports Achievements & Honors
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/achievements` | List all university athletic achievements, trophies, and tournament highlights. |
| `POST` | `/api/achievements` | Upload a new achievement (Title, Sport, Date, Category, Team Roster, Photo, Director Note). |
| `DELETE` | `/api/achievements/:id` | Remove an achievement record. |

#### Example Create Achievement Request:
```http
POST /api/achievements
Content-Type: application/json

{
  "title": "GIKI Padel Team Clinches All-Pakistan Intervarsity Gold 2026",
  "sport": "Padel Tennis",
  "date": "September 2026",
  "category": "National Gold",
  "description": "Defeated IBA Karachi in a 3-set finale to hoist the national university padel trophy.",
  "teamMembers": "Hamza Tariq (Captain), Daniyal Munir, Saad Ali, Rayyan Shah",
  "image": "https://images.unsplash.com/photo-1554068865-24cecd4e34b8?auto=format&fit=crop&w=1200&q=80",
  "directorNote": "Historic victory! The investments in our glass padel courts are already producing national champions.",
  "featured": true
}
```

---

### 📢 Announcements & Notices
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/announcements` | List complex notices (court maintenance, floodlight upgrades, sports gala trials). |
| `POST` | `/api/announcements` | Post a new official notice (`priority`: `urgent`, `info`, or `maintenance`). |

---

### 💳 Passes & Memberships
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/memberships` | List registered membership passes. |
| `POST` | `/api/memberships` | Apply for a court or semester pass (e.g., Padel Priority Pass, Faculty All-Access Pass). |

---

## 🏛️ Sports Facilities Included

1. **Padel (Court 1 & 2)**: Panoramic tempered glass, synthetic turf, 800 Lux LED tournament floodlights.
2. **Badminton**: 4 BWF-certified teakwood sprung courts with climate cooling.
3. **Squash (Courts 1-3)**: Junckers beech flooring and electronic score monitors.
4. **Tennis (Courts 1 & 2)**: US Open-grade acrylic cushioned hard courts.
5. **Futsal**: FIFA Quality artificial turf with rebound netting.
6. **Table Tennis**: 6 ITTF-approved Butterfly tournament tables.
7. **Gymnasium**: Olympic racks, cardio zone, functional rigs.
8. **Swimming Pool**: 25-meter 6-lane temperature-controlled pool with ozone filtration.

---

## 📝 Frontend Development Notes

When you are ready to design your frontend UI:
- Use the typed API client already configured in [`frontend/src/api.ts`](frontend/src/api.ts). It contains methods like `api.getFacilities()`, `api.createBooking()`, `api.createAchievement()`, and `api.getStats()`.
- Data types are defined in [`frontend/src/types.ts`](frontend/src/types.ts).
- Tailwind CSS v4 is configured with `@tailwindcss/vite` in [`frontend/vite.config.ts`](frontend/vite.config.ts).
