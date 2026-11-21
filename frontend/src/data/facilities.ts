import type { LegacyFacility } from "../types";

export const FACILITIES: LegacyFacility[] = [
  {
    slug: "glass-padel-courts",
    name: "Padel Court",
    shortName: "Padel",
    tagline: "Court 1 & 2 — panoramic tempered glass, synthetic turf",
    description:
      "Panoramic tempered glass, synthetic turf, and 800 Lux LED tournament floodlights across two championship-grade courts.",
    icon: "grid",
    meta: [
      { icon: "clock", label: "Open 6am - 10pm" },
      { icon: "users", label: "2 courts" },
      { icon: "shield", label: "800 Lux floodlights" },
    ],
    schedule: [
      { days: "Mon / Wed / Fri", activity: "Open play, 6-9am" },
      { days: "Tue / Thu", activity: "Coached clinics, 4-6pm" },
      { days: "Saturday", activity: "Inter-batch padel league" },
    ],
    coach: { name: "Coach Imran Sheikh", title: "Padel head coach", initials: "IS" },
    galleryCount: 3,
  },
  {
    slug: "badminton-championship-hall",
    name: "Badminton Courts",
    shortName: "Badminton",
    tagline: "4 BWF-certified teakwood sprung courts, climate cooled",
    description:
      "Four BWF-certified teakwood sprung courts with climate cooling, built to host championship-standard play year-round.",
    icon: "feather",
    meta: [
      { icon: "clock", label: "Open 6am - 9pm" },
      { icon: "users", label: "4 courts" },
      { icon: "thermometer", label: "Climate cooled" },
    ],
    schedule: [
      { days: "Mon - Fri", activity: "Open court booking, 6-8am" },
      { days: "Tue / Thu", activity: "Coached training, 5-7pm" },
      { days: "Sunday", activity: "Championship fixtures" },
    ],
    coach: { name: "Coach Ayesha Farooq", title: "Badminton head coach", initials: "AF" },
    galleryCount: 3,
  },
  {
    slug: "glass-back-squash-complex",
    name: "Squash",
    shortName: "Squash",
    tagline: "Courts 1-3 — Junckers beech flooring, electronic scoring",
    description:
      "Three glass-back courts with Junckers beech flooring and electronic score monitors for tournament-grade matches.",
    icon: "target",
    meta: [
      { icon: "clock", label: "Open 6am - 10pm" },
      { icon: "users", label: "3 courts" },
      { icon: "shield", label: "Electronic scoring" },
    ],
    schedule: [
      { days: "Mon / Wed / Fri", activity: "Open play, 7-9am" },
      { days: "Tue / Thu", activity: "Coached sessions, 4-6pm" },
      { days: "Saturday", activity: "Ladder tournament" },
    ],
    coach: { name: "Coach Bilal Raza", title: "Squash head coach", initials: "BR" },
    galleryCount: 3,
  },
  {
    slug: "hard-tennis-courts",
    name: "Tennis Courts",
    shortName: "Tennis",
    tagline: "Courts 1 & 2 — US Open-grade acrylic cushioned hard courts",
    description:
      "Two US Open-grade acrylic cushioned hard courts, resurfaced for consistent bounce and year-round play.",
    icon: "circle",
    meta: [
      { icon: "clock", label: "Open 6am - 9pm" },
      { icon: "users", label: "2 courts" },
      { icon: "shield", label: "Acrylic cushioned surface" },
    ],
    schedule: [
      { days: "Mon / Wed / Fri", activity: "Open play, 6-8am" },
      { days: "Tue / Thu", activity: "Coached clinics, 5-7pm" },
      { days: "Saturday", activity: "Singles ladder" },
    ],
    coach: { name: "Coach Hassan Tariq", title: "Tennis head coach", initials: "HT" },
    galleryCount: 3,
  },
  {
    slug: "floodlit-futsal-court",
    name: "Outdoor BasketBall Court",
    shortName: "BasketBall",
    tagline: "Regulation outdoor basketball court with tournament floodlighting",
    description:
      "Floodlit outdoor basketball court with regulation hoops, rebound backboards, and high-traction sports surfacing.",
    icon: "goal",
    meta: [
      { icon: "clock", label: "Open 4pm - 11pm" },
      { icon: "users", label: "Full-court capacity" },
      { icon: "shield", label: "Floodlit sports surface" },
    ],
    schedule: [
      { days: "Mon - Fri", activity: "Open booking, 4-7pm" },
      { days: "Tue / Thu", activity: "Inter-hostel league, 7-9pm" },
      { days: "Saturday", activity: "Weekend knockout cup" },
    ],
    coach: { name: "Coach Fahad Malik", title: "Basketball coordinator", initials: "FM" },
    galleryCount: 3,
  },
  {
    slug: "table-tennis-hall",
    name: "Table Tennis",
    shortName: "Table Tennis",
    tagline: "6 ITTF-approved Butterfly tournament tables",
    description:
      "Six ITTF-approved Butterfly tournament tables in a dedicated hall, open for casual play and club training alike.",
    icon: "disc",
    meta: [
      { icon: "clock", label: "Open 8am - 10pm" },
      { icon: "users", label: "6 tables" },
      { icon: "shield", label: "ITTF-approved tables" },
    ],
    schedule: [
      { days: "Mon - Fri", activity: "Open play, all day" },
      { days: "Wednesday", activity: "Club training, 6-8pm" },
      { days: "Sunday", activity: "Round-robin tournament" },
    ],
    galleryCount: 3,
  },
  {
    slug: "central-fitness-gym",
    name: "Gym",
    shortName: "Gym",
    tagline: "Olympic racks, cardio zone, functional rigs",
    description:
      "A full strength-and-conditioning floor with Olympic racks, a dedicated cardio zone, and functional training rigs.",
    icon: "dumbbell",
    meta: [
      { icon: "clock", label: "Open 5am - 11pm" },
      { icon: "users", label: "80 capacity" },
      { icon: "shield", label: "Trainer on floor" },
    ],
    schedule: [
      { days: "Mon - Sat", activity: "Open gym, 5am-11pm" },
      { days: "Tue / Thu", activity: "Group strength class, 6pm" },
      { days: "Sunday", activity: "Beginner orientation" },
    ],
    coach: { name: "Coach Zara Iqbal", title: "Head strength coach", initials: "ZI" },
    galleryCount: 3,
  },
  {
    slug: "semi-olympic-swimming-pool",
    name: "Swimming Pool",
    shortName: "Swimming Pool",
    tagline: "25m, 6-lane, temperature-controlled, ozone filtration",
    description:
      "A 25-meter, 6-lane temperature-controlled pool with ozone filtration, used for open swim, training, and galas.",
    icon: "waves",
    meta: [
      { icon: "clock", label: "Open 6am - 9pm" },
      { icon: "users", label: "40 capacity" },
      { icon: "thermometer", label: "Temperature controlled" },
    ],
    schedule: [
      { days: "Mon / Wed / Fri", activity: "Open swim, 6-8am" },
      { days: "Tue / Thu", activity: "Coached training, 4-6pm" },
      { days: "Saturday", activity: "Inter-batch gala" },
    ],
    coach: { name: "Coach Ahmed Khalid", title: "National-level swim coach, 12 years", initials: "AK" },
    galleryCount: 3,
  },
  {
    slug: "snooker",
    name: "Snooker",
    shortName: "Snooker",
    tagline: "Championship 12ft Riley snooker tables with Strachan cloth",
    description:
      "Championship-standard Riley full-size 12ft snooker tables with West of England Strachan cloth, tournament lighting, and cues available for students, faculty, and staff.",
    icon: "target",
    meta: [
      { icon: "clock", label: "Open 10am - 10pm" },
      { icon: "users", label: "3 tables" },
      { icon: "shield", label: "Tournament lighting" },
    ],
    schedule: [
      { days: "Mon - Fri", activity: "Student sessions, 10am - 5pm" },
      { days: "Mon - Fri", activity: "Faculty & Staff, 5pm - 8pm" },
      { days: "Sat - Sun", activity: "Open play, 10am - 10pm" },
    ],
    galleryCount: 2,
  },
  {
    slug: "indoor-basketball-court",
    name: "Indoor BasketBall Court",
    shortName: "Basketball",
    tagline: "Full-size hardwood indoor basketball court with sprung maple flooring",
    description:
      "Full-size hardwood indoor basketball court equipped with competition glass backboards, sprung maple flooring, electronic scoreboard, and spectator stands.",
    icon: "goal",
    meta: [
      { icon: "clock", label: "Open 6am - 9pm" },
      { icon: "users", label: "Full-court capacity" },
      { icon: "shield", label: "Hardwood sprung court" },
    ],
    schedule: [
      { days: "Mon / Wed / Fri", activity: "Student training, 6-11am & 4-8pm" },
      { days: "Tue / Thu", activity: "Women-only session, 4-6pm" },
      { days: "Sat - Sun", activity: "Open weekend play, 8am - 8pm" },
    ],
    galleryCount: 3,
  },
  {
    slug: "indoor-volleyball-court",
    name: "Indoor VolleyBall Court",
    shortName: "Volleyball",
    tagline: "Championship Taraflex sports flooring with spectator gallery",
    description:
      "Championship-standard indoor volleyball court with competition Taraflex sports flooring, adjustable net antennas, and spectator gallery.",
    icon: "circle",
    meta: [
      { icon: "clock", label: "Open 7am - 9pm" },
      { icon: "users", label: "Court capacity 12" },
      { icon: "shield", label: "Taraflex sports flooring" },
    ],
    schedule: [
      { days: "Mon / Wed / Fri", activity: "Student training, 7-11am & 4-8pm" },
      { days: "Tue / Thu", activity: "Women-only session, 4-6pm" },
      { days: "Sat - Sun", activity: "General community, 9am - 3pm" },
    ],
    galleryCount: 3,
  },
];

export function getFacilityBySlug(slug: string): LegacyFacility | undefined {
  return FACILITIES.find((f) => f.slug === slug);
}
