package service

import (
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"sonar-survey-coverage-planner/backend/internal/constants"
	"sonar-survey-coverage-planner/backend/internal/dto"
	"sonar-survey-coverage-planner/backend/internal/model"
	"sonar-survey-coverage-planner/backend/internal/repository"
	"sonar-survey-coverage-planner/backend/pkg/api"
)

const testReviewNote = "已核对坐标系、输入校验和与补测线建议，结论仅用于离线规划。"

type gapTestFixture struct {
	db        *gorm.DB
	service   *CoverageGapService
	audit     *AuditService
	area      model.SurveyArea
	run       model.SonarRun
	reviewers [2]model.User
}

func setupGapTestFixture(t *testing.T) gapTestFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:gap-svc-"+t.Name()+"?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.SurveyArea{}, &model.TransectPlan{}, &model.SonarRun{}, &model.CoverageGap{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("Sonar2026!"), bcrypt.MinCost)
	var reviewers [2]model.User
	for i, username := range []string{"reviewer-a", "reviewer-b"} {
		user := model.User{Username: username, DisplayName: username, PasswordHash: string(hash), Role: constants.RoleReviewer, Active: true}
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("create user %s: %v", username, err)
		}
		reviewers[i] = user
	}
	area := model.SurveyArea{AreaCode: "UT-A01", Name: "测试测区", BoundaryGeoJSON: []byte(`{"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[[[0,0],[1000,0],[1000,600],[0,600],[0,0]]]}}`), TargetResolutionM: 20, CoordinateSystem: "EPSG:32650", DefaultSwathM: 180, OwnerTeam: "测试组", Status: constants.AreaActive, Version: 1}
	if err := db.Create(&area).Error; err != nil {
		t.Fatalf("create area: %v", err)
	}
	plan := model.TransectPlan{SurveyAreaID: area.ID, Name: "基准测线", LineGeoJSON: []byte(`{"type":"Feature","properties":{},"geometry":{"type":"MultiLineString","coordinates":[[[40,100],[960,100]],[[40,300],[960,300]],[[40,500],[960,500]]]}}`), PlannedHeading: 90, PlannedSwathM: 180, LineSpacingM: 200, PlanState: constants.PlanLocked, Version: 1, CreatedBy: reviewers[0].ID}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	started := time.Now().UTC().Add(-2 * time.Hour)
	run := model.SonarRun{TransectPlanID: plan.ID, RunCode: "RUN-UT-001", TrackGeoJSON: []byte(`{"type":"Feature","properties":{},"geometry":{"type":"MultiLineString","coordinates":[[[30,100],[970,100]],[[30,300],[970,300]],[[30,500],[970,500]]]}}`), ActualSwathM: 170, StartedAt: started, EndedAt: started.Add(70 * time.Minute), NavigationQuality: constants.NavGood, RunState: string(constants.RunProcessed), SourceChecksum: "2d447d9407dc470d8c728ea8ed7f5f138d03dddcc6577bfd730b1cb63867a57b", ImportedBy: reviewers[0].ID, Version: 1}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	supportRepo := repository.NewSupportRepository(db)
	audit := NewAuditService(supportRepo)
	service := NewCoverageGapService(repository.NewCoverageGapRepository(db), repository.NewSurveyAreaRepository(db), repository.NewSonarRunRepository(db), audit)
	return gapTestFixture{db: db, service: service, audit: audit, area: area, run: run, reviewers: reviewers}
}

func (f gapTestFixture) actor(index int) Actor {
	user := f.reviewers[index]
	return Actor{RequestID: "test-request", UserID: user.ID, Username: user.Username, Role: user.Role}
}

func (f gapTestFixture) detectGap(t *testing.T, idempotencyKey string) model.CoverageGap {
	t.Helper()
	view, err := f.service.Detect(dto.DetectCoverageRequest{SurveyAreaID: f.area.ID, SourceRunIDs: []uint{f.run.ID}, AlgorithmVersion: "grid-cover-v1.0.0", ResolutionM: 20}, idempotencyKey, f.actor(0))
	if err != nil {
		t.Fatalf("detect coverage: %v", err)
	}
	return view.Gap
}

func appErrorCode(t *testing.T, err error) string {
	t.Helper()
	var appErr *api.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %v", err)
	}
	return appErr.Code
}

func (f gapTestFixture) auditCount(t *testing.T, action string) int64 {
	t.Helper()
	var count int64
	if err := f.db.Model(&model.AuditEvent{}).Where("action = ?", action).Count(&count).Error; err != nil {
		t.Fatalf("count audit events %s: %v", action, err)
	}
	return count
}

func TestDetectAssignsSeverityDeadline(t *testing.T) {
	fixture := setupGapTestFixture(t)
	gap := fixture.detectGap(t, "deadline-key-12345")
	if gap.ClaimedByID != nil {
		t.Fatal("new gap must be unclaimed")
	}
	want := constants.GapDeadline(constants.GapSeverity(gap.Severity))
	deadlineDelta := gap.DeadlineAt.Sub(gap.DetectedAt) - want
	if deadlineDelta > time.Second || deadlineDelta < -time.Second {
		t.Fatalf("deadline offset = %s, want %s", deadlineDelta, want)
	}
}

func TestClaimReleaseTransitionOwnership(t *testing.T) {
	fixture := setupGapTestFixture(t)
	gap := fixture.detectGap(t, "claim-workflow-key-12")

	// 先到先得：认领要求版本匹配，陈旧版本返回冲突
	if _, err := fixture.service.Claim(gap.ID, dto.GapClaimRequest{ExpectedVersion: gap.Version + 1}, fixture.actor(0)); err == nil {
		t.Fatal("stale claim must fail")
	} else if appErrorCode(t, err) != "VERSION_CONFLICT" {
		t.Fatalf("stale claim code = %s, want VERSION_CONFLICT", appErrorCode(t, err))
	}

	claimed, err := fixture.service.Claim(gap.ID, dto.GapClaimRequest{ExpectedVersion: gap.Version, ClaimNote: "我来复核"}, fixture.actor(0))
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if claimed.ClaimedByID == nil || *claimed.ClaimedByID != fixture.reviewers[0].ID || claimed.ClaimedAt == nil {
		t.Fatal("claim fields not persisted")
	}
	if claimed.ClaimedBy == nil || claimed.ClaimedBy.Username != "reviewer-a" {
		t.Fatal("claimant relation not loaded")
	}
	if claimed.Version != gap.Version+1 {
		t.Fatalf("version after claim = %d, want %d", claimed.Version, gap.Version+1)
	}
	if fixture.auditCount(t, "coverage.claim") != 1 {
		t.Fatal("claim must leave one audit event")
	}

	// 同一缺口只能认领一次：第二名复核员得到冲突
	_, err = fixture.service.Claim(gap.ID, dto.GapClaimRequest{ExpectedVersion: claimed.Version}, fixture.actor(1))
	if err == nil || appErrorCode(t, err) != "GAP_ALREADY_CLAIMED" {
		t.Fatalf("second claim code = %v, want GAP_ALREADY_CLAIMED", err)
	}

	// 只有认领人能推进状态或退回
	transitionRequest := dto.GapTransitionRequest{TargetState: "reviewed", ExpectedVersion: claimed.Version, ReviewNote: testReviewNote}
	if _, err := fixture.service.Transition(gap.ID, transitionRequest, fixture.actor(1)); err == nil || appErrorCode(t, err) != "GAP_CLAIM_FORBIDDEN" {
		t.Fatalf("foreign transition code = %v, want GAP_CLAIM_FORBIDDEN", err)
	}
	if _, err := fixture.service.Release(gap.ID, dto.GapReleaseRequest{ExpectedVersion: claimed.Version, Reason: "暂时无法完成，退回"}, fixture.actor(1)); err == nil || appErrorCode(t, err) != "GAP_CLAIM_FORBIDDEN" {
		t.Fatalf("foreign release code = %v, want GAP_CLAIM_FORBIDDEN", err)
	}

	released, err := fixture.service.Release(gap.ID, dto.GapReleaseRequest{ExpectedVersion: claimed.Version, Reason: "暂时无法完成，退回"}, fixture.actor(0))
	if err != nil {
		t.Fatalf("owner release: %v", err)
	}
	if released.ClaimedByID != nil || released.ClaimedAt != nil || released.ClaimNote != "" {
		t.Fatal("release must clear claim fields")
	}
	if fixture.auditCount(t, "coverage.release") != 1 {
		t.Fatal("release must leave one audit event")
	}

	// 待认领缺口不能直接推进状态
	if _, err := fixture.service.Transition(gap.ID, transitionRequest, fixture.actor(0)); err == nil || appErrorCode(t, err) != "GAP_CLAIM_REQUIRED" {
		t.Fatalf("unclaimed transition code = %v, want GAP_CLAIM_REQUIRED", err)
	}

	// 退回后其他复核员可重新认领，然后推进状态
	reclaimed, err := fixture.service.Claim(gap.ID, dto.GapClaimRequest{ExpectedVersion: released.Version}, fixture.actor(1))
	if err != nil {
		t.Fatalf("reclaim after release: %v", err)
	}
	reviewed, err := fixture.service.Transition(gap.ID, dto.GapTransitionRequest{TargetState: "reviewed", ExpectedVersion: reclaimed.Version, ReviewNote: testReviewNote}, fixture.actor(1))
	if err != nil {
		t.Fatalf("owner transition: %v", err)
	}
	if reviewed.GapState != "reviewed" {
		t.Fatalf("state = %s, want reviewed", reviewed.GapState)
	}
}

func TestRedetectSameInputKeepsClaim(t *testing.T) {
	fixture := setupGapTestFixture(t)
	gap := fixture.detectGap(t, "idempotent-claim-key-1")
	claimed, err := fixture.service.Claim(gap.ID, dto.GapClaimRequest{ExpectedVersion: gap.Version}, fixture.actor(0))
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := fixture.service.Transition(gap.ID, dto.GapTransitionRequest{TargetState: "reviewed", ExpectedVersion: claimed.Version, ReviewNote: testReviewNote}, fixture.actor(0)); err != nil {
		t.Fatalf("transition: %v", err)
	}

	view, err := fixture.service.Detect(dto.DetectCoverageRequest{SurveyAreaID: fixture.area.ID, SourceRunIDs: []uint{fixture.run.ID}, AlgorithmVersion: "grid-cover-v1.0.0", ResolutionM: 20}, "idempotent-claim-key-1", fixture.actor(1))
	if err != nil {
		t.Fatalf("redetect: %v", err)
	}
	if !view.Idempotent {
		t.Fatal("same input must be idempotent")
	}
	if view.Gap.ID != gap.ID {
		t.Fatal("same input must not create another gap")
	}
	if view.Gap.ClaimedByID == nil || *view.Gap.ClaimedByID != fixture.reviewers[0].ID {
		t.Fatal("redetect must not overwrite existing claim")
	}
	if view.Gap.GapState != "reviewed" {
		t.Fatal("redetect must not roll back review progress")
	}
	if fixture.auditCount(t, "coverage.detect") != 1 {
		t.Fatal("idempotent redetect must not write a second detect audit")
	}
}

func TestListOrdersUnclaimedByRemainingTime(t *testing.T) {
	fixture := setupGapTestFixture(t)
	now := time.Now().UTC()
	mkGap := func(severity constants.GapSeverity, detectedAgo time.Duration, claimant *uint) model.CoverageGap {
		detectedAt := now.Add(-detectedAgo)
		gap := model.CoverageGap{SurveyAreaID: fixture.area.ID, SourceRunIDs: []byte("[1]"), GapGeoJSON: []byte(`{"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[]}}`), Severity: string(severity), RecommendedLineGeoJSON: []byte(`{"type":"Feature","properties":{},"geometry":{"type":"LineString","coordinates":[]}}`), AlgorithmVersion: "grid-cover-v1.0.0", InputHash: "hash-" + string(severity) + "-" + detectedAgo.String(), GapState: string(constants.GapDetected), Explanation: "test", Version: 1, DetectedAt: detectedAt, DeadlineAt: detectedAt.Add(constants.GapDeadline(severity)), ClaimedByID: claimant}
		if err := fixture.db.Create(&gap).Error; err != nil {
			t.Fatalf("create gap: %v", err)
		}
		return gap
	}
	mkGap(constants.SeverityMinor, time.Hour, nil)                  // 剩余约 71 小时
	critical := mkGap(constants.SeverityCritical, 3*time.Hour, nil) // 剩余约 1 小时
	mkGap(constants.SeverityMajor, 20*time.Hour, nil)               // 剩余约 4 小时
	claimedID := fixture.reviewers[0].ID
	mkGap(constants.SeverityCritical, 5*time.Hour, &claimedID) // 已认领，排到最后

	items, total, err := fixture.service.List(dto.CoverageGapQuery{Claim: "", Page: 1, PageSize: 10})
	if err != nil || total != 4 {
		t.Fatalf("list total=%d err=%v", total, err)
	}
	if len(items) != 4 {
		t.Fatalf("got %d items", len(items))
	}
	// 待认领按剩余时间升序：临界 1h < 主要 4h < 轻微 71h，已认领沉底
	if items[0].ID != critical.ID {
		t.Fatalf("most urgent unclaimed gap must be first, got #%d (want #%d)", items[0].ID, critical.ID)
	}
	for i := 1; i < 3; i++ {
		if items[i].ClaimedByID != nil {
			t.Fatal("claimed gaps must follow unclaimed gaps")
		}
		if items[i].DeadlineAt.Before(items[i-1].DeadlineAt) {
			t.Fatalf("deadlines out of order at index %d", i)
		}
	}
	if items[3].ClaimedByID == nil {
		t.Fatal("claimed gap must sort last")
	}

	unclaimed, unclaimedTotal, err := fixture.service.List(dto.CoverageGapQuery{Claim: "unclaimed", Page: 1, PageSize: 10})
	if err != nil || unclaimedTotal != 3 || len(unclaimed) != 3 {
		t.Fatalf("unclaimed filter total=%d len=%d err=%v", unclaimedTotal, len(unclaimed), err)
	}
	claimedOnly, claimedTotal, err := fixture.service.List(dto.CoverageGapQuery{Claim: "claimed", Page: 1, PageSize: 10})
	if err != nil || claimedTotal != 1 || len(claimedOnly) != 1 {
		t.Fatalf("claimed filter total=%d len=%d err=%v", claimedTotal, len(claimedOnly), err)
	}
}

func TestClosedGapCannotBeClaimed(t *testing.T) {
	fixture := setupGapTestFixture(t)
	gap := fixture.detectGap(t, "closed-claim-key-12")
	claimed, err := fixture.service.Claim(gap.ID, dto.GapClaimRequest{ExpectedVersion: gap.Version}, fixture.actor(0))
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	// detected -> reviewed -> false_positive -> closed
	states := []struct{ state, note string }{
		{"reviewed", testReviewNote},
		{"false_positive", testReviewNote},
		{"closed", testReviewNote},
	}
	current := claimed
	for _, step := range states {
		current, err = fixture.service.Transition(gap.ID, dto.GapTransitionRequest{TargetState: step.state, ExpectedVersion: current.Version, ReviewNote: step.note}, fixture.actor(0))
		if err != nil {
			t.Fatalf("transition %s: %v", step.state, err)
		}
	}
	if _, err := fixture.service.Release(gap.ID, dto.GapReleaseRequest{ExpectedVersion: current.Version, Reason: "关闭后尝试退回"}, fixture.actor(0)); err == nil || appErrorCode(t, err) != "GAP_RELEASE_INVALID" {
		t.Fatalf("release closed code = %v, want GAP_RELEASE_INVALID", err)
	}
}

func TestConcurrentClaimFirstComeFirstServed(t *testing.T) {
	fixture := setupGapTestFixture(t)
	gap := fixture.detectGap(t, "concurrent-claim-key-1")

	const contenders = 8
	hash, _ := bcrypt.GenerateFromPassword([]byte("Sonar2026!"), bcrypt.MinCost)
	actors := make([]Actor, 0, contenders)
	for i := 0; i < contenders; i++ {
		user := model.User{Username: "reviewer-c" + string(rune('0'+i)), DisplayName: "c", PasswordHash: string(hash), Role: constants.RoleReviewer, Active: true}
		if err := fixture.db.Create(&user).Error; err != nil {
			t.Fatalf("create contender: %v", err)
		}
		actors = append(actors, Actor{RequestID: "test-request", UserID: user.ID, Username: user.Username, Role: user.Role})
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	winners := make(chan uint, contenders)
	for _, actor := range actors {
		wg.Add(1)
		go func(actor Actor) {
			defer wg.Done()
			<-start
			claimed, err := fixture.service.Claim(gap.ID, dto.GapClaimRequest{ExpectedVersion: gap.Version}, actor)
			if err == nil {
				winners <- *claimed.ClaimedByID
			}
		}(actor)
	}
	close(start)
	wg.Wait()
	close(winners)

	seen := map[uint]struct{}{}
	for winner := range winners {
		seen[winner] = struct{}{}
	}
	if len(seen) != 1 {
		t.Fatalf("concurrent claims produced %d winners, want exactly 1", len(seen))
	}
	reloaded, err := fixture.service.Get(gap.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.ClaimedByID == nil {
		t.Fatal("gap must remain claimed after concurrent attempts")
	}
	if _, ok := seen[*reloaded.ClaimedByID]; !ok {
		t.Fatal("persisted claimant is not among the concurrent winners")
	}
}
