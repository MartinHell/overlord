package controllers

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
)

// The badge shelf. Career-wide on purpose: a badge is something you keep, so
// unlike the rest of the dashboard these never narrow to one mission, though
// most are earned by what happened within a single one.
//
// The engine works from one batch pull of the player's kills plus a handful of
// grouped aggregates, and derives every badge in Go. Eighteen badges from
// eighteen SQL queries would hammer the database for no reason; from one slice
// of kills, a gun kill, a long shot and a hat trick are all just loops.
//
// Thresholds are tuned for a small server. Every rule reads in one line; argue
// with the numbers freely.

// maxAwardsPerBadge caps the award list that travels to the client. Count
// carries the true number even when the list is trimmed.
const maxAwardsPerBadge = 12

// killRow is one kill by the player, with everything any badge wants to know.
type killRow struct {
	EventID         uint
	MissionID       *uint
	MissionTime     float64
	CreatedAt       time.Time
	WeaponType      string
	UnitType        string
	TargetType      string
	InitiatorLat    float64
	InitiatorLon    float64
	TargetLat       float64
	TargetLon       float64
	Coalition       string
	TargetCoalition string
}

// roleHas classifies a unit by its curated role text. Only curated types can
// match, which under-counts rather than mis-counts: an unlisted SAM earns
// nothing, a listed truck never counts as one.
func roleHas(unitType string, words ...string) bool {
	profile, _ := models.UnitProfile(unitType)
	role := strings.ToLower(profile.Role)
	for _, w := range words {
		if strings.Contains(role, w) {
			return true
		}
	}
	return false
}

// GetBadges computes the shelf for one player.
func GetBadges(playerID uint) ([]*models.Badge, error) {
	db := initializers.DB

	// Mission identities, for stamping awards with a name and a map.
	missions, err := GetMissions()
	if err != nil {
		return nil, err
	}
	missionByID := map[uint]*models.MissionSummary{}
	for _, m := range missions {
		missionByID[m.MissionID] = m
	}

	award := func(missionID *uint, when time.Time, detail string) *models.BadgeAward {
		a := &models.BadgeAward{When: when, Detail: detail}
		if missionID != nil {
			a.MissionID = *missionID
			if m := missionByID[*missionID]; m != nil {
				a.MissionName = m.Name
				a.Theatre = m.Theatre
			}
		}
		return a
	}

	// Every kill by this player, once.
	var kills []killRow
	if err := db.Model(&models.Event{}).
		Select(`events.id AS event_id,
			events.mission_id AS mission_id,
			events.mission_time AS mission_time,
			events.created_at AS created_at,
			weapons.type AS weapon_type,
			units.type AS unit_type,
			tunits.type AS target_type,
			events.initiator_lat AS initiator_lat,
			events.initiator_lon AS initiator_lon,
			events.target_lat AS target_lat,
			events.target_lon AS target_lon,
			events.coalition AS coalition,
			events.target_coalition AS target_coalition`).
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Joins("LEFT JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("LEFT JOIN units AS tunits ON tunits.unit_id = targets.unit_id").
		Joins("LEFT JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Where("events.event = ? AND events.player_id = ? AND targets.kind <> ?",
			"kill", playerID, models.ObjectKindScenery).
		Order("events.id").
		Scan(&kills).Error; err != nil {
		logs.Sugar.Errorf("Failed to load kills for player %d: %v", playerID, err)
		return nil, err
	}

	// Whose was the first kill of each mission: min kill id per mission,
	// matched against this player's kill ids.
	var firstKillIDs []uint
	if err := db.Model(&models.Event{}).
		Select("MIN(events.id)").
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Where("events.event = ? AND targets.kind <> ? AND events.mission_id IS NOT NULL",
			"kill", models.ObjectKindScenery).
		Group("events.mission_id").
		Scan(&firstKillIDs).Error; err != nil {
		logs.Sugar.Errorf("Failed to find first kills: %v", err)
		return nil, err
	}
	firstKillSet := map[uint]bool{}
	for _, id := range firstKillIDs {
		firstKillSet[id] = true
	}

	// Career totals the kill slice cannot provide.
	var totals struct {
		Shots, Takeoffs int
	}
	if err := db.Model(&models.Event{}).
		Select(`SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event IN ('takeoff','runway_takeoff') THEN 1 ELSE 0 END) AS takeoffs`).
		Where("events.player_id = ?", playerID).
		Scan(&totals).Error; err != nil {
		logs.Sugar.Errorf("Failed to total badge stats for player %d: %v", playerID, err)
		return nil, err
	}

	// Loss events with their ids, for the Reaper streak: a kill streak only
	// counts while the pilot stays alive, so kills and losses have to be laid
	// on one timeline. Event id is that timeline -- same ordering the kill
	// slice uses.
	var lossIDs []uint
	if err := db.Model(&models.Event{}).
		Select("events.id").
		Where("events.player_id = ? AND events.event IN ?",
			playerID, []string{"crash", "pilot_dead", "ejection"}).
		Order("events.id").
		Scan(&lossIDs).Error; err != nil {
		logs.Sugar.Errorf("Failed to load losses for player %d: %v", playerID, err)
		return nil, err
	}

	// Per-mission sorties, losses and ejections.
	var perMission []struct {
		MissionID uint
		Sorties   int
		Losses    int
		Ejections int
	}
	if err := db.Model(&models.Event{}).
		Select(`events.mission_id AS mission_id,
			SUM(CASE WHEN events.event IN ('takeoff','runway_takeoff') THEN 1 ELSE 0 END) AS sorties,
			SUM(CASE WHEN events.event IN ('crash','pilot_dead','ejection') THEN 1 ELSE 0 END) AS losses,
			SUM(CASE WHEN events.event = 'ejection' THEN 1 ELSE 0 END) AS ejections`).
		Where("events.player_id = ? AND events.mission_id IS NOT NULL", playerID).
		Group("events.mission_id").
		Order("events.mission_id").
		Scan(&perMission).Error; err != nil {
		logs.Sugar.Errorf("Failed to group missions for player %d: %v", playerID, err)
		return nil, err
	}

	// Individual carrier traps, for per-trap awards. Loose match, documented
	// unvalidated: this server has never recorded a graded landing.
	var traps []struct {
		MissionID *uint
		CreatedAt time.Time
		Comment   string
	}
	if err := db.Model(&models.Event{}).
		Select("events.mission_id AS mission_id, events.created_at AS created_at, events.comment AS comment").
		Where("events.event = ? AND events.player_id = ? AND events.comment LIKE ? AND events.comment LIKE ?",
			"landing_quality_mark", playerID, "%OK%", "%3%").
		Order("events.id").
		Scan(&traps).Error; err != nil {
		logs.Sugar.Errorf("Failed to load traps for player %d: %v", playerID, err)
		return nil, err
	}

	// Collateral, for the joke shelf.
	collateral, err := GetCollateral(&playerID, nil)
	if err != nil {
		return nil, err
	}

	// ---- derive the repeatable badges from the kill slice ----

	type earned struct {
		awards []*models.BadgeAward
	}
	byBadge := map[string]*earned{}
	add := func(id string, a *models.BadgeAward) {
		e := byBadge[id]
		if e == nil {
			e = &earned{}
			byBadge[id] = e
		}
		e.awards = append(e.awards, a)
	}

	killsPerMission := map[uint][]killRow{}
	longestGunM := 0.0

	for _, k := range kills {
		if k.MissionID != nil {
			killsPerMission[*k.MissionID] = append(killsPerMission[*k.MissionID], k)
		}

		target := models.SceneryName(k.TargetType)
		if p, curated := models.UnitProfile(k.TargetType); curated {
			target = p.Name
		}

		isGun := strings.HasPrefix(k.WeaponType, "weapons.shells.")

		if firstKillSet[k.EventID] {
			add("first-blood", award(k.MissionID, k.CreatedAt, "First kill: "+target))
		}
		if isGun {
			add("guns", award(k.MissionID, k.CreatedAt, "Gunned down "+target))
		}
		if k.WeaponType != "" && k.WeaponType == k.UnitType {
			add("ramming-speed", award(k.MissionID, k.CreatedAt, "Rammed "+target))
		}
		if k.InitiatorLat != 0 && k.TargetLat != 0 {
			d := haversineM(k.InitiatorLat, k.InitiatorLon, k.TargetLat, k.TargetLon)
			if d >= 27780 { // 15 nm
				add("long-shot", award(k.MissionID, k.CreatedAt,
					fmt.Sprintf("%.1f nm on %s", d/1852, target)))
			}
			if isGun {
				if d > longestGunM {
					longestGunM = d
				}
				if d >= 3704 { // 2 nm
					add("deadeye", award(k.MissionID, k.CreatedAt,
						fmt.Sprintf("Guns at %.1f nm on %s", d/1852, target)))
				}
			}
		}
		if roleHas(k.TargetType, "air defence", "surface-to-air") {
			add("sam-slayer", award(k.MissionID, k.CreatedAt, "Destroyed "+target))
		}
		if roleHas(k.TargetType, "helicopter") {
			add("helo-hunter", award(k.MissionID, k.CreatedAt, "Downed "+target))
		}
		if k.Coalition != "" && k.Coalition != models.CoalitionUnknown && k.Coalition == k.TargetCoalition {
			add("friendly-reminder", award(k.MissionID, k.CreatedAt, "That was a "+target))
		}
	}

	// Hat trick: three kills inside sixty seconds of mission clock, windows
	// not overlapping so five fast kills are one hat trick, not three. Blitz
	// runs the same sweep with a five-kill window over five minutes.
	for mid, ks := range killsPerMission {
		timed := make([]killRow, 0, len(ks))
		for _, k := range ks {
			if k.MissionTime > 0 {
				timed = append(timed, k)
			}
		}
		sort.Slice(timed, func(i, j int) bool { return timed[i].MissionTime < timed[j].MissionTime })

		for i := 0; i+2 < len(timed); {
			span := timed[i+2].MissionTime - timed[i].MissionTime
			if span <= 60 {
				m := mid
				add("hat-trick", award(&m, timed[i+2].CreatedAt,
					fmt.Sprintf("3 kills in %.0f seconds", span)))
				i += 3
			} else {
				i++
			}
		}

		for i := 0; i+4 < len(timed); {
			span := timed[i+4].MissionTime - timed[i].MissionTime
			if span <= 300 {
				m := mid
				add("blitz", award(&m, timed[i+4].CreatedAt,
					fmt.Sprintf("5 kills in %.0f seconds", span)))
				i += 5
			} else {
				i++
			}
		}
	}

	// Ace, double ace and triple ace, per mission.
	bestKills, bestMission := 0, uint(0)
	for mid, ks := range killsPerMission {
		if n := len(ks); n > bestKills {
			bestKills, bestMission = n, mid
		}
		if len(ks) >= 5 {
			m := mid
			when := ks[len(ks)-1].CreatedAt
			add("ace", award(&m, when, fmt.Sprintf("%d kills", len(ks))))
			if len(ks) >= 10 {
				add("double-ace", award(&m, when, fmt.Sprintf("%d kills", len(ks))))
			}
			if len(ks) >= 15 {
				add("triple-ace", award(&m, when, fmt.Sprintf("%d kills", len(ks))))
			}
		}
	}

	// Reaper: twenty kills in a row with no loss in between, walked on the
	// event-id timeline so a death in one mission breaks a streak begun in
	// another. A forty-kill run is two streaks, not one badge and change.
	maxRun, run := 0, 0
	li := 0
	for _, k := range kills {
		for li < len(lossIDs) && lossIDs[li] < k.EventID {
			run = 0
			li++
		}
		run++
		if run > maxRun {
			maxRun = run
		}
		if run%20 == 0 {
			add("reaper", award(k.MissionID, k.CreatedAt,
				fmt.Sprintf("%d kills without dying", run)))
		}
	}

	// Clean sheets, untouchables and ejections, per mission.
	cleanBest, untouchableBest := 0, 0
	for _, m := range perMission {
		mid := m.MissionID
		started := time.Time{}
		if s := missionByID[mid]; s != nil {
			started = s.StartedAt
		}
		if m.Sorties >= 5 && m.Losses == 0 {
			add("clean-sheet", award(&mid, started, fmt.Sprintf("%d sorties, nothing lost", m.Sorties)))
		}
		if m.Sorties > cleanBest && m.Losses == 0 {
			cleanBest = m.Sorties
		}
		if m.Losses == 0 {
			ks := killsPerMission[mid]
			if len(ks) > untouchableBest {
				untouchableBest = len(ks)
			}
			if len(ks) >= 10 {
				add("untouchable", award(&mid, ks[len(ks)-1].CreatedAt,
					fmt.Sprintf("%d kills, came home untouched", len(ks))))
			}
		}
		if m.Ejections > 0 {
			add("nylon-letdown", award(&mid, started, fmt.Sprintf("%d ejection(s)", m.Ejections)))
		}
	}

	for _, t := range traps {
		add("three-wire", award(t.MissionID, t.CreatedAt, t.Comment))
	}

	// ---- assemble the shelf ----

	totalEjections := 0
	for _, m := range perMission {
		totalEjections += m.Ejections
	}

	marksmanRatio := 0.0
	if totals.Shots > 0 {
		marksmanRatio = float64(len(kills)) / float64(totals.Shots)
	}

	build := func(id, name, emoji, desc string, target int, progress int, lockedDetail string) *models.Badge {
		e := byBadge[id]
		b := &models.Badge{
			ID: id, Name: name, Emoji: emoji, Description: desc,
			Target: target, Detail: lockedDetail,
		}
		if e != nil && len(e.awards) > 0 {
			b.Earned = true
			b.Count = len(e.awards)
			// Newest first for the dialog.
			sort.Slice(e.awards, func(i, j int) bool { return e.awards[i].When.After(e.awards[j].When) })
			if len(e.awards) > maxAwardsPerBadge {
				b.Awards = e.awards[:maxAwardsPerBadge]
			} else {
				b.Awards = e.awards
			}
			b.Detail = b.Awards[0].Detail
			b.Progress = target
		} else {
			b.Progress = progress
			if b.Progress > target {
				b.Progress = target
			}
		}
		return b
	}

	// Career thresholds keep their own construction: earned at most once, and
	// their progress is the career number itself.
	career := func(id, name, emoji, desc string, earned bool, progress, target int, detail string) *models.Badge {
		if progress > target {
			progress = target
		}
		count := 0
		if earned {
			count = 1
		}
		return &models.Badge{
			ID: id, Name: name, Emoji: emoji, Description: desc,
			Earned: earned, Count: count, Progress: progress, Target: target, Detail: detail,
		}
	}

	shelf := []*models.Badge{
		build("first-blood", "First Blood", "🩸", "Score the first kill of a mission.", 1, 0, "No first kills yet"),
		build("ace", "Ace", "🎖️", "Five kills in a single mission.", 5, bestKills,
			fmt.Sprintf("Best so far: %d kills in mission #%d", bestKills, bestMission)),
		build("double-ace", "Double Ace", "🏅", "Ten kills in a single mission.", 10, bestKills,
			fmt.Sprintf("Best so far: %d kills in mission #%d", bestKills, bestMission)),
		build("triple-ace", "Triple Ace", "👑", "Fifteen kills in a single mission.", 15, bestKills,
			fmt.Sprintf("Best so far: %d kills in mission #%d", bestKills, bestMission)),
		build("hat-trick", "Hat Trick", "🎩", "Three kills inside sixty seconds.", 1, 0, "Not yet"),
		build("blitz", "Blitz", "⚡", "Five kills inside five minutes.", 1, 0, "Not yet"),
		build("guns", "Guns Guns Guns", "🔫", "A kill with the cannon.", 1, 0, "No gun kills yet"),
		build("long-shot", "Long Shot", "📏",
			"A kill from more than fifteen nautical miles out. Only kills with recorded positions can qualify.",
			1, 0, "Nothing beyond 15 nm yet"),
		build("deadeye", "Deadeye", "🦅",
			"A gun kill from beyond two nautical miles. Only kills with recorded positions can qualify.",
			1, 0, deadeyeLocked(longestGunM)),
		build("sam-slayer", "SAM Slayer", "📡", "Destroy an air-defence unit.", 1, 0, "No SAMs yet"),
		build("helo-hunter", "Helo Hunter", "🚁", "Shoot down a helicopter.", 1, 0, "No helicopters yet"),
		build("ramming-speed", "Ramming Speed", "🐏", "A kill where the weapon was your own aircraft.", 1, 0, "Airframe intact so far"),
		build("friendly-reminder", "Friendly Reminder", "🫣", "A teamkill. Nobody talks about it, except this badge.", 1, 0, "Clean record"),
		career("marksman", "Marksman", "🎯", "A kill for every other shot, over at least twenty shots.",
			totals.Shots >= 20 && marksmanRatio >= 0.5,
			marksmanProgress(totals.Shots, marksmanRatio), 100,
			fmt.Sprintf("%.2f kills per shot over %d shots", marksmanRatio, totals.Shots)),
		career("centurion", "Centurion", "💯", "One hundred kills, career.",
			len(kills) >= 100, len(kills), 100, fmt.Sprintf("%d kills", len(kills))),
		build("three-wire", "OK 3-Wire", "🪝", "An OK-graded carrier pass on the three wire.", 1, 0, "No graded traps yet"),
		build("clean-sheet", "Clean Sheet", "🛬", "Five sorties in one mission without losing an aircraft.", 5, cleanBest,
			fmt.Sprintf("Best clean run: %d sorties", cleanBest)),
		build("untouchable", "Untouchable", "🛡️",
			"Ten kills in one mission without crashing, ejecting or dying.", 10, untouchableBest,
			fmt.Sprintf("Best clean mission: %d kills", untouchableBest)),
		build("reaper", "Reaper", "☠️",
			"Twenty kills in a row without dying once. A death anywhere resets the run.", 20, maxRun,
			fmt.Sprintf("Best streak: %d kills", maxRun)),
		career("frequent-flyer", "Frequent Flyer", "✈️", "Twenty-five takeoffs, career.",
			totals.Takeoffs >= 25, totals.Takeoffs, 25, fmt.Sprintf("%d takeoffs", totals.Takeoffs)),
		build("nylon-letdown", "Nylon Letdown", "🪂", "Ride the silk. Eject once.", 1, 0, "Dry so far"),
		career("tree-trimmer", "Tree Trimmer", "🌲", "Catch one hundred trees in your blasts.",
			collateral.Trees >= 100, collateral.Trees, 100, fmt.Sprintf("%d trees and shrubs", collateral.Trees)),
		career("urban-renewal", "Urban Renewal", "🧱", "Catch fifty walls, poles or buildings in your blasts.",
			collateral.Structures >= 50, collateral.Structures, 50, fmt.Sprintf("%d structures", collateral.Structures)),
	}

	// Nylon Letdown counts ejections, not missions containing them: the AI
	// that has ejected seventy-nine times has earned it seventy-nine times.
	// The award list stays grouped per mission for readability.
	for _, b := range shelf {
		if b.ID == "nylon-letdown" && b.Earned {
			b.Count = totalEjections
		}
	}

	return shelf, nil
}

// deadeyeLocked phrases the locked state around the player's longest ranged
// gun kill, so the badge taunts with how close they came.
func deadeyeLocked(longestM float64) string {
	if longestM <= 0 {
		return "No gun kills with positions yet"
	}
	return fmt.Sprintf("Longest gun kill: %.1f nm", longestM/1852)
}

// marksmanProgress maps the two gates onto one 0-100 bar: the sample size
// until twenty shots exist, then how much of the required ratio is reached.
func marksmanProgress(shots int, ratio float64) int {
	if shots < 20 {
		return shots * 100 / 20 / 2 // never past half while the sample is short
	}
	p := int(ratio / 0.5 * 100)
	if p > 100 {
		p = 100
	}
	return p
}
