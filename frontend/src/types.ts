export interface Facility {
  id: string;
  name: string;
  category: 'Racquet' | 'Indoor' | 'Outdoor' | 'Fitness' | 'Aquatics';
  description: string;
  location: string;
  maxPlayers: number;
  studentTimings: string;
  facultyTimings: string;
  slots: string[];
  rules: string[];
  equipmentAvailable: string[];
  image: string;
  badge: string;
  isMembershipRequired: boolean;
}

export interface Booking {
  id: string;
  facilityId: string;
  facilityName: string;
  date: string; // YYYY-MM-DD
  timeSlot: string;
  bookerType: 'student' | 'faculty';
  bookerName: string;
  bookerId: string;
  bookerEmail: string;
  bookerPhone: string;
  equipmentRentals: string[];
  status: 'confirmed' | 'cancelled' | 'completed';
  checkInCode: string;
  createdAt: string;
}

export interface Achievement {
  id: string;
  title: string;
  sport: string;
  date: string;
  category: string;
  description: string;
  teamMembers: string;
  image: string;
  directorNote: string;
  featured: boolean;
  createdAt: string;
}

export interface Announcement {
  id: string;
  title: string;
  content: string;
  date: string;
  priority: 'urgent' | 'info' | 'maintenance';
  createdAt: string;
}

export interface Membership {
  id: string;
  applicantName: string;
  applicantId: string;
  applicantType: string;
  plan: string;
  status: string;
  validUntil: string;
  createdAt: string;
}

export interface ComplexStats {
  totalFacilities: number;
  activeBookings: number;
  todayBookings: number;
  achievementsCount: number;
  membershipsCount: number;
  systemStatus: string;
}
