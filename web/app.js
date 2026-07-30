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
  sort: {
    weapons: { key: "shots", dir: -1 },
    pilots: { key: "takeoffs", dir: -1 },
    loadout: { key: "shots", dir: -1 },
    traps: { key: "missionTime", dir: -1 },
    log: { key: "id", dir: -1 },
    "p-aircraft": { key: "sorties", dir: -1 },
    "p-weapons": { key: "shots", dir: -1 },
    "p-matchups": { key: "kills", dir: -1 },
    "p-killedby": { key: "kills", dir: -1 },
    "p-grades": { key: "missionTime", dir: -1 },
  },
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

const QUERY = `{
  killsByCoalition { coalition kills teamkills }
  weaponEffectiveness { weaponType shots hits kills hitsPerShot killsPerShot }
  shotsByPlayers { playerID playerName units { unitType weapons { weaponType count } } }
  playerActivity { playerID playerName takeoffs landings crashes ejections deaths }
  landingGrades(first: 40) { playerName unitType place grade missionTime }
  units { type displayName }
  weapons { type displayName }
}`;

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

function matches(haystack) {
  if (!state.query) return true;
  return haystack.join(" ").toLowerCase().includes(state.query);
}

// Guns report hits with no shot events; airframes appear as weapons when DCS
// names the aircraft for a collision.
function storeClass(row) {
  if (row.weaponType.startsWith("weapons.shells.")) return "gun";
  if (row.shots > 0) return "ordnance";
  return "collision";
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
function render(table, columns, rows, renderRow, redraw = draw) {
  const node = el(table);

  if (!rows.length) {
    node.innerHTML = `<tbody><tr><td class="none">Nothing matches.</td></tr></tbody>`;
    return;
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

  node.innerHTML = `<thead><tr>${head}</tr></thead><tbody>${rows.map(renderRow).join("")}</tbody>`;

  if (held) node.querySelector(`button[data-sort="${CSS.escape(held)}"]`)?.focus();

  node.querySelectorAll("button[data-sort]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const key = btn.dataset.sort;
      const s = state.sort[table];
      s.dir = s.key === key ? -s.dir : -1;
      s.key = key;
      redraw();
    });
  });
}

// --- sections --------------------------------------------------------------

function drawCoalition(rows) {
  const order = { blue: 0, red: 1, neutral: 2, unknown: 3 };
  const sorted = [...rows].sort((a, b) => (order[a.coalition] ?? 9) - (order[b.coalition] ?? 9));
  const max = Math.max(1, ...sorted.map((r) => r.kills));

  el("coalition").innerHTML = sorted
    .map(
      (c) => `<div class="side" data-side="${esc(c.coalition)}">
        <div class="side-top">
          <span class="side-name">${esc(c.coalition)}</span>
          <span class="side-num"><b>${c.kills}</b> <i>kills</i>${
            c.teamkills ? ` <span class="tk">· ${c.teamkills} friendly fire</span>` : ""
          }</span>
        </div>
        <span class="meter"><span style="width:${(c.kills / max) * 100}%"></span></span>
      </div>`
    )
    .join("");
}

function drawWeapons(rows) {
  const filtered = rows
    .filter((r) => r.shots || r.hits || r.kills)
    .filter((r) => state.weaponClass === "all" || storeClass(r) === state.weaponClass)
    .filter((r) => matches([r.weaponType]));

  const max = Math.max(1, ...filtered.map((r) => r.shots));

  render(
    "weapons",
    [
      { label: "Weapon", key: "weaponType" },
      { label: "Shots", key: "shots", num: true },
      { label: "Hits", key: "hits", num: true },
      { label: "Kills", key: "kills", num: true },
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
      <td class="num">${r.shots ? ratio(r.hitsPerShot) : `<span class="zero">—</span>`}</td>
      <td class="num">${r.shots ? ratio(r.killsPerShot) : `<span class="zero">—</span>`}</td>
    </tr>`
  );
}

function drawPilots(rows) {
  // Quiet AI buckets are hidden, but a human is always listed even with nothing
  // against their name yet: they are a real person with a page, and dropping
  // the row is the difference between "no sorties" and "not here".
  const filtered = rows
    .filter(
      (r) =>
        !isAIName(r.playerName) ||
        r.takeoffs || r.landings || r.crashes || r.ejections || r.deaths
    )
    .filter((r) => matches([r.playerName]));

  render(
    "pilots",
    [
      { label: "Pilot", key: "playerName" },
      { label: "Takeoffs", key: "takeoffs", num: true },
      { label: "Landings", key: "landings", num: true },
      { label: "Crashes", key: "crashes", num: true },
      { label: "Ejections", key: "ejections", num: true },
      { label: "Deaths", key: "deaths", num: true },
    ],
    sortRows(filtered, "pilots"),
    (r) => `<tr>
      <td class="name">${pref(r.playerID, r.playerName)}</td>
      <td class="num">${num(r.takeoffs)}</td>
      <td class="num">${num(r.landings)}</td>
      <td class="num">${num(r.crashes)}</td>
      <td class="num">${num(r.ejections)}</td>
      <td class="num">${num(r.deaths)}</td>
    </tr>`
  );
}

function drawTraps(rows) {
  const filtered = rows.filter((r) => matches([r.playerName, r.unitType, r.place, r.grade]));

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

// Flattened so it can be sorted and filtered as one table rather than a tree.
function drawLoadout(players) {
  const rows = [];
  for (const p of players) {
    for (const u of p.units || []) {
      for (const w of u.weapons || []) {
        rows.push({
          playerID: p.playerID,
          playerName: p.playerName,
          unitType: u.unitType,
          weaponType: w.weaponType,
          shots: w.count,
        });
      }
    }
  }

  const filtered = rows.filter((r) => matches([r.playerName, r.unitType, r.weaponType]));
  const max = Math.max(1, ...filtered.map((r) => r.shots));

  render(
    "loadout",
    [
      { label: "Pilot", key: "playerName" },
      { label: "Aircraft", key: "unitType" },
      { label: "Weapon", key: "weaponType" },
      { label: "Shots", key: "shots", num: true },
    ],
    sortRows(filtered, "loadout"),
    (r) => `<tr>
      <td class="name">${pref(r.playerID, r.playerName)}</td>
      <td>${ref("unit", r.unitType)}</td>
      <td>${ref("weapon", r.weaponType)}</td>
      <td class="num"><span class="bar" style="width:${(r.shots / max) * 44}px"></span>${r.shots}</td>
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
        n.player?.playerName, n.initiator?.type, n.initiatorName,
        n.initiatorCallsign, n.initiatorGroup, n.weapon?.type,
        n.target?.unit?.type, n.targetName, n.place,
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

      return `<tr>
        <td class="num">${clock(n.missionTime)}</td>
        <td><span class="ev ${EV_CLASS[n.event] || "ev-sortie"}" title="${esc(n.event)}">${esc(eventLabel(n.event))}</span></td>
        <td class="name">${side}${esc(actor)}</td>
        <td>${n.initiatorType ? ref("unit", n.initiatorType) : "—"}</td>
        <td>${n.weaponType ? ref("weapon", n.weaponType) : `<span class="zero">—</span>`}</td>
        <td>${target}${n.place ? ` <span class="zero">${esc(n.place)}</span>` : ""}</td>
      </tr>`;
    }
  );
}

// --- draw / refresh --------------------------------------------------------

function draw() {
  const d = state.data;
  if (!d) return;

  for (const u of d.units || []) names.unit.set(u.type, u.displayName);
  for (const w of d.weapons || []) names.weapon.set(w.type, w.displayName);

  drawCoalition(d.killsByCoalition || []);
  drawWeapons(d.weaponEffectiveness || []);
  drawPilots(d.playerActivity || []);
  drawTraps(d.landingGrades || []);
  drawLoadout(d.shotsByPlayers || []);
  drawLog(state.log);

  const nodes = (state.log?.edges || []).map((e) => e.node);
  const latest = Math.max(0, ...nodes.map((n) => n.missionTime || 0));
  const sorties = (d.playerActivity || []).reduce((s, p) => s + p.takeoffs, 0);
  const kills = (d.killsByCoalition || []).reduce((s, c) => s + c.kills, 0);

  el("ro-clock").textContent = clock(latest);
  el("ro-events").textContent = nodes.length >= LOG_ROWS ? `${LOG_ROWS}+` : String(nodes.length);
  el("ro-sorties").textContent = String(sorties);
  el("ro-kills").textContent = String(kills);
}

async function refresh() {
  const link = el("link");
  try {
    const [summary, log] = await Promise.all([gql(QUERY), fetchLog()]);
    state.data = summary;
    state.log = log;
    draw();
    link.textContent = `Updated ${new Date().toLocaleTimeString([], { hour12: false })}`;
    link.className = "status up";
    el("foot").textContent = `${API_URL} · last ${LOG_ROWS} events · refreshing every ${REFRESH_MS / 1000}s`;
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

const PLAYER_QUERY = `query($id: ID!) { playerProfile(playerID: $id) {
  playerID playerName isAI coalitions
  sorties landings crashes ejections deaths shots hits kills teamkills timesKilled
  killDeathRatio firstSeen lastSeen
  aircraft { unitType sorties landings shots hits kills losses ejections hitsPerShot killsPerShot }
  weapons { weaponType shots hits kills hitsPerShot killsPerShot }
  matchups { unitType targetType kills }
  killedBy { unitType targetType kills }
  landingGrades { unitType place grade missionTime }
} }`;

function headline(label, value, sub) {
  return `<div><dt>${esc(label)}</dt><dd>${esc(value)}${
    sub ? ` <small>${esc(sub)}</small>` : ""
  }</dd></div>`;
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
  const best = [...(p.aircraft || [])].sort((a, b) => b.kills - a.kills)[0];
  const favourite = [...(p.weapons || [])].sort((a, b) => b.shots - a.shots)[0];

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

  const bits = [];
  if (best && best.kills) bits.push(`Deadliest in the ${unitName(best.unitType)} with ${best.kills} kills.`);
  if (favourite && favourite.shots) bits.push(`Reaches for the ${weaponName(favourite.weaponType)} most often.`);
  if (p.lastSeen) bits.push(`Active between ${clock(p.firstSeen)} and ${clock(p.lastSeen)} mission time.`);
  el("player-note").textContent = bits.join(" ");

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
    ],
    sortRows(p.aircraft || [], "p-aircraft"),
    (a) => `<tr>
      <td class="name">${ref("unit", a.unitType)}</td>
      <td class="num">${num(a.sorties)}</td>
      <td class="num">${num(a.kills)}</td>
      <td class="num">${num(a.shots)}</td>
      <td class="num">${num(a.hits)}</td>
      <td class="num">${num(a.losses)}</td>
      <td class="num">${a.shots ? ratio(a.killsPerShot) : `<span class="zero">—</span>`}</td>
    </tr>`,
    drawPlayer
  );

  render(
    "p-weapons",
    [
      { label: "Weapon", key: "weaponType" },
      { label: "Shots", key: "shots", num: true },
      { label: "Hits", key: "hits", num: true },
      { label: "Kills", key: "kills", num: true },
      { label: "Kills / shot", key: "killsPerShot", num: true },
    ],
    sortRows(p.weapons || [], "p-weapons"),
    (w) => `<tr>
      <td class="name">${ref("weapon", w.weaponType)}</td>
      <td class="num">${num(w.shots)}</td>
      <td class="num">${num(w.hits)}</td>
      <td class="num">${num(w.kills)}</td>
      <td class="num">${w.shots ? ratio(w.killsPerShot) : `<span class="zero">—</span>`}</td>
    </tr>`,
    drawPlayer
  );

  render(
    "p-matchups",
    [
      { label: "Flying", key: "unitType" },
      { label: "Killed", key: "targetType" },
      { label: "Count", key: "kills", num: true },
    ],
    sortRows(p.matchups || [], "p-matchups"),
    (m) => `<tr>
      <td class="name">${ref("unit", m.unitType)}</td>
      <td>${ref("unit", m.targetType)}</td>
      <td class="num">${num(m.kills)}</td>
    </tr>`,
    drawPlayer
  );

  render(
    "p-killedby",
    [
      { label: "Flying", key: "unitType" },
      { label: "Killed by", key: "targetType" },
      { label: "Count", key: "kills", num: true },
    ],
    sortRows(p.killedBy || [], "p-killedby"),
    (m) => `<tr>
      <td class="name">${ref("unit", m.unitType)}</td>
      <td>${ref("unit", m.targetType)}</td>
      <td class="num">${num(m.kills)}</td>
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
    sortRows(p.landingGrades || [], "p-grades"),
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
function playerIDFromPath() {
  const m = location.pathname.match(/^\/player\/([^/]+)\/?$/);
  return m ? decodeURIComponent(m[1]) : null;
}

async function loadPlayer() {
  const id = playerIDFromPath();
  if (!id) return;

  try {
    // The reference cards need display names, which the dashboard query
    // normally fills in. This page has to ask for them itself.
    const [profile, lookup] = await Promise.all([
      gqlVars(PLAYER_QUERY, { id }),
      gql(`{ units { type displayName } weapons { type displayName } }`),
    ]);

    for (const u of lookup.units || []) names.unit.set(u.type, u.displayName);
    for (const w of lookup.weapons || []) names.weapon.set(w.type, w.displayName);

    // The query succeeding and there being no such pilot are different things.
    // Reporting the second as "Offline" blames the connection for a perfectly
    // good answer, and sends you looking for a fault that is not there.
    el("link").textContent = `Updated ${new Date().toLocaleTimeString([], { hour12: false })}`;
    el("link").className = "status up";
    el("foot").textContent = `${API_URL} · refreshing every ${REFRESH_MS / 1000}s`;

    if (!profile.playerProfile) {
      document.title = "Unknown pilot — Overlord";
      el("player-name").textContent = "No such pilot";
      el("player-note").textContent = `Nothing is recorded against player ${id}.`;
      return;
    }

    state.player = profile.playerProfile;
    document.title = `${playerLabel(state.player.playerName)} — Overlord`;
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
  shots hits kills hitsPerShot killsPerShot
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
        `<h4>Recorded this mission</h4>` +
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
        `<h4>Recorded this mission</h4>` +
        `<dl class="card-facts">
          ${fact("shots", p.shots, true)}
          ${fact("hits", p.hits, true)}
          ${fact("kills", p.kills, true)}
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
  const r = e.target.closest(".ref");
  if (r) {
    openCard(r.dataset.ref, r.dataset.type);
    return;
  }
  // A modal dialog fills the viewport for hit-testing purposes, so a click on
  // the backdrop lands on the dialog element itself rather than on anything
  // inside it. That is the signal to close.
  if (e.target.id === "card-close" || e.target.id === "card") closeCard();
});

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

// Whichever page this is, it polls the same way.
const tick = PAGE === "player" ? loadPlayer : refresh;

let timer = null;
function schedule() {
  clearInterval(timer);
  if (el("live").checked) timer = setInterval(tick, REFRESH_MS);
}

function chips(container, attr, onPick) {
  el(container).addEventListener("click", (e) => {
    const btn = e.target.closest("button");
    if (!btn) return;
    el(container).querySelectorAll("button").forEach((b) => b.classList.toggle("on", b === btn));
    onPick(btn.dataset[attr]);
    draw();
  });
}

if (PAGE === "dashboard") {
  chips("weapon-class", "class", (v) => (state.weaponClass = v));

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
    draw();
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

tick().then(openFromHash);
schedule();
