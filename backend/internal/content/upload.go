package content

import (
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

var allowedImageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

// UploadImages handles POST /api/admin/content/upload.
// Accepts multipart/form-data with one or multiple files in "images" or "image".
// Saves allowed image files to ./uploads/ and returns their public URL paths.
func (h *Handler) UploadImages(c *fiber.Ctx) error {
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: fmt.Sprintf("invalid multipart form: %v", err),
		})
	}

	var files []*multipart.FileHeader
	if fList, ok := form.File["images"]; ok {
		files = append(files, fList...)
	}
	if fList, ok := form.File["image"]; ok {
		files = append(files, fList...)
	}

	if len(files) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "no image files provided under 'images' or 'image' field",
		})
	}

	if len(files) > MaxImagesPerItem {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: fmt.Sprintf("cannot upload more than %d images at once", MaxImagesPerItem),
		})
	}

	uploadDir := filepath.Join(".", "uploads")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: fmt.Sprintf("failed to initialize upload directory: %v", err),
		})
	}

	savedURLs := make([]string, 0, len(files))
	const maxFileSize = 10 * 1024 * 1024 // 10 MB per file

	for _, file := range files {
		if file.Size > maxFileSize {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: fmt.Sprintf("file %q exceeds 10MB size limit", file.Filename),
			})
		}

		ext := strings.ToLower(filepath.Ext(file.Filename))
		if !allowedImageExtensions[ext] {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: fmt.Sprintf("unsupported image format for %q (allowed: .jpg, .jpeg, .png, .webp, .gif)", file.Filename),
			})
		}

		uniqueFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
		targetPath := filepath.Join(uploadDir, uniqueFilename)

		if err := c.SaveFile(file, targetPath); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error: fmt.Sprintf("failed to save image %q: %v", file.Filename, err),
			})
		}

		savedURLs = append(savedURLs, "/uploads/"+uniqueFilename)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"urls": savedURLs,
	})
}
