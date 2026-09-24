package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"sonar-survey-coverage-planner/backend/internal/constants"
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
	switch {
	case query.Unclaimed:
		// 待认领池：未被认领且尚未关闭，按剩余期限升序，越紧急越靠前。
		db = db.Where("assignee_id IS NULL").Where("gap_state <> ?", string(constants.GapClosed))
	case query.AssigneeID > 0:
		db = db.Where("assignee_id = ?", query.AssigneeID)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count coverage gaps: %w", err)
	}
	order := "detected_at DESC, id DESC"
	if query.Unclaimed || query.AssigneeID > 0 {
		// CASE 排序在 PostgreSQL 与 SQLite 上行为一致，避免依赖 NULLS 语法差异。
		order = "CASE WHEN due_at IS NULL THEN 1 ELSE 0 END, due_at ASC, id ASC"
	}
	var items []model.CoverageGap
	if err := db.Preload("SurveyArea").Preload("Assignee").Order(order).Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list coverage gaps: %w", err)
	}
	return items, total, nil
}

func (r *CoverageGapRepository) Get(id uint) (model.CoverageGap, error) {
	var item model.CoverageGap
	if err := r.db.Preload("SurveyArea").Preload("Assignee").First(&item, id).Error; err != nil {
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

// Claim 以条件更新实现先到先得：仅当缺口仍处于待认领状态时成功。
func (r *CoverageGapRepository) Claim(id, expectedVersion, assigneeID uint, claimedAt time.Time) (model.CoverageGap, error) {
	query := r.db.Model(&model.CoverageGap{}).
		Where("id = ?", id).
		Where("assignee_id IS NULL").
		Where("gap_state <> ?", string(constants.GapClosed))
	if expectedVersion > 0 {
		query = query.Where("version = ?", expectedVersion)
	}
	result := query.Updates(map[string]any{"assignee_id": assigneeID, "claimed_at": claimedAt, "version": gorm.Expr("version + 1")})
	if result.Error != nil {
		return model.CoverageGap{}, fmt.Errorf("claim coverage gap: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return model.CoverageGap{}, ErrVersionConflict
	}
	return r.Get(id)
}

// Release 仅允许当前认领人把缺口退回待认领池。
func (r *CoverageGapRepository) Release(id, expectedVersion, assigneeID uint, note string) (model.CoverageGap, error) {
	updates := map[string]any{"assignee_id": nil, "claimed_at": nil, "version": gorm.Expr("version + 1")}
	if trimmed := note; trimmed != "" {
		var item model.CoverageGap
		if err := r.db.Select("explanation").First(&item, id).Error; err != nil {
			return model.CoverageGap{}, fmt.Errorf("load coverage gap explanation: %w", err)
		}
		updates["explanation"] = item.Explanation + " 退回待认领：" + trimmed
	}
	query := r.db.Model(&model.CoverageGap{}).Where("id = ? AND assignee_id = ?", id, assigneeID)
	if expectedVersion > 0 {
		query = query.Where("version = ?", expectedVersion)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return model.CoverageGap{}, fmt.Errorf("release coverage gap: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return model.CoverageGap{}, ErrVersionConflict
	}
	return r.Get(id)
}

func (r *CoverageGapRepository) Transition(id, expectedVersion uint, from, to, explanation string) (model.CoverageGap, error) {
	result := r.db.Model(&model.CoverageGap{}).Where("id = ? AND version = ? AND gap_state = ?", id, expectedVersion, from).Updates(map[string]any{"gap_state": to, "explanation": explanation, "version": gorm.Expr("version + 1")})
	if result.Error != nil {
		return model.CoverageGap{}, fmt.Errorf("transition coverage gap: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return model.CoverageGap{}, ErrVersionConflict
	}
	return r.Get(id)
}
