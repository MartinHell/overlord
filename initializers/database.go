package initializers

import (
	"os"

	"github.com/MartinHell/overlord/logs"
	// glebarez/sqlite is a pure Go driver. The gorm.io/driver/sqlite one wraps
	// mattn/go-sqlite3, which needs cgo and therefore a C toolchain — that made
	// SQLite unusable for local development and for chart testing in CI.
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectToDB() {
	var err error

	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "postgres"
	}

	dsn := os.Getenv("DB_URL")

	switch dbType {
	case "postgres":
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			PrepareStmt:                              true,
			DisableForeignKeyConstraintWhenMigrating: true,
		})
	case "sqlite":
		DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
			PrepareStmt:                              true,
			DisableForeignKeyConstraintWhenMigrating: true,
		})
	default:
		logs.Sugar.Fatalln("Unsupported database type")
	}

	if err != nil {
		logs.Sugar.Fatalf("Failed to connect to %s database: %v", dbType, err)
	}
}

func ApplyPreloads(db *gorm.DB) *gorm.DB {
	return db.Preload("Player").
		Preload("Initiator").
		Preload("Target").
		Preload("Target.Player").
		Preload("Target.Unit").
		Preload("Target.Weapon").
		Preload("Weapon")
}
