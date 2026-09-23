package dto

type KilnCreate struct {
	KilnCode        string  `json:"kiln_code" binding:"required"`
	Name            string  `json:"name" binding:"required"`
	CapacityM3      float64 `json:"capacity_m3" binding:"required,gt=0"`
	MaxTemperatureC float64 `json:"max_temperature_c" binding:"required,gt=0"`
	MinHumidityPct  float64 `json:"min_humidity_pct" binding:"gte=0,lte=100"`
	AirflowClass    string  `json:"airflow_class"`
	OwnerTeam       string  `json:"owner_team"`
}
type KilnUpdate struct {
	Name            string  `json:"name"`
	MaxTemperatureC float64 `json:"max_temperature_c"`
	MinHumidityPct  float64 `json:"min_humidity_pct"`
	AirflowClass    string  `json:"airflow_class"`
	OwnerTeam       string  `json:"owner_team"`
	KilnState       string  `json:"kiln_state"`
}
