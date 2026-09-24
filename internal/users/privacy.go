package users

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/dto"
	"github.com/markbeep/hyl/internal/reqctx"
)

// maxPrivacyZones bounds how many zones one account can carry.
const maxPrivacyZones = 20

type privacyZoneRequest struct {
	Label   string  `json:"label"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	RadiusM int64   `json:"radiusM"`
}

// PrivacyZones lists the caller's circular privacy zones.
func (h *Handlers) PrivacyZones(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	rows, err := h.Q.ListPrivacyZones(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, privacyZoneList(rows))
}

// CreatePrivacyZone adds one zone.
func (h *Handlers) CreatePrivacyZone(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	var req privacyZoneRequest
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		return apperr.BadRequest("a zone needs a label")
	}
	if len(label) > 60 {
		return apperr.BadRequest("a zone label must be at most 60 characters")
	}
	if req.Lat < -90 || req.Lat > 90 || req.Lon < -180 || req.Lon > 180 {
		return apperr.BadRequest("the coordinates are outside the valid range")
	}
	if req.RadiusM <= 0 || req.RadiusM > maxTrimRadiusM {
		return apperr.BadRequest("the radius must be between 1 and %d metres", maxTrimRadiusM)
	}

	ctx := c.Request().Context()
	count, err := h.Q.CountPrivacyZones(ctx, user.ID)
	if err != nil {
		return err
	}
	if count >= maxPrivacyZones {
		return apperr.BadRequest("you can define at most %d privacy zones", maxPrivacyZones)
	}

	zone, err := h.Q.CreatePrivacyZone(ctx, user.ID, label, req.Lat, req.Lon, req.RadiusM, nowUnix())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, privacyZone(zone))
}

// DeletePrivacyZone removes one of the caller's zones.
func (h *Handlers) DeletePrivacyZone(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	zoneID, err := dto.IDParam(c, "id")
	if err != nil {
		return err
	}
	affected, err := h.Q.DeletePrivacyZone(c.Request().Context(), zoneID, user.ID)
	if err != nil {
		return err
	}
	if affected == 0 {
		return apperr.NotFound("no such privacy zone")
	}
	return c.NoContent(http.StatusNoContent)
}

func privacyZone(row db.PrivacyZone) api.PrivacyZone {
	return api.PrivacyZone{
		ID:      row.ID,
		Label:   row.Label,
		Lat:     row.Lat,
		Lon:     row.Lon,
		RadiusM: row.RadiusM,
	}
}

func privacyZoneList(rows []db.PrivacyZone) []api.PrivacyZone {
	out := make([]api.PrivacyZone, 0, len(rows))
	for _, row := range rows {
		out = append(out, privacyZone(row))
	}
	return out
}
