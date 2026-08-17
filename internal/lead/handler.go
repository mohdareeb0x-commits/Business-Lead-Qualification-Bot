package lead

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/telegramleadbot/telegram-lead-bot/internal/apierr"
)

// Handler binds HTTP routes to the lead service.
type Handler struct {
	svc *Service
}

// NewHandler returns a Handler bound to svc.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Register wires routes onto rg.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/leads", h.list)
	rg.GET("/leads/:id", h.get)
	rg.PATCH("/leads/:id/status", h.updateStatus)
	rg.GET("/leads/:id/events", h.listEvents)
	rg.GET("/dashboard/stats", h.stats)
}

func (h *Handler) list(c *gin.Context) {
	page, err := parseIntDefault(c.Query("page"), DefaultPage)
	if err != nil {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid page", nil)
		return
	}
	limit, err := parseIntDefault(c.Query("limit"), DefaultLimit)
	if err != nil {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid limit", nil)
		return
	}
	q := ListLeadsQuery{
		Page:        page,
		Limit:       limit,
		Status:      c.Query("status"),
		Temperature: c.Query("temperature"),
	}
	resp, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) get(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid id", nil)
		return
	}
	lead, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, lead)
}

func (h *Handler) updateStatus(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid id", nil)
		return
	}
	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid request", gin.H{"body": err.Error()})
		return
	}
	if !IsValidStatus(req.Status) {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid status", gin.H{
			"allowed": ValidStatuses,
		})
		return
	}
	updated, err := h.svc.UpdateStatus(c.Request.Context(), id, req.Status)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *Handler) listEvents(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid id", nil)
		return
	}
	page, err := parseIntDefault(c.Query("page"), DefaultPage)
	if err != nil {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid page", nil)
		return
	}
	limit, err := parseIntDefault(c.Query("limit"), DefaultLimit)
	if err != nil {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid limit", nil)
		return
	}
	resp, err := h.svc.ListEvents(c.Request.Context(), id, page, limit)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) stats(c *gin.Context) {
	stats, err := h.svc.Stats(c.Request.Context())
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func handleServiceError(c *gin.Context, err error) {
	if errors.Is(err, ErrNotFound) {
		apierr.Write(c, http.StatusNotFound, apierr.CodeNotFound, "not found", nil)
		return
	}
	if errors.Is(err, ErrInvalidInput) {
		apierr.Write(c, http.StatusBadRequest, apierr.CodeValidation, "invalid request", gin.H{"reason": err.Error()})
		return
	}
	apierr.Write(c, http.StatusInternalServerError, apierr.CodeInternal, "internal error", nil)
}

func parseIntDefault(s string, def int) (int, error) {
	if s == "" {
		return def, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return v, nil
}
