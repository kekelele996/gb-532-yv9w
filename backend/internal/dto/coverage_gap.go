package dto

type DetectCoverageRequest struct {
	SurveyAreaID     uint    `json:"survey_area_id" binding:"required,gt=0"`
	SourceRunIDs     []uint  `json:"source_run_ids" binding:"required,min=1,dive,gt=0"`
	AlgorithmVersion string  `json:"algorithm_version" binding:"required,min=3,max=40"`
	ResolutionM      float64 `json:"resolution_m" binding:"omitempty,gt=0,lte=100"`
}

type GapTransitionRequest struct {
	TargetState     string `json:"target_state" binding:"required,oneof=reviewed accepted false_positive resurveyed closed"`
	ExpectedVersion uint   `json:"expected_version" binding:"required,gt=0"`
	ReviewNote      string `json:"review_note" binding:"required,min=8,max=600"`
}

// GapClaimRequest 认领支持空请求体；ExpectedVersion 大于 0 时做乐观锁校验。
type GapClaimRequest struct {
	ExpectedVersion uint `json:"expected_version" binding:"omitempty,gte=0"`
}

// GapReleaseRequest 认领人把缺口退回待认领池，note 为可选退回原因。
type GapReleaseRequest struct {
	ExpectedVersion uint   `json:"expected_version" binding:"omitempty,gte=0"`
	Note            string `json:"note" binding:"omitempty,max=600"`
}

type CoverageGapQuery struct {
	SurveyAreaID uint
	State        string
	Severity     string
	// Unclaimed=true 只看待认领；AssigneeID>0 只看指定复核员；AssigneeID 与 Unclaimed 互斥。
	Unclaimed  bool
	AssigneeID uint
	Page       int
	PageSize   int
}

type CoverageEvidence struct {
	InputHash            string  `json:"input_hash"`
	CoordinateSystem     string  `json:"coordinate_system"`
	AlgorithmVersion     string  `json:"algorithm_version"`
	SourceRunCount       int     `json:"source_run_count"`
	CoverageRatio        float64 `json:"coverage_ratio"`
	OverlapRatio         float64 `json:"overlap_ratio"`
	GapRatio             float64 `json:"gap_ratio"`
	FilteredFragments    int     `json:"filtered_fragments"`
	ProcessingMillis     int64   `json:"processing_millis"`
	DecisionBoundaryNote string  `json:"decision_boundary_note"`
}
