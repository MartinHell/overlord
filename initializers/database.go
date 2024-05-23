package initializers

import (
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectToDB() {
	var err error

	dbType := os.Getenv("DB_TYPE")
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
		log.Fatal("Unsupported database type")
	}

	if err != nil {
		log.Fatalf("Failed to connect to %s database: %v", dbType, err)
	}
}
