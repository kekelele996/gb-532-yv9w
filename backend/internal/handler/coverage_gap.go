package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"sonar-survey-coverage-planner/backend/internal/dto"
	"sonar-survey-coverage-planner/backend/internal/service"
	"sonar-survey-coverage-planner/backend/pkg/api"
)

type CoverageGapHandler struct{ service *service.CoverageGapService }

func NewCoverageGapHandler(service *service.CoverageGapService) *CoverageGapHandler {
	return &CoverageGapHandler{service: service}
}
func (h *CoverageGapHandler) List(c *gin.Context) {
	page, size := pageQuery(c)
	query := dto.CoverageGapQuery{SurveyAreaID: uintQuery(c, "survey_area_id"), State: cleanQuery(c, "state"), Severity: cleanQuery(c, "severity"), Page: page, PageSize: size}
	switch strings.TrimSpace(c.Query("claim")) {
	case "unclaimed":
		query.Unclaimed = true
	case "mine":
		query.AssigneeID = actorFrom(c).UserID
	}
	if assignee := uintQuery(c, "assignee_id"); assignee > 0 {
		query.AssigneeID = assignee
		query.Unclaimed = false
	}
	items, total, err := h.service.List(query)
	if err != nil {
		writeServiceError(c, err, "覆盖缺口")
		return
	}
	api.Page(c, items, page, size, total)
}
func (h *CoverageGapHandler) Get(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	item, err := h.service.Get(id)
	if err != nil {
		writeServiceError(c, err, "覆盖缺口")
		return
	}
	api.Success(c, http.StatusOK, item)
}
func (h *CoverageGapHandler) Detect(c *gin.Context) {
	var request dto.DetectCoverageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		api.BindError(c, err)
		return
	}
	item, err := h.service.Detect(request, strings.TrimSpace(c.GetHeader("Idempotency-Key")), actorFrom(c))
	if err != nil {
		writeServiceError(c, err, "覆盖缺口")
		return
	}
	status := http.StatusCreated
	if item.Idempotent {
		status = http.StatusOK
	}
	api.Success(c, status, item)
}

// bindOptionalJSON 允许空请求体（认领接口支持一键认领，不传字段）。
func bindOptionalJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil && !errors.Is(err, io.EOF) {
		api.BindError(c, err)
		return false
	}
	return true
}

func (h *CoverageGapHandler) Claim(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var request dto.GapClaimRequest
	if !bindOptionalJSON(c, &request) {
		return
	}
	item, err := h.service.Claim(id, request.ExpectedVersion, actorFrom(c))
	if err != nil {
		writeServiceError(c, err, "覆盖缺口")
		return
	}
	api.Success(c, http.StatusOK, item)
}

func (h *CoverageGapHandler) Release(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var request dto.GapReleaseRequest
	if !bindOptionalJSON(c, &request) {
		return
	}
	item, err := h.service.Release(id, request.ExpectedVersion, request.Note, actorFrom(c))
	if err != nil {
		writeServiceError(c, err, "覆盖缺口")
		return
	}
	api.Success(c, http.StatusOK, item)
}

func (h *CoverageGapHandler) Transition(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var request dto.GapTransitionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		api.BindError(c, err)
		return
	}
	item, err := h.service.Transition(id, request, actorFrom(c))
	if err != nil {
		writeServiceError(c, err, "覆盖缺口")
		return
	}
	api.Success(c, http.StatusOK, item)
}
