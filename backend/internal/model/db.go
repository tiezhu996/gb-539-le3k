package model

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log"
	"time"
)

func Open(driver, dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector
	if driver == "postgres" {
		dialector = postgres.Open(dsn)
	} else {
		dialector = sqlite.Open(dsn)
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.AutoMigrate(&DryingKiln{}, &TimberLot{}, &MoistureReading{}, &DryingSchedule{}, &User{}, &AuditEvent{}); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return db, nil
}

func Seed(db *gorm.DB) error {
	var count int64
	if err := db.Model(&User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	users := []User{{ID: "u-admin", Email: "admin@kilncurve.local", Name: "系统管理员", PasswordHash: HashPassword("admin123"), Role: "admin"}, {ID: "u-engineer", Email: "engineer@kilncurve.local", Name: "窑炉工程师", PasswordHash: HashPassword("engineer123"), Role: "kiln_engineer"}, {ID: "u-analyst", Email: "analyst@kilncurve.local", Name: "质量分析师", PasswordHash: HashPassword("analyst123"), Role: "quality_analyst"}, {ID: "u-reviewer", Email: "reviewer@kilncurve.local", Name: "审核员", PasswordHash: HashPassword("reviewer123"), Role: "reviewer"}, {ID: "u-auditor", Email: "auditor@kilncurve.local", Name: "审计员", PasswordHash: HashPassword("auditor123"), Role: "auditor"}}
	for _, user := range users {
		if err := db.Create(&user).Error; err != nil {
			return err
		}
	}
	var kiln DryingKiln
	if err := db.Where("kiln_code = ?", "K-01").First(&kiln).Error; err == gorm.ErrRecordNotFound {
		kiln = DryingKiln{ID: "kiln-01", KilnCode: "K-01", Name: "一号窑", CapacityM3: 85, MaxTemperatureC: 72, MinHumidityPct: 32, AirflowClass: "均匀循环", OwnerTeam: "北区干燥组", KilnState: "ready", CommissionedAt: time.Now().AddDate(-3, 0, 0), UpdatedAt: time.Now()}
		return db.Create(&kiln).Error
	}
	return nil
}

func MustOpen(driver, dsn string) *gorm.DB {
	db, err := Open(driver, dsn)
	if err != nil {
		log.Fatal(err)
	}
	_ = Seed(db)
	return db
}
