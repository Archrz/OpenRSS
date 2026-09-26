"use strict";

import {
    $,
    on,
    api,
    reloadAll,
    refreshFeed,
    refreshAll,
    addFeed,
    removeFeed,
    selectFeed,
    openArticle,
    openArticleId,
    openArticleLink,
    closeReader,
    markAllRead,
} from "./api.js";
import {
    onSearchInput,
    runSearch,
    selectRow,
    openSelectedResult,
    moveSelection,
    openSearch,
    closeSearch,
} from "./search.js";
import { openSettings, closeSettings } from "./settings.js";

const openExternal = (url) =>
    (window.openExternal ?? ((u) => window.open(u, "_blank")))(url);
const closeOnBackdropClick = (id, close) =>
    on(id, "click", (e) => {
        if (e.target.id === id) close();
    });

// dialogs close themselves on Escape; only the docked reader panel needs help
function closeTopOverlay() {
    if (document.querySelector("dialog[open]")) return true;
    if (openArticleId) {
        closeReader();
        return true;
    }
    return false;
}

document.addEventListener("DOMContentLoaded", () => {
    // external links open outside the webview
    document.addEventListener("click", (e) => {
        const a = e.target.closest("a[href]");
        if (a && !a.href.startsWith(window.location.origin)) {
            e.preventDefault();
            openExternal(a.href);
        }
    });

    // search
    on("btn-search", "click", openSearch);
    closeOnBackdropClick("search-overlay", closeSearch);
    on("search-input", "input", (e) => onSearchInput(e.target.value));
    on("search-from", "change", runSearch);
    on("search-to", "change", runSearch);
    on("search-input", "keydown", (e) => {
        if (e.key === "Escape") return closeSearch();
        if (e.key === "ArrowDown") {
            e.preventDefault();
            moveSelection(1);
        }
        if (e.key === "ArrowUp") {
            e.preventDefault();
            moveSelection(-1);
        }
        if (e.key === "Enter") {
            closeSearch();
            openSelectedResult();
        }
    });
    on("search-overlay", "mouseover", (e) => {
        const row = e.target.closest("[data-idx]");
        if (row) selectRow(Number(row.dataset.idx));
    });
    on("search-overlay", "click", (e) => {
        const row = e.target.closest("[data-article-id]");
        if (!row) return;
        closeSearch();
        openArticle(Number(row.dataset.articleId));
    });

    // settings
    on("btn-settings", "click", openSettings);
    on("settings-close", "click", closeSettings);
    closeOnBackdropClick("settings-overlay", closeSettings);
    on("settings-overlay", "close", () => {
        $("opml-hint").textContent = "Import/export OPML 2.0";
    });

    // feeds
    on("btn-refresh-all", "click", refreshAll);
    on("btn-refresh-all-2", "click", () => {
        refreshAll();
        closeSettings();
    });
    on("feed-list", "click", (e) => {
        const row = e.target.closest("[data-feed-id]");
        if (!row) return;
        const id = Number(row.dataset.feedId);
        const action = e.target.closest("[data-action]")?.dataset.action;
        if (action === "refresh") {
            e.stopPropagation();
            return refreshFeed(id);
        }
        if (action === "remove") {
            e.stopPropagation();
            return removeFeed(id);
        }
        selectFeed(id);
    });

    const addInput = $("add-feed-input");
    const addBtn = $("add-feed-btn");
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

    // articles + reader
    on("mark-all-read-btn", "click", markAllRead);
    on("article-list", "click", (e) => {
        const row = e.target.closest("[data-article-id]");
        if (row) openArticle(Number(row.dataset.articleId));
    });
    on("reader-close", "click", closeReader);
    on("reader-open-original", "click", () => {
        if (openArticleLink) openExternal(openArticleLink);
    });

    // opml + storage
    const applyOpmlImport = async (r) => {
        await reloadAll();
        closeSettings();
        $("opml-hint").textContent =
            `Imported ${r.imported} feed${r.imported === 1 ? "" : "s"}.`;
    };

    on("btn-import", "click", async () => {
        if (window.nativeImportOPML) {
            const r = await window.nativeImportOPML();
            if (!r.canceled) await applyOpmlImport(r);
            return;
        }
        $("opml-file-input").click();
    });

    on("opml-file-input", "change", async (e) => {
        const file = e.target.files[0];
        e.target.value = "";
        if (!file) return;
        const form = new FormData();
        form.append("opml", file);
        await applyOpmlImport(
            await api("/api/opml/import", { method: "POST", body: form }),
        );
    });

    on("btn-export", "click", async () => {
        const r = await api("/api/opml/export", { method: "POST" });
        $("opml-hint").textContent = `Exported to ${r.path}`;
    });

    on("btn-save-cap", "click", async () => {
        const articleCap = Number($("article-cap-input").value) || 0;
        await api("/api/settings", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ articleCap }),
        });
    });

    on("btn-clear-cache", "click", async () => {
        await api("/api/articles/clear", { method: "POST" });
        closeReader();
        await reloadAll();
        closeSettings();
    });

    // keys
    window.addEventListener("keydown", (e) => {
        if (e.key === "Escape") return closeTopOverlay();
        if (
            e.key === "/" &&
            !(e.target instanceof HTMLInputElement) &&
            !(e.target instanceof HTMLTextAreaElement)
        ) {
            e.preventDefault();
            openSearch();
        }
    });

    refreshAll(); // auto-fetch on boot; initial list is already server-rendered
});
