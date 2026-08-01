// Overlord debrief.
//
// Dependency-free static files talking to the GraphQL API over HTTP. No
// server-side state and nothing that assumes same-origin beyond the default, so
// moving this out of overlord means serving the directory elsewhere and setting
// window.OVERLORD_API_URL.

const API_URL =
  (typeof window !== "undefined" && window.OVERLORD_API_URL) || "/query";

const REFRESH_MS = 15000;
const LOG_ROWS = 200;

const el = (id) => document.getElementById(id);

// Event types grouped the way a debrief reads them, rather than the way they are
// stored.
const GROUPS = {
  kill: ["kill"],
  hit: ["hit"],
  shot: ["shot"],
  losses: ["crash", "dead", "unit_lost", "pilot_dead", "ejection"],
  sortie: [
    "takeoff", "runway_takeoff", "land", "runway_touch",
    "engine_startup", "engine_shutdown",
    "player_enter_unit", "player_leave_unit", "player_change_slot",
  ],
};

const EV_CLASS = {
  kill: "ev-kill", hit: "ev-hit", shot: "ev-shot",
  crash: "ev-loss", dead: "ev-loss", unit_lost: "ev-loss",
  pilot_dead: "ev-loss", ejection: "ev-loss",
};

// DCS event names are snake_case identifiers. Spell out the ones whose meaning
// is not obvious from the words; everything else just loses its underscores.
const EV_LABEL = {
  pilot_dead: "pilot killed",
  unit_lost: "unit lost",
  runway_takeoff: "takeoff (runway)",
  runway_touch: "touch and go",
  engine_startup: "engine start",
  engine_shutdown: "engine stop",
  player_enter_unit: "entered slot",
  player_leave_unit: "left slot",
  player_change_slot: "changed slot",
};

const eventLabel = (e) => EV_LABEL[e] || String(e || "").replace(/_/g, " ");

// --- state -----------------------------------------------------------------

// DCS type identifier -> readable name, built from the units and weapons
// queries. Falls back to the raw identifier so an unnamed type still renders.
const names = { unit: new Map(), weapon: new Map() };

const unitName = (t) => names.unit.get(t) || shortStore(t || "");
const weaponName = (t) => names.weapon.get(t) || shortStore(t || "");

const state = {
  data: null,
  log: null,
  query: "",
  weaponClass: "all",
  logGroup: "all",
  logSide: "all",
  player: null,
  // scope decides whether aggregates cover the current mission or all of
  // recorded history. Defaults to the mission: numbers that move while you fly
  // are the whole point of the page being live.
  scope: "mission",
  missionID: null,
  // Which pilot the mission index is narrowed to, remembered across visits so
  // "the ones I was in" only has to be answered once. Empty means anyone.
  pilotFilter: (() => {
    try {
      return localStorage.getItem("overlord-pilot") || "";
    } catch {
      return ""; // private mode
    }
  })(),
  sort: {
    weapons: { key: "shots", dir: -1 },
    pilots: { key: "takeoffs", dir: -1 },
    traps: { key: "missionTime", dir: -1 },
    log: { key: "id", dir: -1 },
    "p-aircraft": { key: "sorties", dir: -1 },
    "p-weapons": { key: "shots", dir: -1 },
    "p-grades": { key: "missionTime", dir: -1 },
    tasks: { key: "points", dir: -1 },
    recoveries: { key: "missionTime", dir: -1 },
    airframes: { key: "kills", dir: -1 },
    "player-tasks": { key: "points", dir: -1 },
  },
  // Which state the mission-task panel is narrowed to, and to whose tasks.
  taskState: "all",
  taskPilot: "",
  // Where each paginated table is. Keyed by table id; absent means page one.
  page: {},
};

// The synthetic AI players are stored under a machine-ish name. They are real
// players everywhere else in the app, so they get a real name here.
const isAIName = (name) => /^AI-Unit \(/.test(name || "");

function playerLabel(name) {
  const ai = /^AI-Unit \((.+)\)$/.exec(name || "");
  if (!ai) return name || "—";
  const side = ai[1];
  return side === "unknown" ? "Unattributed AI" : `${side[0].toUpperCase()}${side.slice(1)} AI`;
}

// A link to a pilot's page.
//
// A real anchor with a real href, not a span with a click handler: /player/2 is
// a page of its own, so it has to survive being copied, opened in a new tab,
// bookmarked and shared. That is the whole point of it not being a dialog.
function pref(id, name) {
  if (!id) return esc(playerLabel(name));
  return `<a class="pref" href="/player/${encodeURIComponent(id)}">${esc(playerLabel(name))}</a>`;
}

// --- api -------------------------------------------------------------------

// The missionID argument every aggregate takes, as a query fragment. Empty
// when the scope is all-time, which asks the API for exactly what it did
// before missions existed.
const midArg = () =>
  state.scope === "mission" && state.missionID ? `missionID: "${state.missionID}"` : "";

// The query is composed from the panels the page actually has.
//
// The dashboard used to be every panel at once, so one query fetched
// everything: the weapons table, the whole event log, the map and the records,
// on every page. Split across sections, most of that is waste on most pages --
// /missions needs none of it. Asking the DOM which panels exist keeps the
// query honest without a second list to keep in step with the markup.
const wants = (id) => Boolean(el(id));

function dashQuery() {
  const m = midArg();
  const p = m ? `(${m})` : "";

  // Display names back every reference card and most tables, so they come
  // along wherever anything renders a unit or weapon by name.
  const needNames =
    wants("weapons") ||
    wants("records") ||
    wants("killfeed") ||
    wants("recoveries") ||
    wants("airframes") ||
    wants("af-matchups") ||
    wants("log");

  return `{
  missions { id name theatre startedAt events duration }
  ${
    wants("missions")
      ? `missionIndex {
    id name theatre startedAt events duration kills sorties
    pilots { playerID playerName kills }
    highlight { kind playerID playerName count seconds nm }
  }`
      : ""
  }
  ${wants("tug-blue") ? `killsByCoalition${p} { coalition kills teamkills }` : ""}
  ${wants("wpn-matchups") ? `weaponMatchups${p} { weaponType targetType kills }` : ""}
  ${wants("airframes") ? `airframes${p} { unitType sorties landings shots hits kills collisions losses ejections timesKilled killDeathRatio hitsPerShot killsPerShot }` : ""}
  ${wants("af-matchups") ? `airframeMatchups${p} { unitType targetType kills }` : ""}
  ${wants("weapons") ? `weaponEffectiveness${p} { weaponType shots hits kills collisions hitsPerShot killsPerShot }` : ""}
  ${wants("pilots") ? `playerActivity${p} { playerID playerName kills takeoffs landings crashes ejections deaths }` : ""}
  ${
    wants("landings") || wants("recoveries")
      ? `landingGrades(first: 500${m ? ", " + m : ""}) {
    playerName playerID unitType place missionTime
    mark score scored wire deviations
  }`
      : ""
  }
  ${
    wants("records")
      ? `records${p} {
    firstBlood { playerID playerName unitType targetType missionTime }
    longestKill { playerID playerName unitType weaponType targetType rangeM }
    highestKill { playerID playerName unitType targetType altitudeM }
    deadliest { weaponType shots kills killsPerShot }
  }`
      : ""
  }
  ${wants("collateral") ? `collateral${p} { struck levelled trees structures top { displayName count tree } }` : ""}
  ${wants("tasks") ? `missionTasks${p} { taskKey title state playerName playerID points }` : ""}
  ${wants("missionmap") ? `killPoints${p} { lat lon coalition playerName unitType targetType weaponType missionTime }` : ""}
  ${
    wants("killfeed")
      ? `feed: events(first: 8, eventType: "kill"${m ? ", " + m : ""}) {
    edges { node {
      id missionTime coalition
      player { playerID playerName }
      initiator { type }
      weapon { type }
      target { unit { type } }
      targetName
    } }
  }`
      : ""
  }
  ${needNames ? `units { type displayName }\n  weapons { type displayName }` : ""}
}`;
}

// The log is filtered by the database, not in the browser. Filtering 200
// client-side rows does not work: hits outnumber kills by roughly ten to one, so
// "kills only" came back with two rows while hundreds sat in the table.
//
// The API takes one event type per query, so a grouped filter such as losses
// asks for each of its types and merges the results. That is a handful of small
// queries per click, which is cheaper than fetching everything and discarding
// most of it.
function logQuery(eventType) {
  const args = [`first: ${LOG_ROWS}`];
  if (eventType) args.push(`eventType: "${eventType}"`);
  if (state.logSide !== "all") args.push(`coalition: "${state.logSide}"`);
  const m = midArg();
  if (m) args.push(m);

  return `{ events(${args.join(", ")}) {
    edges { node {
      id event missionTime coalition targetCoalition
      player { playerName }
      initiator { type }
      initiatorName initiatorCallsign initiatorGroup
      weapon { type }
      place
      target { kind unit { type } }
      targetName
    } }
  } }`;
}

// Fetches the log for whatever filter is active and returns it as one
// connection, newest first.
async function fetchLog() {
  const group = GROUPS[state.logGroup];

  if (!group) {
    const d = await gql(logQuery(null));
    return d.events;
  }

  const parts = await Promise.all(group.map((t) => gql(logQuery(t))));

  const edges = parts
    .flatMap((d) => d.events?.edges || [])
    .sort((a, b) => Number(b.node.id) - Number(a.node.id))
    .slice(0, LOG_ROWS);

  return { edges };
}

async function gql(query) {
  const res = await fetch(API_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query }),
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);

  const body = await res.json();
  if (body.errors) throw new Error(body.errors.map((e) => e.message).join("; "));
  return body.data;
}

// --- helpers ---------------------------------------------------------------

function esc(v) {
  return String(v ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

function clock(seconds) {
  if (!seconds) return "--:--:--";
  const s = Math.floor(seconds);
  return [Math.floor(s / 3600), Math.floor((s % 3600) / 60), s % 60]
    .map((n) => String(n).padStart(2, "0"))
    .join(":");
}

function ratio(v) {
  return (v ?? 0).toFixed(2);
}

function num(v) {
  return v ? String(v) : `<span class="zero">0</span>`;
}

// setStat writes a header figure and pulses it when it changed, so a refresh
// that moved a number is visible without staring at it. Rewriting the class
// with a reflow between lets the animation restart on consecutive changes.
function setStat(id, text) {
  const node = el(id);
  if (node.textContent === text) return;
  node.textContent = text;
  node.classList.remove("bump");
  void node.offsetWidth;
  node.classList.add("bump");
}

function matches(haystack) {
  if (!state.query) return true;
  return haystack.join(" ").toLowerCase().includes(state.query);
}

// Search has to match what is on the screen, not the identifier behind it.
// Typing "Phoenix" found nothing, because the row says AIM-54C Phoenix and the
// haystack held AIM_54C_Mk47 -- the one string the reader never sees. Both go
// in, so either works.
const unitHay = (t) => (t ? [t, unitName(t)] : []);
const weaponHay = (t) => (t ? [t, weaponName(t)] : []);
const playerHay = (n) => (n ? [n, playerLabel(n)] : []);

// Guns report hits with no shot events; airframes appear as weapons when DCS
// names the aircraft for a collision.
//
// Collisions come from the server now. The old rule here was "no shots, not a
// gun, so it must be a ramming", which quietly mislabels any store whose shot
// events were dropped upstream -- and shot events do get dropped, which is what
// the DCS-gRPC patch is for.
function storeClass(row) {
  if (row.weaponType.startsWith("weapons.shells.")) return "gun";
  if (row.collisions > 0 && row.shots === 0) return "collision";
  return "ordnance";
}

// A name that opens a reference card. The DCS identifier travels in a data
// attribute so the card can be looked up and linked to.
function ref(kind, type) {
  if (!type) return `<span class="zero">—</span>`;
  const label = kind === "unit" ? unitName(type) : weaponName(type);
  return `<span class="ref" data-ref="${kind}" data-type="${esc(type)}" role="button" tabindex="0">${esc(label)}</span>`;
}

function shortStore(name) {
  return name.replace(/^weapons\.shells\./, "");
}

// --- sortable tables -------------------------------------------------------

function sortRows(rows, table) {
  const { key, dir } = state.sort[table];
  return [...rows].sort((a, b) => {
    const x = a[key], y = b[key];
    if (typeof x === "number" && typeof y === "number") return (x - y) * dir;
    return String(x ?? "").localeCompare(String(y ?? "")) * dir;
  });
}

// redraw is what a sort click re-runs. It defaults to the dashboard, since the
// player page renders a different set of tables and must not redraw the one
// behind it.
// PAGE_ROWS is how many rows a table shows at once.
//
// Long tables used to scroll inside a fixed-height box, which hides how much
// there is, cannot be linked to, and puts a second scrollbar inside the page's
// own. A page count says what you have and where you are in it.
const PAGE_ROWS = 50;

// pageOf returns the slice on show and the numbers to describe it.
//
// The index is clamped rather than trusted: a filter can shrink the set under
// your feet -- narrowing 1,414 tasks to 3 while you are on page 12 -- and an
// out-of-range page renders an empty table with no hint of why.
function pageOf(table, rows) {
  const pages = Math.max(1, Math.ceil(rows.length / PAGE_ROWS));
  const at = Math.min(Math.max(0, state.page[table] || 0), pages - 1);
  state.page[table] = at;

  const from = at * PAGE_ROWS;
  return { rows: rows.slice(from, from + PAGE_ROWS), at, pages, from, total: rows.length };
}

// The pager. Rendered after the table rather than inside it, since a table's
// own markup has nowhere legitimate for controls.
function pager(table, p, redraw) {
  const host = el(`${table}-pager`);
  if (!host) return;

  if (p.pages <= 1) {
    host.innerHTML = "";
    return;
  }

  const to = p.from + p.rows.length;
  host.innerHTML =
    `<button type="button" class="pg-prev"${p.at === 0 ? " disabled" : ""} aria-label="Previous page">←</button>` +
    `<span class="pg-at">${num(p.from + 1)}–${num(to)} of ${num(p.total)}</span>` +
    `<button type="button" class="pg-next"${p.at >= p.pages - 1 ? " disabled" : ""} aria-label="Next page">→</button>`;

  const step = (by) => {
    state.page[table] = p.at + by;
    redraw();
    // The table is now showing different rows some way up the page; without
    // this you page forward and stay looking at the bottom of the last one.
    el(table)?.scrollIntoView({ block: "nearest" });
  };
  host.querySelector(".pg-prev")?.addEventListener("click", () => step(-1));
  host.querySelector(".pg-next")?.addEventListener("click", () => step(1));
}

function render(table, columns, rows, renderRow, redraw = draw) {
  const node = el(table);

  if (!rows.length) {
    node.innerHTML = `<tbody><tr><td class="none">Nothing matches.</td></tr></tbody>`;
    pager(table, { pages: 1 }, redraw);
    return;
  }

  // Paginate only where the page asked for it, by putting a pager element in
  // the markup. A six-row table needs no controls.
  let shownRows = rows;
  if (el(`${table}-pager`)) {
    const p = pageOf(table, rows);
    shownRows = p.rows;
    pager(table, p, redraw);
  }

  // The control is a real <button> inside the <th>, not a click handler on the
  // header cell. A bare th is not focusable and takes no keypress, so sorting
  // used to be reachable with a mouse and by no other means.
  const active = state.sort[table];
  const head = columns
    .map((c) => {
      if (!c.key) return `<th${c.num ? ' class="num"' : ""}>${esc(c.label)}</th>`;
      const on = active.key === c.key;
      const car = on ? `<span class="car" aria-hidden="true">${active.dir < 0 ? "▼" : "▲"}</span>` : "";
      return `<th${on ? ` aria-sort="${active.dir < 0 ? "descending" : "ascending"}"` : ""}${
        c.num ? ' class="num"' : ""
      }><button type="button" class="sort" data-sort="${esc(c.key)}">${esc(c.label)}${car}</button></th>`;
    })
    .join("");

  // Replacing innerHTML destroys whatever had focus inside this table, and the
  // poll redraws every table every 15 seconds. Without this, tabbing to a sort
  // header and pausing drops you back to the top of the document on the next
  // refresh. Remember the column and restore it afterwards.
  const held =
    node.contains(document.activeElement) && document.activeElement.dataset.sort;

  node.innerHTML = `<thead><tr>${head}</tr></thead><tbody>${shownRows.map((r, i) => renderRow(r, i)).join("")}</tbody>`;

  if (held) node.querySelector(`button[data-sort="${CSS.escape(held)}"]`)?.focus();

  node.querySelectorAll("button[data-sort]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const key = btn.dataset.sort;
      const s = state.sort[table];
      s.dir = s.key === key ? -s.dir : -1;
      s.key = key;
      // Re-sorting reorders the whole set, so page 12 of the old order means
      // nothing in the new one.
      state.page[table] = 0;
      redraw();
    });
  });
}

// --- sections --------------------------------------------------------------

// The scoreboard. Blue against red as a tug of war, the mission named, and
// the latest kills as a feed -- the one panel composed for drama on purpose.
function drawHero(d) {
  const missions = d.missions || [];
  const current =
    state.scope === "mission" && state.missionID
      ? missions.find((m) => m.id === state.missionID)
      : null;

  if (current) {
    el("hero-title").textContent = current.name || `Mission #${current.id}`;
    const started = new Date(current.startedAt).toLocaleString([], {
      day: "numeric", month: "short", hour: "2-digit", minute: "2-digit",
    });
    el("hero-sub").textContent = [
      current.theatre || "Unknown map",
      `mission #${current.id}`,
      `started ${started}`,
    ].join(" · ");
  } else {
    el("hero-title").textContent = "All recorded missions";
    el("hero-sub").textContent = `${missions.length} missions on file`;
  }

  // The clock is the mission's own span, not the newest row in the event log.
  // The log is a windowed query that only one section fetches now, and taking
  // the clock from it meant a long-dead run's 01:12:55 could sit in the header
  // while the current mission was ten minutes old. A mission's duration is
  // MAX(mission_time) over its own events, which is the same number and always
  // the right one.
  el("hero-clock").textContent = current ? clock(current.duration || 0) : "--:--:--";

  const tally = { blue: 0, red: 0, unknown: 0, teamkills: 0 };
  for (const c of d.killsByCoalition || []) {
    if (c.coalition === "blue" || c.coalition === "red") tally[c.coalition] = c.kills;
    else tally.unknown += c.kills;
    tally.teamkills += c.teamkills || 0;
  }

  setStat("tug-blue", String(tally.blue));
  setStat("tug-red", String(tally.red));
  const total = Math.max(1, tally.blue + tally.red);
  el("tug-blue-bar").style.width = `${(tally.blue / total) * 100}%`;
  el("tug-red-bar").style.width = `${(tally.red / total) * 100}%`;

  el("tug-note").textContent = [
    tally.unknown ? `${tally.unknown} kills with no side recorded` : "",
    tally.teamkills ? `${tally.teamkills} friendly fire` : "",
  ].filter(Boolean).join(" · ");

  const feed = (d.feed?.edges || []).map((e) => e.node);
  el("killfeed").innerHTML = feed.length
    ? feed
        .map((n) => {
          const side = n.coalition === "blue" || n.coalition === "red"
            ? `<span class="flag flag-${esc(n.coalition)}"></span>`
            : `<span class="flag"></span>`;
          const who = n.player?.playerID
            ? pref(n.player.playerID, n.player.playerName)
            : esc(playerLabel(n.player?.playerName));
          const victim = n.target?.unit?.type
            ? ref("unit", n.target.unit.type)
            : esc(n.targetName || "—");
          return `<li>
            <span class="kf-time">${clock(n.missionTime)}</span>
            ${side}${who}
            ${n.initiator?.type ? `· ${ref("unit", n.initiator.type)}` : ""}
            <span class="to">→</span> ${victim}
            ${n.weapon?.type ? `<span class="kf-weapon">${esc(weaponName(n.weapon.type))}</span>` : ""}
          </li>`;
        })
        .join("")
    : `<li class="none">No kills yet. The feed starts with the first one.</li>`;
}

function drawWeapons(rows) {
  const filtered = rows
    .filter((r) => r.shots || r.hits || r.kills || r.collisions)
    .filter((r) => state.weaponClass === "all" || storeClass(r) === state.weaponClass)
    .filter((r) => matches(weaponHay(r.weaponType)));

  const max = Math.max(1, ...filtered.map((r) => r.shots));

  // The collisions column earns its width only when something in view has one.
  // A ramming record is otherwise a row of zeroes, since collisions are held
  // out of hits and kills.
  const showCollisions = filtered.some((r) => r.collisions > 0);

  const countEl = el("weapons-count");
  if (countEl) countEl.textContent = `${num(filtered.length)} stores`;

  render(
    "weapons",
    [
      { label: "Weapon", key: "weaponType" },
      { label: "Shots", key: "shots", num: true },
      { label: "Hits", key: "hits", num: true },
      { label: "Kills", key: "kills", num: true },
      ...(showCollisions ? [{ label: "Collisions", key: "collisions", num: true }] : []),
      { label: "Hits / shot", key: "hitsPerShot", num: true },
      { label: "Kills / shot", key: "killsPerShot", num: true },
    ],
    sortRows(filtered, "weapons"),
    (r) => `<tr>
      <td class="name">${ref("weapon", r.weaponType)}</td>
      <td class="num">${
        r.shots ? `<span class="bar" style="width:${(r.shots / max) * 44}px"></span>${r.shots}` : num(0)
      }</td>
      <td class="num">${num(r.hits)}</td>
      <td class="num">${num(r.kills)}</td>
      ${showCollisions ? `<td class="num">${num(r.collisions)}</td>` : ""}
      <td class="num">${r.shots ? ratio(r.hitsPerShot) : `<span class="zero">—</span>`}</td>
      <td class="num">${r.shots ? ratio(r.killsPerShot) : `<span class="zero">—</span>`}</td>
    </tr>`
  );
}

function drawPilots(rows) {
  drawOpposition(rows);

  // Humans only, and every one of them is listed even with nothing against
  // their name yet: they are a real person with a page, and dropping the row is
  // the difference between "no sorties" and "not here".
  //
  // The AI is deliberately not here. It is not a rival -- it is three synthetic
  // coalition buckets holding every AI kill on the server, and ranking a person
  // against them means a real pilot with four kills opens the page and finds
  // themselves last behind three robot armies. That is not a leaderboard, it is
  // a discouragement. The AI's tally moves to its own strip below.
  const filtered = rows
    .filter((r) => !isAIName(r.playerName))
    .filter((r) => matches(playerHay(r.playerName)));

  // Ranked by score when the mission awards any, kills otherwise. The medals
  // are the point: a leaderboard nobody can win is just a table.
  const scoreFor = (r) => {
    let sum = 0;
    for (const t of state.data?.missionTasks || []) {
      if (t.playerID ? t.playerID === r.playerID : t.playerName === r.playerName) sum += t.points;
    }
    return sum;
  };
  const anyScore = (state.data?.missionTasks || []).some((t) => t.points > 0);

  const ranked = filtered
    .map((r) => ({
      ...r,
      score: scoreFor(r),
      kd: (r.deaths + r.crashes) > 0 ? r.kills / (r.deaths + r.crashes) : r.kills,
    }))
    .sort((a, b) => (anyScore && b.score !== a.score ? b.score - a.score : b.kills - a.kills));

  const medals = ["🥇", "🥈", "🥉"];

  render(
    "pilots",
    [
      { label: "#" },
      { label: "Pilot", key: "playerName" },
      { label: "Kills", key: "kills", num: true },
      { label: "K/D", key: "kd", num: true },
      ...(anyScore ? [{ label: "Score", key: "score", num: true }] : []),
      { label: "Takeoffs", key: "takeoffs", num: true },
      { label: "Ejections", key: "ejections", num: true },
      { label: "Deaths", key: "deaths", num: true },
    ],
    ranked,
    (r, i) => `<tr${i === 0 && (r.kills || r.score) ? ' class="first-place"' : ""}>
      <td class="rank">${medals[i] || i + 1}</td>
      <td class="name">${pref(r.playerID, r.playerName)}</td>
      <td class="num">${num(r.kills)}</td>
      <td class="num">${ratio(r.kd)}</td>
      ${anyScore ? `<td class="num">${num(r.score)}</td>` : ""}
      <td class="num">${num(r.takeoffs)}</td>
      <td class="num">${num(r.ejections)}</td>
      <td class="num">${num(r.deaths)}</td>
    </tr>`
  );

  if (!filtered.length) {
    el("pilots").innerHTML =
      `<tbody><tr><td class="none">No human pilots on record yet. Fly one sortie and this is your board.</td></tr></tbody>`;
  }
}

// The AI's tally, kept off the leaderboard and rendered as what it is: the
// opposition, and the weather. Still linked, because the AI player pages are
// worth reading -- they are the record of everything the war did.
function drawOpposition(rows) {
  const host = el("opposition");
  if (!host) return;

  const ai = rows
    .filter((r) => isAIName(r.playerName))
    .filter((r) => r.kills || r.takeoffs || r.deaths || r.crashes)
    .sort((a, b) => b.kills - a.kills);

  if (!ai.length) {
    host.innerHTML = "";
    return;
  }

  host.innerHTML =
    `<h3 class="opp-head">The opposition</h3>` +
    `<p class="note">Every AI kill on the server, pooled per coalition. Not ranked against people.</p>` +
    `<ul class="opp-list">` +
    ai
      .map(
        (r) => `<li>
          <span class="opp-name">${pref(r.playerID, r.playerName)}</span>
          <span class="opp-stat"><b>${num(r.kills)}</b> kills</span>
          <span class="opp-stat"><b>${num(r.takeoffs)}</b> sorties</span>
          <span class="opp-stat"><b>${num(r.crashes + r.deaths)}</b> lost</span>
        </li>`
      )
      .join("") +
    `</ul>`;
}

function drawTraps(rows) {
  const filtered = rows.filter((r) =>
    matches([...playerHay(r.playerName), ...unitHay(r.unitType), r.place, r.grade])
  );

  render(
    "traps",
    [
      { label: "Time", key: "missionTime", num: true },
      { label: "Pilot", key: "playerName" },
      { label: "Aircraft", key: "unitType" },
      { label: "Airfield", key: "place" },
      { label: "Grade", key: "grade" },
    ],
    sortRows(filtered, "traps"),
    (r) => `<tr>
      <td class="num">${clock(r.missionTime)}</td>
      <td class="name">${esc(r.playerName || "—")}</td>
      <td>${r.unitType ? ref("unit", r.unitType) : "—"}</td>
      <td>${esc(r.place || "—")}</td>
      <td class="grade">${esc(r.grade || "—")}</td>
    </tr>`
  );
}

function drawLog(connection) {
  const all = (connection?.edges || []).map((e) => ({ ...e.node, id: Number(e.node.id) }));

  // Event type and coalition were applied by the query. Only the free-text
  // filter is left to do here.
  const filtered = all
    .filter((n) =>
      matches([
        n.event, eventLabel(n.event),
        ...playerHay(n.player?.playerName),
        ...unitHay(n.initiator?.type),
        n.initiatorName, n.initiatorCallsign, n.initiatorGroup,
        ...weaponHay(n.weapon?.type),
        ...unitHay(n.target?.unit?.type),
        n.targetName, n.place,
      ])
    )
    .map((n) => ({
      ...n,
      initiatorType: n.initiator?.type || "",
      weaponType: n.weapon?.type || "",
      targetType: n.target?.unit?.type || n.targetName || "",
    }));

  render(
    "log",
    [
      { label: "Time", key: "missionTime", num: true },
      { label: "Event", key: "event" },
      { label: "Who", key: "initiatorCallsign" },
      { label: "Aircraft", key: "initiatorType" },
      { label: "Weapon", key: "weaponType" },
      { label: "Target", key: "targetType" },
    ],
    sortRows(filtered, "log"),
    (n) => {
      // Rows that arrived since the previous refresh pulse once. seenTopId is
      // advanced after every draw, so re-sorting or searching the same data
      // does not re-announce it.
      const fresh = state.seenTopId && n.id > state.seenTopId;

      const side =
        n.coalition === "blue" || n.coalition === "red"
          ? `<span class="flag flag-${esc(n.coalition)}"></span>`
          : `<span class="flag"></span>`;

      // Callsign is what a pilot is actually called on the radio; fall back
      // through the unit name to the player.
      const actor = n.initiatorCallsign || n.initiatorName || n.player?.playerName || "—";

      const target = n.targetType
        ? ref("unit", n.targetType) +
          (n.target?.kind && n.target.kind !== "unit"
            ? ` <span class="zero">${esc(n.target.kind)}</span>`
            : "")
        : `<span class="zero">—</span>`;

      return `<tr${fresh ? ' class="fresh"' : ""}>
        <td class="num">${clock(n.missionTime)}</td>
        <td><span class="ev ${EV_CLASS[n.event] || "ev-sortie"}" title="${esc(n.event)}">${esc(eventLabel(n.event))}</span></td>
        <td class="name">${side}${esc(actor)}</td>
        <td>${n.initiatorType ? ref("unit", n.initiatorType) : "—"}</td>
        <td>${n.weaponType ? ref("weapon", n.weaponType) : `<span class="zero">—</span>`}</td>
        <td>${target}${n.place ? ` <span class="zero">${esc(n.place)}</span>` : ""}</td>
      </tr>`;
    }
  );

  state.seenTopId = Math.max(state.seenTopId || 0, ...all.map((n) => n.id));
}

// --- draw / refresh --------------------------------------------------------

// The one thing worth saying about a run, phrased here from the parts the
// server sends. Wording lives with the rest of the wording.
function highlightLine(h) {
  if (!h) return "";
  const who = pref(h.playerID, h.playerName);
  switch (h.kind) {
    case "multikill":
      return `${who} took <b>${h.count}</b> inside ${Math.round(h.seconds)} seconds`;
    case "ace":
      return `${who} went <b>${h.count}</b> for the night`;
    case "longshot":
      return `a <b>${h.nm.toFixed(1)} nm</b> shot by ${who}`;
    case "topscorer":
      return `${who} led with <b>${h.count}</b>`;
    default:
      return "";
  }
}

// --- weapons -----------------------------------------------------------------

// What the arsenal did as a whole. Four figures and the two extremes, so the
// page opens with a sentence rather than a hundred and forty rows.
function drawArsenal(rows) {
  const host = el("arsenal");
  if (!host) return;

  const stores = (rows || []).filter((w) => w.shots > 0);
  if (!stores.length) {
    host.innerHTML = `<p class="none">Nothing fired in this view.</p>`;
    return;
  }

  const shots = stores.reduce((s, w) => s + w.shots, 0);
  const hits = stores.reduce((s, w) => s + w.hits, 0);
  const kills = stores.reduce((s, w) => s + w.kills, 0);

  // Deadliest by conversion rather than by total, over enough launches for the
  // rate to mean anything -- one lucky shot is not a good weapon.
  const MIN_SHOTS = 10;
  const rated = stores.filter((w) => w.shots >= MIN_SHOTS);
  const best = [...rated].sort((a, b) => b.killsPerShot - a.killsPerShot)[0];
  const busiest = [...stores].sort((a, b) => b.shots - a.shots)[0];
  const deadliest = [...stores].sort((a, b) => b.kills - a.kills)[0];

  const fact = (label, value, sub) =>
    `<div><dt>${esc(label)}</dt><dd>${value}${sub ? `<span class="ars-sub">${sub}</span>` : ""}</dd></div>`;

  host.innerHTML =
    `<dl class="arsenal">
      ${fact("Launched", num(shots))}
      ${fact("Hits", num(hits), `${(hits / shots).toFixed(2)} per shot`)}
      ${fact("Kills", num(kills), `${(kills / shots).toFixed(2)} per shot`)}
      ${fact("Stores used", num(stores.length))}
    </dl>
    <dl class="arsenal arsenal-named">
      ${deadliest ? fact("Most kills", ref("weapon", deadliest.weaponType), `${num(deadliest.kills)} of them`) : ""}
      ${busiest ? fact("Most fired", ref("weapon", busiest.weaponType), `${num(busiest.shots)} launches`) : ""}
      ${
        best
          ? fact(
              "Best conversion",
              ref("weapon", best.weaponType),
              `${best.killsPerShot.toFixed(2)} kills per shot over ${num(best.shots)}`
            )
          : ""
      }
    </dl>`;
}

// --- airframes ---------------------------------------------------------------

// Every type side by side. The reference card answers "how did the Hornet do";
// this answers "compared with what else", which is what the weapons table has
// been doing for stores all along.
function drawAirframes(rows) {
  const list = (rows || []).filter((a) => matches(unitHay(a.unitType)));

  const countEl = el("airframes-count");
  if (countEl) countEl.textContent = `${num(list.length)} types`;

  // The column only earns its width when something in view has rammed
  // something, exactly as on the weapons table.
  const anyCollisions = list.some((a) => a.collisions > 0);

  render(
    "airframes",
    [
      { label: "Type", key: "unitType" },
      { label: "Sorties", key: "sorties", num: true },
      { label: "Shots", key: "shots", num: true },
      { label: "Hits", key: "hits", num: true },
      { label: "Kills", key: "kills", num: true },
      ...(anyCollisions ? [{ label: "Collisions", key: "collisions", num: true }] : []),
      { label: "Lost", key: "timesKilled", num: true },
      { label: "K/D", key: "killDeathRatio", num: true },
      { label: "Hits / shot", key: "hitsPerShot", num: true },
    ],
    sortRows(list, "airframes"),
    (a) => `<tr>
      <td class="name">${ref("unit", a.unitType)}</td>
      <td class="num">${num(a.sorties)}</td>
      <td class="num">${num(a.shots)}</td>
      <td class="num">${num(a.hits)}</td>
      <td class="num">${num(a.kills)}</td>
      ${anyCollisions ? `<td class="num">${num(a.collisions)}</td>` : ""}
      <td class="num">${num(a.timesKilled)}</td>
      <td class="num">${a.kills || a.timesKilled ? ratio(a.killDeathRatio) : `<span class="zero">—</span>`}</td>
      <td class="num">${a.shots ? ratio(a.hitsPerShot) : `<span class="zero">—</span>`}</td>
    </tr>`
  );
}

// --- landings ---------------------------------------------------------------

// The Navy's grades, worst to best, with what each is worth and how it reads.
// Ordered deliberately: a distribution is a shape, and the shape only means
// anything if the axis runs one way.
const LSO_MARKS = [
  { mark: "WO", label: "Wave off", tone: "bad" },
  { mark: "C", label: "Cut", tone: "bad" },
  { mark: "B", label: "Bolter", tone: "mid" },
  { mark: "---", label: "No grade", tone: "mid" },
  { mark: "(OK)", label: "Fair", tone: "ok" },
  { mark: "OK", label: "OK", tone: "ok" },
  { mark: "_OK_", label: "Perfect", tone: "best" },
];

// A row of proportional bars. Used for both distributions, since they are the
// same shape of question: how did these N things fall across these buckets.
function distBar(buckets, total) {
  if (!total) return "";
  return (
    `<div class="dist">` +
    buckets
      .filter((b) => b.n > 0)
      .map(
        (b) =>
          `<span class="dist-seg dist-${b.tone || "mid"}" style="flex-grow:${b.n}"
                 title="${esc(b.label)}: ${b.n}"><b>${b.n}</b></span>`
      )
      .join("") +
    `</div>` +
    `<ul class="dist-key">` +
    buckets
      .filter((b) => b.n > 0)
      .map((b) => `<li><i class="dist-${b.tone || "mid"}"></i>${esc(b.label)} <b>${b.n}</b></li>`)
      .join("") +
    `</ul>`
  );
}

// The landings panel: how the recoveries went, not which ones happened. The
// list of individual passes is a page of its own, because a debrief wants the
// shape and an argument about one trap wants the detail.
function drawLandings(grades) {
  const host = el("landings");
  if (!host) return;

  const passes = (grades || []).filter((g) => g.mark);
  if (!passes.length) {
    host.innerHTML = `<p class="none">No graded carrier recoveries yet.</p>`;
    return;
  }

  const byMark = LSO_MARKS.map((m) => ({
    ...m,
    n: passes.filter((p) => p.mark === m.mark).length,
  }));
  // Anything DCS graded with a token we do not know still happened.
  const unknown = passes.filter((p) => !p.scored).length;
  if (unknown) byMark.push({ mark: "?", label: "Ungraded", tone: "mid", n: unknown });

  const wires = [1, 2, 3, 4].map((w) => ({
    label: `${w} wire`,
    tone: w === 3 ? "best" : "mid",
    n: passes.filter((p) => p.wire === w).length,
  }));
  const bolters = passes.filter((p) => p.wire === 0).length;
  if (bolters) wires.push({ label: "Bolter", tone: "bad", n: bolters });

  const scored = passes.filter((p) => p.scored);
  const avg = scored.length ? scored.reduce((s, p) => s + p.score, 0) / scored.length : 0;

  host.innerHTML =
    `<div class="lso-head">
       <div><dt>Traps</dt><dd>${num(passes.length)}</dd></div>
       <div><dt>Average</dt><dd>${avg.toFixed(2)}<span class="lso-of"> / 4</span></dd></div>
       <div><dt>Three wire</dt><dd>${Math.round((wires[2].n / passes.length) * 100)}<span class="lso-of">%</span></dd></div>
     </div>` +
    `<h3 class="dist-title">Grades</h3>${distBar(byMark, passes.length)}` +
    `<h3 class="dist-title">Wire caught</h3>${distBar(wires, passes.length)}` +
    (el("landings-more") ? "" : `<p class="dist-more"><a href="/landings">Every recovery, one by one →</a></p>`);
}

// Every pass, for the page that exists to show them.
function drawRecoveries(grades) {
  const rows = (grades || [])
    .filter((g) => g.mark)
    .filter((g) => matches([g.playerName, g.unitType, g.place, g.mark, ...(g.deviations || [])]));

  render(
    "recoveries",
    [
      { label: "Pilot", key: "playerName" },
      { label: "Aircraft", key: "unitType" },
      { label: "Grade", key: "mark" },
      { label: "Score", key: "score", num: true },
      { label: "Wire", key: "wire", num: true },
      { label: "What the LSO saw" },
      { label: "Time", key: "missionTime", num: true },
    ],
    sortRows(rows, "recoveries"),
    (g) => `<tr>
      <td class="name">${g.playerID ? pref(g.playerID, g.playerName) : esc(playerLabel(g.playerName))}</td>
      <td>${ref("unit", g.unitType)}</td>
      <td><span class="lso-mark lso-${esc(markTone(g.mark))}">${esc(g.mark)}</span></td>
      <td class="num">${g.scored ? g.score.toFixed(1) : `<span class="zero">—</span>`}</td>
      <td class="num">${g.wire ? g.wire : `<span class="zero">bolter</span>`}</td>
      <td class="lso-devs">${g.deviations.length ? esc(g.deviations.join("; ")) : `<span class="zero">clean pass</span>`}</td>
      <td class="num">${clock(g.missionTime)}</td>
    </tr>`
  );
}

const markTone = (mark) => LSO_MARKS.find((m) => m.mark === mark)?.tone || "mid";

// The index of recorded runs. Newest first, each linking to its own recap.
//
// Quiet runs are dropped: a mission that recorded four events is a server
// restart, not a night worth reading back.
function drawMissions(missions) {
  const host = el("missions");
  if (!host) return;

  fillPilotFilter(missions);

  const rows = missions
    .filter((m) => m.events >= 50)
    .filter((m) => matches([m.name, m.theatre, ...(m.pilots || []).map((p) => p.playerName)]))
    .filter((m) => !state.pilotFilter || (m.pilots || []).some((p) => p.playerID === state.pilotFilter));

  el("mission-count").textContent = rows.length
    ? `${rows.length} of ${missions.filter((m) => m.events >= 50).length} runs`
    : "";

  if (!rows.length) {
    host.innerHTML = `<li class="none">${
      state.pilotFilter ? "No missions recorded for that pilot yet." : "No missions recorded yet."
    }</li>`;
    return;
  }

  host.innerHTML = rows
    .map((m) => {
      const when = m.startedAt ? new Date(m.startedAt) : null;
      const stamp =
        when && !isNaN(when)
          ? when.toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" })
          : "";
      const live = m.id === state.missionID;
      const line = highlightLine(m.highlight);
      const flew = (m.pilots || []).map((p) => pref(p.playerID, p.playerName)).join(", ");

      return `<li class="mission-card${live ? " is-live" : ""}">
        <div class="mc-head">
          <a class="mc-name" href="/mission/${encodeURIComponent(m.id)}">${esc(m.name || `Mission ${m.id}`)}</a>
          ${live ? `<b class="mission-live">live</b>` : ""}
          <span class="mc-when">${esc(m.theatre || "Unknown map")}${stamp ? " · " + esc(stamp) : ""}</span>
        </div>

        <dl class="mc-stats">
          <div><dt>Kills</dt><dd>${num(m.kills)}</dd></div>
          <div><dt>Sorties</dt><dd>${num(m.sorties)}</dd></div>
          <div><dt>Ran for</dt><dd>${dur(m.duration)}</dd></div>
          <div><dt>Events</dt><dd>${num(m.events)}</dd></div>
        </dl>

        ${line ? `<p class="mc-highlight"><span aria-hidden="true">★</span> ${line}</p>` : ""}
        ${flew ? `<p class="mc-pilots">Flown by ${flew}</p>` : ""}
      </li>`;
    })
    .join("");
}

// The pilot picker, filled from the runs themselves: whoever has flown is
// whoever you can filter by. There is no login, so "missions I was in" has to
// be asked as a question -- and the answer is remembered, so it only has to be
// answered once.
function fillPilotFilter(missions) {
  const sel = el("pilot-filter");
  if (!sel) return;

  const seen = new Map();
  for (const m of missions) {
    for (const p of m.pilots || []) if (!seen.has(p.playerID)) seen.set(p.playerID, p.playerName);
  }
  if (!seen.size) return;

  // Rebuilt when the roster changes, not once: somebody's first sortie should
  // put them in the picker on the next poll.
  const signature = [...seen.keys()].sort().join(",");
  if (sel.dataset.signature === signature) return;
  sel.dataset.signature = signature;

  sel.innerHTML =
    `<option value="">Anyone</option>` +
    [...seen.entries()]
      .sort((a, b) => a[1].localeCompare(b[1]))
      .map(([id, name]) => `<option value="${esc(id)}">${esc(playerLabel(name))}</option>`)
      .join("");

  if (state.pilotFilter && seen.has(state.pilotFilter)) sel.value = state.pilotFilter;
  else state.pilotFilter = "";
}

// Every section runs the same draw. Each panel is skipped when the page it
// would render into is not this one, so the sections share one code path
// rather than each growing its own.
function draw() {
  const d = state.data;
  if (!d) return;

  for (const u of d.units || []) names.unit.set(u.type, u.displayName);
  for (const w of d.weapons || []) names.weapon.set(w.type, w.displayName);

  if (wants("hero-title")) drawHero(d);
  if (wants("missions")) drawMissions(d.missionIndex || []);
  if (wants("weapons")) drawWeapons(d.weaponEffectiveness || []);
  if (wants("pilots")) drawPilots(d.playerActivity || []);
  if (wants("arsenal")) drawArsenal(d.weaponEffectiveness || []);
  if (wants("wpn-matchups")) {
    el("wpn-matchups").innerHTML = heatmap(
      (d.weaponMatchups || [])
        .filter((m) => matches([...weaponHay(m.weaponType), ...unitHay(m.targetType)]))
        .map((m) => ({ unitType: m.weaponType, targetType: m.targetType, kills: m.kills })),
      { rowNoun: "stores", colNoun: "target types", rowKind: "weapon" }
    );
  }
  if (wants("airframes")) drawAirframes(d.airframes || []);
  if (wants("af-matchups")) {
    el("af-matchups").innerHTML = heatmap(
      (d.airframeMatchups || []).filter(
        (m) => matches([...unitHay(m.unitType), ...unitHay(m.targetType)])
      ),
      { rowNoun: "airframes", colNoun: "target types" }
    );
  }
  if (wants("landings")) drawLandings(d.landingGrades || []);
  if (wants("recoveries")) drawRecoveries(d.landingGrades || []);
  if (wants("log")) drawLog(state.log);
  if (wants("records")) drawRecords(d.records);
  if (wants("tasks")) drawTasks("tasks", "p-tasks", d.missionTasks);
  if (wants("missionmap")) drawMissionMap(d.killPoints);
  if (wants("collateral")) el("collateral").innerHTML = collateralPanel(d.collateral, "mission");
}

async function refresh(isRerun) {
  const link = el("link");
  try {
    // Scoped queries need the current mission's id before they can ask for it.
    if (state.scope === "mission" && !state.missionID) {
      const m = await gql(`{ missions { id } }`);
      state.missionID = m.missions?.[0]?.id || null;
    }

    // The log is its own query and its own page; skip it everywhere else.
    const [summary, log] = await Promise.all([
      gql(dashQuery()),
      wants("log") ? fetchLog() : Promise.resolve(state.log),
    ]);

    // A new mission can start while the page sits open. Adopt it and refetch
    // once, so the page follows the server rather than a finished run.
    //
    // Except on a mission recap, which is pinned to the run in its URL: that
    // page following the server would be it silently becoming a different page
    // than the link someone shared.
    const newest = summary.missions?.[0]?.id || null;
    if (!state.pinnedMission && state.scope === "mission" && newest && newest !== state.missionID && !isRerun) {
      state.missionID = newest;
      return refresh(true);
    }

    state.data = summary;
    state.log = log;
    draw();
    state.lastSync = Date.now();
    link.textContent = "Live · just now";
    link.className = "status up";
    el("foot").textContent =
      `${API_URL} · last ${LOG_ROWS} events · refreshing every ${REFRESH_MS / 1000}s` +
      (state.scope === "mission" && state.missionID
        ? ` · mission #${state.missionID}`
        : " · all recorded missions");
  } catch (err) {
    link.textContent = "Offline";
    link.className = "status down";
    el("foot").textContent = `${err.message} — check that overlord is running and reachable at ${API_URL}`;
  }
}

async function refreshLog() {
  try {
    state.log = await fetchLog();
    draw();
  } catch (err) {
    el("link").textContent = "Offline";
    el("link").className = "status down";
    el("foot").textContent = `${err.message} — check that overlord is reachable at ${API_URL}`;
  }
}

// --- player page -----------------------------------------------------------

// How a flight ended, as a word and a colour. Unknown is its own outcome and
// says so: DCS simply never reported what happened, and guessing "landed"
// would be worse than admitting it.
const OUTCOME_LABEL = {
  landed: "Landed",
  crashed: "Crashed",
  ejected: "Ejected",
  killed: "Shot down",
  unknown: "Unclear",
};

// A duration in words. Reads better than a clock for a span: nobody thinks of
// a flight as 00:31:20.
function dur(seconds) {
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${String(s % 60).padStart(2, "0")}s`;
  return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, "0")}m`;
}

// Head to head. One line per opponent, with the exchange as a bar: your share
// of it in your colour, theirs in theirs. A rivalry reads faster as a shape
// than as two numbers.
function drawRivals(rivalries) {
  const panel = el("p-rivals");
  if (!panel) return;

  const rows = (rivalries || []).filter(
    (r) => (r.killed || r.lost) && matches(playerHay(r.opponentName))
  );
  panel.hidden = !rows.length;
  if (!rows.length) return;

  el("rivals").innerHTML = rows
    .map((r) => {
      const total = r.killed + r.lost;
      const mine = Math.round((r.killed / total) * 100);
      const verdict = r.killed > r.lost ? "up" : r.killed < r.lost ? "down" : "level";
      return `<li class="rival rival-${verdict}">
        <span class="rival-name">${pref(r.opponentID, r.opponentName)}</span>
        <span class="rival-bar" role="img"
              aria-label="${r.killed} to ${r.lost} in your favour">
          <i style="width:${mine}%"></i>
        </span>
        <span class="rival-score"><b>${num(r.killed)}</b> — <b>${num(r.lost)}</b></span>
      </li>`;
    })
    .join("");
}

// Titles held, as a line in the dossier rather than a panel of its own: it is
// one more superlative, and an empty one should take up no room.
function titlesLine(titles) {
  if (!titles || !titles.length) return "";
  const names = titles.map((t) => unitName(t.unitType));
  const shown = names.slice(0, 3).join(", ");
  const rest = names.length > 3 ? ` +${names.length - 3} more` : "";
  const what = names.length === 1 ? "airframe" : "airframes";
  return { value: shown + rest, sub: `top of the server on ${names.length} ${what}` };
}

// The flight log. One line per sortie: what was flown, how long it lasted,
// what it got, and how it ended.
function drawSorties(log) {
  const panel = el("p-sorties");
  if (!panel) return;

  const rows = log.filter((s) => matches(unitHay(s.unitType)));
  panel.hidden = !rows.length;
  if (!rows.length) return;

  el("sorties").innerHTML = rows
    .map((s) => {
      const outcome = OUTCOME_LABEL[s.outcome] || s.outcome;
      const where = s.missionName || `Mission ${s.missionID}`;
      const length = s.ended ? dur(s.duration) : "still up";
      return `<li class="sortie sortie-${esc(s.outcome)}">
        <span class="sortie-air">${ref("unit", s.unitType)}</span>
        <span class="sortie-meta">${esc(where)}${s.theatre ? " · " + esc(s.theatre) : ""} · ${esc(length)}</span>
        <span class="sortie-kills">${s.kills ? `${s.kills} ${s.kills === 1 ? "kill" : "kills"}` : ""}</span>
        <span class="sortie-end">${esc(outcome)}</span>
      </li>`;
    })
    .join("");
}

const PLAYER_QUERY = `query($id: ID!, $unitType: String, $m: ID) { playerProfile(playerID: $id, unitType: $unitType, missionID: $m) {
  playerID playerName isAI unitType flown coalitions
  sorties landings crashes ejections deaths shots hits kills teamkills timesKilled
  killDeathRatio firstSeen lastSeen
  aircraft { unitType sorties landings shots hits kills losses ejections hitsPerShot killsPerShot }
  weapons { weaponType shots hits kills collisions hitsPerShot killsPerShot }
  sortieLog { missionID missionName theatre unitType startTime duration kills outcome ended }
  rivalries { opponentID opponentName isAI killed lost }
  titles { unitType kills }
  matchups { unitType targetType kills }
  killedBy { unitType targetType kills }
  landingGrades { unitType place grade missionTime }
  bucketSeconds timeline { t sorties kills losses shots }
  killPoints { lat lon targetType weaponType missionTime }
  favourites {
    aircraft { name count }
    weapon { name count }
    prey { name count }
    nemesisUnit { name count }
    nemesisPilot { id name count }
    deadliestWeapon { name count }
    theatre { name count }
  }
} }`;

// Mission-clock activity, drawn as inline SVG. No chart library: this is one
// shape, and a dependency to draw it would be larger than the whole app.
//
// Kills rise from the axis and losses fall below it. The split is carried by
// position rather than hue, which keeps red and blue meaning coalition and
// nothing else, and survives both themes without a second palette.
function timelineChart(buckets, bucketSeconds) {
  const rows = (buckets || []).filter((b) => b.kills || b.losses || b.sorties || b.shots);
  if (!rows.length) return `<p class="none">Nothing to plot yet.</p>`;

  const W = 720;
  const H = 120;
  const mid = H / 2;

  // Kills and losses get their own scale. Shared, the blue AI's 75 losses in a
  // bucket flatten its 21 kills into nothing and the chart only shows one of
  // the two things it is for. The cost is that a bar above cannot be compared
  // by height to one below, so the caption gives both peaks and the axis label
  // says which is which.
  const upPeak = Math.max(1, ...rows.map((b) => b.kills));
  const downPeak = Math.max(1, ...rows.map((b) => b.losses));

  const t0 = rows[0].t;
  const t1 = rows[rows.length - 1].t + bucketSeconds;
  const span = Math.max(bucketSeconds, t1 - t0);
  const x = (t) => ((t - t0) / span) * W;
  const bw = Math.max(1.5, (W / span) * bucketSeconds - 1);

  const bars = rows
    .map((b) => {
      const left = x(b.t);
      const up = (b.kills / upPeak) * (mid - 8);
      const down = (b.losses / downPeak) * (mid - 8);
      const title = `${clock(b.t)} — ${b.kills} kills, ${b.losses} lost, ${b.shots} shots`;
      return (
        `<g><title>${esc(title)}</title>` +
        (b.kills ? `<rect class="tl-up" x="${left}" y="${mid - up}" width="${bw}" height="${up}"/>` : "") +
        (b.losses ? `<rect class="tl-down" x="${left}" y="${mid}" width="${bw}" height="${down}"/>` : "") +
        `</g>`
      );
    })
    .join("");

  return `
    <figure class="chart">
      <svg viewBox="0 0 ${W} ${H}" role="img" preserveAspectRatio="none"
           aria-label="Kills above the line and losses below it, across the mission clock.">
        <line class="tl-axis" x1="0" y1="${mid}" x2="${W}" y2="${mid}"/>
        ${bars}
      </svg>
      <figcaption>
        <span><i class="key key-up"></i>Kills, peak ${upPeak}</span>
        <span><i class="key key-down"></i>Lost, peak ${downPeak}</span>
        <span class="chart-scale">
          ${clock(t0)} → ${clock(t1)} · ${bucketSeconds}s buckets · each half scaled to its own peak
        </span>
      </figcaption>
    </figure>`;
}

// A matchup grid: rows are what the pilot flew, columns what they met. The
// count sits in the cell, so this is the table as well as the picture -- there
// is no second copy of the same numbers to keep in step.
//
// Capped, and says so when it caps: a silent top-N reads as "that is all of
// it", which is exactly the wrong impression on a page about someone's record.
// A newline separator, because a DCS type identifier can contain spaces and
// dots but never a line break, so two different pairs cannot collide on one key.
const cellKey = (a, b) => `${a}\n${b}`;

function heatmap(rows, opts) {
  if (!rows || !rows.length) return `<p class="none">Nothing recorded.</p>`;

  // Rows are aircraft by default. The weapons page passes stores instead, so
  // the label and the reference card it opens have to follow the data.
  const rowKind = opts.rowKind || "unit";
  const rowName = rowKind === "weapon" ? weaponName : unitName;

  const MAX_ROWS = 6;
  const MAX_COLS = 9;

  const total = (key) => {
    const sums = new Map();
    for (const r of rows) sums.set(r[key], (sums.get(r[key]) || 0) + r.kills);
    return [...sums.entries()].sort((a, b) => b[1] - a[1]);
  };

  const allRows = total("unitType");
  const allCols = total("targetType");
  const keepRows = allRows.slice(0, MAX_ROWS).map((e) => e[0]);
  const keepCols = allCols.slice(0, MAX_COLS).map((e) => e[0]);

  const cell = new Map();
  for (const r of rows) cell.set(`${cellKey(r.unitType, r.targetType)}`, r.kills);
  const peak = Math.max(1, ...rows.map((r) => r.kills));

  const head = keepCols
    .map((c) => `<th scope="col"><span class="hm-col">${esc(unitName(c))}</span></th>`)
    .join("");

  const body = keepRows
    .map((rt) => {
      const cells = keepCols
        .map((ct) => {
          const n = cell.get(cellKey(rt, ct)) || 0;
          if (!n) return `<td class="hm-cell"><span class="zero">·</span></td>`;
          const fill = 0.12 + (n / peak) * 0.78;
          return `<td class="hm-cell${fill > 0.55 ? " dense" : ""}" title="${esc(
            `${rowName(rt)} → ${unitName(ct)}: ${n}`
          )}"><span class="hm-fill" style="opacity:${fill.toFixed(3)}"></span><b>${n}</b></td>`;
        })
        .join("");
      return `<tr><th scope="row">${ref(rowKind, rt)}</th>${cells}</tr>`;
    })
    .join("");

  const hiddenRows = allRows.length - keepRows.length;
  const hiddenCols = allCols.length - keepCols.length;
  const note = [
    hiddenRows > 0 ? `${hiddenRows} more ${opts.rowNoun}` : "",
    hiddenCols > 0 ? `${hiddenCols} more ${opts.colNoun}` : "",
  ].filter(Boolean);

  return (
    `<div class="tw"><table class="grid hm"><thead><tr><th></th>${head}</tr></thead><tbody>${body}</tbody></table></div>` +
    (note.length ? `<p class="chart-scale">Showing the busiest — ${note.join(" and ")} not shown.</p>` : "")
  );
}

// Records: the standout moments.
//
// Everything else on this page is an aggregate, which tells you how a mission
// went but never what happened in it. This is the part someone screenshots.
//
// Distances are nautical miles and altitudes are feet, because this is a flight
// sim and that is what the HUD says. The metric value is on the tooltip, since
// the rest of the app is metric and the two should be reconcilable.
const M_TO_NM = 1 / 1852;
const M_TO_FT = 3.28084;

function recordCard(label, value, unit, detail, title) {
  return `<div class="record"${title ? ` title="${esc(title)}"` : ""}>
    <dt>${esc(label)}</dt>
    <dd><b>${value}</b>${unit ? `<span class="unit">${esc(unit)}</span>` : ""}</dd>
    <p>${detail}</p>
  </div>`;
}

function drawRecords(r) {
  if (!r || (!r.firstBlood && !r.longestKill && !r.highestKill && !r.deadliest)) {
    el("records").innerHTML = `<p class="none">No kills yet. Give it time.</p>`;
    return;
  }

  const who = (k) => pref(k.playerID, k.playerName);
  const cards = [];

  // Written as a trail rather than a sentence. "a F-14B" and "an Il-76" cannot
  // both be got right from the spelling -- F is said "eff" and takes an, Su is
  // said "soo" and takes a -- and the terse form scans better here anyway.
  const arrow = `<span class="to">→</span>`;

  if (r.firstBlood) {
    const k = r.firstBlood;
    cards.push(
      recordCard("First blood", clock(k.missionTime), "",
        `${who(k)} · ${ref("unit", k.unitType)} ${arrow} ${ref("unit", k.targetType)}`)
    );
  }

  if (r.longestKill) {
    const k = r.longestKill;
    cards.push(
      recordCard(
        "Longest kill",
        (k.rangeM * M_TO_NM).toFixed(1),
        "nm",
        `${who(k)} · ${ref("unit", k.unitType)} · ${ref("weapon", k.weaponType)} ${arrow} ${ref("unit", k.targetType)}`,
        `${Math.round(k.rangeM).toLocaleString()} m between shooter and target at the moment of the kill`
      )
    );
  }

  if (r.highestKill) {
    const k = r.highestKill;
    cards.push(
      recordCard(
        "Highest kill",
        Math.round(k.altitudeM * M_TO_FT).toLocaleString(),
        "ft",
        `${who(k)} · ${ref("unit", k.unitType)} ${arrow} ${ref("unit", k.targetType)}`,
        `${Math.round(k.altitudeM).toLocaleString()} m`
      )
    );
  }

  if (r.deadliest) {
    const w = r.deadliest;
    cards.push(
      recordCard(
        "Deadliest store",
        w.killsPerShot.toFixed(2),
        "kills/shot",
        `${ref("weapon", w.weaponType)} — ${w.kills} kills from ${w.shots} shots`,
        "Weapons with at least ten shots, so one lucky round cannot take it"
      )
    );
  }

  el("records").innerHTML = `<dl class="records">${cards.join("")}</dl>` +
    `<p class="chart-scale">
       Longest kill is the separation between shooter and target when the target died, not the launch
       range — DCS puts no target on a shot event, and a missile with a minute of flight leaves the
       shooter somewhere else by the time it arrives.
     </p>`;
}

// Mission tasks, when the mission exports any. The panel stays hidden rather
// than showing an empty table on servers whose missions say nothing.
// The mission's own state word, reduced to the three that matter.
//
// State is free text by contract -- the mission says where a task stands and
// overlord displays it without acting on it -- so the filter groups rather than
// matches exactly. Anything that is not plainly finished or plainly lost is
// still in play.
function taskState(state) {
  const v = (state || "").toLowerCase();
  if (["done", "complete", "completed", "success", "succeeded"].includes(v)) return "done";
  if (["failed", "fail", "lost"].includes(v)) return "failed";
  return "active";
}

// Mission tasks, when the mission exports any. The panel stays hidden rather
// than showing an empty table on servers whose missions say nothing.
function drawTasks(tableID, panelID, tasks, redraw = draw) {
  const panel = el(panelID);
  if (!panel) return;
  if (!tasks || !tasks.length) {
    panel.hidden = true;
    return;
  }
  panel.hidden = false;

  fillTaskPilots(tasks);

  // Counts are of everything the mission exported, not of what survives the
  // filter: a chip that reported its own filtered total would always read the
  // same number as the table beside it.
  const tally = { all: tasks.length, done: 0, failed: 0, active: 0 };
  let earned = 0;
  for (const t of tasks) {
    const s = taskState(t.state);
    tally[s]++;
    if (s === "done") earned += t.points || 0;
  }
  labelTaskChips(tally);

  const rows = tasks
    .filter((t) => state.taskState === "all" || taskState(t.state) === state.taskState)
    .filter((t) => {
      if (!state.taskPilot) return true;
      if (state.taskPilot === "none") return !t.playerName;
      return t.playerID === state.taskPilot;
    })
    .filter((t) => matches([t.title, t.taskKey, t.playerName]));

  // The tally and count belong to the filtered panel; the pilot page's own
  // task table has neither and simply skips them.
  const tallyEl = el("task-tally");
  if (tallyEl) {
    tallyEl.innerHTML =
      `<b>${num(tally.done)}</b> done · <b>${num(tally.failed)}</b> failed · ` +
      `<b>${num(tally.active)}</b> still running · <b>${num(earned)}</b> points banked`;
  }

  // The pager says where you are within the filtered set; this says how much
  // the filter itself took out.
  const countEl = el("task-count");
  if (countEl) {
    countEl.textContent = rows.length === tally.all ? "" : `${num(rows.length)} of ${num(tally.all)} match`;
  }

  render(
    tableID,
    [
      { label: "Task", key: "title" },
      { label: "State", key: "state" },
      { label: "Pilot", key: "playerName" },
      { label: "Points", key: "points", num: true },
    ],
    sortRows(rows, tableID),
    (t) => {
      const s = taskState(t.state);
      return `<tr>
        <td class="name">${esc(t.title || t.taskKey)}</td>
        <td><span class="ev ${s === "done" ? "ev-kill" : s === "failed" ? "ev-loss" : ""}">${esc(t.state || "—")}</span></td>
        <td>${t.playerID ? pref(t.playerID, t.playerName) : `<span class="zero">mission</span>`}</td>
        <td class="num">${num(t.points)}</td>
      </tr>`;
    },
    redraw
  );
}

// Chip labels carry their own totals, so the filter says what it would show
// before it is used.
function labelTaskChips(tally) {
  const group = el("task-state");
  if (!group) return;
  for (const btn of group.querySelectorAll("button")) {
    const k = btn.dataset.tstate;
    const word = btn.dataset.label || btn.textContent.trim();
    btn.dataset.label = word;
    btn.innerHTML = `${esc(word)} <b>${num(tally[k] ?? 0)}</b>`;
  }
}

// Whose tasks these are. Most are the mission's own rather than any pilot's,
// so that gets an option of its own instead of being lumped under "anyone".
// Rebuilt whenever the set of names changes rather than once, because it does
// change: the first draw of a fresh mission has no pilots in it at all, and
// switching to all time brings in every pilot who has ever been given a task.
// Filling it once left the picker permanently stuck on that first empty answer.
function fillTaskPilots(tasks) {
  const sel = el("task-pilot");
  if (!sel) return;

  const seen = new Map();
  let missionWide = false;
  for (const t of tasks) {
    if (t.playerID) seen.set(t.playerID, t.playerName);
    else missionWide = true;
  }
  if (!seen.size && !missionWide) return;

  const signature = (missionWide ? "m|" : "") + [...seen.keys()].sort().join(",");
  if (sel.dataset.signature === signature) return;
  sel.dataset.signature = signature;

  sel.innerHTML =
    `<option value="">Anyone</option>` +
    (missionWide ? `<option value="none">The mission itself</option>` : "") +
    [...seen.entries()]
      .sort((a, b) => a[1].localeCompare(b[1]))
      .map(([id, name]) => `<option value="${esc(id)}">${esc(playerLabel(name))}</option>`)
      .join("");

  // A pilot who is no longer in the list cannot stay selected, or the panel
  // would silently show nothing with no way to tell why.
  if (state.taskPilot && state.taskPilot !== "none" && !seen.has(state.taskPilot)) state.taskPilot = "";
  sel.value = state.taskPilot;
}

// Scenery. Deliberately light, deliberately apart from the real figures, and
// deliberately not called "destroyed": DCS emits a hit when a blast touches a
// tree and a kill only when it actually falls, and on this data that is 25,503
// against 80. Labelling the first number "destroyed" would overstate it by
// three hundred times.
function collateralPanel(c, scope) {
  if (!c || !c.struck) {
    return `<p class="none">Nothing hit that could not shoot back. Suspiciously clean.</p>`;
  }

  const n = (v) => v.toLocaleString();
  const top = (c.top || [])
    .slice(0, 8)
    .map(
      (s) =>
        `<li${s.tree ? ' class="is-tree"' : ""}><b>${n(s.count)}</b> ${esc(s.displayName)}</li>`
    )
    .join("");

  return `
    <p class="collateral-lede">
      <b>${n(c.trees)}</b> trees and shrubs and <b>${n(c.structures)}</b> walls, poles and buildings
      caught a blast. <b>${n(c.levelled)}</b> actually came down.
    </p>
    <ul class="scenery">${top}</ul>
    <p class="chart-scale">
      Counted apart from every other figure here, and never as a hit or a kill — a bomb finding a
      wood is not marksmanship. DCS reports a hit when a blast touches scenery and a kill only when
      it is destroyed, which is why the two numbers differ so wildly. Trees are told from buildings
      by their DCS name, so the split is a good guess rather than a fact.${
        scope === "mission"
          ? " Most scenery damage arrives with no initiator attached, so the pilot pages add up to far less than this."
          : ""
      }
    </p>`;
}

// The kill map. Leaflet owns the pan and zoom; this owns one layer of dots.
//
// The map object is created once and kept: recreating it on every refresh
// resets the view the reader has panned to, so refreshes only swap the dot
// layer, and the auto-fit runs only on the first draw with data.
let killMap = null;
let killLayer = null;
let killMapFitted = false;
// Held so the replay can re-plot without another fetch.
let killPts = [];

// Most kills name nothing: DCS fires the event after it has already
// deallocated the victim, so about two thirds of them arrive with coordinates
// and no target object. Those dots are real and belong on the map. They say so
// rather than rendering a blank, and sit at a lower opacity so a glance can
// tell a confirmed victim from an anonymous one.
const targetLabel = (t) => (t ? unitName(t) : "unknown target");
const targetOpacity = (t) => (t ? 0.6 : 0.28);

// ---- map replay -----------------------------------------------------------

// Both kill maps can play the mission back. Every dot already carries a mission
// clock, so this is a filter over points that are on the page already: no track
// sampling, no new table, no StreamUnits.
//
// Offered in mission scope only. Across all time the clock restarts with every
// mission, so one slider would interleave forty of them into nonsense.
const SCRUB_STEPS = 140; // frames in a full playthrough
const SCRUB_FRAME_MS = 110; // ~15s end to end, whatever the mission's length

const scrubs = {};

function scrubFor(id, replot) {
  let s = scrubs[id];
  if (!s) {
    s = scrubs[id] = { max: 0, at: Infinity, playing: false, timer: null, replot };
    wireScrub(id, s);
  }
  s.replot = replot;
  return s;
}

function wireScrub(id, s) {
  const wrap = el(id + "-scrub");
  if (!wrap) return;

  wrap.querySelector(".scrub-range").addEventListener("input", (e) => {
    stopScrub(s);
    s.at = Number(e.target.value);
    paintScrub(id, s);
    s.replot();
  });
  wrap.querySelector(".scrub-play").addEventListener("click", () => toggleScrub(id, s));
}

function toggleScrub(id, s) {
  if (s.playing) {
    stopScrub(s);
    paintScrub(id, s);
    return;
  }
  // Pressing play at the end restarts, rather than doing nothing.
  if (s.at >= s.max) s.at = 0;
  s.playing = true;
  paintScrub(id, s);
  s.replot();

  s.timer = setInterval(() => {
    s.at = Math.min(s.max, s.at + s.max / SCRUB_STEPS);
    if (s.at >= s.max) stopScrub(s);
    paintScrub(id, s);
    s.replot();
  }, SCRUB_FRAME_MS);
}

function stopScrub(s) {
  s.playing = false;
  if (s.timer) clearInterval(s.timer);
  s.timer = null;
}

function paintScrub(id, s) {
  const wrap = el(id + "-scrub");
  if (!wrap) return;

  const range = wrap.querySelector(".scrub-range");
  range.max = Math.max(1, Math.round(s.max));
  range.value = Math.round(Math.min(s.at, s.max));

  const play = wrap.querySelector(".scrub-play");
  play.textContent = s.playing ? "⏸" : "▶";
  play.setAttribute("aria-label", s.playing ? "Pause the replay" : "Play the mission back");
  wrap.querySelector(".scrub-clock").textContent = clock(Math.min(s.at, s.max));
}

// Fit the scrubber to the points in hand and report the current cutoff.
//
// A live mission keeps growing. Someone parked at the end should stay at the
// end as new kills arrive; someone who scrubbed back to watch an engagement
// should not be yanked forward every fifteen seconds.
function scrubCutoff(id, points, replot) {
  const wrap = el(id + "-scrub");
  const perMission = state.scope === "mission";
  const worth = perMission && points.length > 1;
  if (wrap) wrap.hidden = !worth;

  const s = scrubFor(id, replot);
  if (!worth) {
    stopScrub(s);
    s.at = Infinity;
    return Infinity;
  }

  const max = Math.max(...points.map((p) => p.missionTime || 0));
  const wasAtEnd = s.at >= s.max;
  s.max = max;
  if (wasAtEnd) s.at = max;
  paintScrub(id, s);

  return s.at;
}

function drawKillMap(points) {
  const host = el("killmap");
  if (!host) return;

  if (typeof L === "undefined") {
    host.innerHTML = `<p class="none">The map library could not load — offline? The dots will be back with the network.</p>`;
    return;
  }

  if (!killMap) {
    killMap = L.map(host, { zoomControl: true, attributionControl: true, worldCopyJump: true });
    killMap.setView([42.5, 42.0], 7);
    L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
      maxZoom: 15,
      className: "map-tiles",
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    }).addTo(killMap);
    killLayer = L.layerGroup().addTo(killMap);
  }

  killPts = points || [];
  if (!killPts.length) {
    killLayer.clearLayers();
    host.classList.add("map-empty");
    const wrap = el("killmap-scrub");
    if (wrap) wrap.hidden = true;
    return;
  }
  host.classList.remove("map-empty");

  scrubCutoff("killmap", killPts, plotKillPoints);
  plotKillPoints();

  // Framed on every point, not the visible ones, so the view holds still
  // while the replay runs.
  if (!killMapFitted) {
    killMap.fitBounds(L.latLngBounds(killPts.map((p) => [p.lat, p.lon])).pad(0.2));
    killMapFitted = true;
  }
}

function plotKillPoints() {
  if (!killLayer) return;
  killLayer.clearLayers();

  const cut = scrubs.killmap ? scrubs.killmap.at : Infinity;
  for (const p of killPts) {
    if ((p.missionTime || 0) > cut) continue;
    L.circleMarker([p.lat, p.lon], {
      radius: 5,
      weight: 1,
      color: "#ffffff",
      fillColor: "#c0382e",
      fillOpacity: targetOpacity(p.targetType),
    })
      .bindTooltip(
        `${esc(targetLabel(p.targetType))}${p.weaponType ? " · " + esc(weaponName(p.weaponType)) : ""} · ${clock(p.missionTime)}`
      )
      .addTo(killLayer);
  }
}

// The mission map: both sides' kills, coloured by coalition. Same rules as
// the pilot map -- the map object survives refreshes, only the dots swap.
let missionMap = null;
let missionLayer = null;
let missionMapFitted = false;
let missionPts = [];

const SIDE_FILL = { blue: "#2563c9", red: "#c0382e" };

function drawMissionMap(points) {
  const host = el("missionmap");
  if (!host) return;

  if (typeof L === "undefined") {
    host.innerHTML = `<p class="none">The map library could not load — offline? The dots will be back with the network.</p>`;
    return;
  }

  if (!missionMap) {
    missionMap = L.map(host, { zoomControl: true, worldCopyJump: true });
    missionMap.setView([42.5, 42.0], 7);
    L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
      maxZoom: 15,
      className: "map-tiles",
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    }).addTo(missionMap);
    missionLayer = L.layerGroup().addTo(missionMap);
  }

  missionPts = points || [];
  if (!missionPts.length) {
    missionLayer.clearLayers();
    host.classList.add("map-empty");
    const wrap = el("missionmap-scrub");
    if (wrap) wrap.hidden = true;
    return;
  }
  host.classList.remove("map-empty");

  scrubCutoff("missionmap", missionPts, plotMissionPoints);
  plotMissionPoints();

  if (!missionMapFitted) {
    missionMap.fitBounds(L.latLngBounds(missionPts.map((p) => [p.lat, p.lon])).pad(0.2));
    missionMapFitted = true;
  }
}

function plotMissionPoints() {
  if (!missionLayer) return;
  missionLayer.clearLayers();

  const cut = scrubs.missionmap ? scrubs.missionmap.at : Infinity;
  for (const p of missionPts) {
    if ((p.missionTime || 0) > cut) continue;
    L.circleMarker([p.lat, p.lon], {
      radius: 5,
      weight: 1,
      color: "#ffffff",
      fillColor: SIDE_FILL[p.coalition] || "#7a8494",
      fillOpacity: targetOpacity(p.targetType),
    })
      .bindTooltip(
        `${esc(playerLabel(p.playerName))}${p.unitType ? " · " + esc(unitName(p.unitType)) : ""} → ` +
          `${esc(targetLabel(p.targetType))}${p.weaponType ? " · " + esc(weaponName(p.weaponType)) : ""} · ${clock(p.missionTime)}`
      )
      .addTo(missionLayer);
  }
}

// The badge shelf. Earned badges are plain statements; locked ones show their
// progress bar and keep their story quiet. Deliberately career-wide whatever
// the scope toggle says -- a badge is something you keep.
function drawBadges(badges) {
  if (!badges || !badges.length) return;
  el("badges").innerHTML = badges
    .map((b) => {
      const pct = b.target ? Math.min(100, Math.round((b.progress / b.target) * 100)) : 0;
      return `<button type="button" class="medal${b.earned ? " earned" : ""}" data-badge="${esc(b.id)}"
        title="${esc(b.earned ? b.detail : b.description)}">
        <span class="medal-emoji" aria-hidden="true">${b.emoji}</span>
        ${b.earned && b.count > 1 ? `<span class="medal-count">×${b.count}</span>` : ""}
        <span class="medal-name">${esc(b.name)}</span>
        ${b.earned
          ? `<span class="medal-detail">${esc(b.detail)}</span>`
          : `<span class="medal-detail">${esc(b.description)}</span>
             <span class="medal-bar"><i style="width:${pct}%"></i></span>`}
      </button>`;
    })
    .join("");
}

// The award history behind a badge, in the same dialog the reference cards
// use. Mission-granular: sorties are not modelled yet, so a mission with its
// name, map and date is as precise as the data honestly slices.
function openBadgeCard(id) {
  const b = (state.badges || []).find((x) => x.id === id);
  if (!b) return;

  const wrap = el("card");
  el("card-name").textContent = `${b.emoji} ${b.name}`;
  el("card-sub").textContent = b.description;

  let body = "";
  if (!b.earned) {
    const pct = b.target ? Math.min(100, Math.round((b.progress / b.target) * 100)) : 0;
    body = `<p class="card-blurb">Not earned yet.</p>
      <p class="card-makers">${esc(b.detail)}</p>
      <span class="medal-bar big"><i style="width:${pct}%"></i></span>`;
  } else {
    const when = (t) =>
      new Date(t).toLocaleString([], { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
    const rows = (b.awards || [])
      .map(
        (a) => `<tr>
          <td class="num">${esc(when(a.when))}</td>
          <td>#${esc(a.missionID)}${a.missionName ? " · " + esc(a.missionName) : ""}</td>
          <td>${esc(a.theatre || "—")}</td>
          <td class="name">${esc(a.detail)}</td>
        </tr>`
      )
      .join("");

    body =
      `<p class="card-blurb">Earned ${b.count > 1 ? `${b.count} times` : "once"}.</p>` +
      `<div class="tw"><table class="grid"><thead><tr><th>When</th><th>Mission</th><th>Map</th><th>What happened</th></tr></thead><tbody>${rows}</tbody></table></div>` +
      (b.count > (b.awards || []).length
        ? `<p class="chart-scale">Showing the latest ${b.awards.length} — the count is the true total.</p>`
        : "") +
      `<p class="chart-scale">Missions from before name capture existed show only their number.</p>`;
  }

  el("card-body").innerHTML = body;
  if (!wrap.open) wrap.showModal();
}

function headline(label, value, sub) {
  return `<div><dt>${esc(label)}</dt><dd>${esc(value)}${
    sub ? ` <small>${esc(sub)}</small>` : ""
  }</dd></div>`;
}

// The dossier: superlatives with a name on them. Each line is the standout
// answer to one question -- most flown, most lethal, most lethal against them.
// A line with nothing to crown is left out rather than shown empty, and the
// panel disappears entirely for a pilot with no story yet.
function drawDossier(p) {
  const f = p.favourites || {};
  // The tally rides inside the dd: a dl group allows only dt and dd children.
  const line = (label, value, sub) =>
    `<div><dt>${esc(label)}</dt><dd>${value}<span class="dsub">${esc(sub)}</span></dd></div>`;
  const n = (v, one, many) => `${v} ${v === 1 ? one : many}`;

  const lines = [];

  // Titles first when there are any: it is the only line that says something
  // about this pilot's standing rather than their habits.
  const held = titlesLine(p.titles);
  if (held) lines.push(line("Titles held", esc(held.value), held.sub));

  // Trivial answers are omitted: the aircraft line when the page is already
  // narrowed to one airframe, the theatre line when the scope is one mission.
  if (!p.unitType && f.aircraft) {
    lines.push(line("Favourite aircraft", ref("unit", f.aircraft.name), n(f.aircraft.count, "sortie", "sorties")));
  }

  if (f.weapon) {
    lines.push(line("Weapon of choice", ref("weapon", f.weapon.name), n(f.weapon.count, "kill", "kills")));
  } else {
    // Nothing has died to anything yet; most-fired is still an answer.
    const fired = [...(p.weapons || [])].sort((a, b) => b.shots - a.shots)[0];
    if (fired && fired.shots) {
      lines.push(line("Weapon of choice", ref("weapon", fired.weaponType), `${n(fired.shots, "shot", "shots")}, nothing down yet`));
    }
  }

  if (f.prey) lines.push(line("Favourite prey", ref("unit", f.prey.name), n(f.prey.count, "destroyed", "destroyed")));
  if (f.nemesisUnit) {
    lines.push(line("Worst enemy", ref("unit", f.nemesisUnit.name), n(f.nemesisUnit.count, "loss to it", "losses to it")));
  }
  if (f.nemesisPilot) {
    lines.push(line("Nemesis", pref(f.nemesisPilot.id, f.nemesisPilot.name), n(f.nemesisPilot.count, "shoot-down", "shoot-downs")));
  }
  if (f.deadliestWeapon) {
    lines.push(line("Deadliest threat", ref("weapon", f.deadliestWeapon.name), n(f.deadliestWeapon.count, "death to it", "deaths to it")));
  }
  if (state.scope !== "mission" && f.theatre) {
    lines.push(line("Happy hunting ground", esc(f.theatre.name), n(f.theatre.count, "kill there", "kills there")));
  }

  el("p-dossier").hidden = lines.length === 0;
  el("dossier").innerHTML = lines.join("");
}

function drawPlayer() {
  const p = state.player;
  if (!p) return;

  el("player-name").textContent = playerLabel(p.playerName);

  const tags = [
    p.isAI ? `<span class="tag">AI</span>` : `<span class="tag">Pilot</span>`,
    ...(p.coalitions || []).map(
      (c) => `<span class="tag tag-${esc(c)}">${esc(c)}</span>`
    ),
  ];
  el("player-tags").innerHTML = tags.join("");

  // Landings against sorties is the closest thing to "got home in one piece"
  // the event stream offers.
  const survival = p.sorties ? Math.round((p.landings / p.sorties) * 100) : null;

  el("player-headline").innerHTML = [
    headline("Sorties", p.sorties),
    headline("Kills", p.kills, p.teamkills ? `${p.teamkills} friendly` : ""),
    headline("Shot down", p.timesKilled),
    headline("K/D", p.killDeathRatio.toFixed(2)),
    headline("Shots", p.shots),
    headline("Hits", p.hits),
    survival === null ? "" : headline("Landed", `${survival}%`, `${p.landings}/${p.sorties}`),
    headline("Ejections", p.ejections),
  ].join("");

  // The note used to carry "deadliest in" and "reaches for" lines; those are
  // the dossier's job now, with proper links and counts.
  el("player-note").textContent = p.lastSeen
    ? `Active between ${clock(p.firstSeen)} and ${clock(p.lastSeen)} mission time.`
    : "";

  drawDossier(p);
  drawRivals(p.rivalries);
  drawSorties(p.sortieLog || []);

  // Filter and per-model page are the same control. These are real links, so a
  // narrowed view is as shareable as the pilot's own page.
  // Only the busiest few as chips. A pilot with nineteen airframes would
  // otherwise get nineteen chips, which is a wall rather than a filter -- the
  // rest stay one click away in the aircraft table below.
  const base = `/player/${encodeURIComponent(p.playerID)}`;
  const flown = p.flown || [];
  const CHIPS = 7;
  const shown = flown.slice(0, CHIPS);
  if (p.unitType && !shown.includes(p.unitType)) shown.push(p.unitType);

  el("player-filter").innerHTML =
    [
      `<a class="chip${p.unitType ? "" : " on"}" href="${base}">All aircraft</a>`,
      ...shown.map(
        (t) =>
          `<a class="chip${p.unitType === t ? " on" : ""}" href="${base}/${encodeURIComponent(t)}">${esc(
            unitName(t)
          )}</a>`
      ),
    ].join("") +
    (flown.length > shown.length
      ? `<span class="chip-more">+${flown.length - shown.length} more below</span>`
      : "");

  el("player-timeline").innerHTML = timelineChart(p.timeline, p.bucketSeconds);
  el("player-collateral").innerHTML = collateralPanel(state.collateral, "player");
  drawBadges(state.badges);
  drawKillMap(p.killPoints);
  drawTasks(
    "player-tasks",
    "p-player-tasks",
    (state.tasks || []).filter(
      (t) => (t.playerID ? t.playerID === p.playerID : t.playerName === p.playerName)
    ),
    drawPlayer
  );

  const matchHay = (m) => matches([...unitHay(m.unitType), ...unitHay(m.targetType)]);

  el("p-matchups-wrap").innerHTML = heatmap((p.matchups || []).filter(matchHay), {
    rowNoun: "aircraft",
    colNoun: "target types",
  });
  el("p-killedby-wrap").innerHTML = heatmap((p.killedBy || []).filter(matchHay), {
    rowNoun: "aircraft",
    colNoun: "attacker types",
  });

  // With one airframe the aircraft table is a single row restating the
  // headline, so it goes away rather than repeating itself.
  el("p-aircraft-panel").hidden = Boolean(p.unitType);

  render(
    "p-aircraft",
    [
      { label: "Aircraft", key: "unitType" },
      { label: "Sorties", key: "sorties", num: true },
      { label: "Kills", key: "kills", num: true },
      { label: "Shots", key: "shots", num: true },
      { label: "Hits", key: "hits", num: true },
      { label: "Lost", key: "losses", num: true },
      { label: "Kills / shot", key: "killsPerShot", num: true },
      { label: "" },
    ],
    sortRows((p.aircraft || []).filter((a) => matches(unitHay(a.unitType))), "p-aircraft"),
    (a) => `<tr>
      <td class="name">${ref("unit", a.unitType)}</td>
      <td class="num">${num(a.sorties)}</td>
      <td class="num">${num(a.kills)}</td>
      <td class="num">${num(a.shots)}</td>
      <td class="num">${num(a.hits)}</td>
      <td class="num">${num(a.losses)}</td>
      <td class="num">${a.shots ? ratio(a.killsPerShot) : `<span class="zero">—</span>`}</td>
      <td class="num"><a class="row-go" href="${base}/${encodeURIComponent(a.unitType)}"
        aria-label="${esc(`Narrow to ${unitName(a.unitType)}`)}">View →</a></td>
    </tr>`,
    drawPlayer
  );

  const pWeapons = (p.weapons || []).filter((w) => matches(weaponHay(w.weaponType)));
  const pCollisions = pWeapons.some((w) => w.collisions > 0);

  render(
    "p-weapons",
    [
      { label: "Weapon", key: "weaponType" },
      { label: "Shots", key: "shots", num: true },
      { label: "Hits", key: "hits", num: true },
      { label: "Kills", key: "kills", num: true },
      ...(pCollisions ? [{ label: "Collisions", key: "collisions", num: true }] : []),
      { label: "Kills / shot", key: "killsPerShot", num: true },
    ],
    sortRows(pWeapons, "p-weapons"),
    (w) => `<tr>
      <td class="name">${ref("weapon", w.weaponType)}</td>
      <td class="num">${num(w.shots)}</td>
      <td class="num">${num(w.hits)}</td>
      <td class="num">${num(w.kills)}</td>
      ${pCollisions ? `<td class="num">${num(w.collisions)}</td>` : ""}
      <td class="num">${w.shots ? ratio(w.killsPerShot) : `<span class="zero">—</span>`}</td>
    </tr>`,
    drawPlayer
  );

  render(
    "p-grades",
    [
      { label: "Time", key: "missionTime", num: true },
      { label: "Aircraft", key: "unitType" },
      { label: "Airfield", key: "place" },
      { label: "Grade", key: "grade" },
    ],
    sortRows(
      (p.landingGrades || []).filter((g) => matches([...unitHay(g.unitType), g.place, g.grade])),
      "p-grades"
    ),
    (g) => `<tr>
      <td class="num">${clock(g.missionTime)}</td>
      <td>${g.unitType ? ref("unit", g.unitType) : "—"}</td>
      <td>${esc(g.place || "—")}</td>
      <td class="grade">${esc(g.grade || "—")}</td>
    </tr>`,
    drawPlayer
  );
}

// The id is in the path, since this is a page rather than a view: /player/2.
function playerFromPath() {
  const m = location.pathname.match(/^\/player\/([^/]+)(?:\/([^/]+))?\/?$/);
  if (!m) return null;
  return { id: decodeURIComponent(m[1]), unitType: m[2] ? decodeURIComponent(m[2]) : null };
}

async function loadPlayer() {
  const route = playerFromPath();
  if (!route) return;
  const { id, unitType } = route;

  try {
    // The reference cards need display names, which the dashboard query
    // normally fills in. This page has to ask for them itself.
    if (state.scope === "mission" && !state.missionID) {
      const m = await gql(`{ missions { id } }`);
      state.missionID = m.missions?.[0]?.id || null;
    }
    const m = state.scope === "mission" ? state.missionID : null;

    const [profile, lookup, side, shelf, taskData] = await Promise.all([
      gqlVars(PLAYER_QUERY, { id, unitType, m }),
      gql(`{ missions { id } units { type displayName } weapons { type displayName } }`),
      gqlVars(
        `query($id: ID!, $m: ID) { collateral(playerID: $id, missionID: $m) { struck levelled trees structures top { displayName count tree } } }`,
        { id, m }
      ),
      gqlVars(
        `query($id: ID!) { badges(playerID: $id) { id name emoji description earned count progress target detail awards { missionID missionName theatre when detail } } }`,
        { id }
      ),
      gqlVars(
        `query($m: ID) { missionTasks(missionID: $m) { taskKey title state playerName playerID points } }`,
        { m }
      ),
    ]);
    state.collateral = side.collateral;
    state.badges = shelf.badges;
    state.tasks = taskData.missionTasks;

    // Follow the server onto a new mission, same as the dashboard.
    const newest = lookup.missions?.[0]?.id || null;
    if (state.scope === "mission" && newest && newest !== state.missionID) {
      state.missionID = newest;
      return loadPlayer();
    }

    for (const u of lookup.units || []) names.unit.set(u.type, u.displayName);
    for (const w of lookup.weapons || []) names.weapon.set(w.type, w.displayName);

    // The query succeeding and there being no such pilot are different things.
    // Reporting the second as "Offline" blames the connection for a perfectly
    // good answer, and sends you looking for a fault that is not there.
    state.lastSync = Date.now();
    el("link").textContent = "Live · just now";
    el("link").className = "status up";
    el("foot").textContent = `${API_URL} · refreshing every ${REFRESH_MS / 1000}s`;

    if (!profile.playerProfile) {
      document.title = "Unknown pilot — Overlord";
      el("player-name").textContent = "No such pilot";
      el("player-note").textContent = `Nothing is recorded against player ${id}.`;
      return;
    }

    state.player = profile.playerProfile;
    // The airframe belongs in the title too: a narrowed page is its own page,
    // and a tab or bookmark reading only the pilot's name would be a different
    // URL wearing the same label.
    const who = playerLabel(state.player.playerName);
    document.title = unitType ? `${who} · ${unitName(unitType)} — Overlord` : `${who} — Overlord`;
    drawPlayer();
  } catch (err) {
    el("player-name").textContent = "Could not load this pilot";
    el("player-note").textContent = err.message;
    el("link").textContent = "Offline";
    el("link").className = "status down";
  }
}

// --- reference card --------------------------------------------------------

const CARD_UNIT = `query($t: String!) { unitProfile(type: $t) {
  type curated name nickname role origin maker blurb source
  specs { qid lengthM wingspanM heightM massKg firstFlight serviceEntry totalProduced makers }
  sorties shots hits kills losses ejections timesKilled
  stores { weaponType count }
} }`;

const CARD_WEAPON = `query($t: String!) { weaponProfile(type: $t) {
  type curated name nickname role origin maker blurb source
  specs { qid lengthM wingspanM heightM massKg firstFlight serviceEntry totalProduced makers }
  shots hits kills collisions hitsPerShot killsPerShot
  carriers { unitType weapons { count } }
} }`;

async function gqlVars(query, variables) {
  const res = await fetch(API_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, variables }),
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const body = await res.json();
  if (body.errors) throw new Error(body.errors.map((e) => e.message).join("; "));
  return body.data;
}

function fact(label, value, big) {
  return `<div><dt>${esc(label)}</dt><dd${big ? ' class="big"' : ""}>${esc(value)}</dd></div>`;
}

// Always offer somewhere to read further. A recorded article when we have one,
// otherwise a search, which cannot 404.
function readMore(p) {
  const href = p.source || `https://en.wikipedia.org/w/index.php?search=${encodeURIComponent(p.name)}`;
  const label = p.source ? "Read more on Wikipedia" : "Search Wikipedia for this";
  return `<p class="card-more"><a href="${esc(href)}" target="_blank" rel="noopener noreferrer">${label} →</a></p>`;
}

// Wikidata coverage is uneven, so only render what is actually there. A missing
// field means nothing was recorded, which is not the same as zero.
function specFacts(p) {
  const s = p.specs;
  if (!s) return "";

  const rows = [];
  const m = (v) => `${v.toFixed(2).replace(/\.00$/, "")} m`;

  if (s.lengthM) rows.push(fact("length", m(s.lengthM)));
  if (s.wingspanM) rows.push(fact("wingspan", m(s.wingspanM)));
  if (s.heightM) rows.push(fact("height", m(s.heightM)));
  if (s.massKg) {
    rows.push(fact("mass", s.massKg >= 1000
      ? `${(s.massKg / 1000).toFixed(s.massKg >= 10000 ? 0 : 1)} t`
      : `${s.massKg.toFixed(0)} kg`));
  }
  if (s.firstFlight) rows.push(fact("first flight", s.firstFlight));
  if (s.serviceEntry) rows.push(fact("in service", s.serviceEntry));
  if (s.totalProduced) rows.push(fact("built", s.totalProduced.toLocaleString()));

  const makers = (s.makers || []).join(", ");
  if (!rows.length && !makers) return "";

  return `<h4>Specifications</h4>` +
    (rows.length ? `<dl class="card-facts">${rows.join("")}</dl>` : "") +
    (makers ? `<p class="card-makers">Built by ${esc(makers)}</p>` : "") +
    `<p class="card-prov">From <a href="https://www.wikidata.org/wiki/${esc(s.qid)}" target="_blank" rel="noopener noreferrer">Wikidata ${esc(s.qid)}</a>, which is public domain. Fields Wikidata does not hold are omitted rather than guessed.</p>`;
}

function identityFacts(p) {
  const rows = [];
  if (p.role) rows.push(fact("role", p.role));
  if (p.origin) rows.push(fact("origin", p.origin));
  if (p.maker) rows.push(fact("built by", p.maker));
  if (p.nickname) rows.push(fact("known as", p.nickname));
  return rows.join("");
}

// Said plainly rather than dressed up: reference text is curated by hand and
// carries no performance figures, so the card should not imply otherwise.
function provenance(p) {
  return p.curated
    ? ""
    : `<p class="card-uncurated">No reference entry for <code>${esc(p.type)}</code> yet — the name above is derived from the DCS identifier. Everything below is measured from recorded events.</p>`;
}

async function openCard(kind, type) {
  const wrap = el("card");
  el("card-name").textContent = kind === "unit" ? unitName(type) : weaponName(type);
  el("card-sub").textContent = type;
  el("card-body").innerHTML = `<p class="card-blurb">Loading…</p>`;
  if (!wrap.open) wrap.showModal();
  location.hash = `#/${kind}/${encodeURIComponent(type)}`;

  try {
    if (kind === "unit") {
      const p = (await gqlVars(CARD_UNIT, { t: type })).unitProfile;
      if (!p) throw new Error("no profile");

      el("card-name").textContent = p.name;
      el("card-sub").textContent = `${p.type}${p.role ? " · " + p.role : ""}`;

      el("card-body").innerHTML =
        (p.blurb ? `<p class="card-blurb">${esc(p.blurb)}</p>` : "") +
        provenance(p) +
        `<dl class="card-facts">${identityFacts(p)}</dl>` +
        readMore(p) +
        specFacts(p) +
        `<h4>Recorded, all time</h4>` +
        `<dl class="card-facts">
          ${fact("sorties", p.sorties, true)}
          ${fact("shots", p.shots, true)}
          ${fact("hits scored", p.hits, true)}
          ${fact("kills", p.kills, true)}
          ${fact("lost", p.losses)}
          ${fact("ejections", p.ejections)}
          ${fact("killed as target", p.timesKilled)}
        </dl>` +
        (p.stores.length
          ? `<h4>Weapons fired</h4><div class="tw"><table class="grid"><tbody>${p.stores
              .map((s) => `<tr><td>${ref("weapon", s.weaponType)}</td><td class="num">${s.count}</td></tr>`)
              .join("")}</tbody></table></div>`
          : `<h4>Weapons fired</h4><p class="none">Nothing recorded.</p>`);
    } else {
      const p = (await gqlVars(CARD_WEAPON, { t: type })).weaponProfile;
      if (!p) throw new Error("no profile");

      el("card-name").textContent = p.name;
      el("card-sub").textContent = `${p.type}${p.role ? " · " + p.role : ""}`;

      el("card-body").innerHTML =
        (p.blurb ? `<p class="card-blurb">${esc(p.blurb)}</p>` : "") +
        provenance(p) +
        `<dl class="card-facts">${identityFacts(p)}</dl>` +
        readMore(p) +
        specFacts(p) +
        `<h4>Recorded, all time</h4>` +
        `<dl class="card-facts">
          ${fact("shots", p.shots, true)}
          ${fact("hits", p.hits, true)}
          ${fact("kills", p.kills, true)}
          ${p.collisions ? fact("collisions", p.collisions, true) : ""}
          ${fact("hits / shot", p.shots ? p.hitsPerShot.toFixed(2) : "—")}
          ${fact("kills / shot", p.shots ? p.killsPerShot.toFixed(2) : "—")}
        </dl>` +
        (p.carriers.length
          ? `<h4>Carried by</h4><div class="tw"><table class="grid"><tbody>${p.carriers
              .map((c) => `<tr><td>${ref("unit", c.unitType)}</td><td class="num">${c.weapons[0]?.count ?? 0}</td></tr>`)
              .join("")}</tbody></table></div>`
          : `<h4>Carried by</h4><p class="none">Nothing recorded.</p>`);
    }
  } catch (err) {
    el("card-body").innerHTML = `<p class="card-uncurated">Could not load this card: ${esc(err.message)}</p>`;
  }
}

function clearDeepLink() {
  if (location.hash.startsWith("#/")) history.replaceState(null, "", location.pathname);
}

function closeCard() {
  const card = el("card");
  if (card.open) card.close();
  clearDeepLink();
}

// Escape dismisses a modal dialog without any handler of ours, and that path
// does not run closeCard, so the deep link is dropped here as well. Belt and
// braces on purpose: both routes are idempotent, and leaving it to the close
// event alone would strand the URL on a card that is no longer open.
el("card").addEventListener("close", clearDeepLink);

// One listener for the whole document, so it survives every table redraw.
document.addEventListener("click", (e) => {
  // Pilot names are ordinary links now, so they are left to the browser.
  const medal = e.target.closest(".medal");
  if (medal) {
    openBadgeCard(medal.dataset.badge);
    return;
  }
  const r = e.target.closest(".ref");
  if (r) {
    openCard(r.dataset.ref, r.dataset.type);
    return;
  }
  if (e.target.id === "card-close") closeCard();
});

// Clicking the backdrop closes the card: closedby="any" on the element makes
// that native where supported. Elsewhere a backdrop click lands on the dialog
// element itself for hit-testing purposes -- but so does a click on the
// dialog's own padding, so the point has to be tested against the box before
// it counts as a dismissal.
if (!("closedBy" in HTMLDialogElement.prototype)) {
  el("card").addEventListener("click", (e) => {
    if (e.target !== e.currentTarget) return;
    const r = e.currentTarget.getBoundingClientRect();
    const onDialog =
      r.top <= e.clientY && e.clientY <= r.bottom &&
      r.left <= e.clientX && e.clientX <= r.right;
    if (!onDialog) closeCard();
  });
}

// Escape and the close button are the dialog's own job now. This only covers
// activating a reference name, which is a span rather than a button.
document.addEventListener("keydown", (e) => {
  if (e.key !== "Enter" && e.key !== " ") return;
  const r = document.activeElement?.closest?.(".ref");
  if (r) {
    e.preventDefault();
    openCard(r.dataset.ref, r.dataset.type);
  }
});

// Reference cards stay hash-addressed: they are an overlay on whichever page is
// open, so #/unit/F-15C works on both. Pilots are pages and live in the path.
function openFromHash() {
  const card = location.hash.match(/^#\/(unit|weapon)\/(.+)$/);
  if (card) {
    openCard(card[1], decodeURIComponent(card[2]));
    return;
  }
  closeCard();
}
window.addEventListener("hashchange", openFromHash);

// --- wiring ----------------------------------------------------------------

// One script serves both documents. Everything above is shared; what gets wired
// up below depends on which page loaded it, because the dashboard's controls do
// not exist on a pilot page and vice versa.
const PAGE = document.body.dataset.page;

// A mission recap is one finished run, pinned by its own URL. Everything else
// on the page is the dashboard's, asking the same scoped queries -- the only
// difference is that the mission never changes underneath it.
if (PAGE === "mission") {
  const m = location.pathname.match(/^\/mission\/([^/]+)\/?$/);
  state.scope = "mission";
  state.missionID = m ? decodeURIComponent(m[1]) : null;
  state.pinnedMission = true;
}

// Whichever page this is, it polls the same way.
const tick = PAGE === "player" ? loadPlayer : refresh;

let timer = null;
function schedule() {
  clearInterval(timer);
  // Also covers a page loaded straight into a background tab, which the
  // visibilitychange listener below never sees a transition for.
  if (el("live").checked && !document.hidden) timer = setInterval(tick, REFRESH_MS);
}

// A dashboard lives in background tabs, and polling a page nobody can see
// spends battery and server alike. Hiding the tab stops the clock; coming
// back refreshes once so the numbers are current, then restarts it. Live
// unchecked is the user's own freeze and stays frozen across the round trip.
document.addEventListener("visibilitychange", () => {
  if (document.hidden) {
    clearInterval(timer);
    // A replay running in a tab nobody is looking at is the same waste, and
    // it would be most of the way through by the time they came back.
    for (const id of Object.keys(scrubs)) {
      stopScrub(scrubs[id]);
      paintScrub(id, scrubs[id]);
    }
  } else if (el("live").checked) {
    tick();
    schedule();
  }
});

// Narrowing what a table shows invalidates where you were in it: page twelve
// of everything is not page twelve of the four rows that matched.
function resetPages() {
  state.page = {};
}

function chips(container, attr, onPick) {
  // Not every control exists on every page: a mission recap is one finished
  // run, so it has no scope toggle to wire.
  if (!el(container)) return;
  el(container).addEventListener("click", (e) => {
    const btn = e.target.closest("button");
    if (!btn) return;
    el(container).querySelectorAll("button").forEach((b) => b.classList.toggle("on", b === btn));
    onPick(btn.dataset[attr]);
    resetPages();
    draw();
  });
}

// The scope toggle exists on both pages and refetches rather than redraws,
// since it changes what every query asks for.
chips("scope", "scope", (v) => {
  state.scope = v;
  state.missionID = null;
  tick();
});

// Mark the section we are in. A mission recap belongs under Missions -- it is
// one of them -- so it lights that link rather than none.
{
  const here = PAGE === "mission" ? "missions" : PAGE;
  const link = document.querySelector(`.nav a[data-nav="${here}"]`);
  if (link) {
    link.classList.add("on");
    link.setAttribute("aria-current", "page");
  }
}

if (["dashboard", "mission", "weapons", "log", "landings", "airframes"].includes(PAGE)) {
  chips("weapon-class", "class", (v) => (state.weaponClass = v));

  // Both narrow what is already loaded, so they redraw rather than refetch.
  chips("task-state", "tstate", (v) => (state.taskState = v));
  if (el("task-pilot")) {
    el("task-pilot").addEventListener("change", (e) => {
      state.taskPilot = e.target.value;
      resetPages();
      draw();
    });
  }

  // These two change what the query asks for, so they refetch rather than redraw.
  chips("log-filter", "ev", (v) => {
    state.logGroup = v;
    refreshLog();
  });
  chips("log-side", "side", (v) => {
    state.logSide = v;
    refreshLog();
  });

  el("q").addEventListener("input", (e) => {
    state.query = e.target.value.trim().toLowerCase();
    resetPages();
    draw();
  });
}

if (PAGE === "player") {
  el("q").addEventListener("input", (e) => {
    state.query = e.target.value.trim().toLowerCase();
    resetPages();
    drawPlayer();
  });
}

if (["missions", "landings", "airframes"].includes(PAGE)) {
  el("q").addEventListener("input", (e) => {
    state.query = e.target.value.trim().toLowerCase();
    resetPages();
    draw();
  });

  el("pilot-filter").addEventListener("change", (e) => {
    state.pilotFilter = e.target.value;
    resetPages();
    try {
      localStorage.setItem("overlord-pilot", state.pilotFilter);
    } catch {
      /* private mode: the filter still works, it just will not be remembered */
    }
    draw();
  });
}

// Copy link. The URL bar already holds it, but the point of a recap is that it
// gets pasted into chat, and a button says so in a way an address bar does not.
if (el("share")) {
  el("share").addEventListener("click", async () => {
    const btn = el("share");
    try {
      await navigator.clipboard.writeText(location.href);
      btn.textContent = "Copied";
    } catch {
      // Denied, or an insecure origin. Select it instead so the keyboard still
      // works -- failing silently would look like the button is broken.
      btn.textContent = "Copy from the address bar";
    }
    setTimeout(() => (btn.textContent = "Copy link"), 2000);
  });
}

el("live").addEventListener("change", schedule);

// Theme. The inline script in index.html has already resolved and applied one
// before first paint, so this only has to label the button and handle clicks.
// The attribute is the single source of truth -- reading the button's own text
// back to decide the next state breaks the moment the label is reworded.
const themeBtn = el("theme");
const system = window.matchMedia("(prefers-color-scheme: dark)");

function applyTheme(mode) {
  const root = document.documentElement;

  // Swap the tokens with transitions off, then hand them back; see
  // .theme-switching in app.css for why. Reading offsetHeight forces the new
  // colours to be resolved while the guard is still in place.
  //
  // setTimeout rather than requestAnimationFrame: a hidden or non-compositing
  // tab never paints, so rAF never runs and the guard would stay on for the
  // rest of the session.
  root.classList.add("theme-switching");
  root.setAttribute("data-theme", mode);
  void root.offsetHeight;
  setTimeout(() => root.classList.remove("theme-switching"), 0);

  // Keep the meta declaration in step, so the browser paints native surfaces
  // for the pinned theme rather than the light-dark pair declared for load.
  const scheme = document.querySelector('meta[name="color-scheme"]');
  if (scheme) scheme.content = mode;

  // The button names where you are going, not where you are.
  themeBtn.textContent = mode === "dark" ? "☀ Light" : "☾ Dark";
  themeBtn.setAttribute(
    "aria-label",
    mode === "dark" ? "Switch to light theme" : "Switch to dark theme"
  );
}

applyTheme(document.documentElement.getAttribute("data-theme") || "dark");

themeBtn.addEventListener("click", () => {
  const next =
    document.documentElement.getAttribute("data-theme") === "dark" ? "light" : "dark";
  try {
    localStorage.setItem("overlord-theme", next);
  } catch (e) {
    /* private mode: the choice still applies, it just is not remembered */
  }
  applyTheme(next);
});

// Follow the OS while the user has not made a choice of their own.
system.addEventListener("change", (e) => {
  let saved = null;
  try {
    saved = localStorage.getItem("overlord-theme");
  } catch (err) {
    /* ignore */
  }
  if (!saved) applyTheme(e.matches ? "dark" : "light");
});

// The status ages in place -- "12s ago" reads as alive where a fixed
// timestamp reads as a log line. Stops counting the moment a fetch fails,
// since the down state owns the text then.
setInterval(() => {
  if (document.hidden) return;
  const link = el("link");
  if (!state.lastSync || !link.classList.contains("up")) return;
  const age = Math.round((Date.now() - state.lastSync) / 1000);
  link.textContent = age < 3 ? "Live · just now" : `Live · ${age}s ago`;
}, 1000);

tick().then(openFromHash);
schedule();
