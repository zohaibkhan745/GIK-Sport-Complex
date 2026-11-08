import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Home } from "../pages/Home";
import { api } from "../api";
import type { Facility } from "../types";

vi.mock("../api", () => ({
  api: {
    getFacilities: vi.fn(),
    getFacility: vi.fn(),
  },
}));

describe("Facility List / Home Component", () => {
  const mockFacilities: Facility[] = [
    {
      id: "fac-1",
      slug: "demo-badminton-court",
      name: "[Demo] Campus Badminton Court",
      description: "Demonstration indoor court",
      location: "Sports Complex Wing B",
      status: "active",
      schedules: [
        {
          id: "sched-1",
          dayOfWeek: 1,
          startTime: "09:00:00",
          endTime: "12:00:00",
          audience: "student",
        },
      ],
    },
    {
      id: "fac-2",
      slug: "demo-table-tennis",
      name: "[Demo] Table Tennis Arena",
      description: null,
      location: null,
      status: "maintenance",
      schedules: [],
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading state while fetching facilities", () => {
    vi.mocked(api.getFacilities).mockReturnValue(new Promise(() => {})); // Never resolves

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>
    );

    expect(screen.getByText(/Loading live facilities.../i)).toBeInTheDocument();
  });

  it("shows empty state when facility list is empty", async () => {
    vi.mocked(api.getFacilities).mockResolvedValue([]);

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(
        screen.getByText(/No facilities available/i)
      ).toBeInTheDocument();
    });
  });

  it("shows success state with facility cards and slug navigation links", async () => {
    vi.mocked(api.getFacilities).mockResolvedValue(mockFacilities);

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(
        screen.getByText("[Demo] Campus Badminton Court")
      ).toBeInTheDocument();
      expect(screen.getByText("[Demo] Table Tennis Arena")).toBeInTheDocument();
    });

    const badmintonLink = screen
      .getByText("[Demo] Campus Badminton Court")
      .closest("a");
    expect(badmintonLink).toHaveAttribute(
      "href",
      "/facilities/demo-badminton-court"
    );

    const ttLink = screen.getByText("[Demo] Table Tennis Arena").closest("a");
    expect(ttLink).toHaveAttribute("href", "/facilities/demo-table-tennis");
  });

  it("shows error state with retry button on failure, and clicking retry recovers", async () => {
    vi.mocked(api.getFacilities)
      .mockRejectedValueOnce(new Error("Network connection failed"))
      .mockResolvedValueOnce(mockFacilities);

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(
        screen.getByText(/Network connection failed/i)
      ).toBeInTheDocument();
    });

    const retryButton = screen.getByRole("button", { name: /^retry$/i });
    expect(retryButton).toBeInTheDocument();

    fireEvent.click(retryButton);

    await waitFor(() => {
      expect(
        screen.getByText("[Demo] Campus Badminton Court")
      ).toBeInTheDocument();
    });
    expect(api.getFacilities).toHaveBeenCalledTimes(2);
  });
});
