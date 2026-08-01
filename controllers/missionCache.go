package controllers

import (
	"sync"
	"time"

	"github.com/MartinHell/overlord/models"
)

// The mission list is the one aggregate every page pays for.
//
// It is fetched outside the panel guards in the dashboard query, so it is not
// optional the way the weapons table or the map are: every page asks for it,
// every fifteen seconds, for as long as a tab is open. Three controllers --
// badges, sorties and the mission index -- call it again server-side, so a
// single request for /missions ran it twice.
//
// That would be unremarkable if it were cheap, but it is a LEFT JOIN over the
// whole events table grouped down to one row per mission: 298,318 rows in,
// 47 out, about 200ms. The cost that matters is not the latency. SQLite has a
// single writer and that writer is the event ingester, running live while
// people watch. Every open tab was adding a recurring full-table read against
// the file being written to, which is how a busy mission gets SQLITE_BUSY.
//
// So the read is memoised. Nothing here needs to be exact: the list is a
// dropdown, a heading and a mission count.

// missionsTTL is deliberately shorter than the dashboard's fifteen-second
// poll. The goal is not to make one tab's polling cheaper -- one tab is 200ms
// per fifteen seconds, which is nothing -- it is to stop N tabs from costing N
// times that. A window narrower than the poll interval collapses whatever
// overlaps without letting anyone see a list older than they would have seen
// anyway.
const missionsTTL = 10 * time.Second

var missionsCache struct {
	mu sync.Mutex
	// at zero means nothing cached, which is also how invalidation works.
	at   time.Time
	rows []models.MissionSummary
}

// GetMissions lists recorded missions, newest first.
//
// The lock is held across the query rather than only around the fields, so
// callers arriving on a cold cache queue behind one query instead of starting
// one each. Tabs opened together poll together, so that thundering herd is the
// realistic case, not a rare one -- and serialising reads against SQLite is
// what we want regardless.
func GetMissions() ([]*models.MissionSummary, error) {
	missionsCache.mu.Lock()
	defer missionsCache.mu.Unlock()

	if missionsCache.at.IsZero() || time.Since(missionsCache.at) > missionsTTL {
		rows, err := loadMissions()
		if err != nil {
			// Leave whatever was cached alone. A failed refresh is a reason to
			// serve a slightly old list, not to serve nothing.
			if missionsCache.at.IsZero() {
				return nil, err
			}
		} else {
			missionsCache.rows = rows
			missionsCache.at = time.Now()
		}
	}

	// Callers get their own copy. MissionSummary is all value types, so this is
	// a real copy rather than shared backing, and a caller that sorts or edits
	// the slice it was handed cannot poison what everyone else sees.
	out := make([]models.MissionSummary, len(missionsCache.rows))
	copy(out, missionsCache.rows)

	result := make([]*models.MissionSummary, len(out))
	for i := range out {
		result[i] = &out[i]
	}

	return result, nil
}

// invalidateMissions drops the cached list.
//
// Called when a mission starts and when one is named, which are the two changes
// worth seeing immediately: a run appearing, and it stopping being "Mission
// #48". Event counts and durations move constantly and are left to the TTL --
// they are a number beside a heading, and the page they sit on only redraws
// every fifteen seconds in any case.
func invalidateMissions() {
	missionsCache.mu.Lock()
	missionsCache.at = time.Time{}
	missionsCache.mu.Unlock()
}
