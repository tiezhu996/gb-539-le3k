package dto

type LotCreate struct {
	LotCode            string  `json:"lot_code" binding:"required"`
	KilnID             string  `json:"kiln_id" binding:"required"`
	Species            string  `json:"species" binding:"required"`
	ThicknessMM        float64 `json:"thickness_mm" binding:"required,gt=0"`
	VolumeM3           float64 `json:"volume_m3" binding:"required,gt=0"`
	InitialMoisturePct float64 `json:"initial_moisture_pct" binding:"gte=0,lte=100"`
	TargetMoisturePct  float64 `json:"target_moisture_pct" binding:"gte=0,lte=100"`
	QualityGrade       string  `json:"quality_grade"`
}
type LotUpdate struct {
	QualityGrade string `json:"quality_grade"`
	LotState     string `json:"lot_state"`
	Version      int    `json:"version" binding:"required,gt=0"`
}
