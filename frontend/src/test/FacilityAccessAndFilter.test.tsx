import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { FacilityCard } from "../components/FacilityCard";
import { FacilitiesGrid } from "../components/FacilitiesGrid";
import type { Facility } from "../types";

const mockFacilities: Facility[] = [
  {
    id: "fac-padel",
    slug: "glass-padel-courts",
    name: "Padel",
    description: "Championship glass courts with floodlights",
    status: "active",
    schedules: [
      {
        id: "s-padel-1",
        dayOfWeek: 1,
        startTime: "06:00:00",
        endTime: "11:00:00",
        audience: "student",
      },
      {
        id: "s-padel-2",
        dayOfWeek: 7,
        startTime: "14:00:00",
        endTime: "17:00:00",
        audience: "women_only",
      },
    ],
  },
  {
    id: "fac-gym",
    slug: "central-fitness-gym",
    name: "Gymnasium",
    description: "Olympic racks, cardio zone, functional strength rigs",
    status: "active",
    schedules: [
      {
        id: "s-gym-1",
        dayOfWeek: 1,
        startTime: "06:00:00",
        endTime: "11:00:00",
        audience: "student",
      },
      {
        id: "s-gym-2",
        dayOfWeek: 2,
        startTime: "11:00:00",
        endTime: "14:00:00",
        audience: "faculty",
      },
    ],
  },
  {
    id: "fac-badminton",
    slug: "badminton-championship-hall",
    name: "Badminton",
    description: "Teakwood sprung courts with climate cooling",
    status: "active",
    schedules: [
      {
        id: "s-bad-1",
        dayOfWeek: 1,
        startTime: "06:00:00",
        endTime: "12:00:00",
        audience: "student",
      },
    ],
  },
  {
    id: "fac-pool",
    slug: "semi-olympic-swimming-pool",
    name: "Swimming Pool",
    description: "25m 6-lane ozone filtered pool",
    status: "active",
    schedules: [
      {
        id: "s-pool-1",
        dayOfWeek: 6,
        startTime: "09:00:00",
        endTime: "13:00:00",
        audience: "faculty",
      },
    ],
  },
];

describe("FacilityCard Component (Access Model Specialization)", () => {
  it("renders clean Padel facility card with direct 'Book Court' button and without redundant badges", () => {
    render(
      <MemoryRouter>
        <FacilityCard facility={mockFacilities[0]} />
      </MemoryRouter>
    );

    expect(screen.getByText("Padel")).toBeInTheDocument();
    expect(screen.queryByText("Online Booking")).not.toBeInTheDocument();
    expect(screen.queryByText("Women-Only")).not.toBeInTheDocument();

    const bookLink = screen.getByRole("link", { name: /book (court|slot)/i });
    expect(bookLink).toHaveAttribute(
      "href",
      "/padel?facility=glass-padel-courts"
    );

    const titleLink = screen.getByText("Padel").closest("a");
    expect(titleLink).toHaveAttribute("href", "/facilities/glass-padel-courts");
  });

  it("renders clean Gym facility card with direct 'Get Pass' button and without redundant badges", () => {
    render(
      <MemoryRouter>
        <FacilityCard facility={mockFacilities[1]} />
      </MemoryRouter>
    );

    expect(
      screen.getByText("Gymnasium")
    ).toBeInTheDocument();
    expect(screen.queryByText("30-Day Pass")).not.toBeInTheDocument();

    const passLink = screen.getByRole("link", { name: /get( \/ renew)? pass/i });
    expect(passLink).toHaveAttribute("href", "/membership");

    const titleLink = screen
      .getByText("Gymnasium")
      .closest("a");
    expect(titleLink).toHaveAttribute("href", "/facilities/central-fitness-gym");
  });

  it("renders standard sport facility with 'View Schedule' linking to /facilities/:slug", () => {
    render(
      <MemoryRouter>
        <FacilityCard facility={mockFacilities[2]} />
      </MemoryRouter>
    );

    expect(
      screen.getByText("Badminton")
    ).toBeInTheDocument();
    const scheduleLink = screen.getByRole("link", { name: /view schedule/i });
    expect(scheduleLink).toHaveAttribute(
      "href",
      "/facilities/badminton-championship-hall"
    );
  });

  it("navigates to facility detail page when clicking anywhere on the card body", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route path="/" element={<FacilityCard facility={mockFacilities[2]} />} />
          <Route
            path="/facilities/:slug"
            element={<div data-testid="detail-view">Facility Detail Loaded</div>}
          />
        </Routes>
      </MemoryRouter>
    );

    const card = screen.getByTestId("facility-card-badminton-championship-hall");
    fireEvent.click(card);

    expect(screen.getByTestId("detail-view")).toBeInTheDocument();
  });
});

describe("FacilitiesGrid Component (Clean Streamlined Grid)", () => {
  it("renders all facilities directly without redundant search or filter bars", () => {
    render(
      <MemoryRouter>
        <FacilitiesGrid
          facilities={mockFacilities}
          isLoading={false}
          error={null}
          onRetry={vi.fn()}
        />
      </MemoryRouter>
    );

    // Verify all 4 mock facilities render directly
    expect(screen.getByText("Padel")).toBeInTheDocument();
    expect(screen.getByText("Gymnasium")).toBeInTheDocument();
    expect(screen.getByText("Badminton")).toBeInTheDocument();
    expect(screen.getByText("Swimming Pool")).toBeInTheDocument();

    // Verify search and filter elements do not exist
    expect(screen.queryByPlaceholderText(/search facilities/i)).not.toBeInTheDocument();
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/filter by audience timetable/i)).not.toBeInTheDocument();
  });

  it("renders loading state when isLoading is true", () => {
    render(
      <MemoryRouter>
        <FacilitiesGrid
          facilities={[]}
          isLoading={true}
          error={null}
          onRetry={vi.fn()}
        />
      </MemoryRouter>
    );

    expect(screen.getByTestId("facilities-loading")).toBeInTheDocument();
    expect(screen.getByText(/loading live facilities/i)).toBeInTheDocument();
  });

  it("renders error state with retry button when error occurs", () => {
    const handleRetry = vi.fn();
    render(
      <MemoryRouter>
        <FacilitiesGrid
          facilities={[]}
          isLoading={false}
          error="Network error fetching facilities"
          onRetry={handleRetry}
        />
      </MemoryRouter>
    );

    expect(screen.getByTestId("facilities-error")).toBeInTheDocument();
    expect(screen.getByText(/unable to load sports facilities/i)).toBeInTheDocument();
    expect(screen.getByText(/network error fetching facilities/i)).toBeInTheDocument();

    const retryBtn = screen.getByRole("button", { name: /retry/i });
    fireEvent.click(retryBtn);
    expect(handleRetry).toHaveBeenCalledTimes(1);
  });

  it("shows empty state when zero facilities exist in database", () => {
    render(
      <MemoryRouter>
        <FacilitiesGrid
          facilities={[]}
          isLoading={false}
          error={null}
          onRetry={vi.fn()}
        />
      </MemoryRouter>
    );

    expect(screen.getByTestId("facilities-empty")).toBeInTheDocument();
    expect(screen.getByText(/no facilities available/i)).toBeInTheDocument();
  });
});
