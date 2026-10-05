import { useRef, useState } from "react";
import { Camera, Heart, MessageCircle, Image as ImageIcon } from "lucide-react";
import type { AchievementPost } from "../types";
import { api, ApiError } from "../api";

const SEED_POSTS: AchievementPost[] = [
  {
    id: "seed-1",
    author: "Sports Complex Admin",
    initials: "SC",
    caption: "Swimming team brings home gold at the inter-university gala",
    timeAgo: "2 days ago",
    likes: 84,
    comments: 9,
  },
  {
    id: "seed-2",
    author: "Sports Complex Admin",
    initials: "SC",
    caption: "Padel league registrations are now open, sign up before Friday",
    timeAgo: "5 days ago",
    likes: 41,
    comments: 3,
  },
];

export function AchievementsFeed() {
  const [posts, setPosts] = useState<AchievementPost[]>(SEED_POSTS);
  const [caption, setCaption] = useState("");
  const [photo, setPhoto] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  function handlePhotoChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setPhoto(file);
    setPreviewUrl(URL.createObjectURL(file));
  }

  async function handlePost() {
    if (!caption.trim()) return;
    setSubmitting(true);

    // Optimistic local post so the feed updates instantly.
    const optimistic: AchievementPost = {
      id: `local-${Date.now()}`,
      author: "Sports Complex Admin",
      initials: "SC",
      caption: caption.trim(),
      timeAgo: "Just now",
      likes: 0,
      comments: 0,
      imageUrl: previewUrl ?? undefined,
    };
    setPosts((prev) => [optimistic, ...prev]);
    setCaption("");
    setPhoto(null);
    setPreviewUrl(null);
    if (fileInputRef.current) fileInputRef.current.value = "";

    // Try to persist through the Go backend; harmless if it isn't running yet.
    try {
      await api.createAchievement({ caption: optimistic.caption, photo: photo ?? undefined });
    } catch (err) {
      if (!(err instanceof ApiError)) {
        console.warn("Could not reach /api/achievements — showing the post locally only.", err);
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <section id="achievements">
      <h2 className="font-display text-lg font-semibold text-[var(--color-navy)]">
        Achievements and updates
      </h2>

      {/* Composer */}
      <div className="mt-3 rounded-xl border border-dashed border-[var(--color-gold)] bg-[var(--color-surface)] p-4">
        <div className="flex items-start gap-3">
          <span className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-[var(--color-maroon)] text-xs font-semibold text-[var(--color-gold-light)]">
            SC
          </span>
          <textarea
            value={caption}
            onChange={(e) => setCaption(e.target.value)}
            placeholder="Share an update or achievement..."
            rows={2}
            className="w-full resize-none bg-transparent text-sm text-[var(--color-navy)] placeholder:text-[var(--color-navy-soft)] focus:outline-none"
          />
        </div>

        {previewUrl && (
          <img
            src={previewUrl}
            alt="Selected photo preview"
            className="mt-3 h-40 w-full rounded-lg object-cover"
          />
        )}

        <div className="mt-3 flex items-center justify-between">
          <button
            type="button"
            onClick={() => fileInputRef.current?.click()}
            className="flex items-center gap-1.5 text-sm text-[var(--color-gold)] hover:opacity-80"
          >
            <Camera size={16} />
            Add photo
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            onChange={handlePhotoChange}
            className="hidden"
          />
          <button
            type="button"
            onClick={handlePost}
            disabled={!caption.trim() || submitting}
            className="rounded-md bg-[var(--color-gold)] px-4 py-1.5 text-sm font-semibold text-[var(--color-navy)] transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {submitting ? "Posting..." : "Post"}
          </button>
        </div>
      </div>

      {/* Feed */}
      <div className="mt-4 flex flex-col gap-3">
        {posts.map((post) => (
          <article
            key={post.id}
            className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]"
          >
            <div className="flex h-44 items-center justify-center border-b border-[var(--color-border)] bg-white">
              {post.imageUrl ? (
                <img
                  src={post.imageUrl}
                  alt=""
                  className="h-full w-full object-cover"
                />
              ) : (
                <ImageIcon size={28} className="text-[var(--color-border)]" />
              )}
            </div>
            <div className="p-4">
              <div className="mb-1.5 flex items-center gap-2">
                <span className="flex h-6 w-6 items-center justify-center rounded-full bg-[var(--color-maroon)] text-[10px] font-semibold text-[var(--color-gold-light)]">
                  {post.initials}
                </span>
                <p className="text-xs text-[var(--color-navy)]">{post.author}</p>
              </div>
              <p className="text-sm text-[var(--color-navy)]">{post.caption}</p>
              <div className="mt-2 flex items-center gap-4 text-xs text-[var(--color-navy-soft)]">
                <span>{post.timeAgo}</span>
                <span className="flex items-center gap-1">
                  <Heart size={13} /> {post.likes}
                </span>
                <span className="flex items-center gap-1">
                  <MessageCircle size={13} /> {post.comments}
                </span>
              </div>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
