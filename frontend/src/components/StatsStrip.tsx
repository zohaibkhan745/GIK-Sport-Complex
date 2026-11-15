import { Link } from "react-router-dom";
import { Building2, Activity, Dumbbell, Clock, ArrowRight } from "lucide-react";

interface StatsStripProps {
  facilityCount?: number;
}

export function StatsStrip({ facilityCount }: StatsStripProps) {
  const cards = [
    {
      title: "Facilities",
      highlight: facilityCount !== undefined ? `${facilityCount} Venues` : "8 Venues",
      subtitle: "Courts, pool, hall & gym",
      icon: Building2,
      iconColor: "text-[var(--color-navy)]",
      href: "#facilities",
      isExternal: true,
      actionText: "View all",
    },
    {
      title: "Padel Courts",
      highlight: "60-Min Slots",
      subtitle: "Online booking & QR pass",
      icon: Activity,
      iconColor: "text-[var(--color-maroon)]",
      href: "/padel",
      isExternal: false,
      actionText: "Book slot",
    },
    {
      title: "Gym Membership",
      highlight: "PKR 3,000",
      subtitle: "30-day access pass",
      icon: Dumbbell,
      iconColor: "text-amber-700",
      href: "/membership",
      isExternal: false,
      actionText: "Get pass",
    },
    {
      title: "Operating Hours",
      highlight: "Weekly Timetables",
      subtitle: "Student, faculty & women's hours",
      icon: Clock,
      iconColor: "text-purple-700",
      href: "#facilities",
      isExternal: true,
      actionText: "View schedule",
    },
  ];

  return (
    <div
      data-testid="stats-strip"
      className="grid grid-cols-2 gap-3 lg:grid-cols-4"
    >
      {cards.map((card) => {
        const Icon = card.icon;
        const CardInner = (
          <div className="flex h-full flex-col justify-between rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-3.5 transition-all hover:border-[var(--color-gold)] hover:shadow-2xs group">
            <div>
              <div className="flex items-center justify-between">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-[var(--color-navy-soft)]">
                  {card.title}
                </span>
                <Icon size={16} className={card.iconColor} />
              </div>
              <p className="mt-1.5 text-base font-bold text-[var(--color-navy)] group-hover:text-[var(--color-maroon)] transition-colors">
                {card.highlight}
              </p>
              <p className="mt-0.5 text-xs text-[var(--color-navy-soft)]">
                {card.subtitle}
              </p>
            </div>
            <div className="mt-3 flex items-center gap-1 text-xs font-semibold text-[var(--color-maroon)]">
              <span>{card.actionText}</span>
              <ArrowRight
                size={12}
                className="transition-transform group-hover:translate-x-0.5"
              />
            </div>
          </div>
        );

        return card.isExternal ? (
          <a key={card.title} href={card.href} className="block focus:outline-none">
            {CardInner}
          </a>
        ) : (
          <Link key={card.title} to={card.href} className="block focus:outline-none">
            {CardInner}
          </Link>
        );
      })}
    </div>
  );
}
