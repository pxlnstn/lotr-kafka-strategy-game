// Ring of the Middle Earth - browser client.
// One page serves both sides; pick with ?side=light or ?side=dark.

const SIDE = (new URLSearchParams(location.search).get("side") || "light").toLowerCase();
const IS_DARK = SIDE === "dark";
const API = location.origin;
const SVGNS = "http://www.w3.org/2000/svg";

// Region layout, taken straight from the provided map art.
const XY = {
  "the-shire": [100, 250], "bree": [250, 250], "tharbad": [250, 400], "weathertop": [400, 150],
  "rivendell": [550, 150], "fangorn": [500, 450], "fords-of-isen": [350, 550], "rohan-plains": [650, 500],
  "moria": [600, 300], "helms-deep": [500, 700], "isengard": [350, 650], "edoras": [650, 650],
  "lothlorien": [700, 300], "dead-marshes": [1000, 250], "emyn-muil": [850, 350], "minas-tirith": [850, 650],
  "ithilien": [1000, 450], "osgiliath": [1000, 650], "minas-morgul": [1150, 650], "cirith-ungol": [1150, 450],
  "mordor": [1150, 250], "mount-doom": [1300, 350],
};
const CONTROL = { FREE_PEOPLES: "#4b7bec", SHADOW: "#eb3b5a", NEUTRAL: "#a5b1c2" };
const ICON = {
  PLAINS: "icon-plains", SWAMP: "icon-swamp", MOUNTAINS: "icon-mountains",
  FOREST: "icon-forest", FORTRESS: "icon-fortress", VOLCANIC: "icon-volcanic",
};
const SHORT = {
  "ring-bearer": "Frodo", "aragorn": "Aragorn", "legolas": "Legolas", "gimli": "Gimli",
  "rohan-cavalry": "Rohan", "gondor-army": "Gondor", "gandalf": "Gandalf",
  "witch-king": "Witch-King", "nazgul-2": "Dark Marshal", "nazgul-3": "Betrayer",
  "uruk-hai-legion": "Uruk-hai", "saruman": "Saruman", "sauron": "Sauron",
};

let MAP = { regions: [], paths: [], units: [] };
let regionById = {}, pathById = {}, unitCfg = {};
let state = null;
let ringRegion = null;

const $ = (id) => document.getElementById(id);
const el = (name, attrs = {}) => {
  const e = document.createElementNS(SVGNS, name);
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
  return e;
};
const short = (id) => SHORT[id] || id;
const regionName = (id) => (regionById[id] ? regionById[id].name : id);

function logLine(text, hot) {
  const d = document.createElement("div");
  if (hot) d.className = "hot";
  d.textContent = `t${state ? state.turn : "?"} · ${text}`;
  $("log").prepend(d);
}

// ---------- bootstrap ----------

async function boot() {
  $("sideBadge").textContent = IS_DARK ? "DARK SIDE" : "LIGHT SIDE";
  if (IS_DARK) { $("sideBadge").classList.add("dark"); document.body.classList.add("dark"); }
  $("analysisTitle").textContent = IS_DARK ? "Interception plan" : "Route risk";

  MAP = await (await fetch(`${API}/map`)).json();
  MAP.regions.forEach((r) => (regionById[r.id] = r));
  MAP.paths.forEach((p) => (pathById[p.id] = p));
  MAP.units.forEach((u) => (unitCfg[u.id] = u));

  drawBaseMap();
  await refreshState();
  populateUnitSelect();
  connectSSE();

  $("startBtn").onclick = startGame;
  $("advanceBtn").onclick = () => fetch(`${API}/turn/advance`, { method: "POST" });
  $("unitSelect").onchange = loadAvailableOrders;
  $("orderType").onchange = renderTargets;
  $("submitOrder").onclick = submitOrder;
  $("analysisBtn").onclick = refreshAnalysis;
}

async function startGame() {
  ringRegion = null;
  await fetch(`${API}/game/start`, {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mode: "HVH" }),
  });
  logLine("game started");
}

// ---------- map base ----------

function drawBaseMap() {
  const edges = $("edges"), nodes = $("nodes");
  edges.innerHTML = ""; nodes.innerHTML = "";

  MAP.paths.forEach((p) => {
    const [x1, y1] = XY[p.from], [x2, y2] = XY[p.to];
    edges.appendChild(el("line", { x1, y1, x2, y2, id: `edge-${p.id}`, class: "edge" + (p.cost >= 2 ? " cost2" : "") }));
  });

  MAP.regions.forEach((r) => {
    const [x, y] = XY[r.id];
    const g = el("g", { class: "node", id: `node-${r.id}`, transform: `translate(${x},${y})` });
    const w = r.name.length * 7.6 + 18;
    g.appendChild(el("rect", { class: "pill", x: -w / 2, y: 30, width: w, height: 22, rx: 6, filter: "url(#box-shadow)" }));
    const label = el("text", { class: "label", y: 45 });
    label.textContent = r.name;
    g.appendChild(label);
    g.appendChild(el("circle", { class: "body", r: 20, fill: CONTROL[r.startControl], filter: "url(#shadow)" }));
    g.appendChild(el("use", { href: `#${ICON[r.terrain]}` }));
    nodes.appendChild(g);
  });
}

// ---------- dynamic paint ----------

function paintMap() {
  if (!state) return;

  state.paths.forEach((p) => {
    const edge = $(`edge-${p.pathId}`);
    if (!edge || edge.classList.contains("flash")) return;
    const cost = pathById[p.pathId] ? pathById[p.pathId].cost : 1;
    let cls = "edge" + (cost >= 2 ? " cost2" : "");
    if (p.status === "BLOCKED" || p.status === "THREATENED" || p.status === "TEMPORARILY_OPEN") cls = "edge " + p.status;
    else if (p.surveillanceLevel > 0) cls = "edge surv";
    edge.setAttribute("class", cls);
  });

  state.regions.forEach((r) => {
    const node = $(`node-${r.regionId}`);
    if (node) node.querySelector("circle.body").setAttribute("fill", CONTROL[r.controlledBy]);
  });

  renderBadges();
  moveRing(state.ringBearerRegion);
}

function renderBadges() {
  const layer = $("badges");
  layer.innerHTML = "";
  const byRegion = {};
  state.units.forEach((u) => {
    if (u.status !== "ACTIVE" || !u.region) return;
    (byRegion[u.region] ||= []).push(u.unitId);
  });
  Object.entries(byRegion).forEach(([region, ids]) => {
    const [x, y] = XY[region];
    // Stack badges to the right of the node so they never collide with the
    // label pills (which sit below each node).
    const startY = y - 9 - (ids.length - 1) * 8.5;
    ids.forEach((id, i) => {
      const name = short(id);
      const w = name.length * 6.3 + 12;
      const g = el("g", { transform: `translate(${x + 24},${startY + i * 17})` });
      g.appendChild(el("rect", { x: 0, y: -8, width: w, height: 15, rx: 7, fill: CONTROL[unitCfg[id].side], stroke: "#fff", "stroke-width": 1, filter: "url(#box-shadow)" }));
      const t = el("text", { class: "badge-tx", x: w / 2, y: 2.5 });
      t.textContent = name;
      const title = el("title"); title.textContent = unitCfg[id].name;
      g.append(title, t);
      layer.appendChild(g);
    });
  });
}

// ---------- ring marker + animation ----------

function ensureRingMarker() {
  let g = $("ringMarker");
  if (!g) {
    g = el("g", { id: "ringMarker" });
    g.appendChild(el("circle", { class: "ring-glow", r: 30, fill: "url(#ringGlow)" }));
    g.appendChild(el("circle", { r: 27, fill: "none", stroke: "#f6b73c", "stroke-width": 3 }));
    $("fx").appendChild(g);
  }
  return g;
}

function moveRing(region) {
  if (!region || !XY[region]) {
    const m = $("ringMarker");
    if (m) m.remove();
    ringRegion = null;
    return;
  }
  const g = ensureRingMarker();
  if (ringRegion && ringRegion !== region && XY[ringRegion]) {
    flashEdge(ringRegion, region);
    animateMarker(g, XY[ringRegion], XY[region], 650);
  } else {
    g.setAttribute("transform", `translate(${XY[region][0]},${XY[region][1]})`);
  }
  ringRegion = region;
}

function animateMarker(g, from, to, ms) {
  const t0 = performance.now();
  function step(now) {
    let k = Math.min(1, (now - t0) / ms);
    k = k < 0.5 ? 2 * k * k : 1 - Math.pow(-2 * k + 2, 2) / 2; // easeInOut
    g.setAttribute("transform", `translate(${from[0] + (to[0] - from[0]) * k},${from[1] + (to[1] - from[1]) * k})`);
    if (k < 1) requestAnimationFrame(step);
  }
  requestAnimationFrame(step);
}

function flashEdge(a, b) {
  const p = MAP.paths.find((p) => (p.from === a && p.to === b) || (p.from === b && p.to === a));
  if (!p) return;
  const e = $(`edge-${p.id}`);
  if (!e) return;
  e.classList.add("flash");
  setTimeout(() => e.classList.remove("flash"), 750);
}

function pulseDetect(region) {
  if (!XY[region]) return;
  const [x, y] = XY[region];
  const c = el("circle", { cx: x, cy: y, r: 24, fill: "none", stroke: "#eb3b5a", "stroke-width": 3 });
  $("fx").appendChild(c);
  let r = 24;
  const t = setInterval(() => {
    r += 4; c.setAttribute("r", r); c.setAttribute("opacity", Math.max(0, 1 - (r - 24) / 44));
    if (r > 68) { clearInterval(t); c.remove(); }
  }, 35);
}

// ---------- state ----------

async function refreshState() {
  state = await (await fetch(`${API}/game/state?playerId=${SIDE}`)).json();
  $("turn").textContent = state.started ? `Turn ${state.turn}` : "Turn —";
  paintMap();
  if (state.gameOver) showBanner(state.winner);
}

function showBanner(winner) {
  const b = $("banner");
  b.classList.remove("hidden");
  b.textContent = winner === "DRAW" ? "Draw — 40 turns reached"
    : `${winner === "FREE_PEOPLES" ? "Light Side" : "Dark Side"} wins!`;
}

// ---------- orders ----------

function mySide() { return IS_DARK ? "SHADOW" : "FREE_PEOPLES"; }

function populateUnitSelect() {
  const sel = $("unitSelect");
  sel.innerHTML = "";
  MAP.units.filter((u) => u.side === mySide()).forEach((u) => {
    const o = document.createElement("option");
    o.value = u.id; o.textContent = u.name;
    sel.appendChild(o);
  });
  loadAvailableOrders();
}

let available = [];
async function loadAvailableOrders() {
  const unitId = $("unitSelect").value;
  if (!unitId) return;
  available = await (await fetch(`${API}/orders/available?unitId=${unitId}&playerId=${SIDE}`)).json();
  const sel = $("orderType");
  sel.innerHTML = "";
  available.forEach((o) => {
    const opt = document.createElement("option");
    opt.value = o.orderType; opt.textContent = o.orderType.replace(/_/g, " ");
    sel.appendChild(opt);
  });
  renderTargets();
}

function unitRegionUI(unitId) {
  if (unitCfg[unitId] && unitCfg[unitId].class === "RingBearer") return state.ringBearerRegion || "the-shire";
  const u = (state.units || []).find((x) => x.unitId === unitId);
  return u ? u.region : "";
}

// Shortest path (by traversal cost) between two regions, as a list of path ids.
function shortestRoute(from, to) {
  if (!from || from === to) return [];
  const dist = {}, prevPath = {}, prevNode = {};
  MAP.regions.forEach((r) => (dist[r.id] = Infinity));
  dist[from] = 0;
  const pq = [[0, from]];
  while (pq.length) {
    pq.sort((a, b) => a[0] - b[0]);
    const [d, u] = pq.shift();
    if (u === to) break;
    if (d > dist[u]) continue;
    MAP.paths.forEach((p) => {
      const nb = p.from === u ? p.to : (p.to === u ? p.from : null);
      if (nb && d + p.cost < dist[nb]) { dist[nb] = d + p.cost; prevPath[nb] = p.id; prevNode[nb] = u; pq.push([dist[nb], nb]); }
    });
  }
  if (dist[to] === Infinity) return null;
  const route = [];
  let cur = to;
  while (cur !== from) { route.unshift(prevPath[cur]); cur = prevNode[cur]; }
  return route;
}

function renderTargets() {
  const type = $("orderType").value;
  const sel = $("orderTarget");
  sel.innerHTML = "";
  // Route orders pick a DESTINATION region; the path is computed on submit.
  if (type === "ASSIGN_ROUTE" || type === "REDIRECT_UNIT") {
    const cur = unitRegionUI($("unitSelect").value);
    $("targetRow").style.display = "block";
    MAP.regions.filter((r) => r.id !== cur).forEach((r) => {
      const o = document.createElement("option");
      o.value = r.id; o.textContent = r.name;
      sel.appendChild(o);
    });
    if ($("unitSelect").value === "ring-bearer") sel.value = "mount-doom";
    return;
  }
  const opt = available.find((o) => o.orderType === type);
  const targets = (opt && opt.targets) || [];
  $("targetRow").style.display = targets.length ? "block" : "none";
  targets.forEach((t) => {
    const o = document.createElement("option");
    o.value = t; o.textContent = t;
    sel.appendChild(o);
  });
}

async function submitOrder() {
  const unitId = $("unitSelect").value;
  const orderType = $("orderType").value;
  const target = $("orderTarget").value;
  const order = { orderType, playerId: SIDE, unitId, turn: state.turn };
  switch (orderType) {
    case "ASSIGN_ROUTE": {
      const r = shortestRoute(unitRegionUI(unitId), target);
      if (!r || !r.length) { $("orderMsg").textContent = "no route to " + target; return; }
      order.pathIds = r; break;
    }
    case "REDIRECT_UNIT": {
      const r = shortestRoute(unitRegionUI(unitId), target);
      if (!r || !r.length) { $("orderMsg").textContent = "no route to " + target; return; }
      order.newPathIds = r; break;
    }
    case "BLOCK_PATH":
    case "SEARCH_PATH": order.pathId = target; break;
    case "MAIA_ABILITY": order.targetPathId = target; break;
    case "ATTACK_REGION":
    case "REINFORCE_REGION":
    case "DEPLOY_NAZGUL": order.targetRegion = target; break;
  }
  const res = await fetch(`${API}/order`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(order),
  });
  $("orderMsg").textContent = res.ok ? `sent ${orderType.replace(/_/g, " ")} for ${short(unitId)}` : "order failed";
  logLine(`order ${orderType.replace(/_/g, " ")} → ${short(unitId)}`);
}

// ---------- analysis ----------

async function refreshAnalysis() {
  const ep = IS_DARK ? "intercept" : "routes";
  const data = await (await fetch(`${API}/analysis/${ep}?playerId=${SIDE}`)).json();
  const box = $("analysis");
  if (IS_DARK) {
    box.innerHTML = "<table class='routes-tbl'><tr><td>Nazgûl</td><td>Target</td><td>Score</td></tr>" +
      (data.byUnit || []).map((e) =>
        `<tr><td>${short(e.unitId)}</td><td>${e.targetRegion || "—"}</td><td>${e.score.toFixed(2)}</td></tr>`).join("") + "</table>";
  } else {
    box.innerHTML = "<table class='routes-tbl'><tr><td>Route</td><td>Risk</td></tr>" +
      (data.routes || []).sort((a, b) => a.score - b.score).map((r) =>
        `<tr class='${r.name === data.recommended ? "best" : ""}'><td>${r.name}</td><td>${r.score}</td></tr>`).join("") +
      `</table><div class='msg'>Recommended: ${data.recommended || "—"}</div>`;
  }
}

// ---------- SSE ----------

function connectSSE() {
  const es = new EventSource(`${API}/events?playerId=${SIDE}`);
  es.onopen = () => { $("conn").textContent = "live"; $("conn").className = "conn on"; };
  es.onerror = () => { $("conn").textContent = "reconnecting"; $("conn").className = "conn off"; };
  es.onmessage = (ev) => {
    let msg; try { msg = JSON.parse(ev.data); } catch { return; }
    handleEvent(msg);
  };
}

function handleEvent(msg) {
  const d = msg.data || {};
  switch (msg.topic) {
    case "keepalive": return;
    case "game.broadcast": refreshState(); break;
    case "game.ring.position": moveRing(d.trueRegion); logLine(`Ring Bearer moved to ${regionName(d.trueRegion)}`); break;
    case "game.ring.detection":
      if (d.regionId) { pulseDetect(d.regionId); logLine(`RING BEARER DETECTED at ${regionName(d.regionId)}`, true); }
      else if (d.pathId) logLine(`Ring Bearer spotted on ${d.pathId}`, true);
      break;
    case "game.events.path": logLine(`path ${d.pathId} → ${d.newStatus || "corrupted"}`); refreshState(); break;
    case "game.events.unit": logLine(`${short(d.unitId)} moved → ${regionName(d.to)}`); break;
    case "game.events.region": if (d.newController) logLine(`${regionName(d.regionId)} now ${d.newController}`); break;
    case "game.over": showBanner(d.winner); logLine(`GAME OVER — ${d.winner} (${d.cause})`, true); break;
  }
}

boot();
