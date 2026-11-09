(() => {
  "use strict";

  const STORAGE_KEY = "c64catalog.state.v1";
  const FAVORITES_KEY = "c64catalog.favorites.v1";
  const CATALOG_URL = "data/catalog.json";
  const PLACEHOLDER_URL = "static/placeholder.svg";
  const ARCHIVE_PREFIX = "https://archive.org/";
  const LAUNCHBOX_GAME_URL = "https://gamesdb.launchbox-app.com/games/details/";
  const HASH_PREFIX = "#game=";
  // Release types from LaunchBox that are not worth a badge on the tile.
  const PLAIN_SOURCES = new Set(["Released", "Unknown"]);
  const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });
  const SORTS = ["name", "year", "top", "rating", "popular", "genre"];
  const DESCENDING_SORTS = new Set(["year", "top", "rating", "popular"]);
  const RATING_SORTS = new Set(["top", "rating", "popular"]);
  const PAGE_SIZE = 200;

  const el = {
    search: document.getElementById("search"),
    chips: {
      media: document.getElementById("media-chips"),
      genres: document.getElementById("genre-chips"),
    },
    yearMin: document.getElementById("year-min"),
    yearMax: document.getElementById("year-max"),
    rating: document.getElementById("rating-min"),
    ratingOutput: document.getElementById("rating-output"),
    sort: document.getElementById("sort"),
    sortDirection: document.getElementById("sort-direction"),
    favoritesToggle: document.getElementById("favorites-toggle"),
    favoritesCount: document.getElementById("favorites-count"),
    export: document.getElementById("export"),
    import: document.getElementById("import"),
    importFile: document.getElementById("import-file"),
    clearFilters: document.getElementById("clear-filters"),
    emptyClear: document.getElementById("empty-clear"),
    status: document.getElementById("status"),
    grid: document.getElementById("grid"),
    empty: document.getElementById("empty"),
    pager: document.getElementById("pager"),
    prevPage: document.getElementById("prev-page"),
    nextPage: document.getElementById("next-page"),
    pageSelect: document.getElementById("page-select"),
    pageTotal: document.getElementById("page-total"),
    panel: document.getElementById("panel"),
    panelBody: document.querySelector(".panel-body"),
    backdrop: document.getElementById("backdrop"),
    closePanel: document.getElementById("close-panel"),
    prevGame: document.getElementById("prev-game"),
    nextGame: document.getElementById("next-game"),
    detailBox: document.getElementById("detail-box"),
    screenshots: document.getElementById("detail-screenshots"),
    detailShots: document.getElementById("detail-shots"),
    detailTitle: document.getElementById("detail-title"),
    detailSub: document.getElementById("detail-sub"),
    detailAka: document.getElementById("detail-aka"),
    detailRating: document.getElementById("detail-rating"),
    detailFacts: document.getElementById("detail-facts"),
    detailTags: document.getElementById("detail-tags"),
    detailOverview: document.getElementById("detail-overview"),
    detailFavorite: document.getElementById("detail-favorite"),
    downloads: document.getElementById("detail-downloads"),
    downloadLinks: document.getElementById("download-links"),
    downloadNote: document.getElementById("download-note"),
    versions: document.getElementById("detail-versions"),
    versionsSummary: document.getElementById("versions-summary"),
    versionList: document.getElementById("version-list"),
    moreSection: document.getElementById("detail-links"),
    moreLinks: document.getElementById("more-links"),
    lightbox: document.getElementById("lightbox"),
    lightboxImage: document.getElementById("lightbox-image"),
    lightboxClose: document.getElementById("lightbox-close"),
    toast: document.getElementById("toast"),
  };

  // Each chip row maps a facet list in catalog.json to a filter set in state.
  const FACETS = [
    { key: "genres" },
    { key: "media" },
  ];

  let games = [];
  let byId = new Map();
  let favorites = new Set();
  let facets = null;
  let visible = [];
  let selectedId = null;
  let selectedShots = [];
  let lightboxTrigger = null;
  let detailCache = new Map();
  let tiles = new Map();
  let highlightedTile = null;
  let currentPage = 1;
  let renderQueued = false;
  let toastTimer = 0;

  const state = {
    query: "",
    genres: new Set(),
    media: new Set(),
    // The year range is resolved from the catalog facets in applySavedState().
    yearMin: null,
    yearMax: null,
    minRating: 0,
    favoritesOnly: false,
    sort: "top",
    dir: -1,
  };

  const saved = readSavedState();

  function clamp(value, min, max) {
    return Math.min(max, Math.max(min, value));
  }

  function readSavedState() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      return raw ? JSON.parse(raw) : null;
    } catch {
      return null;
    }
  }

  function saveState() {
    try {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          query: state.query,
          genres: [...state.genres],
          media: [...state.media],
          yearMin: state.yearMin,
          yearMax: state.yearMax,
          minRating: state.minRating,
          favoritesOnly: state.favoritesOnly,
          sort: state.sort,
          dir: state.dir,
        }),
      );
    } catch {
      // Storage can be unavailable in private mode; the app still works.
    }
  }

  function loadFavorites() {
    let ids = [];
    try {
      const raw = localStorage.getItem(FAVORITES_KEY);
      if (raw) {
        const parsed = JSON.parse(raw);
        if (Array.isArray(parsed)) {
          ids = parsed;
        } else if (parsed && Array.isArray(parsed.favorites)) {
          ids = parsed.favorites;
        }
      }
    } catch {
      // Ignore malformed or unavailable storage.
    }

    const next = new Set();
    for (const id of ids) {
      if (typeof id === "string" && byId.has(id)) next.add(id);
    }
    favorites = next;
  }

  function persistFavorites() {
    try {
      localStorage.setItem(
        FAVORITES_KEY,
        JSON.stringify({
          version: 1,
          updated_at: new Date().toISOString(),
          favorites: [...favorites],
        }),
      );
    } catch {
      // Storage can be unavailable in private mode; the app still works.
    }
  }

  function applySavedState() {
    const savedState = saved && typeof saved === "object" ? saved : {};

    if (typeof savedState.query === "string") {
      state.query = savedState.query;
    }
    for (const facet of FACETS) {
      if (!Array.isArray(savedState[facet.key])) continue;
      const known = new Set(facets[facet.key].map((entry) => entry.name));
      state[facet.key] = new Set(savedState[facet.key].filter((name) => known.has(name)));
    }

    const yearMin = Number.isFinite(savedState.yearMin) ? savedState.yearMin : facets.year_min;
    const yearMax = Number.isFinite(savedState.yearMax) ? savedState.yearMax : facets.year_max;
    state.yearMin = clamp(Math.trunc(yearMin), facets.year_min, facets.year_max);
    state.yearMax = clamp(Math.trunc(yearMax), facets.year_min, facets.year_max);
    if (state.yearMin > state.yearMax) {
      [state.yearMin, state.yearMax] = [state.yearMax, state.yearMin];
    }

    if (Number.isFinite(savedState.minRating)) {
      state.minRating = clamp(savedState.minRating, 0, 5);
    }
    state.favoritesOnly = Boolean(savedState.favoritesOnly);

    if (SORTS.includes(savedState.sort)) {
      state.sort = savedState.sort;
    }
    if (savedState.dir === 1 || savedState.dir === -1) {
      state.dir = savedState.dir;
    }
  }

  function buildControls(data) {
    for (const facet of FACETS) {
      const container = el.chips[facet.key];
      container.replaceChildren();
      container.parentElement.hidden = data[facet.key].length === 0;
      for (const entry of data[facet.key]) {
        const chip = document.createElement("button");
        chip.type = "button";
        chip.className = "chip";
        chip.dataset.value = entry.name;
        chip.setAttribute("aria-pressed", "false");

        const name = document.createElement("span");
        name.textContent = entry.name;

        const count = document.createElement("span");
        count.className = "chip-count";
        count.textContent = String(entry.count);

        chip.append(name, count);
        container.append(chip);
      }
    }

    for (const select of [el.yearMin, el.yearMax]) {
      select.replaceChildren();
      for (let year = data.year_min; year <= data.year_max; year += 1) {
        const option = document.createElement("option");
        option.value = String(year);
        option.textContent = String(year);
        select.append(option);
      }
    }
  }

  function syncControls() {
    el.search.value = state.query;

    for (const facet of FACETS) {
      for (const chip of el.chips[facet.key].children) {
        const on = state[facet.key].has(chip.dataset.value);
        chip.classList.toggle("active", on);
        chip.setAttribute("aria-pressed", on ? "true" : "false");
      }
    }

    el.yearMin.value = String(state.yearMin);
    el.yearMax.value = String(state.yearMax);
    el.rating.value = String(state.minRating);
    el.ratingOutput.textContent = state.minRating > 0 ? `${state.minRating.toFixed(1)}+` : "Any";
    el.sort.value = state.sort;
    syncSortDirection();
    syncFavoriteUI();
  }

  function syncSortDirection() {
    el.sortDirection.textContent = state.dir === 1 ? "↑" : "↓";
    el.sortDirection.setAttribute("aria-label", state.dir === 1 ? "Sort ascending" : "Sort descending");
  }

  function syncFavoriteUI() {
    el.favoritesCount.textContent = String(favorites.size);
    el.favoritesToggle.classList.toggle("active", state.favoritesOnly);
    el.favoritesToggle.setAttribute("aria-pressed", state.favoritesOnly ? "true" : "false");
    updateFavoriteButton();
  }

  function bindEvents() {
    el.search.addEventListener("input", () => {
      currentPage = 1;
      state.query = el.search.value;
      scheduleRender();
    });

    for (const facet of FACETS) {
      el.chips[facet.key].addEventListener("click", (event) => {
        const chip = event.target.closest(".chip");
        if (!chip) return;
        const selected = state[facet.key];
        const value = chip.dataset.value;
        if (selected.has(value)) {
          selected.delete(value);
        } else {
          selected.add(value);
        }
        currentPage = 1;
        syncControls();
        render();
      });
    }

    el.yearMin.addEventListener("change", () => {
      state.yearMin = Number(el.yearMin.value);
      if (state.yearMin > state.yearMax) {
        state.yearMax = state.yearMin;
      }
      currentPage = 1;
      syncControls();
      render();
    });

    el.yearMax.addEventListener("change", () => {
      state.yearMax = Number(el.yearMax.value);
      if (state.yearMax < state.yearMin) {
        state.yearMin = state.yearMax;
      }
      currentPage = 1;
      syncControls();
      render();
    });

    el.rating.addEventListener("input", () => {
      currentPage = 1;
      state.minRating = Number(el.rating.value);
      el.ratingOutput.textContent = state.minRating > 0 ? `${state.minRating.toFixed(1)}+` : "Any";
      scheduleRender();
    });

    el.sort.addEventListener("change", () => {
      state.sort = el.sort.value;
      state.dir = DESCENDING_SORTS.has(state.sort) ? -1 : 1;
      currentPage = 1;
      syncControls();
      render();
    });

    el.sortDirection.addEventListener("click", () => {
      state.dir *= -1;
      currentPage = 1;
      syncSortDirection();
      render();
    });

    el.clearFilters.addEventListener("click", clearFilters);
    el.emptyClear.addEventListener("click", clearFilters);

    el.favoritesToggle.addEventListener("click", () => {
      state.favoritesOnly = !state.favoritesOnly;
      currentPage = 1;
      syncControls();
      render();
    });

    el.prevPage.addEventListener("click", () => goToPage(currentPage - 1));
    el.nextPage.addEventListener("click", () => goToPage(currentPage + 1));
    el.pageSelect.addEventListener("change", () => goToPage(Number(el.pageSelect.value)));

    el.export.addEventListener("click", exportFavorites);
    el.import.addEventListener("click", () => el.importFile.click());
    el.importFile.addEventListener("change", async () => {
      const file = el.importFile.files && el.importFile.files[0];
      el.importFile.value = "";
      if (file) await importFavorites(file);
    });

    el.grid.addEventListener("click", (event) => {
      const tile = event.target.closest(".tile");
      if (!tile) return;
      openPanel(tile.dataset.id);
    });

    // Image events do not bubble, so listen during the capture phase.
    el.grid.addEventListener(
      "load",
      (event) => {
        if (event.target instanceof HTMLImageElement) {
          event.target.classList.add("loaded");
        }
      },
      true,
    );
    el.grid.addEventListener(
      "error",
      (event) => {
        if (event.target instanceof HTMLImageElement) {
          fallbackImage(event.target);
        }
      },
      true,
    );

    el.closePanel.addEventListener("click", closePanel);
    el.backdrop.addEventListener("click", closePanel);
    el.prevGame.addEventListener("click", () => stepPanel(-1));
    el.nextGame.addEventListener("click", () => stepPanel(1));
    el.detailFavorite.addEventListener("click", () => {
      if (selectedId !== null) toggleFavorite(selectedId);
    });

    el.lightboxClose.addEventListener("click", closeLightbox);
    el.lightbox.addEventListener("click", (event) => {
      if (event.target === el.lightbox) closeLightbox();
    });
    window.addEventListener("resize", () => {
      if (!el.lightbox.hidden) sizeLightboxImage();
    });

    document.addEventListener("keydown", onKeyDown);
    window.addEventListener("hashchange", openFromHash);
    window.addEventListener("storage", (event) => {
      if (event.key !== FAVORITES_KEY && event.key !== null) return;
      loadFavorites();
      syncFavoriteUI();
      render();
    });
  }

  function clearFilters() {
    state.query = "";
    for (const facet of FACETS) state[facet.key].clear();
    state.yearMin = facets.year_min;
    state.yearMax = facets.year_max;
    state.minRating = 0;
    state.favoritesOnly = false;
    currentPage = 1;
    syncControls();
    render();
  }

  function scheduleRender() {
    if (renderQueued) return;
    renderQueued = true;
    requestAnimationFrame(() => {
      renderQueued = false;
      render();
    });
  }

  function matchesAny(selected, values) {
    if (selected.size === 0) return true;
    return (values || []).some((value) => selected.has(value));
  }

  function currentList() {
    const tokens = state.query.trim().toLowerCase().split(/\s+/).filter(Boolean);
    const minYear = state.yearMin;
    const maxYear = state.yearMax;
    const minRating = state.minRating;
    const onlyFavorites = state.favoritesOnly;
    // Games with no known year stay visible until the range is narrowed.
    const fullRange = minYear === facets.year_min && maxYear === facets.year_max;

    const list = games.filter((game) => {
      if (onlyFavorites && !favorites.has(game.id)) return false;
      if (!matchesAny(state.genres, game.genres)) return false;
      if (!matchesAny(state.media, game.media)) return false;
      if (game.year > 0 ? game.year < minYear || game.year > maxYear : !fullRange) return false;
      if (game.rating < minRating) return false;
      for (const token of tokens) {
        if (!game._search.includes(token)) return false;
      }
      return true;
    });

    let primary;
    switch (state.sort) {
      case "year":
        primary = (a, b) => a.year - b.year;
        break;
      case "top":
        primary = (a, b) => a.weighted_rating - b.weighted_rating;
        break;
      case "rating":
        primary = (a, b) => a.rating - b.rating;
        break;
      case "popular":
        primary = (a, b) => a.rating_count - b.rating_count;
        break;
      case "genre":
        primary = (a, b) => collator.compare(a.genres[0] || "", b.genres[0] || "");
        break;
      default:
        primary = (a, b) => collator.compare(a.name, b.name);
    }
    list.sort((a, b) => {
      // Unrated games stay at the end of rating-based sorts in either
      // direction; they are in the catalog, but they are not ranked.
      if (RATING_SORTS.has(state.sort)) {
        const aUnrated = !a.rating_count || a.rating <= 0;
        const bUnrated = !b.rating_count || b.rating <= 0;
        if (aUnrated !== bUnrated) return aUnrated ? 1 : -1;
      }
      const order = primary(a, b);
      if (order !== 0) return order * state.dir;
      return collator.compare(a.name, b.name);
    });
    return list;
  }

  function render() {
    if (!facets) return;

    visible = currentList();
    const totalPages = Math.max(1, Math.ceil(visible.length / PAGE_SIZE));
    currentPage = clamp(currentPage, 1, totalPages);
    const start = (currentPage - 1) * PAGE_SIZE;
    const pageItems = visible.slice(start, start + PAGE_SIZE);

    // Only keep DOM tiles for the page that is on screen. Reusing tiles within
    // a page keeps already-loaded artwork in place.
    const pageIds = new Set(pageItems.map((game) => game.id));
    for (const id of tiles.keys()) {
      if (!pageIds.has(id)) tiles.delete(id);
    }

    const fragment = document.createDocumentFragment();
    let selectedTile = null;

    for (const game of pageItems) {
      const tile = tileFor(game);
      tile.classList.toggle("favorite", favorites.has(game.id));
      const isSelected = game.id === selectedId;
      tile.classList.toggle("selected", isSelected);
      if (isSelected) selectedTile = tile;
      fragment.append(tile);
    }

    highlightedTile = selectedTile;
    el.grid.replaceChildren(fragment);
    el.grid.hidden = visible.length === 0;
    el.empty.hidden = visible.length !== 0;

    updatePager(totalPages);
    el.status.textContent = statusText();

    updatePanelNav();
    saveState();
  }

  function updatePager(totalPages) {
    el.pager.hidden = totalPages <= 1;
    el.prevPage.disabled = currentPage <= 1;
    el.nextPage.disabled = currentPage >= totalPages;
    el.pageTotal.textContent = String(totalPages);

    if (el.pageSelect.options.length !== totalPages) {
      const options = document.createDocumentFragment();
      for (let page = 1; page <= totalPages; page += 1) {
        const option = document.createElement("option");
        option.value = String(page);
        option.textContent = String(page);
        options.append(option);
      }
      el.pageSelect.replaceChildren(options);
    }
    el.pageSelect.value = String(currentPage);
  }

  function goToPage(page) {
    const totalPages = Math.max(1, Math.ceil(visible.length / PAGE_SIZE));
    const next = clamp(Math.trunc(page) || 1, 1, totalPages);
    if (next === currentPage) {
      el.pageSelect.value = String(currentPage);
      return;
    }
    currentPage = next;
    render();
    el.status.scrollIntoView({ block: "start" });
  }

  function sourceLabel(source) {
    return source && !PLAIN_SOURCES.has(source) ? source : "";
  }

  function yearLabel(game) {
    return game.year > 0 ? String(game.year) : "Year unknown";
  }

  function tileFor(game) {
    let tile = tiles.get(game.id);
    if (tile) return tile;

    tile = document.createElement("button");
    tile.type = "button";
    tile.className = "tile";
    tile.dataset.id = game.id;
    tile.setAttribute("aria-label", `${game.name}, ${yearLabel(game)}`);

    const art = document.createElement("div");
    art.className = "tile-art";

    const image = document.createElement("img");
    image.loading = "lazy";
    image.decoding = "async";
    image.width = 300;
    image.height = 400;
    setImage(image, game.box_art, "");

    const badge = document.createElement("span");
    badge.className = "favorite-badge";
    badge.textContent = "★";
    badge.setAttribute("aria-hidden", "true");

    art.append(image);
    if (game.source && !PLAIN_SOURCES.has(game.source)) {
      const sourceBadge = document.createElement("span");
      sourceBadge.className = "source-badge";
      sourceBadge.textContent = game.source;
      art.append(sourceBadge);
    }
    if (game.versions > 1) {
      const versionBadge = document.createElement("span");
      versionBadge.className = "version-badge";
      versionBadge.textContent = `${game.versions} versions`;
      art.append(versionBadge);
    }
    art.append(badge);

    const caption = document.createElement("div");
    caption.className = "tile-caption";

    const title = document.createElement("span");
    title.className = "tile-title";
    title.textContent = game.name;

    const year = document.createElement("span");
    year.className = "tile-year";
    year.textContent = `${yearLabel(game)} · ${game.genres[0] || "Uncategorized"}`;

    caption.append(title, year);
    tile.append(art, caption);
    tiles.set(game.id, tile);
    return tile;
  }

  function setImage(image, src, alt) {
    image.classList.remove("loaded", "missing");
    delete image.dataset.fallback;
    image.alt = alt;
    if (!src) {
      fallbackImage(image);
      return;
    }
    image.onload = () => image.classList.add("loaded");
    image.onerror = () => fallbackImage(image);
    image.src = src;
    if (image.complete && image.naturalWidth > 0) {
      image.classList.add("loaded");
    }
  }

  function fallbackImage(image) {
    if (image.dataset.fallback) return;
    image.dataset.fallback = "1";
    image.onerror = null;
    image.src = PLACEHOLDER_URL;
    image.classList.add("loaded", "missing");
  }

  function updatePanelNav() {
    const index = visible.findIndex((game) => game.id === selectedId);
    el.prevGame.disabled = index <= 0;
    el.nextGame.disabled = index < 0 || index >= visible.length - 1;
  }

  function markSelected() {
    const next = selectedId === null ? null : tiles.get(selectedId) || null;
    if (highlightedTile && highlightedTile !== next) {
      highlightedTile.classList.remove("selected");
    }
    highlightedTile = next;
    if (next) next.classList.add("selected");
  }

  function stepPanel(delta) {
    if (selectedId === null) return;
    const index = visible.findIndex((game) => game.id === selectedId);
    if (index < 0) return;
    const targetIndex = index + delta;
    const next = visible[targetIndex];
    if (!next) return;

    const targetPage = Math.floor(targetIndex / PAGE_SIZE) + 1;
    if (targetPage !== currentPage) {
      currentPage = targetPage;
      render();
    }
    openPanel(next.id, { focus: false });
  }

  function setHash(id) {
    const hash = id === null ? "" : `${HASH_PREFIX}${encodeURIComponent(id)}`;
    if (window.location.hash === hash || (hash === "" && window.location.hash === "")) return;
    history.replaceState(null, "", `${window.location.pathname}${window.location.search}${hash}`);
  }

  function openFromHash() {
    const hash = window.location.hash;
    if (!hash.startsWith(HASH_PREFIX)) {
      if (selectedId !== null) closePanel();
      return;
    }
    let id;
    try {
      id = decodeURIComponent(hash.slice(HASH_PREFIX.length));
    } catch {
      return;
    }
    if (id !== selectedId && byId.has(id)) {
      const index = visible.findIndex((game) => game.id === id);
      if (index >= 0) {
        const targetPage = Math.floor(index / PAGE_SIZE) + 1;
        if (targetPage !== currentPage) {
          currentPage = targetPage;
          render();
        }
      }
      openPanel(id, { focus: false });
    }
  }

  async function openPanel(id, options = {}) {
    const game = byId.get(id);
    if (!game) return;

    selectedId = id;
    setHash(id);
    document.body.classList.add("panel-open");
    el.panel.classList.add("open");
    el.panel.inert = false;
    el.panel.setAttribute("aria-hidden", "false");
    el.backdrop.hidden = false;

    markSelected();
    fillPanelBasic(game);
    updatePanelNav();

    if (options.scroll !== false || options.focus !== false) {
      requestAnimationFrame(() => {
        if (options.scroll !== false) {
          el.panelBody.scrollTop = 0;
          // The reserved panel column reflows the grid; keep the chosen tile
          // in view when it moved past the fold.
          const tile = tiles.get(id);
          if (tile && typeof tile.scrollIntoView === "function") {
            tile.scrollIntoView({ block: "nearest", inline: "nearest" });
          }
        }
        if (options.focus !== false) el.panel.focus({ preventScroll: true });
      });
    }

    try {
      const detail = await getDetail(id);
      if (selectedId !== id) return;
      fillPanelDetail(detail);
    } catch (error) {
      if (selectedId !== id) return;
      console.error(error);
      el.detailOverview.textContent = "Could not load the full details.";
      el.detailOverview.classList.add("muted");
      toast("Could not load game details.");
    }
  }

  function closePanel() {
    const previous = selectedId;
    selectedId = null;
    selectedShots = [];
    setHash(null);
    el.panel.classList.remove("open");
    el.panel.inert = true;
    el.panel.setAttribute("aria-hidden", "true");
    el.backdrop.hidden = true;
    document.body.classList.remove("panel-open");
    markSelected();

    const tile = previous !== null ? tiles.get(previous) : null;
    if (tile) {
      tile.focus({ preventScroll: true });
    } else {
      el.search.focus({ preventScroll: true });
    }
  }

  function playersLabel(maxPlayers, cooperative) {
    if (!maxPlayers || maxPlayers < 1) return "";
    if (maxPlayers === 1) return "1 player";
    return `Up to ${maxPlayers} players${cooperative ? " (co-op)" : ""}`;
  }

  function appendFact(label, value) {
    if (!value) return;
    const term = document.createElement("dt");
    term.textContent = label;
    const detail = document.createElement("dd");
    detail.textContent = value;
    el.detailFacts.append(term, detail);
  }

  function appendFactLink(label, text, href) {
    if (!text) return;
    if (!href) {
      appendFact(label, text);
      return;
    }
    const term = document.createElement("dt");
    term.textContent = label;
    const detail = document.createElement("dd");
    const link = document.createElement("a");
    link.href = href;
    link.rel = "noopener noreferrer";
    link.target = "_blank";
    link.textContent = text;
    detail.append(link);
    el.detailFacts.append(term, detail);
  }

  function stars(rating, scale) {
    const filled = clamp(Math.round((rating / (scale || 5)) * 5), 0, 5);
    return "★".repeat(filled) + "☆".repeat(5 - filled);
  }

  function renderRating(rating, ratingCount, source) {
    el.detailRating.hidden = false;
    el.detailRating.replaceChildren();

    if (!ratingCount || rating <= 0) {
      const none = document.createElement("span");
      none.className = "muted";
      none.textContent = "No ratings";
      el.detailRating.append(none);
      return;
    }

    const starSpan = document.createElement("span");
    starSpan.className = "stars";
    starSpan.textContent = stars(rating, 5);

    const value = document.createElement("span");
    value.textContent = `${rating.toFixed(2)} / 5`;

    const count = document.createElement("span");
    count.className = "muted";
    const label = source;
    const votes = `${ratingCount.toLocaleString()} rating${ratingCount === 1 ? "" : "s"}`;
    count.textContent = label ? `${votes} · ${label}` : votes;

    el.detailRating.append(starSpan, value, count);
  }

  function fillTags(game) {
    el.detailTags.replaceChildren();
    const seen = new Set();
    const add = (text) => {
      if (!text || seen.has(text)) return;
      seen.add(text);
      const span = document.createElement("span");
      span.className = "tag";
      span.textContent = text;
      el.detailTags.append(span);
    };

    for (const genre of game.genres || []) add(genre);
    for (const tag of game.tags || []) {
      // "Shooter / Scrolling shoot'em up" adds the sub-genre as its own tag.
      const [, sub] = tag.split(" / ");
      if (sub) add(sub);
    }
  }

  function fillPanelBasic(game) {
    el.detailTitle.textContent = game.name;
    el.detailSub.textContent = [yearLabel(game), sourceLabel(game.source)].filter(Boolean).join(" · ");
    el.detailAka.hidden = true;
    renderRating(game.rating, game.rating_count, "");

    el.detailFacts.replaceChildren();
    appendFact("Developer", game.developer);
    appendFact("Publisher", game.publisher);
    appendFact("Media", (game.media || []).join(", "));
    appendFact("Players", playersLabel(game.max_players));

    fillTags(game);
    el.detailOverview.textContent = "Loading details…";
    el.detailOverview.classList.add("muted");

    el.downloads.hidden = true;
    el.downloadLinks.replaceChildren();
    el.versions.hidden = true;
    el.versionList.replaceChildren();
    el.moreSection.hidden = true;
    el.moreLinks.replaceChildren();

    setImage(el.detailBox, game.box_art, `${game.name} box art`);
    renderScreenshots(null);
    updateFavoriteButton();
  }

  function fillPanelDetail(game) {
    el.detailTitle.textContent = game.name;
    el.detailSub.textContent = [yearLabel({ year: game.release_year }), sourceLabel(game.source)].filter(Boolean).join(" · ");
    const aka = (game.alt_names || []).slice(0, 4);
    el.detailAka.textContent = aka.length > 0 ? `Also known as ${aka.join(" · ")}` : "";
    el.detailAka.hidden = aka.length === 0;
    renderRating(game.rating, game.rating_count, "LaunchBox");

    el.detailFacts.replaceChildren();
    appendFact("Developer", game.developer);
    appendFact("Publisher", game.publisher);
    appendFact("Media", (game.media || []).join(", "));
    appendFact("Players", playersLabel(game.max_players, game.cooperative));
    appendFact("Age rating", game.age_rating);

    fillTags(game);
    renderDownloads(game);
    renderVersions(game);
    renderMoreLinks(game);
    setImage(el.detailBox, game.box_art, `${game.name} box art`);
    renderScreenshots(game);

    el.detailOverview.textContent = game.overview || "No overview available.";
    el.detailOverview.classList.toggle("muted", !game.overview);
    updateFavoriteButton();
  }

  function archiveLink(url) {
    return typeof url === "string" && url.startsWith(ARCHIVE_PREFIX) ? url : "";
  }

  function webLink(url) {
    if (typeof url !== "string") return "";
    try {
      const parsed = new URL(url);
      return parsed.protocol === "https:" || parsed.protocol === "http:" ? parsed.href : "";
    } catch {
      return "";
    }
  }

  function formatBytes(bytes) {
    if (!Number.isFinite(bytes) || bytes <= 0) return "";
    const units = ["B", "KB", "MB", "GB"];
    let value = bytes;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
      value /= 1024;
      unit += 1;
    }
    const digits = unit === 0 || value >= 100 ? 0 : 1;
    return `${value.toFixed(digits)} ${units[unit]}`;
  }

  function linkButton(label, href, options = {}) {
    const link = document.createElement("a");
    link.className = `button download-button${options.primary ? " primary" : ""}`;
    link.href = href;
    link.rel = "noopener noreferrer";
    if (options.newTab) link.target = "_blank";

    const text = document.createElement("span");
    text.textContent = label;
    link.append(text);

    if (options.size) {
      const size = document.createElement("span");
      size.className = "download-size";
      size.textContent = options.size;
      link.append(size);
    }
    return link;
  }

  // fileLabel shortens a disk image name to the part that tells the disks
  // apart: "Ultima_V_..._Disk_1_of_4_Side_A.d64" becomes "Disk 1 of 4 Side A".
  function fileLabel(name, all) {
    const ext = name.includes(".") ? name.slice(name.lastIndexOf(".") + 1).toLowerCase() : "";
    if (all.length > 1) {
      const match = name.match(/(?:Disk|Disc|Tape|Side|Part)_.*?(?=\.[^.]+$)/i);
      if (match) return `${match[0].replace(/_/g, " ")} (.${ext})`;
    }
    return all.length > 1 ? name : `Download (.${ext})`;
  }

  function versionDetails(version) {
    const parts = [];
    if (version.publisher) parts.push(version.publisher);
    if (version.year) parts.push(String(version.year));
    if (version.language) parts.push(version.language);
    if (version.video) parts.push(version.video);
    parts.push(version.media);
    for (const note of version.notes || []) parts.push(note);
    return parts.filter(Boolean).join(" · ");
  }

  function versionFileLinks(container, version, primary) {
    const files = (version.files || []).filter((file) => archiveLink(file.url));
    files.forEach((file, index) => {
      container.append(
        linkButton(fileLabel(file.name, files), archiveLink(file.url), {
          primary: primary && index === 0 && !archiveLink(version.play),
          size: formatBytes(file.size),
        }),
      );
    });
  }

  function renderDownloads(game) {
    el.downloadLinks.replaceChildren();
    const version = (game.versions || [])[0];
    if (!version) {
      el.downloads.hidden = true;
      return;
    }

    const playHref = archiveLink(version.play);
    if (playHref) {
      el.downloadLinks.append(linkButton("Play on archive.org", playHref, { primary: true, newTab: true }));
    }
    versionFileLinks(el.downloadLinks, version, true);

    const notes = [`Version: ${version.label}.`];
    notes.push("Disk, tape, and cartridge images run in VICE or another C64 emulator; archive.org runs them in the browser.");
    notes.push("Files are hosted by the Internet Archive.");
    el.downloadNote.textContent = notes.join(" ");
    el.downloads.hidden = false;
  }

  function renderVersions(game) {
    el.versionList.replaceChildren();
    const others = (game.versions || []).slice(1);
    const extra = Number.isInteger(game.extra_versions) ? game.extra_versions : 0;
    el.versions.hidden = others.length === 0;
    if (others.length === 0) return;
    el.versionsSummary.textContent = `Other versions (${others.length + extra})`;

    for (const version of others) {
      const item = document.createElement("li");
      item.className = "version";

      const label = document.createElement("p");
      label.className = "version-label";
      label.textContent = version.label;

      const details = document.createElement("p");
      details.className = "version-details muted";
      details.textContent = versionDetails(version);

      const links = document.createElement("div");
      links.className = "download-links compact";
      const playHref = archiveLink(version.play);
      if (playHref) links.append(linkButton("Play", playHref, { newTab: true }));
      versionFileLinks(links, version, false);

      item.append(label, details, links);
      el.versionList.append(item);
    }

    if (extra > 0) {
      const more = document.createElement("li");
      more.className = "version version-more muted";
      more.textContent = `${extra} more version${extra === 1 ? "" : "s"} not listed. The ones above rank best by language, release type, and popularity.`;
      el.versionList.append(more);
    }
  }

  function renderMoreLinks(game) {
    el.moreLinks.replaceChildren();
    const wikipedia = webLink(game.wikipedia);
    if (wikipedia) el.moreLinks.append(linkButton("Wikipedia", wikipedia, { newTab: true }));
    const video = webLink(game.video);
    if (video) el.moreLinks.append(linkButton("Video", video, { newTab: true }));
    if (Number.isInteger(game.launchbox_id) && game.launchbox_id > 0) {
      el.moreLinks.append(linkButton("LaunchBox", `${LAUNCHBOX_GAME_URL}${game.launchbox_id}`, { newTab: true }));
    }
    el.moreSection.hidden = el.moreLinks.children.length === 0;
  }

  function renderScreenshots(game) {
    selectedShots = (game && game.screenshots) || [];
    el.detailShots.replaceChildren();
    el.screenshots.hidden = selectedShots.length === 0;
    if (selectedShots.length === 0) return;

    selectedShots.forEach((_, index) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "screenshot-button";
      button.setAttribute("aria-label", `Enlarge screenshot ${index + 1} of ${selectedShots.length}`);

      const image = document.createElement("img");
      image.loading = "lazy";
      image.decoding = "async";
      setImage(image, selectedShots[index], `${game.name} screenshot ${index + 1}`);

      button.append(image);
      button.addEventListener("click", () => openLightbox(index, button));
      el.detailShots.append(button);
    });
  }

  function updateFavoriteButton() {
    if (selectedId === null) return;
    const on = favorites.has(selectedId);
    el.detailFavorite.classList.toggle("active", on);
    el.detailFavorite.setAttribute("aria-pressed", on ? "true" : "false");
    el.detailFavorite.textContent = on ? "★ Favorited" : "☆ Add to favorites";
  }

  async function getDetail(id) {
    if (detailCache.has(id)) return detailCache.get(id);
    const response = await fetch(`data/games/${encodeURIComponent(id)}.json`);
    if (!response.ok) {
      throw new Error(`game request failed (${response.status})`);
    }
    const detail = await response.json();
    detailCache.set(id, detail);
    return detail;
  }

  function toggleFavorite(id) {
    const wasFavorite = favorites.has(id);
    applyFavorite(id, !wasFavorite);
    persistFavorites();

    if (state.favoritesOnly && wasFavorite) {
      render();
    } else {
      el.status.textContent = statusText();
    }
  }

  function exportFavorites() {
    if (favorites.size === 0) {
      toast("No favorites to export yet.");
      return;
    }

    const payload = {
      version: 1,
      updated_at: new Date().toISOString(),
      favorites: [...favorites].sort(),
    };
    const blob = new Blob([`${JSON.stringify(payload, null, 2)}\n`], {
      type: "application/json",
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    const date = new Date().toISOString().slice(0, 10);
    link.href = url;
    link.download = `c64-catalog-favorites-${date}.json`;
    document.body.append(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  async function importFavorites(file) {
    let parsed;
    try {
      parsed = JSON.parse(await file.text());
    } catch {
      toast("Could not read that favorites file.");
      return;
    }

    let ids;
    if (Array.isArray(parsed)) {
      ids = parsed;
    } else if (parsed && Array.isArray(parsed.favorites)) {
      ids = parsed.favorites;
    } else {
      toast("That file does not contain favorites.");
      return;
    }

    let added = 0;
    let skipped = 0;
    for (const id of ids) {
      if (typeof id !== "string" || !byId.has(id)) {
        skipped++;
        continue;
      }
      if (!favorites.has(id)) {
        favorites.add(id);
        added++;
      }
    }

    persistFavorites();
    currentPage = 1;
    syncControls();
    render();
    if (added === 0 && skipped === 0) {
      toast("No new favorites to import.");
      return;
    }
    const skippedText = skipped > 0 ? `; skipped ${skipped}` : "";
    toast(`Imported ${added} favorite${added === 1 ? "" : "s"}${skippedText}.`);
  }

  function applyFavorite(id, on) {
    if (on) {
      favorites.add(id);
    } else {
      favorites.delete(id);
    }
    const tile = tiles.get(id);
    if (tile) tile.classList.toggle("favorite", on);
    syncFavoriteUI();
  }

  function statusText() {
    const start = visible.length > 0 ? (currentPage - 1) * PAGE_SIZE : 0;
    const end = Math.min(start + PAGE_SIZE, visible.length);
    let shown;
    if (visible.length > 0) {
      shown = `Showing ${(start + 1).toLocaleString()}–${end.toLocaleString()} of ${visible.length.toLocaleString()}`;
      if (visible.length !== games.length) {
        shown += ` (${games.length.toLocaleString()} total)`;
      }
    } else {
      shown = `Showing 0 of ${games.length.toLocaleString()}`;
    }
    const parts = [shown];
    if (favorites.size > 0) {
      parts.push(`${favorites.size} favorite${favorites.size === 1 ? "" : "s"}`);
    }
    if (state.sort === "top") {
      parts.push("weighted by ratings");
    }
    return parts.join(" · ");
  }

  function openLightbox(index, trigger) {
    if (selectedId === null || index < 0 || index >= selectedShots.length) return;
    const game = byId.get(selectedId);
    lightboxTrigger = trigger || null;
    delete el.lightboxImage.dataset.fallback;
    el.lightboxImage.style.removeProperty("width");
    el.lightboxImage.style.removeProperty("height");
    el.lightboxImage.onload = sizeLightboxImage;
    el.lightboxImage.onerror = () => fallbackImage(el.lightboxImage);
    el.lightboxImage.src = selectedShots[index];
    el.lightboxImage.alt = `${game ? game.name : "Game"} screenshot ${index + 1}`;
    el.lightbox.hidden = false;
    if (el.lightboxImage.complete) sizeLightboxImage();
    el.lightboxClose.focus();
  }

  // Scale the image up or down to fit within 80% of the viewport. Setting the
  // exact size keeps the shadow and rounded corners around the image itself
  // rather than around a large letterboxed box.
  function sizeLightboxImage() {
    const image = el.lightboxImage;
    if (!image.naturalWidth || !image.naturalHeight) return;
    const scale = Math.min(
      (window.innerWidth * 0.8) / image.naturalWidth,
      (window.innerHeight * 0.8) / image.naturalHeight,
    );
    image.style.width = `${Math.round(image.naturalWidth * scale)}px`;
    image.style.height = `${Math.round(image.naturalHeight * scale)}px`;
  }

  function closeLightbox() {
    el.lightbox.hidden = true;
    el.lightboxImage.removeAttribute("src");
    el.lightboxImage.style.removeProperty("width");
    el.lightboxImage.style.removeProperty("height");
    if (lightboxTrigger && document.contains(lightboxTrigger)) {
      lightboxTrigger.focus();
    } else if (selectedId !== null) {
      el.panel.focus({ preventScroll: true });
    }
    lightboxTrigger = null;
  }

  function onKeyDown(event) {
    if (event.key === "Escape") {
      if (!el.lightbox.hidden) {
        closeLightbox();
        return;
      }
      if (selectedId !== null) {
        closePanel();
        return;
      }
      if (isTyping(event.target)) {
        event.target.blur();
      }
      return;
    }

    if (!el.lightbox.hidden) return;
    if (isTyping(event.target)) return;
    if (event.ctrlKey || event.metaKey || event.altKey) return;

    if (event.key === "/") {
      event.preventDefault();
      el.search.focus();
      return;
    }

    if (event.key === "f" || event.key === "F") {
      state.favoritesOnly = !state.favoritesOnly;
      currentPage = 1;
      syncControls();
      render();
      return;
    }

    if (selectedId !== null) {
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        stepPanel(-1);
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        stepPanel(1);
      }
    }
  }

  function isTyping(target) {
    return (
      target instanceof HTMLInputElement ||
      target instanceof HTMLSelectElement ||
      target instanceof HTMLTextAreaElement
    );
  }

  function toast(message) {
    el.toast.textContent = message;
    el.toast.hidden = false;
    window.clearTimeout(toastTimer);
    toastTimer = window.setTimeout(() => {
      el.toast.hidden = true;
    }, 2600);
  }

  async function boot() {
    try {
      const response = await fetch(CATALOG_URL, { headers: { Accept: "application/json" } });
      if (!response.ok) {
        throw new Error(`catalog request failed (${response.status})`);
      }
      const data = await response.json();

      games = data.games;
      byId = new Map(games.map((game) => [game.id, game]));
      loadFavorites();
      facets = data;

      for (const game of games) {
        game._search = [
          game.name,
          ...(game.aka || []),
          game.publisher,
          game.developer,
          ...(game.genres || []),
          ...(game.media || []),
          game.source,
        ]
          .join(" ")
          .toLowerCase();
      }

      buildControls(data);
      applySavedState();
      syncControls();
      bindEvents();
      render();
      openFromHash();
    } catch (error) {
      console.error(error);
      el.status.textContent = `Could not load the catalog: ${error.message}`;
      el.status.classList.add("error");
    }
  }

  boot();
})();
