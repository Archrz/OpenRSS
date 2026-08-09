"use strict";

const state = {
  feeds: [],
  articles: [], 
  feedStatus: {},
  globalLoading: false,
  selectedFeedId: null, 
  openArticle: null, 
  searchQuery: "",
  searchResults: [],
  searchSelectedIdx: null,
};

// tiny helpers

async function api(path, opts) {
  const res = await fetch(path, opts);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || res.statusText);
  return body;
}

function escapeHtml(s) {
  const div = document.createElement("div");
  div.textContent = s ?? "";
  return div.innerHTML;
}

// used everywhere a date is shown
function formatRelative(iso, { short } = {}) {
  if (!iso) return "";
  const d = new Date(iso);
  if (isNaN(d)) return "";
  const diffSec = (Date.now() - d.getTime()) / 1000;
  if (diffSec < 3600) return `${Math.max(0, Math.floor(diffSec / 60))}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  if (!short && diffSec < 604800) return `${Math.floor(diffSec / 86400)}d ago`;
  return d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

function formatFull(iso) {
  if (!iso) return "";
  const d = new Date(iso);
  if (isNaN(d)) return "";
  return d.toLocaleDateString("en-US", {
    weekday: "short",
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

// small match icon and the big pie chart
function pieSVG(matched, total, { size = 20, donut = false } = {}) {
  const ratio = total > 0 ? matched / total : 0;
  const r = (size / 2) * 0.82;
  const cx = size / 2,
    cy = size / 2;
  const fill =
    ratio >= 0.9 ? "var(--accent)" : ratio >= 0.6 ? "var(--warn)" : "#888888";
  const track = "#242424",
    bg = "#181818";
  let inner;
  if (ratio >= 1) {
    inner = `
      <circle cx="${cx}" cy="${cy}" r="${r}" fill="${fill}" opacity="0.15"/>
      <circle cx="${cx}" cy="${cy}" r="${r}" fill="none" stroke="${fill}" stroke-width="2"/>
      <text x="${cx}" y="${cy + 3.5}" text-anchor="middle" font-family="var(--font-mono)"
            font-size="${size * 0.3}" fill="${fill}">&#10003;</text>`;
  } else if (ratio <= 0) {
    inner = `<circle cx="${cx}" cy="${cy}" r="${r}" fill="${bg}" stroke="${track}" stroke-width="1.5"/>`;
  } else {
    const angle = ratio * 2 * Math.PI - Math.PI / 2;
    const x = cx + r * Math.cos(angle),
      y = cy + r * Math.sin(angle);
    const large = ratio > 0.5 ? 1 : 0;
    inner = `
      <circle cx="${cx}" cy="${cy}" r="${r}" fill="${bg}" stroke="${track}" stroke-width="1"/>
      <path d="M ${cx} ${cy} L ${cx} ${cy - r} A ${r} ${r} 0 ${large} 1 ${x} ${y} Z" fill="${fill}" opacity="0.85"/>
      <circle cx="${cx}" cy="${cy}" r="${r}" fill="none" stroke="${fill}" stroke-width="1" opacity="0.4"/>`;
  }
  if (donut)
    inner += `<circle cx="${cx}" cy="${cy}" r="${r * 0.55}" fill="#0f0f0f"/>`;
  const label = `${matched}/${total} keywords matched`;
  return `<svg width="${size}" height="${size}" viewBox="0 0 ${size} ${size}" aria-label="${label}">${inner}</svg>`;
}

function matchColor(ratio) {
  return ratio >= 0.9 ? "var(--accent)" : ratio >= 0.6 ? "var(--warn)" : "#555";
}

// data loading

async function loadFeeds() {
  state.feeds = await api("/api/feeds");
}

async function loadArticles() {
  state.articles = await api("/api/articles");
}

async function refreshFeed(feed) {
  state.feedStatus[feed.id] = "loading";
  renderSidebar();
  try {
    const r = await api(`/api/feeds/${feed.id}/refresh`, { method: "POST" });
    state.feedStatus[feed.id] = r.error ? "error" : "done";
  } catch {
    state.feedStatus[feed.id] = "error";
  }
  await loadArticles();
  renderAll();
}

async function refreshAll() {
  state.globalLoading = true;
  renderSidebar();
  try {
    const { results } = await api("/api/refresh", { method: "POST" });
    for (const r of results || []) {
      state.feedStatus[r.feedId] = r.error ? "error" : "done";
    }
  } catch {
  }
  state.globalLoading = false;
  await Promise.all([loadFeeds(), loadArticles()]);
  renderAll();
}

async function addFeed(url) {
  await api("/api/feeds", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  });
  await Promise.all([loadFeeds(), loadArticles()]);
  renderAll();
}

async function removeFeed(id) {
  await api(`/api/feeds/${id}`, { method: "DELETE" });
  if (state.selectedFeedId === id) state.selectedFeedId = null;
  await Promise.all([loadFeeds(), loadArticles()]);
  renderAll();
}

async function openArticle(article) {
  const full = await api(`/api/articles/${article.id}`);
  state.openArticle = full;
  const local = state.articles.find((a) => a.id === article.id);
  if (local) local.read = true;
  renderSidebar();
  renderArticleList();
  renderReader();
}

function closeReader() {
  state.openArticle = null;
  renderReader();
  renderArticleList();
}

async function markAllRead() {
  await api("/api/articles/read-all", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ feedId: state.selectedFeedId || 0 }),
  });
  for (const a of state.articles) {
    if (!state.selectedFeedId || a.feedId === state.selectedFeedId)
      a.read = true;
  }
  renderAll();
}

// rendering

function visibleArticles() {
  const list = state.selectedFeedId
    ? state.articles.filter((a) => a.feedId === state.selectedFeedId)
    : state.articles;
  return [...list].sort(
    (a, b) =>
      new Date(b.pubDate || b.fetchedAt) - new Date(a.pubDate || a.fetchedAt),
  );
}

function unreadCounts() {
  const counts = {};
  for (const a of state.articles)
    if (!a.read) counts[a.feedId] = (counts[a.feedId] || 0) + 1;
  return counts;
}

function badgeHtml(n) {
  if (!n) return "";
  return `<span class="badge">${n > 99 ? "99+" : n}</span>`;
}

function renderSidebar() {
  const counts = unreadCounts();
  const totalUnread = Object.values(counts).reduce((a, b) => a + b, 0);

  document
    .getElementById("all-articles-row")
    .classList.toggle("active", state.selectedFeedId === null);
  const allBadge = document.getElementById("all-unread-badge");
  allBadge.innerHTML =
    totalUnread > 0 ? (totalUnread > 99 ? "99+" : totalUnread) : "";
  allBadge.classList.toggle("hidden", totalUnread === 0);

  const list = document.getElementById("feed-list");
  if (state.feeds.length === 0) {
    list.innerHTML = `<div class="feed-list-empty">No feeds yet.</div>`;
  } else {
    list.innerHTML = state.feeds
      .map((f) => {
        const status = state.feedStatus[f.id];
        const unread = counts[f.id] || 0;
        const active = state.selectedFeedId === f.id;
        const dot =
          status === "loading"
            ? `<span class="dot loading"></span>`
            : status === "error"
              ? `<span class="dot error"></span>`
              : "";
        return `<div class="feed-item${active ? " active" : ""}" data-feed-id="${f.id}">
        <span class="feed-name">${escapeHtml(f.title)}</span>
        ${dot}
        ${unread && status !== "loading" ? badgeHtml(unread) : ""}
        <button class="icon-btn refresh-btn${status === "loading" ? " spin" : ""}" data-action="refresh" title="Refresh"></button>
        <button class="icon-btn remove-btn" data-action="remove" title="Remove feed"></button>
      </div>`;
      })
      .join("");
  }

  const refreshAllBtn = document.getElementById("btn-refresh-all");
  refreshAllBtn.disabled = state.globalLoading;
  refreshAllBtn.classList.toggle("spin", state.globalLoading);
}

function renderArticleList() {
  const articles = visibleArticles();
  const feed = state.feeds.find((f) => f.id === state.selectedFeedId);
  document.getElementById("feed-title-label").textContent = feed
    ? feed.title
    : "All articles";
  document.getElementById("feed-item-count").textContent =
    `${articles.length} item${articles.length === 1 ? "" : "s"}`;

  const list = document.getElementById("article-list");
  if (articles.length === 0) {
    list.innerHTML = `<div class="list-empty">No articles</div>`;
    return;
  }
  list.innerHTML = articles
    .map((a) => {
      const selected = state.openArticle && state.openArticle.id === a.id;
      return `<div class="article-row${a.read ? "" : " unread"}${selected ? " selected" : ""}" data-article-id="${a.id}">
      ${a.read ? "" : '<span class="unread-dot"></span>'}
      <div class="title">${escapeHtml(a.title)}</div>
      ${a.summary ? `<div class="summary">${escapeHtml(a.summary)}</div>` : ""}
      <div class="meta">
        <span>${escapeHtml(a.feedTitle)}</span>
        ${a.pubDate ? `<span class="sep">·</span><span>${formatRelative(a.pubDate)}</span>` : ""}
      </div>
    </div>`;
    })
    .join("");
}

function renderReader() {
  const reader = document.getElementById("reader");
  const a = state.openArticle;
  reader.classList.toggle("open", !!a);
  if (!a) return;
  document.getElementById("reader-feed-title").textContent = a.feedTitle;
  document.getElementById("reader-content").innerHTML = `
    <h1 class="reader-title">${escapeHtml(a.title)}</h1>
    <div class="reader-byline">
      ${
        a.author
          ? `
        <span class="author">
          ${a.authorAvatar ? `<img class="author-avatar" src="${escapeHtml(a.authorAvatar)}" alt="">` : ""}
          <span>${escapeHtml(a.author)}</span>
          ${a.authorEmail ? `<span class="author-email">${escapeHtml(a.authorEmail)}</span>` : ""}
        </span>
      `
          : ""
      }
      ${a.pubDate ? `<span>${formatFull(a.pubDate)}</span>` : ""}
    </div>
    <div class="article-prose">${a.content || ""}</div>
  `;
  document.getElementById("reader-content").scrollTop = 0;
}

function renderAll() {
  renderSidebar();
  renderArticleList();
}

// ---- search ----

let searchDebounce;

function onSearchInput(value) {
  state.searchQuery = value;
  state.searchSelectedIdx = null;
  clearTimeout(searchDebounce);
  if (!value.trim()) {
    state.searchResults = [];
    renderSearch();
    return;
  }
  searchDebounce = setTimeout(runSearch, 120);
}

async function runSearch() {
  const q = state.searchQuery;
  if (!q.trim()) return;
  const params = new URLSearchParams({ q });
  const from = document.getElementById("search-from").value;
  const to = document.getElementById("search-to").value;
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  state.searchResults = await api(`/api/search?${params}`);
  renderSearch();
}

// mirrors Go's Keywords, "quoted phrase" is one keyword
const keywordRE = /"([^"]+)"|([^\s,]+)/g;
function keywordsOf(query) {
  const kws = [];
  for (const m of query.matchAll(keywordRE)) {
    const word = (m[1] ?? m[2]).trim().toLowerCase();
    if (word.length > 1) kws.push(word);
  }
  return kws;
}

function chipsHtml(words, cls = "") {
  return words
    .map((k) => `<span class="chip${cls ? " " + cls : ""}">${escapeHtml(k)}</span>`)
    .join("");
}

function renderSearch() {
  const keywords = keywordsOf(state.searchQuery);
  const countEl = document.getElementById("search-count");
  countEl.textContent = state.searchQuery
    ? `${state.searchResults.length} results`
    : "";

  const kwBox = document.getElementById("search-keywords");
  kwBox.classList.toggle("hidden", keywords.length === 0);
  kwBox.innerHTML =
    chipsHtml(keywords) +
    (keywords.length
      ? `<span class="keyword-count">${keywords.length} keyword${keywords.length !== 1 ? "s" : ""}</span>`
      : "");

  const results = document.getElementById("search-results");
  if (!state.searchQuery.trim()) {
    results.innerHTML = `<div class="search-empty">type to search across all ${state.articles.length} articles</div>`;
  } else if (state.searchResults.length === 0) {
    results.innerHTML = `<div class="search-empty">no results</div>`;
  } else {
    results.innerHTML = state.searchResults
      .map(
        (r, i) => `
      <div class="search-result-row${state.searchSelectedIdx === i ? " selected" : ""}" data-idx="${i}">
        <div>${pieSVG(r.matched, r.total, { size: 22 })}</div>
        <div style="flex:1;min-width:0">
          <div class="title">${escapeHtml(r.article.title)}</div>
          <div class="meta">
            <span>${escapeHtml(r.article.feedTitle)}</span>
            ${r.article.pubDate ? `<span>·</span><span>${formatRelative(r.article.pubDate, { short: true })}</span>` : ""}
          </div>
        </div>
        <div class="match-badge" style="color:${matchColor(r.ratio)}">${r.matched}/${r.total}</div>
      </div>`,
      )
      .join("");
  }

  const preview = document.getElementById("search-preview");
  const sel =
    state.searchSelectedIdx !== null
      ? state.searchResults[state.searchSelectedIdx]
      : null;
  preview.classList.toggle("hidden", !sel);
  if (sel) {
    const missing = keywords.filter((k) => !sel.matchedKeywords.includes(k));
    const matchedChips = chipsHtml(sel.matchedKeywords);
    const missingChips = chipsHtml(missing, "missing");
    preview.innerHTML = `
      <div style="text-align:center">${pieSVG(sel.matched, sel.total, { size: 120, donut: true })}</div>
      <div class="pie-caption">${sel.matched}/${sel.total} keywords</div>
      <div class="kw-label matched">matched</div>
      <div class="chips">${matchedChips}</div>
      ${missing.length ? `<div class="kw-label missing">missing</div><div class="chips">${missingChips}</div>` : ""}
      <div class="preview-summary">${escapeHtml(sel.article.summary)}</div>
    `;
  }
}

function openSearch() {
  document.getElementById("search-overlay").classList.remove("hidden");
  document.getElementById("search-input").focus();
  renderSearch();
}
function closeSearch() {
  document.getElementById("search-overlay").classList.add("hidden");
}

// settings

async function openSettings() {
  document.getElementById("settings-overlay").classList.remove("hidden");
  const { feeds, articles } = await api("/api/stats");
  document.getElementById("stat-feeds").textContent = feeds;
  document.getElementById("stat-articles").textContent = articles;
  const { articleCap } = await api("/api/settings");
  document.getElementById("article-cap-input").value = articleCap || "";
}
function closeSettings() {
  document.getElementById("settings-overlay").classList.add("hidden");
  document.getElementById("opml-hint").textContent = "Import/export OPML 2.0";
}

// wiring

function closeOnBackdropClick(overlayId, close) {
  document.getElementById(overlayId).addEventListener("click", (e) => {
    if (e.target.id === overlayId) close();
  });
}

function closeTopOverlay() {
  if (!document.getElementById("search-overlay").classList.contains("hidden")) {
    closeSearch();
    return true;
  }
  if (
    !document.getElementById("settings-overlay").classList.contains("hidden")
  ) {
    closeSettings();
    return true;
  }
  if (state.openArticle) {
    closeReader();
    return true;
  }
  return false;
}

function init() {
  document.addEventListener("click", (e) => {
    const a = e.target.closest("a[href]");
    if (!a || a.href.startsWith(window.location.origin)) return;
    e.preventDefault();
    if (window.openExternal) {
      window.openExternal(a.href);
    } else {
      window.open(a.href, "_blank");
    }
  });

  document.getElementById("btn-search").addEventListener("click", openSearch);
  closeOnBackdropClick("search-overlay", closeSearch);
  document
    .getElementById("search-input")
    .addEventListener("input", (e) => onSearchInput(e.target.value));
  document.getElementById("search-from").addEventListener("change", runSearch);
  document.getElementById("search-to").addEventListener("change", runSearch);
  document.getElementById("search-input").addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      closeSearch();
      return;
    }
    if (e.key === "ArrowDown") {
      e.preventDefault();
      const max = state.searchResults.length - 1;
      state.searchSelectedIdx = Math.min(
        (state.searchSelectedIdx ?? -1) + 1,
        max,
      );
      renderSearch();
    }
    if (e.key === "ArrowUp") {
      e.preventDefault();
      state.searchSelectedIdx = Math.max((state.searchSelectedIdx ?? 1) - 1, 0);
      renderSearch();
    }
    if (e.key === "Enter" && state.searchSelectedIdx !== null) {
      const r = state.searchResults[state.searchSelectedIdx];
      closeSearch();
      openArticle(r.article);
    }
  });
  document
    .getElementById("search-results")
    .addEventListener("mouseover", (e) => {
      const row = e.target.closest("[data-idx]");
      if (!row) return;
      const idx = Number(row.dataset.idx);
      if (state.searchSelectedIdx === idx) return;
      state.searchSelectedIdx = idx;
      renderSearch();
    });
  document.getElementById("search-results").addEventListener("click", (e) => {
    const row = e.target.closest("[data-idx]");
    if (!row) return;
    const r = state.searchResults[Number(row.dataset.idx)];
    closeSearch();
    openArticle(r.article);
  });

  document
    .getElementById("btn-refresh-all")
    .addEventListener("click", refreshAll);
  document.getElementById("btn-refresh-all-2").addEventListener("click", () => {
    refreshAll();
    closeSettings();
  });
  document
    .getElementById("btn-settings")
    .addEventListener("click", openSettings);
  document
    .getElementById("settings-close")
    .addEventListener("click", closeSettings);
  closeOnBackdropClick("settings-overlay", closeSettings);

  const applyOpmlImport = async (r) => {
    await Promise.all([loadFeeds(), loadArticles()]);
    renderAll();
    closeSettings();
    document.getElementById("opml-hint").textContent =
      `Imported ${r.imported} feed${r.imported === 1 ? "" : "s"}.`;
  };

  document
    .getElementById("btn-import")
    .addEventListener("click", async () => {
      if (window.nativeImportOPML) {
        const r = await window.nativeImportOPML();
        if (r.canceled) return;
        await applyOpmlImport(r);
        return;
      }
      document.getElementById("opml-file-input").click();
    });
  document
    .getElementById("opml-file-input")
    .addEventListener("change", async (e) => {
      const file = e.target.files[0];
      e.target.value = "";
      if (!file) return;
      const form = new FormData();
      form.append("opml", file);
      const r = await api("/api/opml/import", { method: "POST", body: form });
      await applyOpmlImport(r);
    });
  document.getElementById("btn-export").addEventListener("click", async () => {
    const r = await api("/api/opml/export", { method: "POST" });
    document.getElementById("opml-hint").textContent = `Exported to ${r.path}`;
  });
  document.getElementById("btn-save-cap").addEventListener("click", async () => {
    const articleCap = Number(document.getElementById("article-cap-input").value) || 0;
    await api("/api/settings", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ articleCap }),
    });
  });

  document
    .getElementById("btn-clear-cache")
    .addEventListener("click", async () => {
      await api("/api/articles/clear", { method: "POST" });
      await loadArticles();
      closeReader();
      renderAll();
      closeSettings();
    });

  document.getElementById("all-articles-row").addEventListener("click", () => {
    state.selectedFeedId = null;
    renderAll();
  });
  document.getElementById("feed-list").addEventListener("click", (e) => {
    const row = e.target.closest("[data-feed-id]");
    if (!row) return;
    const id = Number(row.dataset.feedId);
    const action = e.target.closest("[data-action]")?.dataset.action;
    if (action === "refresh") {
      e.stopPropagation();
      refreshFeed({ id });
      return;
    }
    if (action === "remove") {
      e.stopPropagation();
      removeFeed(id);
      return;
    }
    state.selectedFeedId = id;
    renderAll();
  });

  document
    .getElementById("mark-all-read-btn")
    .addEventListener("click", markAllRead);
  document.getElementById("article-list").addEventListener("click", (e) => {
    const row = e.target.closest("[data-article-id]");
    if (!row) return;
    const id = Number(row.dataset.articleId);
    const a = state.articles.find((x) => x.id === id);
    if (a) openArticle(a);
  });
  document
    .getElementById("reader-close")
    .addEventListener("click", closeReader);

  const addInput = document.getElementById("add-feed-input");
  const addBtn = document.getElementById("add-feed-btn");
  const submitAdd = async () => {
    const url = addInput.value.trim();
    if (!url) return;
    addBtn.disabled = true;
    try {
      await addFeed(url);
      addInput.value = "";
    } catch (err) {
      alert(`Could not add feed: ${err.message}`);
    } finally {
      addBtn.disabled = false;
    }
  };
  addBtn.addEventListener("click", submitAdd);
  addInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") submitAdd();
  });

  window.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      closeTopOverlay();
      return;
    }
    if (
      e.key === "/" &&
      !(e.target instanceof HTMLInputElement) &&
      !(e.target instanceof HTMLTextAreaElement)
    ) {
      e.preventDefault();
      openSearch();
    }
  });

  (async () => {
    await Promise.all([loadFeeds(), loadArticles()]);
    renderAll();
    refreshAll(); // auto-fetch on boot
  })();
}

document.addEventListener("DOMContentLoaded", init);
