package service

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"sonar-survey-coverage-planner/backend/internal/constants"
	"sonar-survey-coverage-planner/backend/internal/dto"
	"sonar-survey-coverage-planner/backend/internal/model"
	"sonar-survey-coverage-planner/backend/internal/repository"
)

type gapFixture struct {
	db        *gorm.DB
	service   *CoverageGapService
	areaID    uint
	reviewer1 Actor
	reviewer2 Actor
	processor Actor
}

func newGapFixture(t *testing.T) gapFixture {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared&_foreign_keys=on"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	tables := []any{&model.User{}, &model.SurveyArea{}, &model.TransectPlan{}, &model.SonarRun{}, &model.CoverageGap{}, &model.AuditEvent{}}
	if err := db.AutoMigrate(tables...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := []model.User{
		{Username: "rv1", DisplayName: "复核员甲", Role: constants.RoleReviewer, Active: true},
		{Username: "rv2", DisplayName: "复核员乙", Role: constants.RoleReviewer, Active: true},
		{Username: "proc1", DisplayName: "处理员", Role: constants.RoleDataProcessor, Active: true},
	}
	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	area := model.SurveyArea{AreaCode: "AREA-" + t.Name(), Name: "测试测区", BoundaryGeoJSON: []byte(`{"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]}}`), TargetResolutionM: 20, CoordinateSystem: "EPSG:32650", DefaultSwathM: 180, OwnerTeam: "team", Status: constants.AreaActive, Version: 1}
	if err := db.Create(&area).Error; err != nil {
		t.Fatalf("create area: %v", err)
	}
	support := repository.NewSupportRepository(db)
	svc := NewCoverageGapService(repository.NewCoverageGapRepository(db), repository.NewSurveyAreaRepository(db), repository.NewSonarRunRepository(db), NewAuditService(support))
	return gapFixture{db: db, service: svc, areaID: area.ID,
		reviewer1: Actor{RequestID: "req-1", UserID: users[0].ID, Username: "rv1", Role: constants.RoleReviewer},
		reviewer2: Actor{RequestID: "req-2", UserID: users[1].ID, Username: "rv2", Role: constants.RoleReviewer},
		processor: Actor{RequestID: "req-3", UserID: users[2].ID, Username: "proc1", Role: constants.RoleDataProcessor},
	}
}

func TestReviewDeadlineBySeverity(t *testing.T) {
	detected := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		severity constants.GapSeverity
		want     time.Duration
	}{
		{constants.SeverityCritical, 4 * time.Hour},
		{constants.SeverityMajor, 24 * time.Hour},
		{constants.SeverityMinor, 72 * time.Hour},
	}
	for _, tc := range cases {
		if got := constants.ReviewDeadlineFor(tc.severity, detected).Sub(detected); got != tc.want {
			t.Fatalf("%s deadline = %v, want %v", tc.severity, got, tc.want)
		}
	}
}

func seedGap(t *testing.T, fixture gapFixture, severity constants.GapSeverity, detectedAt time.Time) model.CoverageGap {
	t.Helper()
	due := constants.ReviewDeadlineFor(severity, detectedAt)
	gap := model.CoverageGap{
		SurveyAreaID:           fixture.areaID,
		SourceRunIDs:           []byte(`[1]`),
		GapGeoJSON:             []byte(`{"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,1],[0,0]]]}}`),
		Severity:               string(severity),
		RecommendedLineGeoJSON: []byte(`{"type":"Feature","properties":{},"geometry":{"type":"LineString","coordinates":[[0,0],[1,1]]}}`),
		AlgorithmVersion:       "grid-cover-v1.0.0",
		InputHash:              "hash-" + string(severity) + detectedAt.Format("150405.000000000"),
		GapState:               string(constants.GapDetected),
		Explanation:            "fixture",
		DueAt:                  &due,
		Version:                1,
		DetectedAt:             detectedAt,
	}
	if err := fixture.db.Create(&gap).Error; err != nil {
		t.Fatalf("create gap: %v", err)
	}
	return gap
}

func TestUnclaimedListSortedByRemainingTime(t *testing.T) {
	fixture := newGapFixture(t)
	now := time.Now().UTC()
	// 严重项刚检测（剩余 4h）；主要项已挂 23h（剩余 1h）；轻微项已挂 70h（剩余 2h）。
	critical := seedGap(t, fixture, constants.SeverityCritical, now)
	major := seedGap(t, fixture, constants.SeverityMajor, now.Add(-23*time.Hour))
	minor := seedGap(t, fixture, constants.SeverityMinor, now.Add(-70*time.Hour))

	items, total, err := fixture.service.List(dto.CoverageGapQuery{Unclaimed: true, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("total = %d, len = %d, want 3", total, len(items))
	}
	wantOrder := []uint{major.ID, minor.ID, critical.ID}
	for i, want := range wantOrder {
		if items[i].ID != want {
			t.Fatalf("position %d = gap %d, want %d (按剩余时间升序)", i, items[i].ID, want)
		}
	}
}

func TestClaimFirstComeFirstServed(t *testing.T) {
	fixture := newGapFixture(t)
	gap := seedGap(t, fixture, constants.SeverityCritical, time.Now().UTC())

	claimed, err := fixture.service.Claim(gap.ID, 1, fixture.reviewer1)
	if err != nil {
		t.Fatalf("reviewer1 claim: %v", err)
	}
	if claimed.AssigneeID == nil || *claimed.AssigneeID != fixture.reviewer1.UserID || claimed.Version != 2 {
		t.Fatal("claim should assign reviewer1 and bump version")
	}
	if claimed.ClaimedAt == nil {
		t.Fatal("claimed_at should be recorded")
	}

	// 同一缺口不能被第二个复核员重复认领。
	if _, err := fixture.service.Claim(gap.ID, 0, fixture.reviewer2); err == nil {
		t.Fatal("second claim must fail")
	}
}

func TestOnlyAssigneeCanTransition(t *testing.T) {
	fixture := newGapFixture(t)
	gap := seedGap(t, fixture, constants.SeverityMajor, time.Now().UTC())
	if _, err := fixture.service.Claim(gap.ID, 1, fixture.reviewer1); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// 未认领人不能推进状态。
	if _, err := fixture.service.Transition(gap.ID, dto.GapTransitionRequest{TargetState: "reviewed", ExpectedVersion: 2, ReviewNote: "非认领人尝试推进，应被拒绝。"}, fixture.reviewer2); err == nil {
		t.Fatal("non-assignee transition must be rejected")
	}

	// 未认领缺口不能直接推进。
	other := seedGap(t, fixture, constants.SeverityMinor, time.Now().UTC())
	if _, err := fixture.service.Transition(other.ID, dto.GapTransitionRequest{TargetState: "reviewed", ExpectedVersion: 1, ReviewNote: "未认领直接推进，应被拒绝。"}, fixture.reviewer1); err == nil {
		t.Fatal("transition before claim must be rejected")
	}

	updated, err := fixture.service.Transition(gap.ID, dto.GapTransitionRequest{TargetState: "reviewed", ExpectedVersion: 2, ReviewNote: "已复核证据，接受补测建议。"}, fixture.reviewer1)
	if err != nil {
		t.Fatalf("assignee transition: %v", err)
	}
	if updated.GapState != string(constants.GapReviewed) || updated.Version != 3 {
		t.Fatalf("state = %s version = %d", updated.GapState, updated.Version)
	}
}

func TestReleaseReturnsGapToPoolAndRecordsAudit(t *testing.T) {
	fixture := newGapFixture(t)
	gap := seedGap(t, fixture, constants.SeverityCritical, time.Now().UTC())
	if _, err := fixture.service.Claim(gap.ID, 1, fixture.reviewer1); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// 非认领人不能退回。
	if _, err := fixture.service.Release(gap.ID, 2, "", fixture.reviewer2); err == nil {
		t.Fatal("non-assignee release must be rejected")
	}

	released, err := fixture.service.Release(gap.ID, 2, "暂时无法完成复核", fixture.reviewer1)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.AssigneeID != nil || released.ClaimedAt != nil {
		t.Fatal("release must clear assignee and claimed_at")
	}

	// 退回后进入待认领池，其他复核员可认领。
	items, _, err := fixture.service.List(dto.CoverageGapQuery{Unclaimed: true, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list unclaimed: %v", err)
	}
	if len(items) != 1 || items[0].ID != gap.ID {
		t.Fatal("released gap should be back in unclaimed pool")
	}
	if _, err := fixture.service.Claim(gap.ID, 0, fixture.reviewer2); err != nil {
		t.Fatalf("reviewer2 re-claim after release: %v", err)
	}

	// 认领与退回均需留下审计记录。
	var events []model.AuditEvent
	if err := fixture.db.Where("entity_type = ? AND entity_id = ?", "coverage_gap", gap.ID).Order("id ASC").Find(&events).Error; err != nil {
		t.Fatalf("query audits: %v", err)
	}
	actions := map[string]bool{}
	for _, event := range events {
		actions[event.Action] = true
	}
	if !actions["coverage.claim"] || !actions["coverage.release"] {
		t.Fatalf("audit actions = %v, want coverage.claim and coverage.release", actions)
	}
}

func TestDetectSetsDeadlineAndRecomputeKeepsClaim(t *testing.T) {
	fixture := newGapFixture(t)
	boundary := []byte(`{"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[[[0,0],[1000,0],[1000,600],[0,600],[0,0]]]}}`)
	area := model.SurveyArea{AreaCode: "T-A01", Name: "测试测区", BoundaryGeoJSON: boundary, TargetResolutionM: 20, CoordinateSystem: "EPSG:32650", DefaultSwathM: 180, OwnerTeam: "team", Status: constants.AreaActive, Version: 1}
	if err := fixture.db.Create(&area).Error; err != nil {
		t.Fatalf("create area: %v", err)
	}
	plan := model.TransectPlan{SurveyAreaID: area.ID, Name: "plan", LineGeoJSON: []byte(`{"type":"Feature","properties":{},"geometry":{"type":"MultiLineString","coordinates":[[[40,100],[960,100]]]}}`), PlannedHeading: 90, PlannedSwathM: 180, LineSpacingM: 200, PlanState: constants.PlanLocked, Version: 1, CreatedBy: fixture.processor.UserID}
	if err := fixture.db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	tracks := []byte(`{"type":"Feature","properties":{},"geometry":{"type":"MultiLineString","coordinates":[[[30,100],[970,100]]]}}`)
	run := model.SonarRun{TransectPlanID: plan.ID, RunCode: "RUN-T-001", TrackGeoJSON: tracks, ActualSwathM: 170, StartedAt: time.Now().Add(-2 * time.Hour), EndedAt: time.Now().Add(-1 * time.Hour), NavigationQuality: constants.NavGood, RunState: string(constants.RunProcessed), SourceChecksum: "1111111111111111111111111111111111111111111111111111111111111111", ImportedBy: fixture.processor.UserID, Version: 1}
	if err := fixture.db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}

	request := dto.DetectCoverageRequest{SurveyAreaID: area.ID, SourceRunIDs: []uint{run.ID}, AlgorithmVersion: "grid-cover-v1.0.0", ResolutionM: 20}
	first, err := fixture.service.Detect(request, "idem-key-0001", fixture.processor)
	if err != nil {
		t.Fatalf("first detect: %v", err)
	}
	if first.Idempotent || first.Gap.DueAt == nil {
		t.Fatal("new gap must have a review deadline")
	}
	severity := constants.GapSeverity(first.Gap.Severity)
	wantDue := constants.ReviewDeadlineFor(severity, first.Gap.DetectedAt)
	if !first.Gap.DueAt.Equal(wantDue) {
		t.Fatalf("due_at = %v, want %v for severity %s", first.Gap.DueAt, wantDue, severity)
	}

	if _, err := fixture.service.Claim(first.Gap.ID, 1, fixture.reviewer1); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// 相同输入重新计算走幂等路径，不能覆盖已有认领与状态。
	second, err := fixture.service.Detect(request, "idem-key-0002", fixture.processor)
	if err != nil {
		t.Fatalf("second detect: %v", err)
	}
	if !second.Idempotent || second.Gap.ID != first.Gap.ID {
		t.Fatal("recomputation with same input must return the existing gap")
	}
	if second.Gap.AssigneeID == nil || *second.Gap.AssigneeID != fixture.reviewer1.UserID {
		t.Fatal("recomputation must not overwrite the existing claim")
	}
	if second.Gap.Version != 2 {
		t.Fatalf("version = %d, recomputation must not bump version", second.Gap.Version)
	}
}
