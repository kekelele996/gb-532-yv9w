package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"sonar-survey-coverage-planner/backend/internal/dto"
	"sonar-survey-coverage-planner/backend/internal/model"
)

type CoverageGapRepository struct{ db *gorm.DB }

func NewCoverageGapRepository(db *gorm.DB) *CoverageGapRepository {
	return &CoverageGapRepository{db: db}
}

func (r *CoverageGapRepository) List(query dto.CoverageGapQuery) ([]model.CoverageGap, int64, error) {
	db := r.db.Model(&model.CoverageGap{})
	if query.SurveyAreaID > 0 {
		db = db.Where("survey_area_id = ?", query.SurveyAreaID)
	}
	if query.State != "" {
		db = db.Where("gap_state = ?", query.State)
	}
	if query.Severity != "" {
		db = db.Where("severity = ?", query.Severity)
	}
	switch query.Claim {
	case "unclaimed":
		db = db.Where("claimed_by_id IS NULL")
	case "claimed":
		db = db.Where("claimed_by_id IS NOT NULL")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count coverage gaps: %w", err)
	}
	var items []model.CoverageGap
	// 待认领缺口按剩余期限升序置顶，已认领缺口排在其后，同等期限按检测先后排列
	if err := db.Preload("SurveyArea").Preload("ClaimedBy").Order("CASE WHEN claimed_by_id IS NULL THEN 0 ELSE 1 END, deadline_at ASC, id ASC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list coverage gaps: %w", err)
	}
	return items, total, nil
}

func (r *CoverageGapRepository) Get(id uint) (model.CoverageGap, error) {
	var item model.CoverageGap
	if err := r.db.Preload("SurveyArea").Preload("ClaimedBy").First(&item, id).Error; err != nil {
		return item, fmt.Errorf("get coverage gap: %w", err)
	}
	return item, nil
}

func (r *CoverageGapRepository) ByInputHash(hash string) (model.CoverageGap, error) {
	var item model.CoverageGap
	if err := r.db.Where("input_hash = ?", hash).First(&item).Error; err != nil {
		return item, fmt.Errorf("find coverage input hash: %w", err)
	}
	return item, nil
}

func (r *CoverageGapRepository) Create(item *model.CoverageGap) error {
	if err := r.db.Create(item).Error; err != nil {
		return fmt.Errorf("create coverage gap: %w", err)
	}
	return nil
}

func (r *CoverageGapRepository) Claim(id, expectedVersion, actorID uint, note string, now time.Time) (model.CoverageGap, error) {
	updates := map[string]any{"claimed_by_id": actorID, "claimed_at": now, "claim_note": note, "version": gorm.Expr("version + 1")}
	result := r.db.Model(&model.CoverageGap{}).Where("id = ? AND version = ? AND claimed_by_id IS NULL AND gap_state <> ?", id, expectedVersion, "closed").Updates(updates)
	if result.Error != nil {
		return model.CoverageGap{}, fmt.Errorf("claim coverage gap: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return model.CoverageGap{}, ErrVersionConflict
	}
	return r.Get(id)
}

func (r *CoverageGapRepository) Release(id, expectedVersion, actorID uint) (model.CoverageGap, error) {
	updates := map[string]any{"claimed_by_id": nil, "claimed_at": nil, "claim_note": "", "version": gorm.Expr("version + 1")}
	result := r.db.Model(&model.CoverageGap{}).Where("id = ? AND version = ? AND claimed_by_id = ? AND gap_state <> ?", id, expectedVersion, actorID, "closed").Updates(updates)
	if result.Error != nil {
		return model.CoverageGap{}, fmt.Errorf("release coverage gap: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return model.CoverageGap{}, ErrVersionConflict
	}
	return r.Get(id)
}

func (r *CoverageGapRepository) Transition(id, expectedVersion uint, from, to, explanation string, actorID uint) (model.CoverageGap, error) {
	result := r.db.Model(&model.CoverageGap{}).Where("id = ? AND version = ? AND gap_state = ? AND claimed_by_id = ?", id, expectedVersion, from, actorID).Updates(map[string]any{"gap_state": to, "explanation": explanation, "version": gorm.Expr("version + 1")})
	if result.Error != nil {
		return model.CoverageGap{}, fmt.Errorf("transition coverage gap: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return model.CoverageGap{}, ErrVersionConflict
	}
	return r.Get(id)
}
