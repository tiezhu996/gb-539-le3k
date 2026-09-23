package model

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type DryingKiln struct {
	ID              string      `gorm:"primaryKey" json:"id"`
	KilnCode        string      `gorm:"uniqueIndex;not null" json:"kiln_code"`
	Name            string      `json:"name"`
	CapacityM3      float64     `json:"capacity_m3"`
	MaxTemperatureC float64     `json:"max_temperature_c"`
	MinHumidityPct  float64     `json:"min_humidity_pct"`
	AirflowClass    string      `json:"airflow_class"`
	OwnerTeam       string      `json:"owner_team"`
	KilnState       string      `json:"kiln_state"`
	CommissionedAt  time.Time   `json:"commissioned_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
	Lots            []TimberLot `gorm:"foreignKey:KilnID;references:ID" json:"lots,omitempty"`
}

type TimberLot struct {
	ID                 string     `gorm:"primaryKey" json:"id"`
	LotCode            string     `gorm:"uniqueIndex;not null" json:"lot_code"`
	KilnID             string     `gorm:"index;not null" json:"kiln_id"`
	Species            string     `json:"species"`
	ThicknessMM        float64    `json:"thickness_mm"`
	VolumeM3           float64    `json:"volume_m3"`
	InitialMoisturePct float64    `json:"initial_moisture_pct"`
	TargetMoisturePct  float64    `json:"target_moisture_pct"`
	QualityGrade       string     `json:"quality_grade"`
	LoadedAt           time.Time  `json:"loaded_at"`
	LotState           string     `json:"lot_state"`
	CreatedBy          string     `json:"created_by"`
	Version            int        `gorm:"not null;default:1" json:"version"`
	Kiln               DryingKiln `gorm:"foreignKey:KilnID;references:ID" json:"kiln,omitempty"`
}

type MoistureReading struct {
	ID             string     `gorm:"primaryKey" json:"id"`
	TimberLotID    string     `gorm:"index;not null" json:"timber_lot_id"`
	SamplePosition string     `json:"sample_position"`
	MeasuredAt     time.Time  `json:"measured_at"`
	MoisturePct    float64    `json:"moisture_pct"`
	DryBulbC       float64    `json:"dry_bulb_c"`
	WetBulbC       float64    `json:"wet_bulb_c"`
	SourceChecksum string     `json:"source_checksum"`
	ReadingQuality string     `json:"reading_quality"`
	QualityNote    string     `json:"quality_note"`
	ImportedBy     string     `json:"imported_by"`
	VoidedAt       *time.Time `json:"voided_at"`
	VoidedBy       string     `json:"voided_by"`
	VoidReason     string     `json:"void_reason"`
	SupersedesID   string     `gorm:"index" json:"supersedes_id"`
	Version        int        `gorm:"not null;default:1" json:"version"`
}

type DryingSchedule struct {
	ID                     string     `gorm:"primaryKey" json:"id"`
	TimberLotID            string     `gorm:"index;not null;uniqueIndex:idx_schedule_input" json:"timber_lot_id"`
	KilnSnapshot           string     `json:"kiln_snapshot"`
	AlgorithmVersion       string     `gorm:"uniqueIndex:idx_schedule_input" json:"algorithm_version"`
	InputHash              string     `gorm:"uniqueIndex:idx_schedule_input" json:"input_hash"`
	IdempotencyKey         string     `gorm:"index" json:"idempotency_key"`
	RuleSetVersion         string     `json:"rule_set_version"`
	StagesJSON             string     `json:"stages_json"`
	RecommendedChangesJSON string     `json:"recommended_changes_json"`
	RuleEvidenceJSON       string     `json:"rule_evidence_json"`
	PredictedFinishAt      *time.Time `json:"predicted_finish_at"`
	DefectRiskScore        float64    `json:"defect_risk_score"`
	ScheduleState          string     `json:"schedule_state"`
	Explanation            string     `json:"explanation"`
	FailureReason          string     `json:"failure_reason"`
	FrozenAt               *time.Time `json:"frozen_at"`
	FrozenBy               string     `json:"frozen_by"`
	FrozenSnapshot         string     `json:"frozen_snapshot"`
	BaselineScheduleID     string     `gorm:"index" json:"baseline_schedule_id"`
	ComparisonJSON         string     `json:"comparison_json"`
	CalculatedAt           time.Time  `json:"calculated_at"`
	CreatedBy              string     `json:"created_by"`
	ReviewedBy             string     `json:"reviewed_by"`
	Version                int        `json:"version"`
}

type User struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	Email        string    `gorm:"uniqueIndex;not null" json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

type AuditEvent struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	RequestID  string    `gorm:"index" json:"request_id"`
	Entity     string    `gorm:"index" json:"entity"`
	EntityID   string    `json:"entity_id"`
	Action     string    `json:"action"`
	ActorID    string    `json:"actor_id"`
	BeforeJSON string    `json:"before_json"`
	AfterJSON  string    `json:"after_json"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

func HashPassword(value string) string {
	hashed, _ := bcrypt.GenerateFromPassword([]byte(value), bcrypt.DefaultCost)
	return string(hashed)
}

func CheckPassword(hash, value string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(value)) == nil
}
