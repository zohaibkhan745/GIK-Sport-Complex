import {
  Grid2x2,
  Feather,
  Target,
  CircleDot,
  Goal,
  Disc3,
  Dumbbell,
  Waves,
  Clock,
  Users,
  Thermometer,
  ShieldCheck,
  type LucideIcon,
} from "lucide-react";
import type { FacilityIconName } from "../types";

export const FACILITY_ICONS: Record<FacilityIconName, LucideIcon> = {
  grid: Grid2x2,
  feather: Feather,
  target: Target,
  circle: CircleDot,
  goal: Goal,
  disc: Disc3,
  dumbbell: Dumbbell,
  waves: Waves,
};

export const META_ICONS: Record<string, LucideIcon> = {
  clock: Clock,
  users: Users,
  thermometer: Thermometer,
  shield: ShieldCheck,
};
