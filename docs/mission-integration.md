# Mission integration: exporting tasks and scores to overlord

Overlord can show tasks and scores that the mission itself defines — who was
told to destroy the convoy, whether they did, and what it was worth. The
mission side of this is one Lua table. This document is everything the mission
scripter needs.

## The contract in one sentence

Keep a global table called `OVERLORD_EXPORT` up to date, and overlord will
poll it every ten seconds and display it.

## The table

```lua
OVERLORD_EXPORT = {
  version = 1,
  tasks = {
    {
      id     = "convoy-1",                     -- REQUIRED, stable, unique within the mission
      title  = "Destroy the supply convoy",    -- what the dashboard shows
      state  = "active",                       -- "active" | "done" | "failed" (your words; displayed, never interpreted)
      player = "Meekss",                       -- exact in-game player name, or "" for a mission-wide entry
      points = 0,                              -- integer; whatever your scoring says right now
    },
  },
}
```

Rules that matter:

- **It is a snapshot, not a log.** Every poll reads the whole table as it
  stands. Update entries in place; do not append "events". If a task is worth
  150 points once done, set `points = 150` when it is done.
- **`id` must be stable.** Overlord upserts by it. If the id changes
  mid-mission, the dashboard shows two tasks.
- **`player` is the exact in-game name.** Overlord matches it against the name
  DCS reports for the connected player (for example `Meekss`). Empty string
  means the entry belongs to the mission, not a pilot.
- **Missing table is fine.** A mission that never defines `OVERLORD_EXPORT`
  simply shows no task panel. Nothing errors.

## A worked example

```lua
-- Somewhere in mission init:
OVERLORD_EXPORT = { version = 1, tasks = {} }

local function upsertTask(t)
  for i, existing in ipairs(OVERLORD_EXPORT.tasks) do
    if existing.id == t.id then
      OVERLORD_EXPORT.tasks[i] = t
      return
    end
  end
  table.insert(OVERLORD_EXPORT.tasks, t)
end

-- When you assign the task:
upsertTask({
  id = "convoy-1",
  title = "Destroy the supply convoy",
  state = "active",
  player = "Meekss",
  points = 0,
})

-- In your ON_DEAD / goal logic, when the convoy is gone:
upsertTask({
  id = "convoy-1",
  title = "Destroy the supply convoy",
  state = "done",
  player = "Meekss",
  points = 150,
})
```

That is the entire mission-side implementation. No sockets, no files, no
DCS-gRPC calls from the mission script — overlord reaches in and reads the
table.

## Crews and shared tasks

A package with several humans exports **one entry per pilot** — `pkg-3-p1`,
`pkg-3-p2` — same title and state, each with its own `player`, ordinals
assigned on first sighting so ids never renumber. This is the shape the
dashboard wants: the panel sorts by points then id, so crew rows sit together,
and each pilot's page picks up exactly their rows. Do not merge a crew into
one entry with a combined name; that breaks per-pilot attribution.

Points on shared tasks are the mission's design decision — the dashboard sums
whatever it is given, per pilot, and shows no mission-wide total that would
require points to be conserved. Both conventions work mechanically.

Entries should outlive the thing that created them: keep publishing a closed
package as `done`/`failed` rather than dropping it, so a pilot's work does not
vanish at the moment it paid off.

Absence is information too: **an entry dropped from the export is removed from
the current mission's panel** on the next poll. The snapshot is the truth in
both directions — publish what should persist, drop what should disappear.
That also means "pilot left the package" is a real design choice: publish the
per-pilot entry as `failed` (or your word for it) to keep the record, or drop
it to make it vanish. Finished missions are never reconciled; they keep
whatever their final published set was.

## Player identity

Export the DCS player name; that is all the mission sandbox knows, and it is
enough. Overlord already keys players by UCID (obtained from the server hooks
side over the net API) and resolves each exported name to that stable identity
at ingest, while the pilot is connected — which is exactly when a live mission
publishes tasks about them. A later rename does not orphan old tasks: matching
is by resolved identity first, name only as a fallback for entries that never
resolved.

Two edges worth knowing. An entry published only after its pilot disconnected
may fail to resolve and falls back to name matching. And two simultaneously
connected players with the identical name are ambiguous — the first match
wins. Neither justifies a version 2; if UCID ever turns out to be reachable
from the sandbox, an optional field can be added without breaking version 1.

## Server configuration (one-time, already done on this machine)

Overlord reads the table via DCS-gRPC's `CustomService.Eval`, which is off by
default. In `Saved Games/DCS.openbeta/Config/dcs-grpc.lua`:

```lua
evalEnabled = true
```

This applies from the next mission start. Eval is arbitrary Lua execution in
the mission environment, which is acceptable while DCS-gRPC listens on
`127.0.0.1` only — anything that could reach it is already on the machine. If
the gRPC host is ever opened to the network, turn this off or put
authentication in front.

## How to tell it is working

- Overlord's log prints `Mission export is flowing again` (or nothing at all
  if it worked from the first poll — failures are what get logged).
- The dashboard grows a **Mission tasks** panel; each pilot's page shows the
  tasks carrying their name.
- Polling is every 10 seconds, so a state change in the mission appears on the
  dashboard within ten seconds plus one page refresh (15 s).

## What the dashboard does with it

- The **Mission tasks** panel lists every entry for the current mission:
  title, state, pilot, points. `done` and `failed` are highlighted; any other
  state renders plainly.
- The **pilot pages** show each pilot the entries carrying their name.
- Entries are kept per mission, so a restarted mission starts a clean sheet
  and history remains queryable per run (`missionTasks(missionID: ...)` in
  GraphQL).

## Extending later

The `version` field is there so the contract can grow without breaking either
side: overlord ignores fields it does not know, and a future `version = 2`
can add shapes (per-side scores, objectives with progress) while `1` keeps
working. Talk to the overlord side before inventing fields, so both ends agree
on what they mean.
