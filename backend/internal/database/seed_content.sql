-- Seed realistic announcements and achievements for GIKI Sports Complex
BEGIN;

-- Clear previous announcements and achievements
DELETE FROM giki_sports.content_items;

-- Insert Announcements (optional images supported)
INSERT INTO giki_sports.content_items (id, kind, title, content, image_urls, status, creator_id, created_at, updated_at, published_at)
VALUES
(
    gen_random_uuid(),
    'announcement',
    'Annual GIKI All-Pakistan Sports Gala 2026: Team Trials & Registrations Open',
    'The Sports Society and Directorate of Physical Education announce varsity trials for the upcoming All-Pakistan Sports Gala.

Trials Schedule:
- Padel & Tennis: Monday, Nov 24, 4:00 PM - 7:00 PM (Glass Courts North)
- Badminton & Squash: Tuesday, Nov 25, 5:00 PM - 8:30 PM (Indoor Arena)
- Futsal & Basketball: Wednesday, Nov 26, 6:00 PM - 9:00 PM (Floodlit Multipurpose Court)
- Cricket & Table Tennis: Thursday, Nov 27, 4:00 PM - 7:30 PM (Main Ground & Hall B)

Registered students must present valid GIKI student cards and appropriate non-marking sports footwear. Register via your campus portal.',
    ARRAY['https://images.unsplash.com/photo-1461896836934-ffe607ba8211?auto=format&fit=crop&w=1200&q=80']::text[],
    'published',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now() - INTERVAL '1 day',
    now() - INTERVAL '1 day',
    now() - INTERVAL '1 day'
),
(
    gen_random_uuid(),
    'announcement',
    'Glass Padel Courts: Floodlight Upgrade & Extended Night Bookings',
    'We are pleased to announce the installation of anti-glare 800-lux tournament LED luminaires on Glass Padel Courts #1 and #2.

Operating Hours Update:
- General Student Bookings: 06:00 AM - 11:00 AM & 04:00 PM - 11:30 PM daily
- Faculty / Staff Priority: 05:00 PM - 07:00 PM

Please note: Only approved non-marking synthetic turf shoes are allowed on the turf. Racquet safety wrist leashes are mandatory at all times.',
    '{}'::text[],
    'published',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now() - INTERVAL '3 days',
    now() - INTERVAL '3 days',
    now() - INTERVAL '3 days'
),
(
    gen_random_uuid(),
    'announcement',
    'Indoor Squash Complex: Routine Court #2 Floor Buffing & Maintenance',
    'Please be advised that Squash Court #2 will undergo routine maple floor buffing, line repainting, and wall glass alignment this Saturday between 08:00 AM and 02:00 PM.

Courts #1 and #3 will remain fully operational according to regular slot bookings. We apologize for any temporary inconvenience.',
    '{}'::text[],
    'published',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now() - INTERVAL '5 days',
    now() - INTERVAL '5 days',
    now() - INTERVAL '5 days'
),
(
    gen_random_uuid(),
    'announcement',
    'Revised Women-Only Timings for Gymnasium & Fitness Suite (Fall/Winter 2026)',
    'The Directorate of Physical Education has updated dedicated women-only facility hours to ensure maximum accessibility and privacy:

- Monday, Wednesday, Friday: 02:00 PM - 04:30 PM
- Saturday: 10:00 AM - 01:00 PM

A certified female fitness trainer and student gym attendants will be on duty during all reserved sessions.',
    '{}'::text[],
    'published',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now() - INTERVAL '7 days',
    now() - INTERVAL '7 days',
    now() - INTERVAL '7 days'
),
(
    gen_random_uuid(),
    'announcement',
    'Sports Equipment Desk: New Rackets, Balls & Issue Policy',
    'The central equipment desk has received new stock of Babolat padel racquets, Yonex Aerosensa feather shuttlecocks, and Dunlop Pro double-yellow squash balls.

Equipment check-out is available against student/employee ID card verification. Borrowed equipment must be returned within 15 minutes of slot conclusion.',
    '{}'::text[],
    'published',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now() - INTERVAL '10 days',
    now() - INTERVAL '10 days',
    now() - INTERVAL '10 days'
);

-- Insert Achievements with Realistic Action & Trophy Images (Multiple and Single)
INSERT INTO giki_sports.content_items (id, kind, title, content, image_urls, status, creator_id, created_at, updated_at, published_at)
VALUES
(
    gen_random_uuid(),
    'achievement',
    'GIKI Padel Titans Crowned Champions at HEC All-Pakistan Intervarsity Tournament 2026',
    'The GIKI Varsity Padel squad made history in Islamabad, securing the Gold Trophy at the 2026 Higher Education Commission (HEC) Intervarsity Championship.

In a thrilling 3-set grand finale against IBA Karachi, the duo of Hamza Tariq (Captain, FCSE) and Daniyal Munir (FME) rallied back from match point to clinch victory 6-4, 4-6, 7-5. Congratulations to the entire team and coaching staff!',
    ARRAY[
        'https://images.unsplash.com/photo-1554068865-24cecd4e34b8?auto=format&fit=crop&w=1200&q=80',
        'https://images.unsplash.com/photo-1599474924187-334a4ae5bd3c?auto=format&fit=crop&w=1200&q=80'
    ]::text[],
    'published',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now() - INTERVAL '2 days',
    now() - INTERVAL '2 days',
    now() - INTERVAL '2 days'
),
(
    gen_random_uuid(),
    'achievement',
    'Silver Medal at the 34th All-Pakistan Intervarsity Badminton Championship',
    'GIKI Badminton doubles pair Shahzaib Khan (Batch 32) and Waleed Ahmed (Batch 33) earned Silver at the national university games hosted at Punjab University.

After overcoming LUMS (21-17, 21-14) in the quarterfinals and UET Lahore (21-19, 18-21, 21-15) in the semifinals, our duo fought valiantly in an electric 58-minute final battle against defending champions NUST.',
    ARRAY[
        'https://images.unsplash.com/photo-1626224583764-f87db24ac4ea?auto=format&fit=crop&w=1200&q=80'
    ]::text[],
    'published',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now() - INTERVAL '4 days',
    now() - INTERVAL '4 days',
    now() - INTERVAL '4 days'
),
(
    gen_random_uuid(),
    'achievement',
    'Rector''s Futsal League 2026: Mechanical Hawks Hoist Championship Cup',
    'The 2026 Rector''s Inter-Faculty Futsal Championship concluded under the campus floodlights before an enthusiastic crowd of over 600 students.

The Mechanical Hawks (Faculty of Mechanical Engineering) triumphed over the CS CyberKnights in a sudden-death penalty shootout (4-3) after regulation ended locked at 2-2. Tournament MVP was awarded to goalkeeper Bilal Farooq for 9 crucial saves.',
    ARRAY[
        'https://images.unsplash.com/photo-1574629810360-7efbbe195018?auto=format&fit=crop&w=1200&q=80',
        'https://images.unsplash.com/photo-1508098682722-e99c43a406b2?auto=format&fit=crop&w=1200&q=80'
    ]::text[],
    'published',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now() - INTERVAL '6 days',
    now() - INTERVAL '6 days',
    now() - INTERVAL '6 days'
),
(
    gen_random_uuid(),
    'achievement',
    'KPK Provincial Squash Open: Sophomore Hassan Raza Reaches Semifinals',
    'Sophomore student-athlete Hassan Raza (Electrical Engineering, Batch 33) showcased exceptional grit at the Khyber Pakhtunkhwa Open Squash Championship in Peshawar.

Competing in the Men''s Open category, Hassan defeated two seeded provincial players to advance to the semifinals, earning GIKI''s highest individual squash standing in five years.',
    ARRAY[
        'https://images.unsplash.com/photo-1595435934249-5df7ed86e1c0?auto=format&fit=crop&w=1200&q=80'
    ]::text[],
    'published',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now() - INTERVAL '8 days',
    now() - INTERVAL '8 days',
    now() - INTERVAL '8 days'
),
(
    gen_random_uuid(),
    'achievement',
    'National Women''s Table Tennis Invitational: GIKI Secures Bronze Honors',
    'The GIKI Women''s Table Tennis squad achieved 3rd place on the podium at the National Inter-University Invitational in Lahore.

Led by singles seed Ayesha Malik (Chemical Engineering) and doubles partner Fatima Zahra (Materials Engineering), the team secured decisive 3-1 match victories over FAST-NUCES and GIK host opponents.',
    ARRAY[
        'https://images.unsplash.com/photo-1534158914592-062992fbe900?auto=format&fit=crop&w=1200&q=80'
    ]::text[],
    'published',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now() - INTERVAL '12 days',
    now() - INTERVAL '12 days',
    now() - INTERVAL '12 days'
),
(
    gen_random_uuid(),
    'achievement',
    'All-Pakistan Engineering Universities Cricket Cup: GIKI Lifts Gold in Topi',
    'Playing on home soil at the GIKI Main Sports Ground, the GIKI Cricket XI defeated PIEAS Islamabad by 38 runs in the grand final of the Engineering Universities Championship.

Top-order batsman Saad Ur Rehman scored a blistering 78 off 46 deliveries, backed by vice-captain Usama Tariq''s decisive 4-wicket haul in the death overs.',
    ARRAY[
        'https://images.unsplash.com/photo-1540747913346-19e32dc3e97e?auto=format&fit=crop&w=1200&q=80',
        'https://images.unsplash.com/photo-1531415074868-036b107e775a?auto=format&fit=crop&w=1200&q=80'
    ]::text[],
    'published',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now() - INTERVAL '15 days',
    now() - INTERVAL '15 days',
    now() - INTERVAL '15 days'
);

-- Draft items for admin view testing
INSERT INTO giki_sports.content_items (id, kind, title, content, image_urls, status, creator_id, created_at, updated_at)
VALUES
(
    gen_random_uuid(),
    'announcement',
    'Upcoming Summer Badminton & Tennis Camp 2027 (Draft Outline)',
    'Draft planning outline for professional coaching camp during summer recess.',
    '{}'::text[],
    'draft',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now(),
    now()
),
(
    gen_random_uuid(),
    'achievement',
    'Inter-Hostel Athletics Track & Field Meet: Results Pending Ratification',
    'Preliminary scores for 100m, 400m, and long jump events pending final referee sign-off.',
    '{}'::text[],
    'draft',
    'c426f6dc-3f53-46c8-bd38-a920485e0ab7',
    now(),
    now()
);

COMMIT;
