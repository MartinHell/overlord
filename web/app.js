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

// --- state -----------------------------------------------------------------

const state = {
  data: null,
  log: null,
  query: "",
  weaponClass: "all",
  logGroup: "all",
  logSide: "all",
  sort: {
    weapons: { key: "shots", dir: -1 },
    pilots: { key: "takeoffs", dir: -1 },
    loadout: { key: "shots", dir: -1 },
    traps: { key: "missionTime", dir: -1 },
    log: { key: "id", dir: -1 },
  },
};

// --- api -------------------------------------------------------------------

const QUERY = `{
  killsByCoalition { coalition kills teamkills }
  weaponEffectiveness { weaponType shots hits kills hitsPerShot killsPerShot }
  shotsByPlayers { playerID playerName units { unitType weapons { weaponType count } } }
  playerActivity { playerID playerName takeoffs landings crashes ejections deaths }
  landingGrades(first: 40) { playerName unitType place grade missionTime }
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

function render(table, columns, rows, renderRow) {
  const node = el(table);

  if (!rows.length) {
    node.innerHTML = `<tbody><tr><td class="none">Nothing matches.</td></tr></tbody>`;
    return;
  }

  const active = state.sort[table];
  const head = columns
    .map((c) => {
      if (!c.key) return `<th${c.num ? ' class="num"' : ""}>${esc(c.label)}</th>`;
      const on = active.key === c.key;
      const car = on ? `<span class="car">${active.dir < 0 ? "▼" : "▲"}</span>` : "";
      return `<th data-sort="${c.key}"${
        on ? ` aria-sort="${active.dir < 0 ? "descending" : "ascending"}"` : ""
      }${c.num ? ' class="num"' : ""}>${esc(c.label)} ${car}</th>`;
    })
    .join("");

  node.innerHTML = `<thead><tr>${head}</tr></thead><tbody>${rows.map(renderRow).join("")}</tbody>`;

  node.querySelectorAll("th[data-sort]").forEach((th) => {
    th.addEventListener("click", () => {
      const key = th.dataset.sort;
      const s = state.sort[table];
      s.dir = s.key === key ? -s.dir : -1;
      s.key = key;
      draw();
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
        <span class="side-name">${esc(c.coalition)}</span>
        <span class="meter"><span style="width:${(c.kills / max) * 100}%"></span></span>
        <span class="side-num"><b>${c.kills}</b> <i>splash</i>${
          c.teamkills ? ` <span class="tk">· ${c.teamkills} blue-on-blue</span>` : ""
        }</span>
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
      { label: "store", key: "weaponType" },
      { label: "shots", key: "shots", num: true },
      { label: "hits", key: "hits", num: true },
      { label: "splash", key: "kills", num: true },
      { label: "hits/shot", key: "hitsPerShot", num: true },
      { label: "kills/shot", key: "killsPerShot", num: true },
    ],
    sortRows(filtered, "weapons"),
    (r) => `<tr>
      <td class="name">${esc(shortStore(r.weaponType))}</td>
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
  const filtered = rows
    .filter((r) => r.takeoffs || r.landings || r.crashes || r.ejections || r.deaths)
    .filter((r) => matches([r.playerName]));

  render(
    "pilots",
    [
      { label: "pilot", key: "playerName" },
      { label: "t/o", key: "takeoffs", num: true },
      { label: "ldg", key: "landings", num: true },
      { label: "crash", key: "crashes", num: true },
      { label: "ejct", key: "ejections", num: true },
      { label: "kia", key: "deaths", num: true },
    ],
    sortRows(filtered, "pilots"),
    (r) => `<tr>
      <td class="name">${esc(r.playerName)}</td>
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
      { label: "time", key: "missionTime", num: true },
      { label: "pilot", key: "playerName" },
      { label: "airframe", key: "unitType" },
      { label: "field", key: "place" },
      { label: "grade", key: "grade" },
    ],
    sortRows(filtered, "traps"),
    (r) => `<tr>
      <td class="num">${clock(r.missionTime)}</td>
      <td class="name">${esc(r.playerName || "—")}</td>
      <td>${esc(r.unitType || "—")}</td>
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
      { label: "pilot", key: "playerName" },
      { label: "airframe", key: "unitType" },
      { label: "store", key: "weaponType" },
      { label: "shots", key: "shots", num: true },
    ],
    sortRows(filtered, "loadout"),
    (r) => `<tr>
      <td class="name">${esc(r.playerName)}</td>
      <td>${esc(r.unitType)}</td>
      <td>${esc(shortStore(r.weaponType))}</td>
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
        n.event, n.player?.playerName, n.initiator?.type, n.initiatorName,
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
      { label: "time", key: "missionTime", num: true },
      { label: "event", key: "event" },
      { label: "actor", key: "initiatorCallsign" },
      { label: "airframe", key: "initiatorType" },
      { label: "store", key: "weaponType" },
      { label: "target", key: "targetType" },
    ],
    sortRows(filtered, "log"),
    (n) => {
      const side =
        n.coalition === "blue" || n.coalition === "red"
          ? `<span class="flag flag-${n.coalition}"></span>`
          : `<span class="flag"></span>`;

      // Callsign is what a pilot is actually called on the radio; fall back
      // through the unit name to the player.
      const actor = n.initiatorCallsign || n.initiatorName || n.player?.playerName || "—";

      const target = n.targetType
        ? esc(n.targetType) +
          (n.target?.kind && n.target.kind !== "unit"
            ? ` <span class="zero">${esc(n.target.kind)}</span>`
            : "")
        : `<span class="zero">—</span>`;

      return `<tr>
        <td class="num">${clock(n.missionTime)}</td>
        <td><span class="ev ${EV_CLASS[n.event] || "ev-sortie"}">${esc(n.event)}</span></td>
        <td class="name">${side}${esc(actor)}</td>
        <td>${esc(n.initiatorType || "—")}</td>
        <td>${n.weaponType ? esc(shortStore(n.weaponType)) : `<span class="zero">—</span>`}</td>
        <td>${target}${n.place ? ` <span class="zero">${esc(n.place)}</span>` : ""}</td>
      </tr>`;
    }
  );
}

// --- draw / refresh --------------------------------------------------------

function draw() {
  const d = state.data;
  if (!d) return;

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
    link.textContent = `link ${new Date().toLocaleTimeString([], { hour12: false })}`;
    link.className = "link up";
    el("foot").textContent = `${API_URL} · last ${LOG_ROWS} events · refresh ${REFRESH_MS / 1000}s`;
  } catch (err) {
    link.textContent = "no link";
    link.className = "link down";
    el("foot").textContent = `${err.message} — check that overlord is running and reachable at ${API_URL}`;
  }
}

async function refreshLog() {
  try {
    state.log = await fetchLog();
    draw();
  } catch (err) {
    el("link").textContent = "no link";
    el("link").className = "link down";
    el("foot").textContent = `${err.message} — check that overlord is reachable at ${API_URL}`;
  }
}

// --- wiring ----------------------------------------------------------------

let timer = null;
function schedule() {
  clearInterval(timer);
  if (el("live").checked) timer = setInterval(refresh, REFRESH_MS);
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

el("live").addEventListener("change", schedule);

// Dark is the MFD, light is the kneeboard. Remembered per browser.
const themeBtn = el("theme");

function applyTheme(mode) {
  if (mode) document.documentElement.setAttribute("data-theme", mode);
  else document.documentElement.removeAttribute("data-theme");

  const dark = mode
    ? mode === "dark"
    : !window.matchMedia("(prefers-color-scheme: light)").matches;

  // The button names where you are going, not where you are.
  themeBtn.textContent = dark ? "kneeboard" : "mfd";
}

applyTheme(localStorage.getItem("overlord-theme"));

themeBtn.addEventListener("click", () => {
  const next = themeBtn.textContent === "kneeboard" ? "light" : "dark";
  localStorage.setItem("overlord-theme", next);
  applyTheme(next);
});

refresh();
schedule();
