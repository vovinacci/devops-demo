// reports-ui SPA (RFC-0002 D3): vanilla JS, no build step. Every request goes
// to the same origin under /api/*, which Caddy reverse-proxies to the reports
// service (prefix stripped by handle_path). No CORS, no hardcoded host.
"use strict";

const API = "/api";
const POLL_MS = 1500;
const POLL_TIMEOUT_MS = 60000;
const REFRESH_MS = 5000;
const TERMINAL = new Set(["SUCCEEDED", "FAILED"]);

const el = (id) => document.getElementById(id);
const form = el("report-form");
const submitBtn = el("submit");
const banner = el("banner");
const current = el("current");
const currentId = el("current-id");
const currentStatus = el("current-status");
const currentDownload = el("current-download");
const jobsBody = el("jobs-body");
const refreshBtn = el("refresh");

let pollTimer = null;
let refreshTimer = null;

// --- helpers ---------------------------------------------------------------

function showBanner(message) {
  banner.textContent = message;
  banner.hidden = false;
}

function clearBanner() {
  banner.hidden = true;
  banner.textContent = "";
}

function setStatusPill(node, status) {
  node.textContent = status;
  node.dataset.status = status;
}

function fmtTime(iso) {
  if (!iso) return "-";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function fmtBytes(n) {
  if (n == null || Number.isNaN(n)) return "-";
  if (n < 1024) return `${n} B`;
  const kb = n / 1024;
  if (kb < 1024) return `${kb.toFixed(1)} KB`;
  return `${(kb / 1024).toFixed(1)} MB`;
}

async function apiFetch(path, options) {
  const res = await fetch(`${API}${path}`, options);
  if (!res.ok) {
    const err = new Error(`HTTP ${res.status}`);
    err.status = res.status;
    throw err;
  }
  return res;
}

// --- submit + poll ---------------------------------------------------------

async function submitReport(evt) {
  evt.preventDefault();
  clearBanner();
  if (pollTimer) {
    clearTimeout(pollTimer);
    pollTimer = null;
  }

  const type = el("type").value;
  const format = new FormData(form).get("format");

  submitBtn.disabled = true;
  submitBtn.textContent = "Submitting...";

  try {
    const res = await apiFetch("/reports", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ type, format }),
    });
    const job = await res.json();
    current.hidden = false;
    currentId.textContent = job.id;
    currentDownload.hidden = true;
    setStatusPill(currentStatus, job.status || "PENDING");
    pollJob(job.id, Date.now());
    loadJobs();
  } catch (err) {
    showBanner(
      `Could not submit the report (${err.message}). Is the reports service running?`,
    );
  } finally {
    submitBtn.disabled = false;
    submitBtn.textContent = "Generate report";
  }
}

async function pollJob(id, startedAt) {
  try {
    const res = await apiFetch(`/reports/${encodeURIComponent(id)}`);
    const job = await res.json();
    setStatusPill(currentStatus, job.status);

    if (job.status === "SUCCEEDED" && job.download) {
      currentDownload.hidden = false;
      currentDownload.href = `${API}${job.download}`;
    }

    if (TERMINAL.has(job.status)) {
      loadJobs();
      return;
    }
  } catch (err) {
    showBanner(`Lost contact with the reports service while polling (${err.message}).`);
    return;
  }

  if (Date.now() - startedAt > POLL_TIMEOUT_MS) {
    showBanner("Still running after 60s -- stopped auto-refresh; use Refresh below.");
    return;
  }
  pollTimer = setTimeout(() => pollJob(id, startedAt), POLL_MS);
}

// --- recent jobs -----------------------------------------------------------

function renderEmpty(message) {
  const row = document.createElement("tr");
  row.className = "empty-row";
  const cell = document.createElement("td");
  cell.colSpan = 6;
  cell.textContent = message;
  row.appendChild(cell);
  jobsBody.replaceChildren(row);
}

function renderJobs(jobs) {
  if (!jobs.length) {
    renderEmpty("No reports yet. Submit one above.");
    return;
  }
  const rows = jobs.map((job) => {
    const tr = document.createElement("tr");

    const created = document.createElement("td");
    created.textContent = fmtTime(job.createdAt);

    const type = document.createElement("td");
    type.textContent = job.type || "-";

    const format = document.createElement("td");
    format.textContent = job.format || "-";

    const status = document.createElement("td");
    const pill = document.createElement("span");
    pill.className = "pill";
    setStatusPill(pill, job.status);
    status.appendChild(pill);

    const size = document.createElement("td");
    size.textContent = fmtBytes(job.artifactBytes);

    const action = document.createElement("td");
    if (job.download) {
      const link = document.createElement("a");
      link.className = "dl-link";
      link.href = `${API}${job.download}`;
      link.textContent = "Download";
      action.appendChild(link);
    } else {
      const dash = document.createElement("span");
      dash.className = "dash";
      dash.textContent = "-";
      action.appendChild(dash);
    }

    tr.append(created, type, format, status, size, action);
    return tr;
  });
  jobsBody.replaceChildren(...rows);
}

async function loadJobs() {
  try {
    const res = await apiFetch("/reports?limit=20");
    const jobs = await res.json();
    renderJobs(Array.isArray(jobs) ? jobs : []);
    clearBanner();
  } catch (err) {
    // D10 graceful degradation: the page still serves; the list just shows a
    // friendly unavailable state instead of crashing.
    renderEmpty("Reports service unavailable -- the list will refresh when it is back.");
    showBanner(`Reports service unavailable (${err.message}).`);
  }
}

// --- wiring ----------------------------------------------------------------

form.addEventListener("submit", submitReport);
refreshBtn.addEventListener("click", loadJobs);

loadJobs();
refreshTimer = setInterval(loadJobs, REFRESH_MS);
