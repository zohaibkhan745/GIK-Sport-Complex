import { useState, useEffect, useRef, useCallback } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { Navbar } from "../components/Navbar";
import { Footer } from "../components/Footer";
import { useAuth } from "../context/AuthContext";
import { api, ApiError } from "../api";
import type { ContentKind, Draft, CreateDraftInput, UpdateDraftInput } from "../types";

export function DraftEditor() {
  const { id } = useParams<{ id?: string }>();
  const navigate = useNavigate();
  const { user, isLoading: authLoading, checkError, retryCheckSession } = useAuth();

  // Mode: if id exists and is not "new", we are editing an existing draft
  const isInitialNew = !id || id === "new";
  const [currentDraftId, setCurrentDraftId] = useState<string | null>(
    isInitialNew ? null : id
  );

  const [kind, setKind] = useState<ContentKind>("announcement");
  const [title, setTitle] = useState<string>("");
  const [content, setContent] = useState<string>("");
  const [imageUrls, setImageUrls] = useState<string[]>([]);

  // Persisted state from server to track unsaved edits and check completeness
  const [savedTitle, setSavedTitle] = useState<string>("");
  const [savedContent, setSavedContent] = useState<string>("");
  const [savedImageUrls, setSavedImageUrls] = useState<string[]>([]);

  // Media upload state
  const [isUploading, setIsUploading] = useState<boolean>(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [manualUrlInput, setManualUrlInput] = useState<string>("");

  const [isFetchingDraft, setIsFetchingDraft] = useState<boolean>(!isInitialNew);
  const [fetchError, setFetchError] = useState<string | null>(null);

  const [isSaving, setIsSaving] = useState<boolean>(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState<string | null>(null);

  // Publish flow state (Step 10K)
  const [isPublishing, setIsPublishing] = useState<boolean>(false);
  const [publishError, setPublishError] = useState<string | null>(null);
  const [showConfirmModal, setShowConfirmModal] = useState<boolean>(false);

  // Keyboard accessibility and focus management refs (Step 10L)
  const publishButtonRef = useRef<HTMLButtonElement | null>(null);
  const modalCancelButtonRef = useRef<HTMLButtonElement | null>(null);
  const modalConfirmButtonRef = useRef<HTMLButtonElement | null>(null);
  const modalRef = useRef<HTMLDivElement | null>(null);

  // Guard against late fetch responses replacing different drafts after navigation
  const activeDraftIdRef = useRef<string | null>(currentDraftId);
  const justSavedIdRef = useRef<string | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const isMountedRef = useRef<boolean>(true);

  const canManageContent = Boolean(
    user &&
      (user.roles.includes("sports_director") ||
        user.roles.includes("super_admin"))
  );

  // Sync currentDraftId if param changes via navigation
  useEffect(() => {
    isMountedRef.current = true;
    const targetId = id && id !== "new" ? id : null;
    setCurrentDraftId(targetId);
    activeDraftIdRef.current = targetId;
    if (!targetId) {
      setTitle("");
      setContent("");
      setSavedTitle("");
      setSavedContent("");
      setKind("announcement");
      setFetchError(null);
      setSaveError(null);
      setSaveSuccess(null);
      setPublishError(null);
    }
    return () => {
      isMountedRef.current = false;
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, [id]);

  // Focus Cancel button on confirmation modal open (Step 10L)
  useEffect(() => {
    if (showConfirmModal) {
      modalCancelButtonRef.current?.focus();
    }
  }, [showConfirmModal]);

  // Keyboard trap (Tab) and dismiss (Escape) for confirmation modal (Step 10L)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!showConfirmModal) return;

      if (e.key === "Escape" && !isPublishing) {
        e.preventDefault();
        setShowConfirmModal(false);
        publishButtonRef.current?.focus();
        return;
      }

      if (e.key === "Tab") {
        const cancelBtn = modalCancelButtonRef.current;
        const confirmBtn = modalConfirmButtonRef.current;

        if (!cancelBtn || !confirmBtn) return;

        if (e.shiftKey) {
          // Shift+Tab: if activeElement is Cancel button or outside, wrap to Confirm button
          if (
            document.activeElement === cancelBtn ||
            !modalRef.current?.contains(document.activeElement)
          ) {
            e.preventDefault();
            confirmBtn.focus();
          }
        } else {
          // Tab: if activeElement is Confirm button or outside, wrap to Cancel button
          if (
            document.activeElement === confirmBtn ||
            !modalRef.current?.contains(document.activeElement)
          ) {
            e.preventDefault();
            cancelBtn.focus();
          }
        }
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [showConfirmModal, isPublishing]);

  // Load existing draft if in edit mode
  const loadDraft = useCallback(async (targetId: string) => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    const controller = new AbortController();
    abortControllerRef.current = controller;

    setIsFetchingDraft(true);
    setFetchError(null);

    try {
      const draft = await api.getDraft(targetId, { signal: controller.signal });
      if (!isMountedRef.current || activeDraftIdRef.current !== targetId) {
        return;
      }
      setKind(draft.kind);
      setTitle(draft.title ?? "");
      setContent(draft.content ?? "");
      setImageUrls(draft.imageUrls ?? []);
      setSavedTitle(draft.title ?? "");
      setSavedContent(draft.content ?? "");
      setSavedImageUrls(draft.imageUrls ?? []);
      setFetchError(null);
    } catch (err: unknown) {
      if (
        (err instanceof DOMException && err.name === "AbortError") ||
        (err instanceof Error && err.name === "AbortError")
      ) {
        return;
      }
      if (!isMountedRef.current || activeDraftIdRef.current !== targetId) {
        return;
      }

      if (err instanceof ApiError) {
        if (err.status === 404) {
          setFetchError(
            "Draft not found. The draft may have been deleted or the ID is invalid."
          );
        } else if (err.status === 409) {
          setFetchError(
            "This content item has already been published and can no longer be edited as a draft."
          );
        } else {
          setFetchError(err.message);
        }
      } else {
        setFetchError(
          err instanceof Error ? err.message : "Failed to load draft details"
        );
      }
    } finally {
      if (isMountedRef.current && activeDraftIdRef.current === targetId) {
        setIsFetchingDraft(false);
      }
    }
  }, []);

  useEffect(() => {
    // Only fetch draft details if session check completed and role is authorized
    if (!authLoading && canManageContent && currentDraftId) {
      if (justSavedIdRef.current === currentDraftId) {
        justSavedIdRef.current = null;
        return;
      }
      loadDraft(currentDraftId);
    }
  }, [authLoading, canManageContent, currentDraftId, loadDraft]);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (isSaving || isFetchingDraft || isPublishing) return;

    setIsSaving(true);
    setSaveError(null);
    setSaveSuccess(null);
    setPublishError(null);

    // Send null for blank/whitespace values to allow incomplete drafts while
    // avoiding phantom whitespace-only entries.
    const titlePayload = title.trim().length === 0 ? null : title;
    const contentPayload = content.trim().length === 0 ? null : content;

    try {
      if (!currentDraftId) {
        // Create new draft (POST /api/admin/content/drafts)
        const createInput: CreateDraftInput = {
          kind,
          title: titlePayload,
          content: contentPayload,
        };
        if (imageUrls.length > 0) {
          createInput.imageUrls = imageUrls;
        }
        const created: Draft = await api.createDraft(createInput);

        if (!isMountedRef.current) return;

        // Transition editor to edit mode for the newly created draft ID
        justSavedIdRef.current = created.id;
        setCurrentDraftId(created.id);
        activeDraftIdRef.current = created.id;
        setIsFetchingDraft(false);
        setKind(created.kind);
        setTitle(created.title ?? "");
        setContent(created.content ?? "");
        setImageUrls(created.imageUrls ?? []);
        setSavedTitle(created.title ?? "");
        setSavedContent(created.content ?? "");
        setSavedImageUrls(created.imageUrls ?? []);
        setSaveSuccess("Draft created successfully.");
        setPublishError(null);

        // Replace history URL to /admin/content/:id so next save updates existing draft
        navigate(`/admin/content/${created.id}`, { replace: true });
      } else {
        // Update existing draft (PUT /api/admin/content/drafts/:id)
        // Must send fields because omitted fields clear their values
        const updateInput: UpdateDraftInput = {
          title: titlePayload,
          content: contentPayload,
        };
        if (imageUrls.length > 0 || savedImageUrls.length > 0) {
          updateInput.imageUrls = imageUrls;
        }
        const updated: Draft = await api.updateDraft(currentDraftId, updateInput);

        if (!isMountedRef.current) return;

        setTitle(updated.title ?? "");
        setContent(updated.content ?? "");
        setImageUrls(updated.imageUrls ?? []);
        setSavedTitle(updated.title ?? "");
        setSavedContent(updated.content ?? "");
        setSavedImageUrls(updated.imageUrls ?? []);
        setSaveSuccess("Draft saved successfully.");
        setPublishError(null);
      }
    } catch (err: unknown) {
      if (!isMountedRef.current) return;

      // Retain the entered draft text on failure without wiping user inputs
      if (err instanceof ApiError) {
        if (err.status === 404) {
          setSaveError(
            "Draft not found. The draft may have been deleted or the ID is invalid."
          );
        } else if (err.status === 409) {
          setSaveError(
            "This content item has already been published and can no longer be edited as a draft."
          );
        } else {
          setSaveError(err.message);
        }
      } else {
        setSaveError(
          err instanceof Error ? err.message : "Failed to save draft."
        );
      }
    } finally {
      if (isMountedRef.current) {
        setIsSaving(false);
      }
    }
  };

  const handleOpenPublish = () => {
    setSaveSuccess(null);
    setSaveError(null);
    setPublishError(null);

    // 1. If editor has unsaved changes, explain that they must be saved first.
    // Do not auto-save or publish stale text.
    if (
      title !== savedTitle ||
      content !== savedContent ||
      JSON.stringify(imageUrls) !== JSON.stringify(savedImageUrls)
    ) {
      setPublishError(
        "You have unsaved changes. Please save the draft first before publishing."
      );
      return;
    }

    // 2. Require a saved nonblank title and body before opening confirmation.
    if (savedTitle.trim().length === 0 || savedContent.trim().length === 0) {
      setPublishError(
        "Cannot publish: the draft is incomplete. Both a non-blank title and body content must be saved before publishing."
      );
      return;
    }

    setShowConfirmModal(true);
  };

  const handleCancelPublish = () => {
    setShowConfirmModal(false);
    publishButtonRef.current?.focus();
  };

  const handleBackdropClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget && !isPublishing) {
      handleCancelPublish();
    }
  };

  const handleConfirmPublish = async () => {
    if (isPublishing || isSaving || !currentDraftId) return;

    setIsPublishing(true);
    setPublishError(null);

    const targetId = currentDraftId;

    try {
      const published = await api.publishDraft(targetId, {
        signal: abortControllerRef.current?.signal,
      });

      if (!isMountedRef.current || activeDraftIdRef.current !== targetId) {
        return;
      }

      setShowConfirmModal(false);
      navigate("/admin/content", {
        state: {
          publishedSuccess: `"${published.title}" has been published and is now public.`,
        },
      });
    } catch (err: unknown) {
      if (
        (err instanceof DOMException && err.name === "AbortError") ||
        (err instanceof Error && err.name === "AbortError")
      ) {
        return;
      }
      if (!isMountedRef.current || activeDraftIdRef.current !== targetId) {
        return;
      }

      setShowConfirmModal(false);

      if (err instanceof ApiError) {
        if (err.status === 422) {
          setPublishError(
            "Cannot publish: the saved draft is incomplete. Both a non-blank title and body are required."
          );
        } else if (err.status === 409) {
          setPublishError(
            "Cannot publish: this item has already been published and is no longer a draft."
          );
        } else if (err.status === 401 || err.status === 403) {
          setPublishError(
            "You do not have permission to publish content."
          );
        } else if (err.status >= 500) {
          setPublishError(
            "Publishing failed due to a server error. Please refresh the page to verify the draft's current status before trying again."
          );
        } else {
          setPublishError(err.message);
        }
      } else {
        setPublishError(
          "Publishing may not have completed due to a server or network failure. Please refresh the page to verify the draft's current status before trying again."
        );
      }
    } finally {
      if (isMountedRef.current && activeDraftIdRef.current === targetId) {
        setIsPublishing(false);
      }
    }
  };

  const handleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (!files || files.length === 0) return;
    if (imageUrls.length + files.length > 10) {
      setUploadError("Maximum 10 images allowed per content item.");
      return;
    }
    setIsUploading(true);
    setUploadError(null);
    try {
      const fileList = Array.from(files);
      const uploaded = await api.uploadContentImages(fileList);
      setImageUrls((prev) => [...prev, ...uploaded]);
    } catch (err: unknown) {
      setUploadError(err instanceof Error ? err.message : "Failed to upload images.");
    } finally {
      setIsUploading(false);
      e.target.value = "";
    }
  };

  const handleAddManualUrl = (e: React.MouseEvent) => {
    e.preventDefault();
    const trimmed = manualUrlInput.trim();
    if (!trimmed) return;
    if (imageUrls.length >= 10) {
      setUploadError("Maximum 10 images allowed.");
      return;
    }
    setImageUrls((prev) => [...prev, trimmed]);
    setManualUrlInput("");
    setUploadError(null);
  };

  const handleRemoveImage = (indexToRemove: number) => {
    setImageUrls((prev) => prev.filter((_, idx) => idx !== indexToRemove));
  };

  const handleCancel = () => {
    navigate("/admin/content");
  };

  const isEditMode = Boolean(currentDraftId);

  return (
    <div className="flex min-h-screen flex-col bg-white">
      <Navbar />

      <main className="flex-1 py-10 px-6 max-w-4xl mx-auto w-full">
        {/* State 1: Initial Session Check in progress */}
        {authLoading && (
          <div
            role="status"
            aria-live="polite"
            className="flex flex-col items-center justify-center py-20 space-y-3"
          >
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--color-maroon)]" />
            <p className="text-sm text-[var(--color-navy-soft)]">
              Checking authorization...
            </p>
          </div>
        )}

        {/* State 2: Session check outage (server or network failure) */}
        {!authLoading && checkError && !user && (
          <div
            role="alert"
            className="max-w-xl mx-auto my-12 rounded-xl border border-amber-300 bg-amber-50 p-6 text-center"
          >
            <h2 className="text-lg font-bold text-amber-900">
              Session Check Unavailable
            </h2>
            <p className="mt-2 text-sm text-amber-800">
              We could not verify your session due to a server or network outage:{" "}
              <span className="font-medium">{checkError}</span>
            </p>
            <button
              onClick={() => retryCheckSession()}
              className="mt-4 inline-flex items-center px-4 py-2 rounded-md bg-[var(--color-maroon)] text-xs font-semibold text-white hover:bg-[var(--color-maroon-dark)] transition cursor-pointer"
            >
              Retry Session Check
            </button>
          </div>
        )}

        {/* State 3: Signed-out visitor */}
        {!authLoading && !checkError && !user && (
          <div
            role="region"
            aria-label="Sign In Required"
            className="max-w-xl mx-auto my-12 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center"
          >
            <h2 className="text-xl font-bold text-[var(--color-navy)]">
              Sign In Required
            </h2>
            <p className="mt-2 text-sm text-[var(--color-navy-soft)]">
              You must be signed in with a Sports Director or Administrator account
              to create or edit content drafts.
            </p>
            <div className="mt-6">
              <Link
                to="/login"
                className="inline-block px-5 py-2.5 rounded-md bg-[var(--color-maroon)] text-white text-xs font-semibold hover:bg-[var(--color-maroon-dark)] shadow-sm transition"
              >
                Sign in
              </Link>
            </div>
          </div>
        )}

        {/* State 4: Signed-in user without allowed role */}
        {!authLoading && user && !canManageContent && (
          <div
            role="alert"
            className="max-w-xl mx-auto my-12 rounded-xl border border-red-200 bg-red-50 p-8 text-center"
          >
            <h2 className="text-xl font-bold text-red-900">Access Denied</h2>
            <p className="mt-2 text-sm text-red-800">
              Your account ({user.displayName} &lt;{user.email}&gt;) does not
              have permission to view or edit content drafts. This area is
              reserved for Sports Directors and Super Administrators.
            </p>
            <div className="mt-6">
              <Link
                to="/"
                className="inline-block px-4 py-2 rounded-md border border-[var(--color-border)] bg-white text-xs font-semibold text-[var(--color-navy)] hover:bg-[var(--color-surface)]"
              >
                Return to Home
              </Link>
            </div>
          </div>
        )}

        {/* State 5: Authorized editor */}
        {!authLoading && canManageContent && (
          <div className="space-y-6">
            {/* Breadcrumb / Back Link */}
            <div>
              <Link
                to="/admin/content"
                className="inline-flex items-center text-xs font-semibold text-[var(--color-navy-soft)] hover:text-[var(--color-maroon)] transition"
              >
                &larr; Back to draft list
              </Link>
            </div>

            {/* Page Header */}
            <div className="border-b border-[var(--color-border)] pb-4">
              <h1 className="text-2xl font-bold tracking-tight text-[var(--color-navy)] font-display sm:text-3xl">
                {isEditMode ? "Edit Draft" : "New Content Draft"}
              </h1>
              <p className="mt-1 text-xs text-[var(--color-navy-soft)]">
                {isEditMode
                  ? "Update title or body text. Drafts can be saved in an incomplete state before publication."
                  : "Choose kind and begin drafting. Incomplete drafts can be saved and finalized later."}
              </p>
            </div>

            {/* Draft detail fetching state */}
            {isFetchingDraft && (
              <div
                role="status"
                aria-live="polite"
                className="flex flex-col items-center justify-center py-16 space-y-3"
              >
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--color-maroon)]" />
                <p className="text-sm text-[var(--color-navy-soft)]">
                  Loading draft details...
                </p>
              </div>
            )}

            {/* Fetch Error Banner */}
            {!isFetchingDraft && fetchError && (
              <div
                role="alert"
                className="rounded-lg border border-red-200 bg-red-50 p-6 text-center space-y-3"
              >
                <p className="text-sm font-semibold text-red-800">
                  {fetchError}
                </p>
                <div>
                  <button
                    type="button"
                    onClick={() => currentDraftId && loadDraft(currentDraftId)}
                    className="px-4 py-1.5 bg-[var(--color-maroon)] text-white text-xs font-semibold rounded-md hover:bg-[var(--color-maroon-dark)] transition cursor-pointer"
                  >
                    Retry loading
                  </button>
                </div>
              </div>
            )}

            {/* Editor Form */}
            {!isFetchingDraft && !fetchError && (
              <form onSubmit={handleSave} className="space-y-6">
                {/* Save Feedback Alerts */}
                {saveSuccess && (
                  <div
                    role="status"
                    className="rounded-md border border-emerald-300 bg-emerald-50 px-4 py-3 text-xs font-semibold text-emerald-800"
                  >
                    {saveSuccess}
                  </div>
                )}

                {saveError && (
                  <div
                    role="alert"
                    className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800"
                  >
                    {saveError}
                  </div>
                )}

                {publishError && (
                  <div
                    role="alert"
                    className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800"
                  >
                    {publishError}
                  </div>
                )}

                {/* Field 1: Kind (Selectable on create, read-only on edit) */}
                <div>
                  <label
                    htmlFor="draft-kind"
                    className="block text-xs font-semibold text-[var(--color-navy)] mb-1.5"
                  >
                    Content Kind
                  </label>
                  {isEditMode ? (
                    <div className="flex items-center gap-2">
                      <span
                        id="draft-kind"
                        className={`inline-flex items-center px-3 py-1 rounded-full text-xs font-semibold border ${
                          kind === "announcement"
                            ? "bg-blue-50 text-blue-700 border-blue-200"
                            : "bg-amber-50 text-amber-800 border-amber-200"
                        }`}
                      >
                        {kind === "announcement"
                          ? "Announcement"
                          : "Achievement"}
                      </span>
                      <span className="text-xs text-neutral-500 italic">
                        (Kind is fixed once created)
                      </span>
                    </div>
                  ) : (
                    <div className="flex items-center gap-4">
                      <label className="inline-flex items-center gap-2 text-xs font-medium text-[var(--color-navy)] cursor-pointer">
                        <input
                          type="radio"
                          name="kind"
                          value="announcement"
                          checked={kind === "announcement"}
                          onChange={() => setKind("announcement")}
                          disabled={isSaving || isPublishing}
                          className="text-[var(--color-maroon)] focus:ring-[var(--color-gold)]"
                        />
                        Announcement
                      </label>
                      <label className="inline-flex items-center gap-2 text-xs font-medium text-[var(--color-navy)] cursor-pointer">
                        <input
                          type="radio"
                          name="kind"
                          value="achievement"
                          checked={kind === "achievement"}
                          onChange={() => setKind("achievement")}
                          disabled={isSaving || isPublishing}
                          className="text-[var(--color-maroon)] focus:ring-[var(--color-gold)]"
                        />
                        Achievement
                      </label>
                    </div>
                  )}
                </div>

                {/* Field 2: Title */}
                <div>
                  <label
                    htmlFor="draft-title"
                    className="block text-xs font-semibold text-[var(--color-navy)] mb-1.5"
                  >
                    Title
                  </label>
                  <input
                    id="draft-title"
                    type="text"
                    maxLength={255}
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                    disabled={isSaving || isPublishing}
                    placeholder="Draft headline or title (optional)"
                    className="w-full rounded-md border border-[var(--color-border)] bg-white px-3.5 py-2 text-sm text-[var(--color-navy)] placeholder:text-neutral-400 focus:border-[var(--color-maroon)] focus:ring-1 focus:ring-[var(--color-maroon)] disabled:opacity-50"
                  />
                  <p className="mt-1 text-[11px] text-[var(--color-navy-soft)]">
                    Maximum 255 characters. Can be left empty for incomplete drafts.
                  </p>
                </div>

                {/* Field 3: Body / Content */}
                <div>
                  <label
                    htmlFor="draft-content"
                    className="block text-xs font-semibold text-[var(--color-navy)] mb-1.5"
                  >
                    Body Content
                  </label>
                  <textarea
                    id="draft-content"
                    rows={10}
                    value={content}
                    onChange={(e) => setContent(e.target.value)}
                    disabled={isSaving || isPublishing}
                    placeholder="Write announcement details or achievement description... (optional)"
                    className="w-full rounded-md border border-[var(--color-border)] bg-white p-3.5 text-sm text-[var(--color-navy)] placeholder:text-neutral-400 focus:border-[var(--color-maroon)] focus:ring-1 focus:ring-[var(--color-maroon)] disabled:opacity-50 font-sans"
                  />
                  <p className="mt-1 text-[11px] text-[var(--color-navy-soft)]">
                    Incomplete drafts can be saved without body text and filled in later.
                  </p>
                </div>

                {/* Field 4: Media & Photos (Optional) */}
                <div className="space-y-3 rounded-lg border border-[var(--color-border)] bg-neutral-50/60 p-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h4 className="text-xs font-semibold text-[var(--color-navy)]">
                        Photos & Media (Optional)
                      </h4>
                      <p className="text-[11px] text-[var(--color-navy-soft)]">
                        Attach tournament photos, medal ceremonies, or posters.
                      </p>
                    </div>
                    <span className="text-[11px] font-medium text-neutral-500">
                      {imageUrls.length} / 10 images
                    </span>
                  </div>

                  {uploadError && (
                    <div
                      role="alert"
                      className="rounded border border-red-200 bg-red-50 p-2 text-xs text-red-700"
                    >
                      {uploadError}
                    </div>
                  )}

                  {/* Upload Controls */}
                  <div className="flex flex-wrap items-center gap-2">
                    <label className="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-[var(--color-border)] bg-white px-3 py-1.5 text-xs font-semibold text-[var(--color-navy)] shadow-2xs hover:bg-neutral-50 transition">
                      <svg
                        xmlns="http://www.w3.org/2000/svg"
                        className="h-3.5 w-3.5 text-[var(--color-maroon)]"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"
                        />
                      </svg>
                      {isUploading ? "Uploading..." : "Upload from Computer"}
                      <input
                        type="file"
                        multiple
                        accept="image/png, image/jpeg, image/webp, image/gif"
                        onChange={handleFileUpload}
                        disabled={isSaving || isPublishing || isUploading || imageUrls.length >= 10}
                        className="hidden"
                      />
                    </label>

                    <span className="text-xs text-neutral-400">or</span>

                    {/* Manual URL Input */}
                    <div className="flex flex-1 items-center gap-1.5 min-w-[200px]">
                      <input
                        type="url"
                        value={manualUrlInput}
                        onChange={(e) => setManualUrlInput(e.target.value)}
                        placeholder="Paste image link (https://...)"
                        disabled={isSaving || isPublishing || imageUrls.length >= 10}
                        className="flex-1 rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1.5 text-xs text-[var(--color-navy)] placeholder:text-neutral-400 focus:border-[var(--color-maroon)] focus:ring-1 focus:ring-[var(--color-maroon)]"
                      />
                      <button
                        type="button"
                        onClick={handleAddManualUrl}
                        disabled={!manualUrlInput.trim() || isSaving || isPublishing || imageUrls.length >= 10}
                        className="rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1.5 text-xs font-semibold text-[var(--color-navy)] hover:bg-neutral-50 disabled:opacity-40 transition cursor-pointer"
                      >
                        Add URL
                      </button>
                    </div>
                  </div>

                  {/* Thumbnail Previews */}
                  {imageUrls.length > 0 && (
                    <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 pt-2">
                      {imageUrls.map((url, idx) => (
                        <div
                          key={idx}
                          className="group relative h-24 overflow-hidden rounded-md border border-[var(--color-border)] bg-neutral-200"
                        >
                          <img
                            src={url}
                            alt={`Attached preview ${idx + 1}`}
                            className="h-full w-full object-cover"
                            onError={(e) => {
                              (e.target as HTMLImageElement).src =
                                "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' width='100' height='100'><rect width='100' height='100' fill='%23eee'/><text x='50%' y='50%' dominant-baseline='middle' text-anchor='middle' fill='%23999' font-size='10'>Image Error</text></svg>";
                            }}
                          />
                          <button
                            type="button"
                            onClick={() => handleRemoveImage(idx)}
                            aria-label={`Remove photo ${idx + 1}`}
                            className="absolute top-1 right-1 flex h-5 w-5 items-center justify-center rounded-full bg-red-600 text-white text-[10px] font-bold shadow opacity-80 group-hover:opacity-100 hover:bg-red-700 transition cursor-pointer"
                          >
                            ✕
                          </button>
                          <span className="absolute bottom-1 left-1 rounded bg-black/60 px-1 py-0.5 text-[9px] font-medium text-white">
                            #{idx + 1}
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* Action Buttons: Save draft, Publish draft & Cancel */}
                <div className="flex items-center gap-3 pt-4 border-t border-[var(--color-border)]">
                  <button
                    type="submit"
                    disabled={isSaving || isPublishing}
                    className="inline-flex items-center justify-center gap-2 px-5 py-2 rounded-md bg-[var(--color-maroon)] text-white text-xs font-semibold hover:bg-[var(--color-maroon-dark)] transition shadow-sm disabled:opacity-50 cursor-pointer"
                  >
                    {isSaving && (
                      <span className="inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-white border-t-transparent" />
                    )}
                    {isSaving ? "Saving draft..." : "Save draft"}
                  </button>

                  {isEditMode && (
                    <button
                      ref={publishButtonRef}
                      type="button"
                      onClick={handleOpenPublish}
                      disabled={isSaving || isPublishing}
                      className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-md bg-emerald-700 text-white text-xs font-semibold hover:bg-emerald-800 transition shadow-sm disabled:opacity-50 cursor-pointer"
                    >
                      Publish draft
                    </button>
                  )}

                  <button
                    type="button"
                    onClick={handleCancel}
                    disabled={isSaving || isPublishing}
                    className="px-4 py-2 rounded-md border border-[var(--color-border)] bg-white text-xs font-semibold text-[var(--color-navy)] hover:bg-[var(--color-surface)] transition disabled:opacity-50 cursor-pointer"
                  >
                    Cancel
                  </button>
                </div>
              </form>
            )}

            {/* Accessible Confirmation Modal (Step 10K / 10L Keyboard Trap & Focus Management) */}
            {showConfirmModal && (
              <div
                className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
                role="dialog"
                aria-modal="true"
                aria-labelledby="publish-dialog-title"
                aria-describedby="publish-dialog-description"
                onClick={handleBackdropClick}
              >
                <div
                  ref={modalRef}
                  className="relative w-full max-w-md rounded-xl bg-white p-6 shadow-xl border border-[var(--color-border)]"
                  onClick={(e) => e.stopPropagation()}
                >
                  <h3
                    id="publish-dialog-title"
                    className="text-lg font-bold text-[var(--color-navy)]"
                  >
                    Confirm Publication
                  </h3>
                  <p
                    id="publish-dialog-description"
                    className="mt-2 text-sm text-[var(--color-navy-soft)] leading-relaxed"
                  >
                    Are you sure you want to publish{" "}
                    <span className="font-semibold text-[var(--color-navy)]">
                      {savedTitle ? `"${savedTitle}"` : "this draft"}
                    </span>
                    ?
                  </p>
                  <p className="mt-3 text-xs text-amber-800 bg-amber-50 border border-amber-200 rounded p-2.5 leading-relaxed">
                    Once published, this item will immediately become visible to all
                    students, faculty, and campus visitors on the public portal. This action
                    cannot be undone.
                  </p>

                  <div className="mt-6 flex justify-end gap-3">
                    <button
                      ref={modalCancelButtonRef}
                      type="button"
                      onClick={handleCancelPublish}
                      disabled={isPublishing}
                      className="px-4 py-2 rounded-md border border-[var(--color-border)] bg-white text-xs font-semibold text-[var(--color-navy)] hover:bg-[var(--color-surface)] transition disabled:opacity-50 cursor-pointer"
                    >
                      Cancel
                    </button>
                    <button
                      ref={modalConfirmButtonRef}
                      type="button"
                      onClick={handleConfirmPublish}
                      disabled={isPublishing}
                      className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-md bg-emerald-700 text-xs font-semibold text-white hover:bg-emerald-800 transition shadow-sm disabled:opacity-50 cursor-pointer"
                    >
                      {isPublishing && (
                        <span className="inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-white border-t-transparent" />
                      )}
                      {isPublishing ? "Publishing..." : "Confirm & Publish"}
                    </button>
                  </div>
                </div>
              </div>
            )}
          </div>
        )}
      </main>

      <Footer />
    </div>
  );
}
