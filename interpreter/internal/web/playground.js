// playground.js — drives the in-browser schema playground. It boots the Go WASM
// compiler (graph-playground.wasm, which exposes window.kesselCompile), maintains
// the editable in-memory set of schema files, and on each compile feeds the
// resulting Cytoscape elements to the shared rendering core (render.js). The
// whole Starlark -> graph.json -> elements pipeline runs client-side.
(function () {
  "use strict";

  // localStorage keys: the working file set and each panel's collapsed flag.
  var LS_FILES = "kessel.playground.files";
  var LS_SIDEBAR = "kessel.playground.sidebar";
  var LS_PANEL = "kessel.playground.panel";
  var LS_LAYOUT = "kessel.playground.layout";
  // Longest share-hash we will produce. Well under every modern browser's URL
  // limit and fine for Slack/GitHub/email; larger schemas fall back to Download.
  var MAX_HASH = 32000;

  // The pristine schema the build inlined — the baseline for "Reset" and for the
  // delta a share link encodes.
  var seed = JSON.parse(document.getElementById("schema-files").textContent);

  // Working copy of every schema file: { "path/to/file.star": "source" }. A share
  // link in the URL wins over locally saved edits, which win over the seed. A
  // share payload is decoded asynchronously after boot (see applyShare).
  var pendingShare = getShareParam();
  var files = pendingShare ? cloneFiles(seed) : (readSavedFiles() || cloneFiles(seed));
  var paths = Object.keys(files).sort();

  // Starter body for a newly created file: the prelude load plus a blank line.
  var STARTER_TEMPLATE = 'load("kessel.star", "resource", "uuid")\n\n';

  var statusEl = document.getElementById("status");
  var details = document.getElementById("details");
  var select = document.getElementById("file-select");
  var cy = null;
  var current = defaultFile(paths);
  var ready = false;
  var debounce = null;

  // Restore each panel's persisted collapsed state before the graph builds so it
  // fits the correct width on first render.
  if (readSidebarCollapsed()) document.body.classList.add("editor-collapsed");
  if (readPanelCollapsed()) document.body.classList.add("panel-collapsed");

  // Graph layout: dagre is inlined and always available; the others are fetched
  // on demand (see ensureLayoutLib). currentLayout is the persisted choice, used
  // for the first render and reflected in the selector.
  var loadedLayouts = { dagre: true };
  var currentLayout = readLayout();
  window.GRAPH_LAYOUT = currentLayout;

  // --- File switcher ---------------------------------------------------------

  rebuildSwitcher();

  var editor = CodeMirror.fromTextArea(document.getElementById("code"), {
    mode: "python", // Starlark is a Python dialect; python mode is a close fit.
    theme: "dracula",
    lineNumbers: true,
    indentUnit: 4,
    lineWrapping: false,
  });
  editor.setValue(files[current]);

  select.addEventListener("change", function () {
    saveBuffer();
    current = select.value;
    loadCurrent();
  });

  // --- File management (Phase 1) ---------------------------------------------

  document.getElementById("file-new").addEventListener("click", addFile);
  document.getElementById("file-rename").addEventListener("click", renameFile);
  document.getElementById("file-delete").addEventListener("click", deleteFile);

  // rebuildSwitcher rebuilds the <option> list from the current files, sorted, so
  // a new/renamed file lands in order and matches paths usage elsewhere.
  function rebuildSwitcher() {
    paths = Object.keys(files).sort();
    select.innerHTML = "";
    paths.forEach(function (p) {
      var opt = document.createElement("option");
      opt.value = p;
      opt.textContent = p;
      select.appendChild(opt);
    });
    select.value = current;
  }

  // loadCurrent loads the current file into the editor with a clean undo history.
  function loadCurrent() {
    editor.setValue(files[current]);
    editor.clearHistory();
  }

  // validatePath returns an error string for an invalid new file path, or null if
  // it is acceptable. The compiler keys files relative to the schema root, so
  // reject absolute paths and ".." segments (see readSchemaFiles / CompileGraph).
  function validatePath(p) {
    if (!p) return "Path is required.";
    if (!/\.star$/.test(p)) return "Path must end in .star";
    if (p.charAt(0) === "/") return "Path must be relative (no leading /).";
    if (p.split("/").indexOf("..") !== -1) return "Path may not contain '..' segments.";
    if (Object.prototype.hasOwnProperty.call(files, p)) return "A file with that path already exists.";
    return null;
  }

  function addFile() {
    var p = window.prompt("New file path (relative, .star):", "foo/bar.star");
    if (p == null) return;
    p = p.trim();
    var err = validatePath(p);
    if (err) {
      window.alert(err);
      return;
    }
    saveBuffer();
    files[p] = STARTER_TEMPLATE;
    current = p;
    rebuildSwitcher();
    loadCurrent();
    compile();
  }

  function renameFile() {
    if (current === "kessel.star") {
      window.alert("kessel.star is the prelude and can't be renamed.");
      return;
    }
    var p = window.prompt("Rename " + current + " to:", current);
    if (p == null) return;
    p = p.trim();
    if (p === current) return;
    var err = validatePath(p);
    if (err) {
      window.alert(err);
      return;
    }
    saveBuffer();
    files[p] = files[current];
    delete files[current];
    current = p;
    rebuildSwitcher();
    loadCurrent();
    compile();
  }

  function deleteFile() {
    if (current === "kessel.star") {
      window.alert("kessel.star is the prelude and can't be deleted.");
      return;
    }
    if (Object.keys(files).length <= 1) {
      window.alert("Can't delete the last file.");
      return;
    }
    if (!window.confirm("Delete " + current + "? This can't be undone.")) return;
    delete files[current];
    current = defaultFile(Object.keys(files).sort());
    rebuildSwitcher();
    loadCurrent();
    compile();
  }

  // --- Collapsible sidebar (Phase 2) -----------------------------------------

  document.getElementById("editor-hide").addEventListener("click", toggleSidebar);
  document.getElementById("editor-show").addEventListener("click", toggleSidebar);
  document.getElementById("panel-hide").addEventListener("click", togglePanel);
  document.getElementById("panel-show").addEventListener("click", togglePanel);

  // The editor (left) and detail panel (right) collapse independently; each
  // persists its own state and re-fits the graph to the reclaimed width.
  function toggleSidebar() {
    var collapsed = document.body.classList.toggle("editor-collapsed");
    try { localStorage.setItem(LS_SIDEBAR, collapsed ? "1" : "0"); } catch (e) {}
    refitGraph();
  }

  function togglePanel() {
    var collapsed = document.body.classList.toggle("panel-collapsed");
    try { localStorage.setItem(LS_PANEL, collapsed ? "1" : "0"); } catch (e) {}
    refitGraph();
  }

  function refitGraph() {
    // Re-measure Cytoscape once the layout has reflowed to the new width.
    requestAnimationFrame(function () {
      if (window.kesselGraph) {
        window.kesselGraph.resize();
        window.kesselGraph.fit(undefined, 30);
      }
    });
  }

  // --- Layout switcher -------------------------------------------------------

  var layoutSelect = document.getElementById("layout-select");
  layoutSelect.value = currentLayout;
  // Guard against a persisted value whose <option> no longer exists.
  if (layoutSelect.value !== currentLayout) {
    currentLayout = "dagre";
    window.GRAPH_LAYOUT = currentLayout;
    layoutSelect.value = currentLayout;
  }

  layoutSelect.addEventListener("change", function () {
    var name = layoutSelect.value;
    currentLayout = name;
    window.GRAPH_LAYOUT = name;
    try { localStorage.setItem(LS_LAYOUT, name); } catch (e) {}
    if (!window.kesselGraph) return; // nothing rendered yet; first compile will use it.
    setStatus("", "Loading " + name + " layout…");
    ensureLayoutLib(name).then(function () {
      window.kesselGraph.layout(KesselRender.layoutOptions(name)).run();
      setStatus("ok", name + " layout applied ✓");
    }).catch(function (e) {
      setStatus("err", "Could not load the " + name + " layout: " + String(e && e.message || e));
    });
  });

  // ensureLayoutLib lazily injects the sidecar script for a layout the first time
  // it is used, then resolves. dagre is inlined, so it resolves immediately; a
  // failed load rejects so the caller can report it and keep the current layout.
  function ensureLayoutLib(name) {
    if (loadedLayouts[name]) return Promise.resolve();
    return new Promise(function (resolve, reject) {
      var s = document.createElement("script");
      s.src = "layout-" + name + ".js";
      s.onload = function () { loadedLayouts[name] = true; resolve(); };
      s.onerror = function () { reject(new Error("failed to fetch layout-" + name + ".js")); };
      document.head.appendChild(s);
    });
  }

  editor.on("change", function () {
    if (!ready) return;
    if (debounce) clearTimeout(debounce);
    debounce = setTimeout(compile, 700);
  });

  document.getElementById("compile").addEventListener("click", compile);
  editor.setOption("extraKeys", {
    "Ctrl-Enter": compile,
    "Cmd-Enter": compile,
  });

  function saveBuffer() {
    files[current] = editor.getValue();
  }

  // defaultFile prefers a real schema module over the kessel.star prelude so the
  // editor opens on something interesting.
  function defaultFile(all) {
    for (var i = 0; i < all.length; i++) {
      if (all[i] !== "kessel.star" && /\.star$/.test(all[i])) return all[i];
    }
    return all[0];
  }

  // --- WASM compiler bootstrap ----------------------------------------------

  bootWasm().then(function () {
    ready = true;
    if (pendingShare) return applyShare(pendingShare);
  }).then(function () {
    // Preload the persisted layout so the first render uses it; fall back to the
    // inlined dagre if the sidecar can't be fetched.
    return ensureLayoutLib(currentLayout).catch(function () {
      currentLayout = "dagre";
      window.GRAPH_LAYOUT = currentLayout;
      layoutSelect.value = currentLayout;
    });
  }).then(function () {
    compile();
  }).catch(function (err) {
    setStatus("err", "Failed to load the WASM compiler: " + String(err && err.message || err));
  });

  // applyShare decodes a share payload (delta vs seed) into the working files,
  // rebuilds the switcher, and clears the hash so a later reload uses saved edits.
  function applyShare(payload) {
    return b64urlUngzip(payload).then(function (json) {
      var delta = JSON.parse(json);
      files = applyDelta(seed, delta);
      current = defaultFile(Object.keys(files).sort());
      rebuildSwitcher();
      loadCurrent();
      persistFiles();
      history.replaceState(null, "", location.pathname + location.search);
    }).catch(function (e) {
      setStatus("err", "Could not decode the shared link: " + String(e && e.message || e));
    });
  }

  function bootWasm() {
    if (typeof Go !== "function") {
      return Promise.reject(new Error("wasm_exec.js did not load (Go runtime missing)."));
    }
    var go = new Go();
    // Fetch as bytes and instantiate — avoids relying on the server sending the
    // application/wasm MIME type that instantiateStreaming requires.
    return fetch(window.WASM_URL)
      .then(function (r) {
        if (!r.ok) throw new Error("HTTP " + r.status + " fetching " + window.WASM_URL);
        return r.arrayBuffer();
      })
      .then(function (bytes) {
        return WebAssembly.instantiate(bytes, go.importObject);
      })
      .then(function (result) {
        // go.run never resolves (main blocks in select{}); it registers
        // window.kesselCompile before blocking, so we do not await it.
        go.run(result.instance);
      });
  }

  // --- Compile + render ------------------------------------------------------

  function compile() {
    if (!ready || typeof window.kesselCompile !== "function") return;
    saveBuffer();
    persistFiles();
    setStatus("", "Compiling…");

    var res;
    try {
      res = window.kesselCompile(files);
    } catch (e) {
      setStatus("err", String(e && e.message || e));
      return;
    }

    if (!res || !res.ok) {
      setStatus("err", (res && res.error) || "Unknown compile error.");
      return;
    }

    var elements;
    try {
      elements = JSON.parse(res.elements);
    } catch (e) {
      setStatus("err", "Compiler returned invalid elements JSON: " + e.message);
      return;
    }

    render(elements);
    var counts = countElements(elements);
    setStatus("ok", "Compiled ✓  " + counts.types + " types, " + counts.edges + " edges.");
  }

  function render(elements) {
    if (cy) cy.destroy();
    cy = KesselRender.create({
      container: document.getElementById("cy"),
      elements: elements,
      layoutName: window.GRAPH_LAYOUT || "dagre",
      detailsEl: details,
      searchEl: document.getElementById("search"),
    });
  }

  function countElements(elements) {
    var types = 0, edges = 0;
    elements.forEach(function (el) {
      if (el.classes === "resource") types++;
      else if (el.data && (el.data.kind === "relation" || el.data.kind === "inherits")) edges++;
    });
    return { types: types, edges: edges };
  }

  function setStatus(cls, msg) {
    statusEl.className = cls;
    statusEl.textContent = msg;
  }

  // --- Persistence (Phase 4) -------------------------------------------------

  // cloneFiles returns a shallow copy of a { path: source } map (values are
  // immutable strings, so a shallow copy is a full copy).
  function cloneFiles(src) {
    var out = {};
    Object.keys(src).forEach(function (k) { out[k] = src[k]; });
    return out;
  }

  // readSavedFiles returns the file map persisted to localStorage, or null if
  // absent/corrupt. A malformed entry is dropped rather than left to break boot.
  function readSavedFiles() {
    var raw;
    try { raw = localStorage.getItem(LS_FILES); } catch (e) { return null; }
    if (!raw) return null;
    try {
      var obj = JSON.parse(raw);
      if (obj && typeof obj === "object" && Object.keys(obj).length) return obj;
    } catch (e) {}
    return null;
  }

  function persistFiles() {
    try { localStorage.setItem(LS_FILES, JSON.stringify(files)); } catch (e) {}
  }

  function readSidebarCollapsed() {
    try { return localStorage.getItem(LS_SIDEBAR) === "1"; } catch (e) { return false; }
  }

  function readPanelCollapsed() {
    try { return localStorage.getItem(LS_PANEL) === "1"; } catch (e) { return false; }
  }

  // readLayout returns the persisted layout name, defaulting to the layout the
  // page was generated with (window.GRAPH_LAYOUT), then the inlined dagre as a
  // last resort.
  function readLayout() {
    try { return localStorage.getItem(LS_LAYOUT) || (window.GRAPH_LAYOUT || "dagre"); }
    catch (e) { return window.GRAPH_LAYOUT || "dagre"; }
  }

  document.getElementById("reset").addEventListener("click", resetSchema);

  // resetSchema discards all local edits and restores the shipped schema.
  function resetSchema() {
    if (!window.confirm("Discard your edits and restore the shipped schema?")) return;
    files = cloneFiles(seed);
    try { localStorage.removeItem(LS_FILES); } catch (e) {}
    current = defaultFile(Object.keys(files).sort());
    rebuildSwitcher();
    loadCurrent();
    compile();
  }

  // --- Downloads (Phase 4) ---------------------------------------------------

  Array.prototype.forEach.call(document.querySelectorAll("[data-dl]"), function (btn) {
    btn.addEventListener("click", function () { doDownload(btn.getAttribute("data-dl")); });
  });

  // doDownload exports a PNG of the current graph, rendered from the live
  // Cytoscape instance at 2x for a crisp image.
  function doDownload(kind) {
    if (kind === "png") {
      if (!window.kesselGraph) { showToast("No graph to export yet.", true); return; }
      var uri = window.kesselGraph.png({ full: true, bg: "#0f1115", scale: 2 });
      downloadURI("kessel-diagram.png", uri);
      showToast("Diagram saved as kessel-diagram.png ✓");
    }
  }

  // downloadURI triggers a browser download of uri as name.
  function downloadURI(name, uri) {
    var a = document.createElement("a");
    a.href = uri;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    a.remove();
  }

  // --- Shareable URL (Phase 4) -----------------------------------------------

  document.getElementById("share").addEventListener("click", shareLink);

  // shareLink builds a link to the current view and copies it to the clipboard.
  // With no edits it shares the plain page URL (the shipped schema); with edits it
  // encodes the delta vs seed (gzip + base64url) into the "#s=" fragment so only
  // changed/added/removed files travel and typical links stay short. Every path
  // gives the user explicit toast feedback. Oversized payloads are refused.
  function shareLink() {
    saveBuffer();
    var base = location.origin + location.pathname + location.search;
    var delta = computeDelta(seed, files);

    if (!delta.set && !delta.del) {
      history.replaceState(null, "", location.pathname + location.search);
      finishShare(base, "Link copied — shipped schema (no edits) ✓");
      return;
    }

    gzipB64url(JSON.stringify(delta)).then(function (payload) {
      var url = base + "#s=" + payload;
      if (url.length > MAX_HASH) {
        showToast("Your edits are too large for a link (" + url.length +
          " chars). Use “Export diagram as PNG” or copy the files manually.", true);
        return;
      }
      history.replaceState(null, "", "#s=" + payload);
      finishShare(url, "Share link copied to clipboard ✓");
    }).catch(function (e) {
      showToast("Could not build a share link: " + String(e && e.message || e), true);
    });
  }

  // finishShare copies url and toasts, falling back to a message pointing at the
  // address bar when the clipboard API is unavailable (e.g. an insecure origin).
  function finishShare(url, okMsg) {
    copyText(url).then(function (ok) {
      showToast(ok ? okMsg : "Share link is in the address bar — copy it from there.", !ok);
    });
  }

  // showToast flashes a transient message centered at the bottom of the viewport,
  // visible no matter which panels are collapsed. err styles it red.
  function showToast(msg, err) {
    var el = document.getElementById("toast");
    if (!el) return;
    el.textContent = msg;
    el.className = err ? "err show" : "show";
    if (showToast._t) clearTimeout(showToast._t);
    showToast._t = setTimeout(function () { el.className = err ? "err" : ""; }, 3200);
  }

  // getShareParam reads the "#s=<payload>" fragment, if any. The fragment never
  // reaches the server, so the edits stay client-side.
  function getShareParam() {
    var h = location.hash || "";
    var m = /[#&]s=([^&]+)/.exec(h);
    return m ? m[1] : null;
  }

  // computeDelta diffs edited files against the seed: `set` holds new/changed
  // files, `del` lists seed paths the user removed. Only present when non-empty.
  function computeDelta(base, edited) {
    var set = {}, del = [], hasSet = false, hasDel = false;
    Object.keys(edited).forEach(function (p) {
      if (base[p] !== edited[p]) { set[p] = edited[p]; hasSet = true; }
    });
    Object.keys(base).forEach(function (p) {
      if (!Object.prototype.hasOwnProperty.call(edited, p)) { del.push(p); hasDel = true; }
    });
    var out = {};
    if (hasSet) out.set = set;
    if (hasDel) out.del = del;
    return out;
  }

  // applyDelta reconstructs a file map from the seed plus a delta.
  function applyDelta(base, delta) {
    var out = cloneFiles(base);
    if (delta.set) Object.keys(delta.set).forEach(function (p) { out[p] = delta.set[p]; });
    if (delta.del) delta.del.forEach(function (p) { delete out[p]; });
    return out;
  }

  // --- Encoding helpers ------------------------------------------------------

  // gzipB64url gzips a string (via CompressionStream) and base64url-encodes it.
  function gzipB64url(str) {
    var bytes = new TextEncoder().encode(str);
    var cs = new CompressionStream("gzip");
    var stream = new Response(new Blob([bytes]).stream().pipeThrough(cs));
    return stream.arrayBuffer().then(function (buf) {
      return bytesToB64url(new Uint8Array(buf));
    });
  }

  // b64urlUngzip is the inverse of gzipB64url.
  function b64urlUngzip(payload) {
    var bytes = b64urlToBytes(payload);
    var ds = new DecompressionStream("gzip");
    var stream = new Response(new Blob([bytes]).stream().pipeThrough(ds));
    return stream.arrayBuffer().then(function (buf) {
      return new TextDecoder().decode(buf);
    });
  }

  function bytesToB64url(bytes) {
    var bin = "";
    for (var i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
    return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }

  function b64urlToBytes(s) {
    s = s.replace(/-/g, "+").replace(/_/g, "/");
    while (s.length % 4) s += "=";
    var bin = atob(s);
    var out = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  }

  // copyText copies to the clipboard, resolving to false if the API is
  // unavailable (e.g. non-secure context) so the caller can fall back.
  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text).then(function () { return true; }, function () { return false; });
    }
    return Promise.resolve(false);
  }
})();
