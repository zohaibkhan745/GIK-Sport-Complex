import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { CreateFacilityModal, slugifyText } from "../components/CreateFacilityModal";
import { EditFacilityModal } from "../components/EditFacilityModal";
import { FacilitiesGrid } from "../components/FacilitiesGrid";
import { AuthContext, type AuthContextType } from "../context/AuthContext";
import { api, ApiError } from "../api";
import type { Facility } from "../types";

vi.mock("../api", () => {
  class MockApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.name = "ApiError";
      this.status = status;
    }
  }
  return {
    api: {
      createFacility: vi.fn(),
      updateFacility: vi.fn(),
      getFacilities: vi.fn(),
    },
    ApiError: MockApiError,
  };
});

describe("slugifyText Helper", () => {
  it("converts spaces and uppercase to lowercase hyphens", () => {
    expect(slugifyText("Gymnasium")).toBe("gymnasium");
    expect(slugifyText("Table Tennis")).toBe("table-tennis");
    expect(slugifyText("Olympic Swimming Pool")).toBe("olympic-swimming-pool");
  });

  it("removes special characters and handles repeated hyphens", () => {
    expect(slugifyText("Padel Court #1 (Glass)")).toBe("padel-court-1-glass");
    expect(slugifyText("---Leading and Trailing---")).toBe("leading-and-trailing");
  });
});

describe("CreateFacilityModal Component", () => {
  const onCreatedMock = vi.fn();
  const onCloseMock = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders all form fields when open", () => {
    render(
      <CreateFacilityModal
        isOpen={true}
        onClose={onCloseMock}
        onCreated={onCreatedMock}
      />
    );

    expect(screen.getByRole("heading", { name: /Add New Facility/i })).toBeInTheDocument();
    expect(screen.getByTestId("create-facility-name-input")).toBeInTheDocument();
    expect(screen.getByTestId("create-facility-slug-input")).toBeInTheDocument();
    expect(screen.getByTestId("status-opt-active")).toBeInTheDocument();
    expect(screen.getByTestId("status-opt-maintenance")).toBeInTheDocument();
    expect(screen.getByTestId("status-opt-closed")).toBeInTheDocument();
    expect(screen.getByTestId("create-facility-description-input")).toBeInTheDocument();
    expect(screen.getByTestId("create-facility-submit-btn")).toBeInTheDocument();
  });

  it("auto-generates slug from name and allows manual edit", () => {
    render(
      <CreateFacilityModal
        isOpen={true}
        onClose={onCloseMock}
        onCreated={onCreatedMock}
      />
    );

    const nameInput = screen.getByTestId("create-facility-name-input");
    const slugInput = screen.getByTestId("create-facility-slug-input") as HTMLInputElement;

    fireEvent.change(nameInput, { target: { value: "Squash Courts" } });
    expect(slugInput.value).toBe("squash-courts");

    // Toggle manual editing
    fireEvent.click(screen.getByText(/Edit manually/i));
    fireEvent.change(slugInput, { target: { value: "custom-squash" } });
    expect(slugInput.value).toBe("custom-squash");

    // Auto-generate back
    fireEvent.click(screen.getByText(/Auto-generate/i));
    expect(slugInput.value).toBe("squash-courts");
  });

  it("submits createFacility with valid fields and calls onCreated callback", async () => {
    const createdFacility: Facility = {
      id: "new-fac-id",
      slug: "squash-arena",
      name: "Squash Arena",
      description: "Two international glass back courts",
      status: "active",
      schedules: [],
    };

    vi.mocked(api.createFacility).mockResolvedValue(createdFacility);

    render(
      <CreateFacilityModal
        isOpen={true}
        onClose={onCloseMock}
        onCreated={onCreatedMock}
      />
    );

    fireEvent.change(screen.getByTestId("create-facility-name-input"), {
      target: { value: "Squash Arena" },
    });
    fireEvent.change(screen.getByTestId("create-facility-description-input"), {
      target: { value: "Two international glass back courts" },
    });

    fireEvent.click(screen.getByTestId("create-facility-submit-btn"));

    await waitFor(() => {
      expect(api.createFacility).toHaveBeenCalledWith({
        name: "Squash Arena",
        slug: "squash-arena",
        status: "active",
        statusUntil: null,
        statusNote: null,
        reservedFor: null,
        description: "Two international glass back courts",
      });
      expect(onCreatedMock).toHaveBeenCalledWith(createdFacility);
      expect(onCloseMock).toHaveBeenCalled();
    });
  });

  it("displays conflict error banner when slug already exists", async () => {
    vi.mocked(api.createFacility).mockRejectedValue(
      new ApiError(409, "facility already exists")
    );

    render(
      <CreateFacilityModal
        isOpen={true}
        onClose={onCloseMock}
        onCreated={onCreatedMock}
      />
    );

    fireEvent.change(screen.getByTestId("create-facility-name-input"), {
      target: { value: "Gymnasium" },
    });
    fireEvent.click(screen.getByTestId("create-facility-submit-btn"));

    await waitFor(() => {
      expect(screen.getByTestId("create-facility-error")).toHaveTextContent(
        /A facility with slug "gymnasium" already exists/i
      );
    });
  });

  it("closes dialog on Cancel button click", () => {
    render(
      <CreateFacilityModal
        isOpen={true}
        onClose={onCloseMock}
        onCreated={onCreatedMock}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCloseMock).toHaveBeenCalledTimes(1);
  });
});

describe("EditFacilityModal Component", () => {
  const mockFacility: Facility = {
    id: "fac-1",
    slug: "gymnasium",
    name: "Gymnasium",
    description: "Olympic racks, cardio zone, functional strength rigs",
    status: "active",
    schedules: [],
  };

  const onUpdatedMock = vi.fn();
  const onCloseMock = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("pre-populates form with existing facility values", () => {
    render(
      <EditFacilityModal
        isOpen={true}
        facility={mockFacility}
        onClose={onCloseMock}
        onUpdated={onUpdatedMock}
      />
    );

    expect(screen.getByRole("heading", { name: /Edit Facility Details/i })).toBeInTheDocument();
    expect(screen.getByTestId("edit-facility-name-input")).toHaveValue("Gymnasium");
    expect(screen.getByTestId("edit-facility-description-input")).toHaveValue(
      "Olympic racks, cardio zone, functional strength rigs"
    );
  });

  it("submits updated values and triggers onUpdated callback", async () => {
    const updatedFacility: Facility = {
      ...mockFacility,
      name: "Main Gymnasium",
      status: "maintenance",
    };

    vi.mocked(api.updateFacility).mockResolvedValue(updatedFacility);

    render(
      <EditFacilityModal
        isOpen={true}
        facility={mockFacility}
        onClose={onCloseMock}
        onUpdated={onUpdatedMock}
      />
    );

    fireEvent.change(screen.getByTestId("edit-facility-name-input"), {
      target: { value: "Main Gymnasium" },
    });
    fireEvent.click(screen.getByTestId("edit-status-opt-maintenance"));

    fireEvent.click(screen.getByTestId("edit-facility-submit-btn"));

    await waitFor(() => {
      expect(api.updateFacility).toHaveBeenCalledWith("gymnasium", {
        name: "Main Gymnasium",
        status: "maintenance",
        statusUntil: null,
        statusNote: null,
        reservedFor: null,
        description: "Olympic racks, cardio zone, functional strength rigs",
      });
      expect(onUpdatedMock).toHaveBeenCalledWith(updatedFacility);
      expect(onCloseMock).toHaveBeenCalled();
    });
  });

  it("submits updateFacility with 'reserved' status, reservedFor, and statusNote", async () => {
    const reservedFacility: Facility = {
      ...mockFacility,
      status: "reserved",
      reservedFor: "GIKI Sports Gala",
      statusNote: "Open for spectators",
    };

    vi.mocked(api.updateFacility).mockResolvedValue(reservedFacility);

    render(
      <EditFacilityModal
        isOpen={true}
        facility={mockFacility}
        onClose={onCloseMock}
        onUpdated={onUpdatedMock}
      />
    );

    fireEvent.click(screen.getByTestId("edit-status-opt-reserved"));

    expect(screen.getByTestId("edit-facility-reserved-for-input")).toBeInTheDocument();
    fireEvent.change(screen.getByTestId("edit-facility-reserved-for-input"), {
      target: { value: "GIKI Sports Gala" },
    });
    fireEvent.change(screen.getByTestId("edit-facility-status-note-input"), {
      target: { value: "Open for spectators" },
    });

    fireEvent.click(screen.getByTestId("edit-facility-submit-btn"));

    await waitFor(() => {
      expect(api.updateFacility).toHaveBeenCalledWith("gymnasium", {
        name: "Gymnasium",
        status: "reserved",
        statusUntil: null,
        statusNote: "Open for spectators",
        reservedFor: "GIKI Sports Gala",
        description: "Olympic racks, cardio zone, functional strength rigs",
      });
      expect(onUpdatedMock).toHaveBeenCalledWith(reservedFacility);
      expect(onCloseMock).toHaveBeenCalled();
    });
  });
});

describe("FacilitiesGrid RBAC for Add Facility", () => {
  const dummyFacilities: Facility[] = [
    {
      id: "fac-1",
      slug: "gymnasium",
      name: "Gymnasium",
      description: "Campus gym",
      status: "active",
      schedules: [],
    },
  ];

  const createMockAuthContext = (roles: string[] | null): AuthContextType => ({
    user: roles
      ? {
          id: "usr-1",
          email: "user@example.test",
          displayName: "Test User",
          roles,
        }
      : null,
    isLoading: false,
    checkError: null,
    logoutError: null,
    login: vi.fn(),
    logout: vi.fn(),
    retryCheckSession: vi.fn(),
    clearLogoutError: vi.fn(),
  });

  it("renders '+ Add Facility' button for sports_director", () => {
    render(
      <AuthContext.Provider value={createMockAuthContext(["sports_director"])}>
        <MemoryRouter>
          <FacilitiesGrid
            facilities={dummyFacilities}
            isLoading={false}
            error={null}
            onRetry={vi.fn()}
          />
        </MemoryRouter>
      </AuthContext.Provider>
    );

    expect(screen.getByTestId("add-facility-btn")).toBeInTheDocument();
  });

  it("renders '+ Add Facility' button for super_admin", () => {
    render(
      <AuthContext.Provider value={createMockAuthContext(["super_admin"])}>
        <MemoryRouter>
          <FacilitiesGrid
            facilities={dummyFacilities}
            isLoading={false}
            error={null}
            onRetry={vi.fn()}
          />
        </MemoryRouter>
      </AuthContext.Provider>
    );

    expect(screen.getByTestId("add-facility-btn")).toBeInTheDocument();
  });

  it("does NOT render '+ Add Facility' button for student", () => {
    render(
      <AuthContext.Provider value={createMockAuthContext(["student"])}>
        <MemoryRouter>
          <FacilitiesGrid
            facilities={dummyFacilities}
            isLoading={false}
            error={null}
            onRetry={vi.fn()}
          />
        </MemoryRouter>
      </AuthContext.Provider>
    );

    expect(screen.queryByTestId("add-facility-btn")).not.toBeInTheDocument();
  });

  it("does NOT render '+ Add Facility' button for unauthenticated visitor", () => {
    render(
      <AuthContext.Provider value={createMockAuthContext(null)}>
        <MemoryRouter>
          <FacilitiesGrid
            facilities={dummyFacilities}
            isLoading={false}
            error={null}
            onRetry={vi.fn()}
          />
        </MemoryRouter>
      </AuthContext.Provider>
    );

    expect(screen.queryByTestId("add-facility-btn")).not.toBeInTheDocument();
  });
});
