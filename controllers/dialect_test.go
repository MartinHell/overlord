package controllers

import (
	"strings"
	"testing"

	"github.com/MartinHell/overlord/models"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// dialects returns a DryRun session per supported driver. DryRun builds the SQL
// without executing it, and sql.Open does not dial eagerly, so this needs no
// running Postgres.
func dialects(t *testing.T) map[string]*gorm.DB {
	t.Helper()

	pg, err := gorm.Open(
		postgres.New(postgres.Config{DSN: "host=localhost user=x dbname=x port=5432"}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true},
	)
	if err != nil {
		t.Fatalf("postgres dialector: %v", err)
	}

	lite, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("sqlite dialector: %v", err)
	}

	return map[string]*gorm.DB{"postgres": pg, "sqlite": lite}
}

// The aggregate rewrite in #46 replaces "load every row and count in Go" with a
// GROUP BY. This asserts the generated SQL stays portable across both drivers,
// which nothing else in the project checks.
func TestAggregateSQLIsPortable(t *testing.T) {
	for name, db := range dialects(t) {
		stmt := db.Model(&models.Event{}).
			Select("coalition, COUNT(*) AS kills").
			Where("event = ?", "kill").
			Group("coalition").
			Find(&[]struct {
				Coalition string
				Kills     int
			}{}).Statement

		sql := stmt.SQL.String()
		t.Logf("%s: %s", name, sql)

		for _, want := range []string{"COUNT(*)", "GROUP BY", "coalition"} {
			if !strings.Contains(sql, want) {
				t.Errorf("%s: expected %q in generated SQL, got %s", name, want, sql)
			}
		}

		// Conditional aggregation via CASE works on both. COUNT(*) FILTER is
		// Postgres-first and unevenly supported by SQLite builds, so it must not
		// creep in.
		if strings.Contains(sql, "FILTER (WHERE") {
			t.Errorf("%s: FILTER is not portable, use SUM(CASE WHEN ...)", name)
		}
	}
}

// Keyset pagination from #34 is the other hand-written query shape.
func TestPaginationSQLIsPortable(t *testing.T) {
	for name, db := range dialects(t) {
		stmt := db.Model(&models.Event{}).
			Where("event = ?", "shot").
			Where("id < ?", 100).
			Order("id DESC").
			Limit(51).
			Find(&[]models.Event{}).Statement

		sql := stmt.SQL.String()
		t.Logf("%s: %s", name, sql)

		if !strings.Contains(sql, "ORDER BY") || !strings.Contains(sql, "LIMIT") {
			t.Errorf("%s: expected ORDER BY and LIMIT, got %s", name, sql)
		}
	}
}
