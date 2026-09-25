import { Facility, Booking, Achievement, Announcement, Membership, ComplexStats } from './types';

const API_BASE = '/api';

export const api = {
  async getHealth() {
    const res = await fetch(`${API_BASE}/health`);
    if (!res.ok) throw new Error('Health check failed');
    return res.json();
  },

  async getFacilities(): Promise<Facility[]> {
    const res = await fetch(`${API_BASE}/facilities`);
    if (!res.ok) throw new Error('Failed to fetch facilities');
    return res.json();
  },

  async getBookings(date?: string, facilityId?: string): Promise<Booking[]> {
    const params = new URLSearchParams();
    if (date) params.append('date', date);
    if (facilityId) params.append('facilityId', facilityId);
    
    const res = await fetch(`${API_BASE}/bookings?${params.toString()}`);
    if (!res.ok) throw new Error('Failed to fetch bookings');
    return res.json();
  },

  async createBooking(data: {
    facilityId: string;
    date: string;
    timeSlot: string;
    bookerType: string;
    bookerName: string;
    bookerId: string;
    bookerEmail: string;
    bookerPhone: string;
    equipmentRentals: string[];
  }): Promise<Booking> {
    const res = await fetch(`${API_BASE}/bookings`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    const json = await res.json();
    if (!res.ok) {
      throw new Error(json.error || 'Failed to reserve court slot');
    }
    return json;
  },

  async cancelBooking(id: string): Promise<{ success: boolean; message?: string }> {
    const res = await fetch(`${API_BASE}/bookings/${id}`, {
      method: 'DELETE',
    });
    if (!res.ok) throw new Error('Failed to cancel booking');
    return res.json();
  },

  async getAchievements(): Promise<Achievement[]> {
    const res = await fetch(`${API_BASE}/achievements`);
    if (!res.ok) throw new Error('Failed to fetch achievements');
    return res.json();
  },

  async createAchievement(data: Partial<Achievement>): Promise<Achievement> {
    const res = await fetch(`${API_BASE}/achievements`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    const json = await res.json();
    if (!res.ok) throw new Error(json.error || 'Failed to publish achievement');
    return json;
  },

  async deleteAchievement(id: string): Promise<void> {
    const res = await fetch(`${API_BASE}/achievements/${id}`, {
      method: 'DELETE',
    });
    if (!res.ok) throw new Error('Failed to delete achievement');
  },

  async getAnnouncements(): Promise<Announcement[]> {
    const res = await fetch(`${API_BASE}/announcements`);
    if (!res.ok) throw new Error('Failed to fetch announcements');
    return res.json();
  },

  async createAnnouncement(data: Partial<Announcement>): Promise<Announcement> {
    const res = await fetch(`${API_BASE}/announcements`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    const json = await res.json();
    if (!res.ok) throw new Error(json.error || 'Failed to post announcement');
    return json;
  },

  async getMemberships(): Promise<Membership[]> {
    const res = await fetch(`${API_BASE}/memberships`);
    if (!res.ok) throw new Error('Failed to fetch memberships');
    return res.json();
  },

  async createMembership(data: {
    applicantName: string;
    applicantId: string;
    applicantType: string;
    plan: string;
  }): Promise<Membership> {
    const res = await fetch(`${API_BASE}/memberships`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    const json = await res.json();
    if (!res.ok) throw new Error(json.error || 'Failed to register membership pass');
    return json;
  },

  async getStats(): Promise<ComplexStats> {
    const res = await fetch(`${API_BASE}/stats`);
    if (!res.ok) throw new Error('Failed to fetch stats');
    return res.json();
  },
};
