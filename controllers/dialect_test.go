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

// The real shot aggregation, three joins and a four-column GROUP BY. Postgres
// rejects any selected column missing from the GROUP BY, where SQLite would
// quietly return an arbitrary row, so this shape is the one most at risk.
func TestShotAggregateSQLIsPortable(t *testing.T) {
	for name, db := range dialects(t) {
		stmt := shotQuery(db).
			Order("players.player_name, units.type, weapons.type").
			Scan(&[]shotRow{}).Statement

		sql := stmt.SQL.String()
		t.Logf("%s: %s", name, sql)

		for _, want := range []string{"JOIN players", "JOIN units", "JOIN weapons", "GROUP BY", "COUNT(*)"} {
			if !strings.Contains(sql, want) {
				t.Errorf("%s: expected %q, got %s", name, want, sql)
			}
		}

		// Every non-aggregated selected column must also be grouped, or
		// Postgres will reject the query at execution time.
		groupBy := sql[strings.Index(sql, "GROUP BY"):]
		for _, col := range []string{"players.player_id", "players.player_name", "units.type", "weapons.type"} {
			if !strings.Contains(groupBy, col) {
				t.Errorf("%s: %q is selected but missing from GROUP BY: %s", name, col, groupBy)
			}
		}
	}
}

// Conditional counting has to use SUM(CASE ...) because COUNT(*) FILTER is not
// portable.
func TestTeamkillAggregateAvoidsFilter(t *testing.T) {
	for name, db := range dialects(t) {
		const expr = `CASE WHEN events.coalition IS NULL OR events.coalition = '' THEN 'unknown' ELSE events.coalition END`

		stmt := db.Model(&models.Event{}).
			Select(expr+` AS coalition, COUNT(*) AS kills,
				SUM(CASE WHEN events.coalition <> '' AND events.coalition <> 'unknown'
					AND events.target_coalition = events.coalition THEN 1 ELSE 0 END) AS teamkills`).
			Where("events.event = ?", "kill").
			Group(expr).
			Scan(&[]models.CoalitionKills{}).Statement

		sql := stmt.SQL.String()
		t.Logf("%s: %s", name, sql)

		if strings.Contains(sql, "FILTER (WHERE") {
			t.Errorf("%s: FILTER is not portable, use SUM(CASE WHEN ...)", name)
		}
		if !strings.Contains(sql, "SUM(CASE WHEN") {
			t.Errorf("%s: expected conditional counting via SUM(CASE WHEN ...), got %s", name, sql)
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
