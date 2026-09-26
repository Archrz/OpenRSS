import { $, openArticle } from "./api.js";

let searchQuery = "";
let searchDebounce;

export function onSearchInput(value) {
    searchQuery = value;
    clearTimeout(searchDebounce);
    searchDebounce = setTimeout(runSearch, 120);
}

export async function runSearch() {
    const params = new URLSearchParams({ q: searchQuery });
    const from = $("search-from").value;
    const to = $("search-to").value;
    if (from) params.set("from", from);
    if (to) params.set("to", to);

    const res = await fetch(`/fragments/search?${params}`);
    $("search-count").textContent = searchQuery
        ? `${res.headers.get("X-Result-Count")} results`
        : "";
    $("search-dynamic").innerHTML = await res.text();
}

const resultRows = () => [
    ...document.querySelectorAll("#search-results [data-idx]"),
];

export function selectRow(idx) {
    for (const row of resultRows())
        row.classList.toggle("selected", Number(row.dataset.idx) === idx);
}

export function openSelectedResult() {
    const row = document.querySelector("#search-results .selected");
    if (row) openArticle(Number(row.dataset.articleId));
}

export function moveSelection(delta) {
    const rows = resultRows();
    if (!rows.length) return;
    const current = rows.findIndex((r) => r.classList.contains("selected"));
    selectRow(Math.min(Math.max(current + delta, 0), rows.length - 1));
}

export function openSearch() {
    $("search-overlay").showModal();
    $("search-input").focus();
}
export const closeSearch = () => $("search-overlay").close();
