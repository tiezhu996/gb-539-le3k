package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"timber-kiln-drying-optimizer/backend/internal/constants"
	"timber-kiln-drying-optimizer/backend/internal/dto"
	"timber-kiln-drying-optimizer/backend/internal/model"
	"timber-kiln-drying-optimizer/backend/internal/repository"
	"timber-kiln-drying-optimizer/backend/internal/util"
)

type ReadingService struct {
	Repo  repository.ReadingRepository
	Lots  repository.LotRepository
	Audit AuditService
}

type ReadingAssessment struct {
	ReadingID     string   `json:"reading_id"`
	Quality       string   `json:"quality"`
	Score         int      `json:"score"`
	Reasons       []string `json:"reasons"`
	RequireReview bool     `json:"require_review"`
}

type QualitySummary struct {
	Accepted       int      `json:"accepted"`
	Flagged        int      `json:"flagged"`
	ReviewRequired bool     `json:"review_required"`
	AverageScore   float64  `json:"average_score"`
	Reasons        []string `json:"reasons"`
}

func (s ReadingService) List(ctx context.Context, lotID string) ([]model.MoistureReading, error) {
	return s.Repo.List(ctx, lotID)
}

func (s ReadingService) Void(ctx context.Context, id string, input dto.ReadingVoid, actor, requestID string) (model.MoistureReading, error) {
	reading, err := s.Repo.Get(ctx, id)
	if err != nil {
		return reading, ErrNotFound
	}
	if input.Version != reading.Version || reading.ReadingQuality != "accepted" {
		return reading, ErrConflict
	}
	lot, err := s.Lots.Get(ctx, reading.TimberLotID)
	if err != nil {
		return reading, fmt.Errorf("lot: %w", ErrValidation)
	}
	if lot.LotState == constants.LotCompleted || lot.LotState == constants.LotAborted {
		return reading, fmt.Errorf("cannot void reading for terminal lot: %w", ErrConflict)
	}
	before := reading
	now := time.Now().UTC()
	updated, err := s.Repo.Void(ctx, id, input.Version, actor, input.Reason, now)
	if err != nil {
		return reading, err
	}
	if !updated {
		return reading, ErrConflict
	}
	reading.ReadingQuality, reading.VoidedAt, reading.VoidedBy, reading.VoidReason, reading.Version = "voided", &now, actor, input.Reason, reading.Version+1
	_ = s.Audit.Record(ctx, requestID, "reading", id, "voided", actor, before, reading)
	return reading, nil
}

func (s ReadingService) Correct(ctx context.Context, id string, input dto.ReadingImport, actor, requestID string, version int) ([]model.MoistureReading, error) {
	current, err := s.Repo.Get(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	if current.TimberLotID != input.TimberLotID || version != current.Version {
		return nil, ErrConflict
	}
	if len(input.Readings) == 0 {
		return nil, fmt.Errorf("replacement reading is required: %w", ErrValidation)
	}
	lot, err := s.Lots.Get(ctx, current.TimberLotID)
	if err != nil {
		return nil, fmt.Errorf("lot: %w", ErrValidation)
	}
	if lot.LotState == constants.LotCompleted || lot.LotState == constants.LotAborted {
		return nil, fmt.Errorf("cannot correct reading for terminal lot: %w", ErrConflict)
	}
	for index := range input.Readings {
		input.Readings[index].SupersedesID = current.ID
	}
	replacements, err := prepareReadings(lot, input, actor, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	summary := assessReadings(replacements)
	now := time.Now().UTC()
	if err := s.Repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated, txErr := s.Repo.VoidWithDB(ctx, tx, current.ID, version, actor, "replaced by corrected import", now)
		if txErr != nil {
			return txErr
		}
		if !updated {
			return ErrConflict
		}
		for _, replacement := range replacements {
			exists, txErr := s.Repo.ExistsChecksumWithDB(ctx, tx, lot.ID, replacement.SourceChecksum)
			if txErr != nil {
				return txErr
			}
			if exists {
				return fmt.Errorf("duplicate source reading: %w", ErrConflict)
			}
		}
		return s.Repo.CreateBatchWithDB(ctx, tx, replacements)
	}); err != nil {
		return nil, err
	}
	voided := current
	voided.ReadingQuality, voided.VoidedAt, voided.VoidedBy, voided.VoidReason, voided.Version = "voided", &now, actor, "replaced by corrected import", version+1
	_ = s.Audit.Record(ctx, requestID, "reading", current.ID, "corrected", actor, current, struct {
		Voided       model.MoistureReading
		Replacements []model.MoistureReading
		Quality      QualitySummary
	}{voided, replacements, summary})
	return replacements, nil
}

// Import validates the complete batch before writing so a partial set of samples
// cannot enter a schedule calculation. Times are strict RFC3339 rather than a
// best-effort conversion to the current clock.
func (s ReadingService) Import(ctx context.Context, input dto.ReadingImport, actor, requestID string) ([]model.MoistureReading, error) {
	lot, err := s.Lots.Get(ctx, input.TimberLotID)
	if err != nil {
		return nil, fmt.Errorf("lot: %w", ErrValidation)
	}
	if lot.LotState == constants.LotCompleted || lot.LotState == constants.LotAborted {
		return nil, fmt.Errorf("cannot import readings into terminal lot: %w", ErrConflict)
	}
	created, err := prepareReadings(lot, input, actor, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	summary := assessReadings(created)
	for _, item := range created {
		exists, checkErr := s.Repo.ExistsChecksum(ctx, lot.ID, item.SourceChecksum)
		if checkErr != nil {
			return nil, checkErr
		}
		if exists {
			return nil, fmt.Errorf("duplicate source reading: %w", ErrConflict)
		}
	}
	if err := s.Repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(&created).Error
	}); err != nil {
		return nil, err
	}
	_ = s.Audit.Record(ctx, requestID, "reading", input.TimberLotID, "imported", actor, nil, struct {
		Readings []model.MoistureReading
		Quality  QualitySummary
	}{created, summary})
	return created, nil
}

func assessReadings(readings []model.MoistureReading) QualitySummary {
	assessments := AssessBatch(readings)
	for index := range readings {
		readings[index].ReadingQuality = assessments[index].Quality
		readings[index].QualityNote = strings.Join(assessments[index].Reasons, "; ")
	}
	summary := SummarizeAssessments(assessments)
	if summary.ReviewRequired {
		for index := range readings {
			if readings[index].ReadingQuality == "accepted" {
				readings[index].QualityNote += "; batch contains flagged samples and requires analyst review"
			}
		}
	}
	return summary
}

func SummarizeAssessments(assessments []ReadingAssessment) QualitySummary {
	summary := QualitySummary{Reasons: []string{}}
	if len(assessments) == 0 {
		return summary
	}
	seen := map[string]bool{}
	for _, assessment := range assessments {
		summary.AverageScore += float64(assessment.Score)
		if assessment.Quality == "flagged" {
			summary.Flagged++
			summary.ReviewRequired = true
		} else {
			summary.Accepted++
		}
		for _, reason := range assessment.Reasons {
			if !seen[reason] {
				seen[reason] = true
				summary.Reasons = append(summary.Reasons, reason)
			}
		}
	}
	summary.AverageScore /= float64(len(assessments))
	sort.Strings(summary.Reasons)
	return summary
}

// AssessBatch records explainable data quality, while retaining flagged input
// for traceability. Safety calculation later treats flagged rows as anomalies.
func AssessBatch(readings []model.MoistureReading) []ReadingAssessment {
	results := make([]ReadingAssessment, len(readings))
	for index, reading := range readings {
		assessment := ReadingAssessment{ReadingID: reading.ID, Quality: "accepted", Score: 100}
		if reading.DryBulbC-reading.WetBulbC < 0.5 {
			assessment.Score -= 30
			assessment.Reasons = append(assessment.Reasons, "dry and wet bulb separation is implausibly small")
		}
		if reading.DryBulbC-reading.WetBulbC > 25 {
			assessment.Score -= 20
			assessment.Reasons = append(assessment.Reasons, "large bulb separation needs instrument check")
		}
		if reading.MoisturePct < 4 || reading.MoisturePct > 80 {
			assessment.Score -= 20
			assessment.Reasons = append(assessment.Reasons, "moisture is at an unusual operating extreme")
		}
		if reading.SamplePosition != "core" && reading.SamplePosition != "surface" {
			assessment.Score -= 50
			assessment.Reasons = append(assessment.Reasons, "sample position is not a controlled location")
		}
		if index > 0 {
			previous := readings[index-1]
			if reading.MeasuredAt.Sub(previous.MeasuredAt) == 0 && reading.SamplePosition == previous.SamplePosition {
				assessment.Score -= 40
				assessment.Reasons = append(assessment.Reasons, "duplicate position in the same sampling event")
			}
			if abs(reading.DryBulbC-previous.DryBulbC) > 12 {
				assessment.Score -= 15
				assessment.Reasons = append(assessment.Reasons, "temperature differs sharply from adjacent sample")
			}
		}
		if assessment.Score < 70 {
			assessment.Quality, assessment.RequireReview = "flagged", true
		}
		if len(assessment.Reasons) == 0 {
			assessment.Reasons = []string{"within import plausibility checks"}
		}
		results[index] = assessment
	}
	return results
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func prepareReadings(lot model.TimberLot, input dto.ReadingImport, actor string, now time.Time) ([]model.MoistureReading, error) {
	if len(input.Readings) == 0 || len(input.Readings) > 500 {
		return nil, fmt.Errorf("reading batch must contain 1..500 rows: %w", ErrValidation)
	}
	seenSamples := map[string]struct{}{}
	seenChecksums := map[string]struct{}{}
	created := make([]model.MoistureReading, 0, len(input.Readings))
	for _, row := range input.Readings {
		if row.TimberLotID != "" && row.TimberLotID != input.TimberLotID {
			return nil, fmt.Errorf("row lot does not match import lot: %w", ErrValidation)
		}
		position, err := normalizeSamplePosition(row.SamplePosition)
		if err != nil {
			return nil, err
		}
		measuredAt, err := time.Parse(time.RFC3339, row.MeasuredAt)
		if err != nil {
			return nil, fmt.Errorf("measured_at must be RFC3339: %w", ErrValidation)
		}
		measuredAt = measuredAt.UTC()
		if measuredAt.After(now.Add(5*time.Minute)) || (!lot.LoadedAt.IsZero() && measuredAt.Before(lot.LoadedAt.Add(-5*time.Minute))) {
			return nil, fmt.Errorf("measured_at is outside lot lifetime: %w", ErrValidation)
		}
		if row.MoisturePct < 0 || row.MoisturePct > 100 || row.DryBulbC < 0 || row.DryBulbC > 120 || row.WetBulbC < 0 || row.WetBulbC > row.DryBulbC || row.DryBulbC-row.WetBulbC > 35 {
			return nil, fmt.Errorf("reading physical bounds are invalid: %w", ErrValidation)
		}
		sampleKey := measuredAt.Format(time.RFC3339Nano) + "|" + position
		if _, duplicate := seenSamples[sampleKey]; duplicate {
			return nil, fmt.Errorf("duplicate sample position at measured_at: %w", ErrValidation)
		}
		seenSamples[sampleKey] = struct{}{}
		checksum := util.Hash(fmt.Sprintf("%s|%s|%s|%.4f|%.4f|%.4f", lot.ID, position, measuredAt.Format(time.RFC3339Nano), row.MoisturePct, row.DryBulbC, row.WetBulbC))
		if _, duplicate := seenChecksums[checksum]; duplicate {
			return nil, fmt.Errorf("duplicate reading content: %w", ErrValidation)
		}
		seenChecksums[checksum] = struct{}{}
		created = append(created, model.MoistureReading{ID: util.ID(), TimberLotID: lot.ID, SamplePosition: position, MeasuredAt: measuredAt, MoisturePct: row.MoisturePct, DryBulbC: row.DryBulbC, WetBulbC: row.WetBulbC, ReadingQuality: "accepted", ImportedBy: actor, SourceChecksum: checksum, SupersedesID: row.SupersedesID, Version: 1})
	}
	return created, nil
}

func normalizeSamplePosition(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "core", "center", "中心", "芯材":
		return "core", nil
	case "surface", "表层", "表面":
		return "surface", nil
	default:
		return "", fmt.Errorf("sample_position must be core or surface: %w", ErrValidation)
	}
}
