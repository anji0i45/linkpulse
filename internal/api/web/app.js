const state = { links: [], analyticsCode: null };
const byId = (id) => document.getElementById(id);
const number = new Intl.NumberFormat();

function setError(message) {
  const box = byId("error");
  box.textContent = message;
  box.hidden = false;
}

function clearError() {
  byId("error").hidden = true;
}

async function request(path, options) {
  const response = await fetch(path, options);
  let body = {};
  try {
    body = await response.json();
  } catch (_) {
    // DELETE returns 204 with no JSON body.
  }
  if (!response.ok) {
    throw new Error(body.error || "Request failed (" + response.status + ")");
  }
  return body;
}

function shortURL(code) {
  return window.location.origin + "/" + code;
}

function formatDate(value) {
  if (!value) return "";
  return new Date(value).toLocaleString();
}

function statusOf(link) {
  if (link.disabled) return "disabled";
  if (link.expires_at && new Date(link.expires_at) <= new Date()) return "expired";
  return "active";
}

function filteredLinks() {
  const query = byId("search-input").value.trim().toLowerCase();
  const status = byId("status-filter").value;
  return state.links.filter((link) => {
    const matchesText = !query ||
      link.code.toLowerCase().includes(query) ||
      link.long_url.toLowerCase().includes(query);
    return matchesText && (status === "all" || statusOf(link) === status);
  });
}

function renderLinks() {
  const container = byId("links");
  const links = filteredLinks();
  container.replaceChildren();

  if (!links.length) {
    const empty = document.createElement("div");
    empty.className = "empty";
    empty.textContent = state.links.length
      ? "No links match this search or filter."
      : "No links yet. Paste a long URL above to create your first one.";
    container.append(empty);
    return;
  }

  for (const link of links) {
    const row = document.createElement("div");
    row.className = "link-row";

    const main = document.createElement("div");
    main.className = "link-main";

    const title = document.createElement("div");
    title.className = "link-title";

    const code = document.createElement("a");
    code.className = "link-code";
    code.href = shortURL(link.code);
    code.target = "_blank";
    code.rel = "noopener";
    code.textContent = shortURL(link.code);

    const linkStatus = statusOf(link);
    const status = document.createElement("span");
    status.className = "status status-" + linkStatus;
    status.textContent = linkStatus;

    title.append(code, status);

    const url = document.createElement("span");
    url.className = "link-url";
    url.textContent = link.long_url;
    url.title = link.long_url;

    const detail = document.createElement("span");
    detail.className = "link-detail";
    let detailText = "Created " + formatDate(link.created_at);
    if (link.expires_at) {
      detailText += " · Expires " + formatDate(link.expires_at);
    }
    detail.textContent = detailText;

    main.append(title, url, detail);

    const meta = document.createElement("div");
    meta.className = "link-meta";

    const clicks = document.createElement("span");
    clicks.className = "clicks";
    clicks.textContent = number.format(link.clicks) + " clicks";

    const actions = document.createElement("div");
    actions.className = "link-actions";

    const view = document.createElement("button");
    view.type = "button";
    view.className = "ghost";
    view.textContent = "Analytics";
    view.addEventListener("click", () => openAnalytics(link.code));

    const qr = document.createElement("a");
    qr.className = "ghost";
    qr.href = "/api/qr/" + encodeURIComponent(link.code);
    qr.target = "_blank";
    qr.rel = "noopener";
    qr.textContent = "QR";

    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "ghost";
    toggle.textContent = link.disabled ? "Enable" : "Disable";
    toggle.addEventListener("click", () => toggleLink(link, toggle));

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "ghost danger";
    remove.textContent = "Delete";
    remove.addEventListener("click", () => deleteLink(link, remove));

    actions.append(view);
    if (linkStatus === "active") actions.append(qr);
    actions.append(toggle, remove);
    meta.append(clicks, actions);
    row.append(main, meta);
    container.append(row);
  }
}

async function loadLinks() {
  try {
    const data = await request("/api/links");
    state.links = data.links || [];
    renderLinks();

    const totalClicks = state.links.reduce((sum, link) => sum + link.clicks, 0);
    byId("total-links").textContent = number.format(state.links.length);
    byId("total-clicks").textContent = number.format(totalClicks);

    const top = [...state.links].sort((a, b) => b.clicks - a.clicks)[0];
    byId("top-link").textContent = top && top.clicks ? "/" + top.code : "—";

    if (state.analyticsCode && !state.links.some((link) => link.code === state.analyticsCode)) {
      closeAnalytics();
    }
  } catch (error) {
    setError(error.message);
    byId("links").replaceChildren();
    const empty = document.createElement("div");
    empty.className = "empty";
    empty.textContent = "Could not load links. Is the server running?";
    byId("links").append(empty);
  }
}

async function toggleLink(link, button) {
  button.disabled = true;
  try {
    await request("/api/links/" + encodeURIComponent(link.code), {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ disabled: !link.disabled }),
    });
    await loadLinks();
  } catch (error) {
    setError(error.message);
    button.disabled = false;
  }
}

async function deleteLink(link, button) {
  if (!window.confirm("Delete /" + link.code + "? This also removes its clicks.")) return;
  button.disabled = true;
  try {
    await request("/api/links/" + encodeURIComponent(link.code), { method: "DELETE" });
    if (state.analyticsCode === link.code) closeAnalytics();
    await loadLinks();
  } catch (error) {
    setError(error.message);
    button.disabled = false;
  }
}

byId("create-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  clearError();

  const input = byId("long-url");
  const button = byId("shorten-btn");
  const value = input.value.trim();
  if (!value) {
    setError("Please paste a URL first.");
    input.focus();
    return;
  }

  const customCode = byId("custom-code").value.trim();
  if (customCode && !/^[A-Za-z0-9_-]{3,32}$/.test(customCode)) {
    setError("Custom alias must be 3-32 letters, numbers, hyphens or underscores.");
    byId("custom-code").focus();
    return;
  }

  const expiryHours = byId("expiry").value;
  const payload = { long_url: value };
  if (customCode) payload.custom_code = customCode;
  if (expiryHours) {
    payload.expires_at = new Date(Date.now() + Number(expiryHours) * 60 * 60 * 1000).toISOString();
  }

  button.disabled = true;
  button.textContent = "Creating…";

  try {
    const data = await request("/api/links", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });

    const shortLink = byId("short-link");
    shortLink.href = data.short_url;
    shortLink.textContent = data.short_url;
    byId("qr-link").href = "/api/qr/" + encodeURIComponent(data.code);
    byId("result").hidden = false;
    input.value = "";
    byId("custom-code").value = "";
    byId("expiry").value = "";
    await loadLinks();
  } catch (error) {
    setError(error.message);
  } finally {
    button.disabled = false;
    button.textContent = "Shorten";
  }
});

byId("copy-btn").addEventListener("click", async () => {
  const text = byId("short-link").textContent;
  try {
    await navigator.clipboard.writeText(text);
  } catch (_) {
    const area = document.createElement("textarea");
    area.value = text;
    document.body.append(area);
    area.select();
    document.execCommand("copy");
    area.remove();
  }
  const button = byId("copy-btn");
  button.textContent = "Copied!";
  window.setTimeout(() => { button.textContent = "Copy"; }, 1400);
});

byId("refresh-btn").addEventListener("click", loadLinks);
byId("search-input").addEventListener("input", renderLinks);
byId("status-filter").addEventListener("change", renderLinks);

function renderBars(container, items, labelKey, valueKey) {
  container.replaceChildren();
  if (!items || !items.length) {
    const empty = document.createElement("div");
    empty.className = "empty";
    empty.textContent = "No data yet.";
    container.append(empty);
    return;
  }

  const max = Math.max(...items.map((item) => Number(item[valueKey]) || 0), 1);
  for (const item of items) {
    const row = document.createElement("div");
    row.className = "bar-row";

    const label = document.createElement("span");
    label.className = "bar-label";
    label.textContent = item[labelKey];
    label.title = item[labelKey];

    const track = document.createElement("div");
    track.className = "bar-track";

    const fill = document.createElement("div");
    fill.className = "bar-fill";
    fill.style.width = Math.max(3, (Number(item[valueKey]) / max) * 100) + "%";
    track.append(fill);

    const value = document.createElement("span");
    value.className = "bar-value";
    value.textContent = number.format(item[valueKey]);

    row.append(label, track, value);
    container.append(row);
  }
}

async function openAnalytics(code) {
  const panel = byId("analytics-panel");
  state.analyticsCode = code;
  panel.hidden = false;
  byId("analytics-title").textContent = "/" + code;
  byId("day-chart").replaceChildren();
  byId("referrer-chart").replaceChildren();

  try {
    const data = await request("/api/analytics/" + encodeURIComponent(code));
    byId("analytics-url").textContent = data.long_url;
    byId("analytics-total").textContent = number.format(data.total_clicks);
    byId("analytics-unique").textContent = number.format(data.unique_clicks);
    renderBars(byId("day-chart"), data.clicks_by_day || [], "day", "clicks");
    renderBars(byId("referrer-chart"), data.top_referrers || [], "name", "clicks");
  } catch (error) {
    setError(error.message);
  }

  panel.scrollIntoView({ behavior: "smooth", block: "start" });
}

function closeAnalytics() {
  state.analyticsCode = null;
  byId("analytics-panel").hidden = true;
}

byId("close-analytics").addEventListener("click", closeAnalytics);

loadLinks();
