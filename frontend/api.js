export const $ = (id) => document.getElementById(id);
export const on = (id, ev, fn) => $(id).addEventListener(ev, fn);

// state
export let selectedFeedId = null;
export let openArticleId = null;
export let openArticleLink = null;

export async function api(path, opts) {
  const res = await fetch(path, opts);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || res.statusText);
  return body;
}

async function fragment(path) {
  const res = await fetch(path);
  if (!res.ok) throw new Error(res.statusText);
  return res;
}

// lists
export async function reloadFeedList() {
  const res = await fragment(`/fragments/feeds?selected=${selectedFeedId || 0}`);
  $("feed-list").innerHTML = await res.text();
}

export async function reloadArticleList() {
  const res = await fragment(`/fragments/articles?feed=${selectedFeedId || 0}&selected=${openArticleId || 0}`);
  $("feed-title-label").textContent = res.headers.get("X-Feed-Title");
  $("feed-item-count").textContent = res.headers.get("X-Item-Count");
  $("article-list").innerHTML = await res.text();
}

export const reloadAll = () => Promise.all([reloadFeedList(), reloadArticleList()]);

export function selectFeed(id) {
  selectedFeedId = id;
  reloadFeedList();
  reloadArticleList();
}

// feeds
export async function refreshFeed(feedId) {
  document.querySelector(`[data-feed-id="${feedId}"] .refresh-btn`)?.classList.add("spin");
  try {
    await api(`/api/feeds/${feedId}/refresh`, { method: "POST" });
  } finally {
    await reloadAll();
  }
}

export async function refreshAll() {
  const btn = $("btn-refresh-all");
  btn.disabled = true;
  btn.classList.add("spin");
  try {
    await api("/api/refresh", { method: "POST" });
  } finally {
    btn.disabled = false;
    btn.classList.remove("spin");
    await reloadAll();
  }
}

export async function addFeed(url) {
  await api("/api/feeds", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  });
  await reloadAll();
}

export async function removeFeed(id) {
  await api(`/api/feeds/${id}`, { method: "DELETE" });
  if (selectedFeedId === id) selectedFeedId = null;
  await reloadAll();
}

export async function markAllRead() {
  await api("/api/articles/read-all", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ feedId: selectedFeedId || 0 }),
  });
  await reloadAll();
}

// reader
export async function openArticle(id) {
  const res = await fragment(`/fragments/article/${id}`);
  openArticleId = id;
  openArticleLink = res.headers.get("X-Article-Link");
  $("reader-feed-title").textContent = res.headers.get("X-Feed-Title");
  const content = $("reader-content");
  content.innerHTML = await res.text();
  content.scrollTop = 0;
  $("reader").classList.add("open");
  await reloadArticleList();
}

export function closeReader() {
  openArticleId = null;
  document.querySelector("#article-list .selected")?.classList.remove("selected");
  $("reader").classList.remove("open");
}
