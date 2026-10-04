"use strict";

// Read-only account viewer. It only ever issues GET /api/snapshot.
// Symbols and statuses come from the brokerage, so they are always inserted
// with textContent, never as HTML.

const LARGEST_COUNT = 15;
const PL_COUNT = 8;

const money = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" });
const wholeMoney = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 0 });
const shares = new Intl.NumberFormat("en-US", { maximumFractionDigits: 4 });
const percent = new Intl.NumberFormat("en-US", { minimumFractionDigits: 1, maximumFractionDigits: 1 });
const dateTime = new Intl.DateTimeFormat("en-US", { dateStyle: "medium", timeStyle: "short" });

const $ = (id) => document.getElementById(id);

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function signedMoney(value, formatter = money) {
  if (value > 0) return "+" + formatter.format(value);
  if (value < 0) return "−" + formatter.format(-value);
  return formatter.format(0);
}

function signedPercent(value) {
  if (!Number.isFinite(value)) return "—";
  const sign = value > 0 ? "+" : value < 0 ? "−" : "";
  return sign + percent.format(Math.abs(value)) + "%";
}

function titleCase(value) {
  return value ? value.charAt(0).toUpperCase() + value.slice(1).toLowerCase() : "—";
}

// ---------------------------------------------------------------- tooltip

const tooltip = $("tooltip");

function showTooltip(title, rows, x, y) {
  tooltip.replaceChildren(el("div", "tooltip-title", title));
  for (const [label, value] of rows) {
    const row = el("div", "tooltip-row");
    row.append(el("span", "", label), el("span", "", value));
    tooltip.append(row);
  }
  tooltip.hidden = false;

  const box = tooltip.getBoundingClientRect();
  const left = Math.min(x + 14, window.innerWidth - box.width - 8);
  const top = y + 14 + box.height > window.innerHeight ? y - box.height - 10 : y + 14;
  tooltip.style.left = Math.max(8, left) + "px";
  tooltip.style.top = Math.max(8, top) + "px";
}

function hideTooltip() {
  tooltip.hidden = true;
}

// The same details show on hover and on keyboard focus.
function attachTooltip(node, title, rows) {
  node.tabIndex = 0;
  node.setAttribute("role", "img");
  node.setAttribute("aria-label", title + ": " + rows.map(([label, value]) => label + " " + value).join(", "));
  node.addEventListener("pointermove", (event) => showTooltip(title, rows, event.clientX, event.clientY));
  node.addEventListener("pointerleave", hideTooltip);
  node.addEventListener("focus", () => {
    const box = node.getBoundingClientRect();
    showTooltip(title, rows, box.left + box.width / 2, box.bottom - 10);
  });
  node.addEventListener("blur", hideTooltip);
}

function positionRows(position, totalValue) {
  return [
    ["Market value", money.format(position.market_value)],
    ["Share of positions", totalValue > 0 ? percent.format((position.market_value / totalValue) * 100) + "%" : "—"],
    ["Shares", shares.format(position.quantity)],
    ["Unrealized gain/loss", signedMoney(position.unrealized_pl) + " (" + signedPercent(position.unrealized_pl_pct) + ")"],
  ];
}

// ---------------------------------------------------------------- figures

function renderFigures(summary, positions) {
  const positionsValue = positions.reduce((sum, p) => sum + p.market_value, 0);
  const unrealized = positions.reduce((sum, p) => sum + p.unrealized_pl, 0);
  const cost = positionsValue - unrealized;

  $("account-value").textContent = money.format(positionsValue + summary.total_cash);

  const tiles = [
    { label: "Positions value", value: money.format(positionsValue), note: positions.length + (positions.length === 1 ? " position" : " positions") },
    { label: "Total cash", value: money.format(summary.total_cash), note: summary.pending_deposits ? money.format(summary.pending_deposits) + " pending deposits" : "" },
    { label: "Available to trade", value: money.format(summary.cash_available_for_trading), note: money.format(summary.cash_available_for_withdrawal) + " available to withdraw" },
    { label: "Unrealized gain/loss", value: signedMoney(unrealized), delta: cost > 0 ? (unrealized / cost) * 100 : NaN },
  ];

  const container = $("tiles");
  container.replaceChildren();
  for (const tile of tiles) {
    const node = el("div", "tile");
    node.append(el("p", "label", tile.label), el("p", "tile-value", tile.value));
    if (tile.delta !== undefined && Number.isFinite(tile.delta)) {
      const arrow = tile.delta > 0 ? "▲ " : tile.delta < 0 ? "▼ " : "";
      const direction = tile.delta > 0 ? " up" : tile.delta < 0 ? " down" : "";
      node.append(el("p", "delta" + direction, arrow + signedPercent(tile.delta) + " vs cost"));
    } else if (tile.note) {
      node.append(el("p", "note", tile.note));
    }
    container.append(node);
  }

  return positionsValue;
}

// ---------------------------------------------------------------- charts

// Width of a bar as a share of its track, leaving room for the value at the tip.
function barWidth(fraction, reserve) {
  return "calc((100% - " + reserve + "px) * " + Math.max(0, Math.min(1, fraction)).toFixed(4) + ")";
}

function renderLargest(positions, totalValue) {
  const largest = [...positions].sort((a, b) => b.market_value - a.market_value).slice(0, LARGEST_COUNT);
  const max = Math.max(...largest.map((p) => p.market_value), 0);

  $("largest-subtitle").textContent =
    positions.length > largest.length
      ? "Market value and share of positions · " + largest.length + " largest of " + positions.length + ", all are in the table below"
      : "Market value and share of positions";

  const container = $("largest");
  container.replaceChildren();
  for (const position of largest) {
    const row = el("div", "bar-row");
    const track = el("div", "bar-track");
    const bar = el("div", "bar");
    bar.style.width = barWidth(max > 0 ? position.market_value / max : 0, 124);
    const share = totalValue > 0 ? " · " + percent.format((position.market_value / totalValue) * 100) + "%" : "";
    track.append(bar, el("span", "bar-value", wholeMoney.format(position.market_value) + share));
    row.append(el("span", "bar-label", position.symbol), track);
    attachTooltip(row, position.symbol, positionRows(position, totalValue));
    container.append(row);
  }
  if (largest.length === 0) container.append(el("p", "empty", "No positions."));
}

function renderGainsAndLosses(positions, totalValue) {
  const sorted = [...positions].sort((a, b) => b.unrealized_pl - a.unrealized_pl);
  const gains = sorted.filter((p) => p.unrealized_pl > 0).slice(0, PL_COUNT);
  const losses = sorted.filter((p) => p.unrealized_pl < 0).slice(-PL_COUNT);
  const shown = [...gains, ...losses];
  const max = Math.max(...shown.map((p) => Math.abs(p.unrealized_pl)), 0);

  $("pl-subtitle").textContent =
    "Dollars against cost · up to " + PL_COUNT + " largest each way, all are in the table below";

  const container = $("pl");
  container.replaceChildren();
  for (const position of shown) {
    const row = el("div", "bar-row");
    const halves = el("div", "bar-halves");
    const negative = el("div", "bar-half negative");
    const positive = el("div", "bar-half positive");

    const side = position.unrealized_pl < 0 ? negative : positive;
    const bar = el("div", "bar");
    bar.style.width = barWidth(max > 0 ? Math.abs(position.unrealized_pl) / max : 0, 70);
    side.append(bar, el("span", "bar-value", signedMoney(position.unrealized_pl, wholeMoney)));

    halves.append(negative, positive);
    row.append(el("span", "bar-label", position.symbol), halves);
    attachTooltip(row, position.symbol, positionRows(position, totalValue));
    container.append(row);
  }
  if (shown.length === 0) container.append(el("p", "empty", "No unrealized gains or losses."));
}

// ---------------------------------------------------------------- tables

const columns = [
  { key: "symbol", label: "Symbol", text: (p) => p.symbol },
  { key: "quantity", label: "Shares", num: true, text: (p) => shares.format(p.quantity) },
  { key: "average_price", label: "Avg cost", num: true, text: (p) => money.format(p.average_price) },
  { key: "current_price", label: "Price", num: true, text: (p) => money.format(p.current_price) },
  { key: "market_value", label: "Market value", num: true, text: (p) => money.format(p.market_value) },
  { key: "weight", label: "Share", num: true, text: (p) => percent.format(p.weight) + "%" },
  { key: "unrealized_pl", label: "Unrealized gain/loss", num: true, text: (p) => signedMoney(p.unrealized_pl) },
  { key: "unrealized_pl_pct", label: "Gain/loss %", num: true, text: (p) => signedPercent(p.unrealized_pl_pct) },
];

const sort = { key: "market_value", descending: true };
let tableRows = [];

function renderPositionsTable() {
  const table = $("positions");
  const header = table.querySelector("thead tr");
  header.replaceChildren();
  for (const column of columns) {
    const th = el("th", column.num ? "num" : "");
    th.scope = "col";
    const active = sort.key === column.key;
    th.setAttribute("aria-sort", active ? (sort.descending ? "descending" : "ascending") : "none");
    const button = el("button", "", column.label + (active ? (sort.descending ? " ↓" : " ↑") : ""));
    button.type = "button";
    button.addEventListener("click", () => {
      if (sort.key === column.key) {
        sort.descending = !sort.descending;
      } else {
        sort.key = column.key;
        sort.descending = column.key !== "symbol";
      }
      renderPositionsTable();
    });
    th.append(button);
    header.append(th);
  }

  const direction = sort.descending ? -1 : 1;
  const rows = [...tableRows].sort((a, b) => {
    const x = a[sort.key];
    const y = b[sort.key];
    return (typeof x === "string" ? x.localeCompare(y) : x - y) * direction;
  });

  const body = table.querySelector("tbody");
  body.replaceChildren();
  for (const position of rows) {
    const tr = el("tr");
    for (const column of columns) tr.append(el("td", column.num ? "num" : "", column.text(position)));
    body.append(tr);
  }
  if (rows.length === 0) {
    const td = el("td", "empty", "No positions.");
    td.colSpan = columns.length;
    const tr = el("tr");
    tr.append(td);
    body.append(tr);
  }
}

function renderOrders(orders, ordersError) {
  $("orders-subtitle").textContent = ordersError
    ? "Orders could not be loaded: " + ordersError
    : "The " + orders.length + " most recent, newest first as the brokerage returns them";

  const body = $("orders").querySelector("tbody");
  body.replaceChildren();
  for (const order of orders) {
    const tr = el("tr");
    const priceText =
      order.filled_qty > 0 && order.filled_price > 0
        ? money.format(order.filled_price)
        : order.limit_price
          ? "limit " + money.format(order.limit_price)
          : "—";
    tr.append(
      el("td", "", order.submitted_at ? dateTime.format(new Date(order.submitted_at)) : "—"),
      el("td", "", order.symbol || "—"),
      el("td", "", titleCase(order.action)),
      el("td", "", titleCase(order.type)),
      el("td", "num", shares.format(order.quantity)),
      el("td", "", titleCase(order.status)),
      el("td", "num", shares.format(order.filled_qty)),
      el("td", "num", priceText),
    );
    body.append(tr);
  }
  if (orders.length === 0) {
    const td = el("td", "empty", ordersError ? "Unavailable." : "No recent orders.");
    td.colSpan = 8;
    const tr = el("tr");
    tr.append(td);
    body.append(tr);
  }
}

// ---------------------------------------------------------------- loading

function render(snapshot) {
  const positions = snapshot.positions;
  const totalValue = renderFigures(snapshot.summary, positions);

  renderLargest(positions, totalValue);
  renderGainsAndLosses(positions, totalValue);

  tableRows = positions.map((p) => ({ ...p, weight: totalValue > 0 ? (p.market_value / totalValue) * 100 : 0 }));
  $("positions-subtitle").textContent = "All " + positions.length + " positions · select a column heading to sort";
  renderPositionsTable();

  renderOrders(snapshot.orders, snapshot.orders_error);

  const parts = [];
  if (snapshot.brokerage) parts.push(titleCase(snapshot.brokerage));
  if (snapshot.summary.type) parts.push(titleCase(snapshot.summary.type) + " account");
  parts.push("Updated " + dateTime.format(new Date(snapshot.fetched_at)));
  $("meta").textContent = parts.join(" · ");
}

async function load() {
  const button = $("refresh");
  const content = $("content");
  const error = $("error");

  button.disabled = true;
  content.classList.add("stale");
  try {
    const response = await fetch("api/snapshot", { cache: "no-store" });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || "The request failed with status " + response.status + ".");
    render(body);
    content.hidden = false;
    error.hidden = true;
  } catch (err) {
    error.textContent = "Could not load the account: " + err.message;
    error.hidden = false;
    if (content.hidden) $("meta").textContent = "Not loaded";
  } finally {
    content.classList.remove("stale");
    button.disabled = false;
  }
}

// ---------------------------------------------------------------- backtrack

const SVG = "http://www.w3.org/2000/svg";
const monthYear = new Intl.DateTimeFormat("en-US", { month: "short", year: "numeric", timeZone: "UTC" });
const fullDate = new Intl.DateTimeFormat("en-US", { dateStyle: "medium", timeZone: "UTC" });

let backtrackResult = null;

function svg(tag, attributes, text) {
  const node = document.createElementNS(SVG, tag);
  for (const [name, value] of Object.entries(attributes || {})) node.setAttribute(name, value);
  if (text !== undefined) node.textContent = text;
  return node;
}

// Round tick values (1, 2, 2.5 or 5 times a power of ten) that cover min..max.
function niceTicks(min, max, count) {
  const span = max - min || Math.abs(max) || 1;
  const raw = span / count;
  const power = Math.pow(10, Math.floor(Math.log10(raw)));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * power).find((s) => s >= raw);
  const ticks = [];
  for (let v = Math.floor(min / step) * step; v <= max + step * 0.999; v += step) ticks.push(v);
  return ticks;
}

function drawBacktrackChart() {
  const container = $("backtrack-chart");
  if (!backtrackResult || !container.clientWidth) return;

  const points = backtrackResult.points.map((p) => ({ time: Date.parse(p.date), value: p.value }));
  const start = backtrackResult.initial_amount;
  const width = container.clientWidth;
  const height = 280;
  const margin = { top: 20, right: 16, bottom: 28, left: 62 };
  const plotWidth = width - margin.left - margin.right;
  const plotHeight = height - margin.top - margin.bottom;

  const ticks = niceTicks(Math.min(...points.map((p) => p.value)), Math.max(...points.map((p) => p.value)), 4);
  const yMin = ticks[0];
  const yMax = ticks[ticks.length - 1];
  const tMin = points[0].time;
  const tMax = points[points.length - 1].time;
  const x = (time) => margin.left + ((time - tMin) / (tMax - tMin || 1)) * plotWidth;
  const y = (value) => margin.top + (1 - (value - yMin) / (yMax - yMin || 1)) * plotHeight;

  const last = points[points.length - 1];
  const chart = svg("svg", { viewBox: "0 0 " + width + " " + height, height, tabindex: 0, role: "img" });
  chart.setAttribute(
    "aria-label",
    "Value of $10,000 from " + fullDate.format(tMin) + " to " + fullDate.format(tMax) + ", ending at " + money.format(last.value) +
      ". Use the left and right arrow keys to read values.",
  );

  for (const tick of ticks) {
    chart.append(svg("line", { class: "grid", x1: margin.left, x2: width - margin.right, y1: y(tick), y2: y(tick) }));
    chart.append(svg("text", { class: "tick", x: margin.left - 8, y: y(tick) + 4, "text-anchor": "end" }, wholeMoney.format(tick)));
  }
  chart.append(svg("line", { class: "axis", x1: margin.left, x2: width - margin.right, y1: height - margin.bottom, y2: height - margin.bottom }));

  // Date labels, as many as fit, anchored so the first and last stay inside the plot.
  const labelCount = Math.max(2, Math.min(6, Math.floor(plotWidth / 110)));
  for (let i = 0; i < labelCount; i++) {
    const time = tMin + ((tMax - tMin) * i) / (labelCount - 1);
    const anchor = i === 0 ? "start" : i === labelCount - 1 ? "end" : "middle";
    chart.append(svg("text", { class: "tick", x: x(time), y: height - 8, "text-anchor": anchor }, monthYear.format(time)));
  }

  const path = points.map((p, i) => (i ? "L" : "M") + x(p.time).toFixed(1) + " " + y(p.value).toFixed(1)).join(" ");
  const bottom = height - margin.bottom;
  chart.append(svg("path", { class: "area", d: path + " L" + x(tMax).toFixed(1) + " " + bottom + " L" + x(tMin).toFixed(1) + " " + bottom + " Z" }));
  chart.append(svg("path", { class: "line", d: path }));

  // Only the end of the line is labeled; every other value is in the tooltip and the table.
  chart.append(svg("circle", { class: "dot", cx: x(last.time), cy: y(last.value), r: 4 }));
  // The label sits above the highest point under it, so it never lands on the line.
  const labelWidth = 96;
  const peakUnderLabel = Math.max(...points.filter((p) => x(p.time) >= x(last.time) - labelWidth).map((p) => p.value));
  chart.append(svg("text", { class: "end-label", x: x(last.time), y: Math.max(12, y(peakUnderLabel) - 10), "text-anchor": "end" }, money.format(last.value)));

  const crosshair = svg("line", { class: "crosshair", y1: margin.top, y2: bottom, visibility: "hidden" });
  const marker = svg("circle", { class: "dot", r: 4, visibility: "hidden" });
  chart.append(crosshair, marker);

  let selected = points.length - 1;
  function select(index, clientX, clientY) {
    selected = Math.max(0, Math.min(points.length - 1, index));
    const point = points[selected];
    crosshair.setAttribute("x1", x(point.time));
    crosshair.setAttribute("x2", x(point.time));
    marker.setAttribute("cx", x(point.time));
    marker.setAttribute("cy", y(point.value));
    crosshair.setAttribute("visibility", "visible");
    marker.setAttribute("visibility", "visible");

    const box = chart.getBoundingClientRect();
    showTooltip(
      fullDate.format(point.time),
      [
        ["Value", money.format(point.value)],
        ["Change from start", signedPercent((point.value / start - 1) * 100)],
      ],
      clientX ?? box.left + x(point.time),
      clientY ?? box.top + y(point.value),
    );
  }
  function clear() {
    crosshair.setAttribute("visibility", "hidden");
    marker.setAttribute("visibility", "hidden");
    hideTooltip();
  }

  // The pointer only has to be near a date, never on the line itself.
  chart.addEventListener("pointermove", (event) => {
    const box = chart.getBoundingClientRect();
    const time = tMin + ((event.clientX - box.left - margin.left) / plotWidth) * (tMax - tMin);
    let nearest = 0;
    for (let i = 1; i < points.length; i++) {
      if (Math.abs(points[i].time - time) < Math.abs(points[nearest].time - time)) nearest = i;
    }
    select(nearest, event.clientX, event.clientY);
  });
  chart.addEventListener("pointerleave", clear);
  chart.addEventListener("focus", () => select(selected));
  chart.addEventListener("blur", clear);
  chart.addEventListener("keydown", (event) => {
    const step = { ArrowLeft: -1, ArrowRight: 1, PageUp: -21, PageDown: 21 }[event.key];
    if (event.key === "Home") select(0);
    else if (event.key === "End") select(points.length - 1);
    else if (step) select(selected + step);
    else return;
    event.preventDefault();
  });

  container.replaceChildren(chart);
}

function renderBacktrack(result) {
  backtrackResult = result;

  const tiles = [
    { label: "$10,000 became", value: money.format(result.final_amount) },
    { label: "Total change", value: signedPercent(result.total_return_pct) },
    { label: "Per year, compounded", value: signedPercent(result.annualized_return_pct) },
    { label: "Largest drop from a peak", value: "\u2212" + percent.format(result.max_drawdown_pct) + "%" },
  ];
  const container = $("backtrack-tiles");
  container.replaceChildren();
  for (const tile of tiles) {
    const node = el("div", "tile");
    node.append(el("p", "label", tile.label), el("p", "tile-value", tile.value));
    container.append(node);
  }

  $("backtrack-chart-title").textContent =
    "Value of $10,000, " + fullDate.format(Date.parse(result.from)) + " to " + fullDate.format(Date.parse(result.to)) +
    (result.rebalance === "never" ? ", bought once and held" : ", rebalanced " + result.rebalance);

  // The table twin of the chart: the last trading day of every month.
  const values = $("backtrack-values").querySelector("tbody");
  values.replaceChildren();
  result.points.forEach((point, i) => {
    const next = result.points[i + 1];
    if (next && next.date.slice(0, 7) === point.date.slice(0, 7)) return;
    const tr = el("tr");
    tr.append(
      el("td", "", fullDate.format(Date.parse(point.date))),
      el("td", "num", money.format(point.value)),
      el("td", "num", signedPercent((point.value / result.initial_amount - 1) * 100)),
    );
    values.append(tr);
  });

  const excluded = $("backtrack-excluded");
  if (result.excluded.length > 0) {
    const weight = result.excluded.reduce((sum, e) => sum + e.weight, 0) * 100;
    excluded.textContent =
      "Left out for lack of price history at the start of the period (" + percent.format(weight) + "% of the positions; their weight is spread over the rest): " +
      result.excluded.map((e) => e.symbol + " (" + e.reason + ")").join(", ") + ".";
  } else {
    excluded.textContent = "";
  }

  const body = $("backtrack-slices").querySelector("tbody");
  body.replaceChildren();
  for (const slice of [...result.slices].sort((a, b) => b.contribution - a.contribution)) {
    const tr = el("tr");
    tr.append(
      el("td", "", slice.symbol),
      el("td", "num", percent.format(slice.weight * 100) + "%"),
      el("td", "num", money.format(slice.start_price)),
      el("td", "num", money.format(slice.end_price)),
      el("td", "num", signedPercent(slice.price_return_pct)),
      el("td", "num", signedMoney(slice.contribution)),
    );
    body.append(tr);
  }

  $("backtrack-result").hidden = false;
  drawBacktrackChart();
}

async function runBacktrack(event) {
  event.preventDefault();
  const form = $("backtrack-form");
  const button = $("backtrack-run");
  const status = $("backtrack-status");
  const error = $("backtrack-error");
  const result = $("backtrack-result");

  button.disabled = true;
  result.classList.add("stale");
  status.textContent = "Loading price history for every position\u2026";
  try {
    const query = new URLSearchParams(new FormData(form));
    const response = await fetch("api/backtrack?" + query, { cache: "no-store" });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || "The request failed with status " + response.status + ".");
    renderBacktrack(body);
    error.hidden = true;
  } catch (err) {
    error.textContent = "Could not run the backtrack: " + err.message;
    error.hidden = false;
  } finally {
    status.textContent = "";
    result.classList.remove("stale");
    button.disabled = false;
  }
}

$("backtrack-form").addEventListener("submit", runBacktrack);

let resizeTimer;
window.addEventListener("resize", () => {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(drawBacktrackChart, 120);
});

$("refresh").addEventListener("click", load);
load();
