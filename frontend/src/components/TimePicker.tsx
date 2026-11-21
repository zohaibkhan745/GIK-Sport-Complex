import { useState, useRef, useEffect, useMemo, type ChangeEvent } from "react";
import {
  Clock,
  ChevronDown,
  Check,
  Sunrise,
  Sun,
  Sunset,
  Moon,
  Sparkles,
} from "lucide-react";
import { formatClockTime } from "../types";

export interface TimePickerProps {
  id: string;
  ariaLabel: string;
  value: string; // ISO time string: "HH:MM:SS" (or "24:00:00")
  onChange: (value: string) => void;
  isEndTime?: boolean;
  disabled?: boolean;
  placeholder?: string;
  midnightButtonTestId?: string;
}

interface TimeSlotOption {
  value: string; // "09:00:00"
  label: string; // "9:00 AM"
  category: "morning" | "afternoon" | "evening" | "night";
}

/**
 * Generate 30-minute intervals covering campus operating hours (06:00 to 24:00).
 */
function generateTimeSlots(isEndTime: boolean): TimeSlotOption[] {
  const slots: TimeSlotOption[] = [];

  // From 06:00 (6 AM) to 23:30 (11:30 PM)
  for (let h = 6; h < 24; h++) {
    for (const m of [0, 30]) {
      const hh = String(h).padStart(2, "0");
      const mm = String(m).padStart(2, "0");
      const timeVal = `${hh}:${mm}:00`;
      const label = formatClockTime(timeVal, false);

      let category: "morning" | "afternoon" | "evening" | "night" = "morning";
      if (h >= 12 && h < 17) category = "afternoon";
      else if (h >= 17 && h < 21) category = "evening";
      else if (h >= 21) category = "night";

      slots.push({ value: timeVal, label, category });
    }
  }

  // If this is an end time, midnight (24:00:00) is the canonical end-of-day option
  if (isEndTime) {
    slots.push({
      value: "24:00:00",
      label: "12:00 AM (Midnight)",
      category: "night",
    });
  }

  return slots;
}

export function TimePicker({
  id,
  ariaLabel,
  value,
  onChange,
  isEndTime = false,
  disabled = false,
  placeholder = "09:00:00",
  midnightButtonTestId,
}: TimePickerProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<"slots" | "custom">("slots");
  const containerRef = useRef<HTMLDivElement>(null);
  const slotListRef = useRef<HTMLDivElement>(null);

  const slots = useMemo(() => generateTimeSlots(isEndTime), [isEndTime]);

  // Derived 12-hour components for custom dial
  const { customHour, customMinute, customPeriod } = useMemo(() => {
    if (value === "24:00:00") {
      return { customHour: 12, customMinute: 0, customPeriod: "AM" as const };
    }
    const parts = value.split(":").map(Number);
    if (parts.length >= 2 && !isNaN(parts[0]) && !isNaN(parts[1])) {
      let h = parts[0];
      const m = parts[1];
      const period = h >= 12 ? ("PM" as const) : ("AM" as const);
      h = h % 12;
      if (h === 0) h = 12;
      return { customHour: h, customMinute: m, customPeriod: period };
    }
    return { customHour: 9, customMinute: 0, customPeriod: "AM" as const };
  }, [value]);

  // Close dropdown on click outside or Escape
  useEffect(() => {
    if (!isOpen) return;

    const handlePointerDown = (e: MouseEvent | TouchEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setIsOpen(false);
      }
    };

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [isOpen]);

  // Auto-scroll to selected slot when popover opens
  useEffect(() => {
    if (isOpen && slotListRef.current) {
      const activeEl = slotListRef.current.querySelector('[data-selected="true"]');
      if (activeEl && typeof activeEl.scrollIntoView === "function") {
        activeEl.scrollIntoView({ block: "center", behavior: "smooth" });
      }
    }
  }, [isOpen]);

  const handleSlotSelect = (val: string) => {
    onChange(val);
    setIsOpen(false);
  };

  const handleApplyCustom = (hour: number, minute: number, period: "AM" | "PM") => {
    let h = hour % 12;
    if (period === "PM") h += 12;
    const hh = String(h).padStart(2, "0");
    const mm = String(minute).padStart(2, "0");
    onChange(`${hh}:${mm}:00`);
    setIsOpen(false);
  };

  const formattedDisplay = value ? formatClockTime(value, isEndTime) : "";

  // Common quick presets
  const presets = isEndTime
    ? [
        { label: "11:00 AM", value: "11:00:00" },
        { label: "1:00 PM", value: "13:00:00" },
        { label: "5:00 PM", value: "17:00:00" },
        { label: "8:00 PM", value: "20:00:00" },
        { label: "10:00 PM", value: "22:00:00" },
        { label: "Midnight", value: "24:00:00" },
      ]
    : [
        { label: "6:00 AM", value: "06:00:00" },
        { label: "8:00 AM", value: "08:00:00" },
        { label: "9:00 AM", value: "09:00:00" },
        { label: "2:00 PM", value: "14:00:00" },
        { label: "4:00 PM", value: "16:00:00" },
        { label: "6:00 PM", value: "18:00:00" },
      ];

  return (
    <div ref={containerRef} className="relative w-full">
      {/* Visual Trigger Card */}
      <div
        className={`group flex items-center justify-between rounded-lg border bg-white px-2.5 py-1.5 shadow-2xs transition-all ${
          isOpen
            ? "border-[var(--color-maroon)] ring-2 ring-[var(--color-maroon)]/15 shadow-sm"
            : "border-[var(--color-border)] hover:border-gray-400"
        } ${disabled ? "opacity-50 pointer-events-none" : "cursor-pointer"}`}
        onClick={() => !disabled && setIsOpen((prev) => !prev)}
      >
        <div className="flex items-center gap-2 min-w-0 flex-1">
          <div className="flex h-6 w-6 items-center justify-center rounded-md bg-[var(--color-maroon)]/10 text-[var(--color-maroon)]">
            <Clock size={13} />
          </div>

          <div className="flex flex-col min-w-0">
            {formattedDisplay ? (
              <span className="text-xs font-bold text-[var(--color-navy)] truncate">
                {formattedDisplay}
              </span>
            ) : (
              <span className="text-xs font-normal text-gray-400">
                {placeholder}
              </span>
            )}
          </div>
        </div>

        {/* Action Controls & Fast Buttons */}
        <div className="flex items-center gap-1.5 flex-shrink-0" onClick={(e) => e.stopPropagation()}>
          {isEndTime && (
            <button
              type="button"
              disabled={disabled}
              onClick={() => onChange("24:00:00")}
              data-testid={midnightButtonTestId}
              aria-label="Set 24:00:00 (Midnight)"
              className="rounded-md border border-[var(--color-maroon)]/30 bg-rose-50/80 px-2 py-0.5 text-[10px] font-bold text-[var(--color-maroon)] hover:bg-[var(--color-maroon)] hover:text-white transition-all cursor-pointer shadow-2xs"
            >
              Set 24:00:00
            </button>
          )}

          <button
            type="button"
            disabled={disabled}
            onClick={() => setIsOpen((prev) => !prev)}
            aria-label={`Toggle ${ariaLabel.toLowerCase()} picker`}
            className="rounded p-1 text-gray-400 hover:text-[var(--color-navy)] hover:bg-gray-100 transition-colors cursor-pointer"
          >
            <ChevronDown
              size={14}
              className={`transition-transform duration-200 ${isOpen ? "rotate-180 text-[var(--color-maroon)]" : ""}`}
            />
          </button>
        </div>

        {/* Real DOM input for accessibility, direct typing, and test compatibility */}
        <input
          id={id}
          aria-label={ariaLabel}
          type="text"
          disabled={disabled}
          value={value}
          onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
          placeholder={placeholder}
          className="sr-only"
          tabIndex={-1}
        />
      </div>

      {/* Big-Tech Popover Modal */}
      {isOpen && (
        <div
          role="dialog"
          aria-label={`${ariaLabel} selector`}
          className="absolute left-0 z-50 mt-1.5 w-80 max-w-[92vw] rounded-2xl border border-gray-200 bg-white/95 p-3.5 shadow-2xl backdrop-blur-md ring-1 ring-black/5 animate-in fade-in zoom-in-95"
        >
          {/* Header & Tabs */}
          <div className="flex items-center justify-between border-b border-gray-100 pb-2.5">
            <div className="flex items-center gap-1 text-xs font-bold text-[var(--color-navy)]">
              <Sparkles size={13} className="text-[var(--color-gold-dark)]" />
              <span>Select {ariaLabel}</span>
            </div>

            <div className="flex items-center rounded-lg bg-gray-100 p-0.5 text-[11px] font-medium">
              <button
                type="button"
                onClick={() => setActiveTab("slots")}
                className={`rounded-md px-2 py-1 transition-all cursor-pointer ${
                  activeTab === "slots"
                    ? "bg-white font-bold text-[var(--color-maroon)] shadow-xs"
                    : "text-gray-500 hover:text-gray-900"
                }`}
              >
                Preset Slots
              </button>
              <button
                type="button"
                onClick={() => setActiveTab("custom")}
                className={`rounded-md px-2 py-1 transition-all cursor-pointer ${
                  activeTab === "custom"
                    ? "bg-white font-bold text-[var(--color-maroon)] shadow-xs"
                    : "text-gray-500 hover:text-gray-900"
                }`}
              >
                Custom Time
              </button>
            </div>
          </div>

          {/* Quick Presets Bar */}
          <div className="mt-2.5">
            <span className="block text-[10px] font-semibold uppercase tracking-wider text-gray-400 mb-1">
              Quick Presets
            </span>
            <div className="flex flex-wrap gap-1">
              {presets.map((preset) => {
                const isSelected = value === preset.value;
                return (
                  <button
                    key={preset.value}
                    type="button"
                    onClick={() => handleSlotSelect(preset.value)}
                    className={`rounded-full px-2.5 py-0.5 text-[11px] font-semibold transition-all cursor-pointer ${
                      isSelected
                        ? "bg-[var(--color-maroon)] text-white shadow-xs scale-105"
                        : "bg-gray-100 text-gray-700 hover:bg-gray-200"
                    }`}
                  >
                    {preset.label}
                  </button>
                );
              })}
            </div>
          </div>

          {/* View Tab 1: Curated 30-min Slots List (Google Calendar / Calendly style) */}
          {activeTab === "slots" ? (
            <div
              ref={slotListRef}
              className="mt-3 max-h-56 overflow-y-auto pr-1 space-y-1 divide-y divide-gray-50 scrollbar-thin scrollbar-thumb-gray-200"
            >
              {slots.map((slot) => {
                const isSelected = value === slot.value;
                return (
                  <button
                    key={slot.value}
                    type="button"
                    data-selected={isSelected ? "true" : undefined}
                    onClick={() => handleSlotSelect(slot.value)}
                    className={`flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-xs transition-colors cursor-pointer ${
                      isSelected
                        ? "bg-[var(--color-maroon)] text-white font-bold shadow-xs"
                        : "text-[var(--color-navy)] hover:bg-gray-100/80 font-medium"
                    }`}
                  >
                    <div className="flex items-center gap-2">
                      {slot.category === "morning" && (
                        <Sunrise
                          size={13}
                          className={isSelected ? "text-amber-200" : "text-amber-500"}
                        />
                      )}
                      {slot.category === "afternoon" && (
                        <Sun
                          size={13}
                          className={isSelected ? "text-amber-200" : "text-amber-600"}
                        />
                      )}
                      {slot.category === "evening" && (
                        <Sunset
                          size={13}
                          className={isSelected ? "text-orange-200" : "text-orange-500"}
                        />
                      )}
                      {slot.category === "night" && (
                        <Moon
                          size={13}
                          className={isSelected ? "text-indigo-200" : "text-indigo-500"}
                        />
                      )}
                      <span>{slot.label}</span>
                    </div>

                    <div className="flex items-center gap-1.5">
                      <span
                        className={`font-mono text-[10px] ${
                          isSelected ? "text-white/80" : "text-gray-400"
                        }`}
                      >
                        {slot.value}
                      </span>
                      {isSelected && <Check size={13} className="text-white" />}
                    </div>
                  </button>
                );
              })}
            </div>
          ) : (
            /* View Tab 2: Custom Stepper Dial (Apple / iOS 12-Hour Style) */
            <div className="mt-3 p-2 bg-gray-50/70 rounded-xl border border-gray-100">
              <div className="flex items-center justify-center gap-2 py-2">
                {/* Hours Selector */}
                <div className="flex flex-col items-center">
                  <span className="text-[10px] font-semibold text-gray-400 uppercase mb-1">
                    Hour
                  </span>
                  <select
                    value={customHour}
                    onChange={(e) =>
                      handleApplyCustom(Number(e.target.value), customMinute, customPeriod)
                    }
                    className="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm font-bold text-[var(--color-navy)] shadow-2xs focus:border-[var(--color-maroon)] focus:outline-none cursor-pointer"
                  >
                    {Array.from({ length: 12 }, (_, i) => i + 1).map((h) => (
                      <option key={h} value={h}>
                        {h}
                      </option>
                    ))}
                  </select>
                </div>

                <span className="text-xl font-bold text-gray-400 mt-3">:</span>

                {/* Minutes Selector */}
                <div className="flex flex-col items-center">
                  <span className="text-[10px] font-semibold text-gray-400 uppercase mb-1">
                    Min
                  </span>
                  <select
                    value={customMinute}
                    onChange={(e) =>
                      handleApplyCustom(customHour, Number(e.target.value), customPeriod)
                    }
                    className="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm font-bold text-[var(--color-navy)] shadow-2xs focus:border-[var(--color-maroon)] focus:outline-none cursor-pointer"
                  >
                    {[0, 15, 30, 45].map((m) => (
                      <option key={m} value={m}>
                        {String(m).padStart(2, "0")}
                      </option>
                    ))}
                  </select>
                </div>

                {/* AM / PM Segmented Toggle */}
                <div className="flex flex-col items-center ml-2">
                  <span className="text-[10px] font-semibold text-gray-400 uppercase mb-1">
                    Period
                  </span>
                  <div className="flex rounded-lg border border-gray-300 bg-white p-0.5 shadow-2xs">
                    <button
                      type="button"
                      onClick={() => handleApplyCustom(customHour, customMinute, "AM")}
                      className={`rounded-md px-2.5 py-1 text-xs font-bold transition-all cursor-pointer ${
                        customPeriod === "AM" && value !== "24:00:00"
                          ? "bg-[var(--color-maroon)] text-white shadow-xs"
                          : "text-gray-600 hover:text-gray-900"
                      }`}
                    >
                      AM
                    </button>
                    <button
                      type="button"
                      onClick={() => handleApplyCustom(customHour, customMinute, "PM")}
                      className={`rounded-md px-2.5 py-1 text-xs font-bold transition-all cursor-pointer ${
                        customPeriod === "PM"
                          ? "bg-[var(--color-maroon)] text-white shadow-xs"
                          : "text-gray-600 hover:text-gray-900"
                      }`}
                    >
                      PM
                    </button>
                  </div>
                </div>
              </div>

              {/* Direct manual input option */}
              <div className="mt-2 pt-2 border-t border-gray-200/80 flex items-center justify-between text-[11px]">
                <span className="text-gray-500">Selected value:</span>
                <span className="font-mono font-bold text-[var(--color-maroon)]">
                  {value || "--:--:--"} ({formattedDisplay})
                </span>
              </div>
            </div>
          )}

          {/* Footer close button */}
          <div className="mt-3 flex items-center justify-end border-t border-gray-100 pt-2">
            <button
              type="button"
              onClick={() => setIsOpen(false)}
              className="rounded-md bg-gray-100 px-3 py-1 text-xs font-semibold text-gray-700 hover:bg-gray-200 transition-colors cursor-pointer"
            >
              Done
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
