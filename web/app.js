// Overlord dashboard.
//
// Deliberately dependency-free and served as static files. It talks to the
// GraphQL API over HTTP and holds no server-side state, so moving it out of
// overlord later means serving this directory from somewhere else and pointing
// API_URL at the API. Nothing here assumes it is same-origin except the default.
const API_URL =
  (typeof window !== "undefined" && window.OVERLORD_API_URL) || "/query";

const REFRESH_MS = 15000;

const el = (id) => document.getElementById(id);

async function gql(query) {
  const res = await fetch(API_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query }),
  });

  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`);
  }

  const body = await res.json();
  if (body.errors) {
    throw new Error(body.errors.map((e) => e.message).join("; "));
  }

  return body.data;
}

const QUERY = `{
  killsByCoalition { coalition kills teamkills }
  weaponEffectiveness { weaponType shots hits kills hitsPerShot killsPerShot }
  shotsByPlayers { playerID playerName units { unitType weapons { weaponType count } } }
  playerActivity { playerID playerName takeoffs landings crashes ejections deaths }
  landingGrades(first: 15) { playerName unitType place grade missionTime }
  events(first: 25) {
    edges { node {
      id event missionTime coalition targetCoalition
      player { playerName }
      initiator { type }
      weapon { type }
      target { kind unit { type } }
    } }
  }
}`;

// --- rendering helpers -----------------------------------------------------

function table(node, columns, rows, renderRow) {
  if (!rows || rows.length === 0) {
    node.innerHTML = `<tbody><tr><td class="empty">Nothing recorded yet.</td></tr></tbody>`;
    return;
  }

  const head = columns
    .map((c) => `<th class="${c.num ? "num" : ""}">${esc(c.label)}</th>`)
    .join("");

  node.innerHTML =
    `<thead><tr>${head}</tr></thead><tbody>` +
    rows.map(renderRow).join("") +
    `</tbody>`;
}

function esc(v) {
  return String(v ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

// Ratios, not percentages: see the WeaponEffectiveness comment in models. A
// value above 1 means one launch damaged several things.
function ratio(v) {
  return (v ?? 0).toFixed(2);
}

// Mission time is seconds on the DCS clock, not a wall-clock timestamp.
function missionClock(seconds) {
  if (!seconds) return "—";
  const s = Math.floor(seconds);
  const h = String(Math.floor(s / 3600)).padStart(2, "0");
  const m = String(Math.floor((s % 3600) / 60)).padStart(2, "0");
  const sec = String(s % 60).padStart(2, "0");
  return `${h}:${m}:${sec}`;
}

function side(coalition) {
  const c = coalition || "unknown";
  return `<span class="tag ${c === "blue" || c === "red" ? c : ""}">${esc(c)}</span>`;
}

// --- sections --------------------------------------------------------------

function renderCoalitions(rows) {
  const order = { blue: 0, red: 1, neutral: 2, unknown: 3 };
  const sorted = [...(rows || [])].sort(
    (a, b) => (order[a.coalition] ?? 9) - (order[b.coalition] ?? 9)
  );

  el("coalitions").innerHTML = sorted
    .map(
      (c) => `<div class="card ${esc(c.coalition)}">
        <div class="label">${esc(c.coalition)}</div>
        <div class="value">${c.kills}</div>
        <div class="detail">kills${
          c.teamkills > 0 ? ` · ${c.teamkills} teamkill${c.teamkills === 1 ? "" : "s"}` : ""
        }</div>
      </div>`
    )
    .join("");
}

function renderWeapons(rows) {
  // Rows where everything is zero carry no information: they exist because the
  // weapon appeared on an event whose only hits and kills were against scenery,
  // which the aggregate excludes.
  const useful = (rows || []).filter((r) => r.shots || r.hits || r.kills);
  const sorted = useful.sort((a, b) => b.shots - a.shots || b.hits - a.hits);
  const maxShots = Math.max(1, ...sorted.map((r) => r.shots));

  table(
    el("weapons"),
    [
      { label: "Weapon" },
      { label: "Shots", num: true },
      { label: "Hits", num: true },
      { label: "Kills", num: true },
      { label: "Hits / shot", num: true },
      { label: "Kills / shot", num: true },
    ],
    sorted,
    (r) => `<tr>
      <td>${esc(r.weaponType)}</td>
      <td class="num"><span class="bar" style="width:${
        (r.shots / maxShots) * 60
      }px"></span>${r.shots}</td>
      <td class="num">${r.hits}</td>
      <td class="num">${r.kills}</td>
      <td class="num">${r.shots ? ratio(r.hitsPerShot) : "—"}</td>
      <td class="num">${r.shots ? ratio(r.killsPerShot) : "—"}</td>
    </tr>`
  );
}

function renderShots(players) {
  const rows = [];
  for (const p of players || []) {
    const total = (p.units || []).reduce(
      (sum, u) => sum + u.weapons.reduce((s, w) => s + w.count, 0),
      0
    );
    rows.push({ kind: "player", name: p.playerName, total });
    for (const u of p.units || []) {
      for (const w of u.weapons || []) {
        rows.push({ kind: "weapon", unit: u.unitType, weapon: w.weaponType, count: w.count });
      }
    }
  }

  table(
    el("shots"),
    [{ label: "Player / airframe / weapon" }, { label: "Shots", num: true }],
    rows,
    (r) =>
      r.kind === "player"
        ? `<tr><td><strong>${esc(r.name)}</strong></td><td class="num"><strong>${r.total}</strong></td></tr>`
        : `<tr><td class="sub">${esc(r.unit)} · ${esc(r.weapon)}</td><td class="num">${r.count}</td></tr>`
  );
}

function renderActivity(rows) {
  const sorted = [...(rows || [])].sort(
    (a, b) => b.takeoffs - a.takeoffs || b.landings - a.landings
  );

  table(
    el("activity"),
    [
      { label: "Player" },
      { label: "Takeoffs", num: true },
      { label: "Landings", num: true },
      { label: "Crashes", num: true },
      { label: "Ejections", num: true },
      { label: "Deaths", num: true },
    ],
    sorted,
    (r) => `<tr>
      <td>${esc(r.playerName)}</td>
      <td class="num">${r.takeoffs}</td>
      <td class="num">${r.landings}</td>
      <td class="num">${r.crashes}</td>
      <td class="num">${r.ejections}</td>
      <td class="num">${r.deaths}</td>
    </tr>`
  );
}

function renderLandings(rows) {
  table(
    el("landings"),
    [{ label: "Player" }, { label: "Airframe" }, { label: "Place" }, { label: "Grade" }, { label: "Mission time", num: true }],
    rows,
    (r) => `<tr>
      <td>${esc(r.playerName || "—")}</td>
      <td>${esc(r.unitType || "—")}</td>
      <td>${esc(r.place || "—")}</td>
      <td>${esc(r.grade || "—")}</td>
      <td class="num">${missionClock(r.missionTime)}</td>
    </tr>`
  );
}

function renderEvents(connection) {
  const rows = (connection?.edges || []).map((e) => e.node);

  table(
    el("events"),
    [
      { label: "Time", num: true },
      { label: "Event" },
      { label: "Side" },
      { label: "Player" },
      { label: "Airframe" },
      { label: "Weapon" },
      { label: "Target" },
    ],
    rows,
    (n) => {
      const target = n.target?.unit?.type
        ? `${esc(n.target.unit.type)}${
            n.target.kind && n.target.kind !== "unit" ? ` <span class="tag">${esc(n.target.kind)}</span>` : ""
          }`
        : "—";

      return `<tr>
        <td class="num">${missionClock(n.missionTime)}</td>
        <td>${esc(n.event)}</td>
        <td>${side(n.coalition)}</td>
        <td>${esc(n.player?.playerName || "—")}</td>
        <td>${esc(n.initiator?.type || "—")}</td>
        <td>${esc(n.weapon?.type || "—")}</td>
        <td>${target}</td>
      </tr>`;
    }
  );
}

// --- refresh loop ----------------------------------------------------------

let timer = null;

async function refresh() {
  const status = el("status");

  try {
    const data = await gql(QUERY);

    renderCoalitions(data.killsByCoalition);
    renderWeapons(data.weaponEffectiveness);
    renderShots(data.shotsByPlayers);
    renderActivity(data.playerActivity);
    renderLandings(data.landingGrades);
    renderEvents(data.events);

    status.textContent = `updated ${new Date().toLocaleTimeString()}`;
    status.className = "status ok";
    el("footer-note").textContent = `${API_URL} · refreshes every ${REFRESH_MS / 1000}s`;
  } catch (err) {
    status.textContent = `error: ${err.message}`;
    status.className = "status err";
  }
}

function scheduleRefresh() {
  clearInterval(timer);
  if (el("autorefresh").checked) {
    timer = setInterval(refresh, REFRESH_MS);
  }
}

el("refresh").addEventListener("click", refresh);
el("autorefresh").addEventListener("change", scheduleRefresh);

refresh();
scheduleRefresh();
