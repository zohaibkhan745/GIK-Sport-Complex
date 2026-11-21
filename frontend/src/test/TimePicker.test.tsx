import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { TimePicker } from "../components/TimePicker";

describe("TimePicker Component", () => {
  it("renders 12-hour AM/PM formatted label for a standard morning time", () => {
    const handleChange = vi.fn();
    render(
      <TimePicker
        id="test-start"
        ariaLabel="Start time"
        value="09:00:00"
        onChange={handleChange}
      />
    );

    expect(screen.getByText("9:00 AM")).toBeInTheDocument();
    const input = screen.getByLabelText("Start time") as HTMLInputElement;
    expect(input.value).toBe("09:00:00");
  });

  it("renders 12-hour AM/PM formatted label for afternoon and evening times", () => {
    const handleChange = vi.fn();
    const { rerender } = render(
      <TimePicker
        id="test-start"
        ariaLabel="Start time"
        value="16:30:00"
        onChange={handleChange}
      />
    );

    expect(screen.getByText("4:30 PM")).toBeInTheDocument();

    rerender(
      <TimePicker
        id="test-start"
        ariaLabel="Start time"
        value="20:00:00"
        onChange={handleChange}
      />
    );
    expect(screen.getByText("8:00 PM")).toBeInTheDocument();
  });

  it("renders Midnight label when value is 24:00:00 and isEndTime is true", () => {
    const handleChange = vi.fn();
    render(
      <TimePicker
        id="test-end"
        ariaLabel="End time"
        value="24:00:00"
        onChange={handleChange}
        isEndTime={true}
      />
    );

    expect(screen.getByText("12:00 AM (Midnight)")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /set 24:00:00/i })).toBeInTheDocument();
  });

  it("clicking the 'Set 24:00:00' shortcut button calls onChange with '24:00:00'", () => {
    const handleChange = vi.fn();
    render(
      <TimePicker
        id="test-end"
        ariaLabel="End time"
        value="22:00:00"
        onChange={handleChange}
        isEndTime={true}
        midnightButtonTestId="add-set-midnight-btn"
      />
    );

    const midnightBtn = screen.getByTestId("add-set-midnight-btn");
    expect(midnightBtn).toBeInTheDocument();
    fireEvent.click(midnightBtn);

    expect(handleChange).toHaveBeenCalledWith("24:00:00");
  });

  it("opens popover on click and allows selecting a curated 30-min slot", () => {
    const handleChange = vi.fn();
    render(
      <TimePicker
        id="test-start"
        ariaLabel="Start time"
        value="09:00:00"
        onChange={handleChange}
      />
    );

    // Click trigger to open popover
    fireEvent.click(screen.getByText("9:00 AM"));

    expect(screen.getByRole("dialog", { name: /Start time selector/i })).toBeInTheDocument();
    expect(screen.getByText("Quick Presets")).toBeInTheDocument();

    // Select 4:00 PM from the curated slot list
    const fourPmSlot = screen.getByRole("button", { name: /4:00 PM 16:00:00/i });
    fireEvent.click(fourPmSlot);

    expect(handleChange).toHaveBeenCalledWith("16:00:00");
  });

  it("allows selecting quick preset pills", () => {
    const handleChange = vi.fn();
    render(
      <TimePicker
        id="test-start"
        ariaLabel="Start time"
        value="09:00:00"
        onChange={handleChange}
      />
    );

    fireEvent.click(screen.getByText("9:00 AM"));

    // Click 2:00 PM preset
    const presetBtn = screen.getByRole("button", { name: "2:00 PM" });
    fireEvent.click(presetBtn);

    expect(handleChange).toHaveBeenCalledWith("14:00:00");
  });

  it("supports custom time stepper tab", () => {
    const handleChange = vi.fn();
    render(
      <TimePicker
        id="test-start"
        ariaLabel="Start time"
        value="09:00:00"
        onChange={handleChange}
      />
    );

    fireEvent.click(screen.getByText("9:00 AM"));

    // Switch to Custom Time tab
    fireEvent.click(screen.getByRole("button", { name: "Custom Time" }));

    expect(screen.getByText("Hour")).toBeInTheDocument();
    expect(screen.getByText("Min")).toBeInTheDocument();
    expect(screen.getByText("Period")).toBeInTheDocument();

    // Toggle PM
    fireEvent.click(screen.getByRole("button", { name: "PM" }));
    expect(handleChange).toHaveBeenCalledWith("21:00:00");
  });

  it("supports direct typing into the underlying input for full test & keyboard compatibility", () => {
    const handleChange = vi.fn();
    render(
      <TimePicker
        id="test-start"
        ariaLabel="Start time"
        value="09:00:00"
        onChange={handleChange}
      />
    );

    const input = screen.getByLabelText("Start time");
    fireEvent.change(input, { target: { value: "10:30:00" } });

    expect(handleChange).toHaveBeenCalledWith("10:30:00");
  });
});
