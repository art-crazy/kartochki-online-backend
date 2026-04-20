package handlers

import (
	"errors"
	"io"
	"net/http"

	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
	"kartochki-online-backend/internal/settings"
)

const maxUploadAvatarSizeBytes = 10 << 20

// UploadAvatar принимает multipart upload пользовательского аватара.
func (h SettingsHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	user, _, ok := h.currentAuth(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadAvatarSizeBytes)
	if err := r.ParseMultipartForm(maxUploadAvatarSizeBytes); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_multipart", "request must contain one image file")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "file_required", "multipart field file is required")
		return
	}
	defer file.Close()

	body, err := io.ReadAll(file)
	if err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_upload", "failed to read uploaded avatar")
		return
	}

	uploadedAvatar, err := h.settingsService.UploadAvatar(r.Context(), user.ID, settings.UploadedAvatar{
		FileName:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Body:        body,
	})
	if err != nil {
		switch {
		case errors.Is(err, settings.ErrAvatarRequired):
			response.WriteError(w, r, http.StatusBadRequest, "file_required", "multipart field file is required")
		case errors.Is(err, settings.ErrAvatarTypeNotSupported):
			response.WriteError(w, r, http.StatusBadRequest, "unsupported_image_type", "only png, jpg and webp images are supported")
		default:
			logger := requestctx.Logger(r.Context(), h.logger)
			logger.Error().Err(err).Str("user_id", user.ID).Msg("не удалось загрузить аватар пользователя")
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to upload avatar")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusCreated, openapi.SettingsAvatarResponse{AvatarUrl: uploadedAvatar.AvatarURL})
}
