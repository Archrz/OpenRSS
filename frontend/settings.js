import { $, api } from "./api.js";

export async function openSettings() {
  $("settings-overlay").showModal();
  const { feeds, articles } = await api("/api/stats");
  $("stat-feeds").textContent = feeds;
  $("stat-articles").textContent = articles;
  const { articleCap } = await api("/api/settings");
  $("article-cap-input").value = articleCap || "";
}

export const closeSettings = () => $("settings-overlay").close();
