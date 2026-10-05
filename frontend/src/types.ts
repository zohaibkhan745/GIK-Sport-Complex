/** One row in a facility's weekly schedule table. */
export interface ScheduleEntry {
  days: string; // e.g. "Mon / Wed / Fri"
  activity: string; // e.g. "Open swim, 6-8am"
}

/** The coach or supervisor assigned to a facility, shown as a small card. */
export interface Coach {
  name: string;
  title: string;
  initials: string; // used for the avatar badge
}

/** A quick fact shown in the meta strip at the top of a facility page. */
export interface FacilityMeta {
  icon: "clock" | "users" | "thermometer" | "shield";
  label: string;
}

/** The name of a lucide-react icon used to represent a facility. */
export type FacilityIconName =
  | "grid"
  | "feather"
  | "target"
  | "circle"
  | "goal"
  | "disc"
  | "dumbbell"
  | "waves";

export interface Facility {
  slug: string;
  name: string;
  shortName: string; // used on the card / nav where space is tight
  tagline: string; // one line, shown under the name
  description: string; // longer sentence, shown on the detail page
  icon: FacilityIconName;
  meta: FacilityMeta[];
  schedule: ScheduleEntry[];
  coach?: Coach;
  galleryCount: number; // number of placeholder gallery tiles to render
}

export interface AchievementPost {
  id: string;
  author: string;
  initials: string;
  caption: string;
  timeAgo: string;
  likes: number;
  comments: number;
  imageUrl?: string; // populated once a real photo is attached/uploaded
}

export interface Stat {
  value: string;
  label: string;
}

/** Payload sent to the backend when a new achievement post is created. */
export interface CreateAchievementPostInput {
  caption: string;
  photo?: File;
}

/** A single bookable slot for a facility, as returned by the backend. */
export interface Slot {
  id: string;
  facilitySlug: string;
  startTime: string; // ISO timestamp
  endTime: string; // ISO timestamp
  isAvailable: boolean;
}

export interface CreateBookingInput {
  slotId: string;
  name: string;
  email: string;
}

export interface Booking {
  id: string;
  slotId: string;
  name: string;
  email: string;
  createdAt: string;
}
