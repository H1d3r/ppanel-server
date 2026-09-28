package orm

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type uniqueRow struct {
	ID   int64  `gorm:"primaryKey"`
	Code string `gorm:"uniqueIndex"`
}

// Callers detect a lost race on a unique key with errors.Is(err,
// gorm.ErrDuplicatedKey); the application connection must translate driver
// errors for that to match.
func TestGormConfigTranslatesDuplicateKeys(t *testing.T) {
	cfg := (&Mysql{}).gormConfig()
	if !cfg.TranslateError {
		t.Fatal("the application connection does not translate driver errors")
	}
	db, err := gorm.Open(sqlite.Open("file:translate-errors?mode=memory&cache=shared"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&uniqueRow{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&uniqueRow{Code: "same"}).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Create(&uniqueRow{Code: "same"}).Error
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate insert error = %v, want gorm.ErrDuplicatedKey", err)
	}
}
