package dto

type ScheduleCalculate struct {
	TimberLotID    string `json:"timber_lot_id" binding:"required"`
	IdempotencyKey string `json:"-"`
}
type ScheduleReview struct {
	Decision string `json:"decision" binding:"required"`
	Note     string `json:"note"`
	Version  int    `json:"version" binding:"required,gt=0"`
}

type ScheduleFreeze struct {
	Version int `json:"version" binding:"required,gt=0"`
}

type ScheduleCompare struct {
	BaselineScheduleID string `json:"baseline_schedule_id" binding:"required"`
}
