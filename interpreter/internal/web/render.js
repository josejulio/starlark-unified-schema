// render.js — the Cytoscape rendering core driven by the live WASM playground
// (playground.js): given a container, an element list and the panel/search DOM,
// it builds the graph, wires tap-to-inspect, neighbor highlighting and the search
// filter, and returns the Cytoscape instance.
window.KesselRender = (function () {
  "use strict";

  // create builds a Cytoscape graph and wires all interactions.
  // opts: { container, elements, layoutName, detailsEl, searchEl }
  function create(opts) {
    var details = opts.detailsEl;
    var cy = cytoscape({
      container: opts.container,
      elements: opts.elements,
      wheelSensitivity: 0.2,
      style: [
        {
          selector: "node",
          style: {
            "label": "data(label)",
            "color": "#e6e9ef",
            "font-size": 11,
            "text-valign": "center",
            "text-halign": "center",
          },
        },
        // Compound parent per resource type: a labeled container box.
        {
          selector: "node.resource",
          style: {
            "background-color": "#12151c",
            "background-opacity": 0.6,
            "border-color": "#3a4152",
            "border-width": 1,
            "shape": "round-rectangle",
            "text-valign": "top",
            "text-halign": "center",
            "font-size": 13,
            "font-weight": "bold",
            "color": "#9aa4b2",
            "padding": 14,
          },
        },
        {
          selector: "node.common",
          style: {
            "background-color": "#8a5cf6",
            "shape": "round-rectangle",
            "width": 70,
            "height": 30,
          },
        },
        {
          selector: "node.reporter",
          style: {
            "background-color": "#2f6feb",
            "shape": "round-rectangle",
            "width": 70,
            "height": 30,
          },
        },
        // Phase A: cost badge styling - the badge symbol in the label is colored
        // to match its cost severity. Using text-outline to make it stand out.
        {
          selector: "node.reporter.cost-cheap",
          style: {
            "text-outline-color": "#4bbf8a",
            "text-outline-width": 0.5,
          },
        },
        {
          selector: "node.reporter.cost-depth",
          style: {
            "text-outline-color": "#f0c674",
            "text-outline-width": 0.5,
          },
        },
        {
          selector: "node.reporter.cost-fanout",
          style: {
            "text-outline-color": "#ff6b6b",
            "text-outline-width": 0.5,
          },
        },
        {
          selector: "edge",
          style: {
            "label": "data(label)",
            "font-size": 9,
            "color": "#9aa4b2",
            "text-background-color": "#0f1115",
            "text-background-opacity": 0.85,
            "text-background-padding": 2,
            "curve-style": "bezier",
            "target-arrow-shape": "triangle",
            "width": 1.5,
          },
        },
        {
          selector: "edge.relation",
          style: {
            "line-color": "#5b6472",
            "target-arrow-color": "#5b6472",
          },
        },
        // The common representation shared into each reporter facet: a purple
        // dotted edge at the same weight as the "extends" edge (mirrors the
        // Mermaid dotted shared link), distinct from the solid relation edges.
        {
          selector: "edge.shared",
          style: {
            "line-style": "dotted",
            "line-color": "#8a5cf6",
            "target-arrow-color": "#8a5cf6",
          },
        },
        // A relation that targets its own facet renders as a self-loop.
        {
          selector: "edge.self",
          style: {
            "line-color": "#c98a2b",
            "target-arrow-color": "#c98a2b",
          },
        },
        // Inheritance (child -> parent) is a dashed edge.
        {
          selector: "edge.inherits",
          style: {
            "line-style": "dashed",
            "line-color": "#4bbf8a",
            "target-arrow-color": "#4bbf8a",
            "target-arrow-shape": "vee",
          },
        },
        {
          selector: ":selected",
          style: {
            "border-color": "#ffd166",
            "border-width": 3,
            "line-color": "#ffd166",
            "target-arrow-color": "#ffd166",
          },
        },
        { selector: ".faded", style: { "opacity": 0.12 } },
        // A relation edge a selected permission's rewrite depends on (Phase 3):
        // brighter and thicker so it stands out from the rest of the graph.
        {
          selector: "edge.perm-affected",
          style: {
            "line-color": "#ffd166",
            "target-arrow-color": "#ffd166",
            "width": 3.5,
            "z-index": 10,
          },
        },
        // Phase B: cost-coloured permission overlay — recursion and fan-out hops.
        {
          selector: "edge.perm-affected.perm-recursive",
          style: {
            "line-color": "#f0c674",
            "target-arrow-color": "#f0c674",
            "line-style": "dashed",
          },
        },
        {
          selector: "edge.perm-affected.perm-fanout",
          style: {
            "line-color": "#ff6b6b",
            "target-arrow-color": "#ff6b6b",
            "width": 4.5,
          },
        },
      ],
      layout: layoutOptions(opts.layoutName || "dagre"),
    });

    // Exposed for console debugging (e.g. kesselGraph.$('#workspace__features')).
    window.kesselGraph = cy;

    // Phase A: apply cost heatmap to reporter facets if cost provider is present.
    applyCostHeatmap(cy);

    // Keep the graph in sync with its container. Cytoscape caches the container
    // size and only re-reads it on resize(), so a window resize otherwise leaves
    // the canvas at its old dimensions — content clipped or stranded in dead
    // space. Debounce so a drag-resize doesn't fire a relayout per pixel, and
    // preserve the user's zoom/pan (resize only, no fit). The listener removes
    // itself when this instance is destroyed (the playground rebuilds cy on
    // every compile, so a stale listener would point at a dead graph).
    var onWinResize = debounce(function () {
      if (cy.destroyed()) return;
      cy.resize();
    }, 150);
    window.addEventListener("resize", onWinResize);
    cy.on("destroy", function () {
      window.removeEventListener("resize", onWinResize);
    });

    if (details) {
      cy.on("tap", "node, edge", function (evt) {
        var ele = evt.target;
        showDetails(details, ele, cy);
        highlightNeighborhood(cy, ele);
      });
      // Tapping empty background clears the panel and any highlight.
      cy.on("tap", function (evt) {
        if (evt.target === cy) {
          resetDetails(details);
          clearFade(cy);
        }
      });
      // Phase 3: one delegated listener on the detail panel. Clicking a permission
      // (or a single reference leaf) highlights the relation edges its rewrite
      // depends on. Both cy and details are in scope here.
      details.addEventListener("click", function (evt) {
        var fid = details.getAttribute("data-facet-id");
        if (!fid) return;
        var tname = details.getAttribute("data-facet-type");
        var leaf = evt.target.closest("[data-ref]");
        var perm = evt.target.closest("[data-perm]");
        if (!leaf && !perm) return;
        // Mark what was clicked so it is visually clear which permission/reference
        // the graph highlight corresponds to (a leaf takes precedence — it is
        // inside the perm; for a bare perm click we mark its name).
        markSelection(details, leaf || (perm.closest(".perm") || perm).querySelector(".pname") || perm);
        // A leaf click resolves just that reference; a permission click resolves
        // the whole rewrite body.
        if (leaf) {
          highlightPermission(cy, fid, tname, leaf.getAttribute("data-ref"), leaf.getAttribute("data-sub") || null);
        } else {
          highlightPermission(cy, fid, tname, perm.getAttribute("data-perm"), null);
        }
      });
    }

    wireSearch(cy, opts.searchEl);
    return cy;
  }

  // debounce coalesces rapid calls (e.g. a window-resize drag) into a single
  // call `wait` ms after the last one.
  function debounce(fn, wait) {
    var t = null;
    return function () {
      var self = this, args = arguments;
      if (t) clearTimeout(t);
      t = setTimeout(function () { t = null; fn.apply(self, args); }, wait);
    };
  }

  function layoutOptions(name) {
    var base = { name: name, fit: true, padding: 30, animate: false };
    if (name === "dagre") {
      // Hierarchical/directed, left-to-right. Not compound-aware: type boxes are
      // just bounding rectangles around wherever the facets land.
      base.rankDir = "LR";
      base.nodeSep = 30;
      base.rankSep = 70;
    } else if (name === "fcose") {
      // Compound-aware force layout: keeps a type's facets tight inside its box.
      base.quality = "default";
      base.nodeSeparation = 75;
      base.packComponents = true;
      base.randomize = true;
    } else if (name === "cola") {
      // Constraint force layout with a left-to-right flow bias, so the directed
      // relations still read as a flow, plus overlap avoidance for the boxes.
      base.nodeSpacing = 12;
      base.avoidOverlap = true;
      base.flow = { axis: "x", minSeparation: 40 };
      base.maxSimulationTime = 2000;
    } else if (name === "elk") {
      // Layered hierarchy with proper compound support — the closest to dagre's
      // flow but honoring the type boxes.
      base.elk = {
        algorithm: "layered",
        "elk.direction": "RIGHT",
        "elk.spacing.nodeNode": 40,
        "elk.layered.spacing.nodeNodeBetweenLayers": 70,
      };
    }
    return base;
  }

  // --- Phase A: cost heatmap ---------------------------------------------------

  // applyCostHeatmap adds a small cost badge icon to each reporter facet node
  // showing the worst read cost among its permissions (fan-out > recursion > cheap).
  // Appends a colored symbol to the node label instead of using borders, so it
  // stays visible even when the node is selected. Feature-detected: returns
  // immediately if no cost provider.
  function applyCostHeatmap(cy) {
    if (typeof window.KesselCost !== "function") return;
    cy.nodes(".reporter").forEach(function (node) {
      var perms = node.data("permissions");
      if (!perms || !perms.length) return;
      var typeName = node.data("typeName");
      var reporter = node.data("reporter");
      var worst = null; // null | "cheap" | "depth" | "fanout"
      perms.forEach(function (p) {
        var res = window.KesselCost(typeName + "." + reporter + "#" + p.name);
        if (!res || res.error || !res.cost) return;
        var severity = "cheap";
        if (res.cost.fanoutSites > 0) severity = "fanout";
        else if (res.cost.recursive) severity = "depth";
        // Severity precedence: fanout > depth > cheap.
        if (!worst || severity === "fanout" || (severity === "depth" && worst === "cheap")) {
          worst = severity;
        }
      });
      if (worst) {
        node.addClass("cost-" + worst);
        // Append a small colored badge symbol to the label
        var badges = {
          cheap: " ●",    // green dot
          depth: " ◐",    // amber half-circle (recursion)
          fanout: " ◆"    // red diamond (fan-out)
        };
        var originalLabel = node.data("label");
        // Only append if not already added
        if (originalLabel && !originalLabel.match(/[●◐◆]$/)) {
          node.data("label", originalLabel + badges[worst]);
        }
      }
    });
  }

  // --- Neighbor highlighting -------------------------------------------------

  function highlightNeighborhood(cy, ele) {
    cy.elements().removeClass("perm-affected perm-fanout perm-recursive");
    var keep;
    if (ele.isNode()) {
      // The node, its compound descendants/ancestors, its edges and the nodes at
      // the other end of those edges.
      keep = ele
        .closedNeighborhood()
        .union(ele.descendants())
        .union(ele.ancestors())
        .union(ele.connectedEdges().connectedNodes());
    } else {
      keep = ele.union(ele.connectedNodes()).union(ele.connectedNodes().ancestors());
    }
    cy.elements().addClass("faded");
    keep.removeClass("faded");
  }

  function clearFade(cy) {
    cy.elements().removeClass("faded");
    cy.elements().removeClass("perm-affected perm-fanout perm-recursive");
  }

  // markSelection flags the clicked permission/reference in the detail panel as
  // selected, clearing any previous one, so the panel shows which rewrite drives
  // the current graph highlight.
  function markSelection(details, el) {
    var prev = details.querySelectorAll(".sel");
    for (var i = 0; i < prev.length; i++) prev[i].classList.remove("sel");
    if (el) el.classList.add("sel");
  }

  // --- Permission -> relation highlighting (Phase 3) -------------------------

  // inheritedFacets returns the facet node `fid` followed by every facet it
  // extends, transitively (child -> parent via `inherits` edges). The clicked
  // facet comes first so its own definitions win over inherited ones of the same
  // name.
  function inheritedFacets(cy, fid) {
    var seen = {};
    var order = [];
    (function walk(id) {
      if (seen[id]) return;
      seen[id] = true;
      order.push(id);
      cy.edges().forEach(function (e) {
        if (e.data("kind") === "inherits" && e.data("source") === id) {
          walk(e.data("target"));
        }
      });
    })(fid);
    return order;
  }

  // relationScope indexes the relation edges and permissions "in scope" for the
  // facet node `fid`: those on the facet itself, on its type's common node (common
  // relations apply to every reporter), AND those inherited from any facet it
  // extends (a facet borrows the relations/permissions of its parents). It returns
  // { edgeByName, permByName } — a relation-name -> edge collection map and a
  // permission-name -> rewrite-body map. `tname` is retained for the call sites.
  function relationScope(cy, fid, tname) {
    var facets = inheritedFacets(cy, fid);
    var facetSet = {};
    var commonSet = {};
    facets.forEach(function (id) {
      facetSet[id] = true;
      var node = cy.getElementById(id);
      if (node.nonempty()) commonSet[node.data("typeName") + "__common"] = true;
    });

    var edgeByName = {};
    cy.edges().forEach(function (e) {
      if (e.data("kind") !== "relation") return;
      var src = e.data("source");
      if (!facetSet[src] && !commonSet[src]) return;
      var n = e.data("name");
      edgeByName[n] = (edgeByName[n] || cy.collection()).union(e);
    });

    var permByName = {};
    facets.forEach(function (id) {
      var node = cy.getElementById(id);
      (node.data("permissions") || []).forEach(function (p) {
        // First writer wins: the clicked facet (index 0) overrides inherited ones.
        if (!Object.prototype.hasOwnProperty.call(permByName, p.name)) {
          permByName[p.name] = p.body;
        }
      });
    });
    return { edgeByName: edgeByName, permByName: permByName };
  }

  // highlightPermission fades the graph down to EVERY relation edge involved in a
  // permission (or a single reference leaf), following the whole rewrite tree: a
  // reference to another permission is walked transitively, and a subreference
  // (relation -> sub) is followed across types — the relation is highlighted and
  // then `sub` is resolved on the *target* facet, recursing. This surfaces the
  // full chain of relations a permission depends on, not just the direct ones.
  //
  // Phase B: when a cost provider is present and the clicked item is a permission
  // (not a single leaf), the affected edges are cost-coloured (recursion/fan-out).
  //
  // `start` is the clicked permission/reference name on facet `fid`; `startSub`
  // is set only when the clicked leaf was itself a subreference.
  function highlightPermission(cy, fid, tname, start, startSub) {
    var affected = cy.collection();
    var visited = {}; // "facetId|name" guard against permission/relation cycles.

    // resolveName resolves a reference name on a facet: a relation edge is
    // highlighted; another permission on that facet is walked; anything else
    // (e.g. a relation inherited from a parent facet, not drawn from this node)
    // is a dead end.
    function resolveName(facetId, typeName, name) {
      var key = facetId + "|" + name;
      if (visited[key]) return;
      visited[key] = true;
      var scope = relationScope(cy, facetId, typeName);
      if (scope.edgeByName[name]) {
        affected = affected.union(scope.edgeByName[name]);
        return;
      }
      if (scope.permByName[name]) {
        walkExpr(facetId, typeName, scope.permByName[name]);
      }
    }

    // walkSub highlights relation `name` on `facetId`, then follows each of its
    // edges to the target facet and resolves the downstream `sub` there.
    function walkSub(facetId, typeName, name, sub) {
      var scope = relationScope(cy, facetId, typeName);
      var edges = scope.edgeByName[name];
      if (!edges) { resolveName(facetId, typeName, name); return; }
      affected = affected.union(edges);
      edges.forEach(function (e) {
        var tgt = cy.getElementById(e.data("target"));
        if (tgt.empty() || !sub) return;
        resolveName(tgt.id(), tgt.data("typeName"), sub);
      });
    }

    function walkExpr(facetId, typeName, expr) {
      if (!expr || !expr.kind) return;
      switch (expr.kind) {
        case "reference":
          resolveName(facetId, typeName, expr.name);
          break;
        case "subreference":
          walkSub(facetId, typeName, expr.name, expr.sub);
          break;
        case "and":
        case "or":
        case "unless":
          walkExpr(facetId, typeName, expr.left);
          walkExpr(facetId, typeName, expr.right);
          break;
      }
    }

    if (startSub) walkSub(fid, tname, start, startSub);
    else resolveName(fid, tname, start);

    // Keep the inheritance links from the clicked facet visible so an inherited
    // relation reads as connected to the facet that borrows it.
    var chain = {};
    inheritedFacets(cy, fid).forEach(function (id) { chain[id] = true; });
    var inheritEdges = cy.collection();
    cy.edges().forEach(function (e) {
      if (e.data("kind") === "inherits" && chain[e.data("source")]) {
        inheritEdges = inheritEdges.union(e);
      }
    });

    cy.elements().removeClass("perm-affected perm-fanout perm-recursive");
    cy.elements().addClass("faded");
    var node = cy.getElementById(fid);
    var keep = affected
      .union(affected.connectedNodes())
      .union(affected.connectedNodes().ancestors())
      .union(inheritEdges)
      .union(inheritEdges.connectedNodes())
      .union(inheritEdges.connectedNodes().ancestors())
      .union(node)
      .union(node.ancestors());
    keep.removeClass("faded");
    affected.addClass("perm-affected");

    // Phase B: if this is a permission click (not a leaf) and the cost provider
    // is present, overlay cost classes on the affected edges based on the proof tree.
    if (!startSub && typeof window.KesselCost === "function") {
      var reporter = cy.getElementById(fid).data("reporter");
      if (reporter) {
        var provided = window.KesselCost(tname + "." + reporter + "#" + start);
        if (provided && !provided.error && provided.root) {
          var roleMap = buildRoleMap(provided.root);
          affected.forEach(function (edge) {
            var role = roleMap[edge.data("name")];
            if (role === "fanout") edge.addClass("perm-fanout");
            else if (role === "recursive") edge.addClass("perm-recursive");
          });
        }
      }
    }
  }

  // buildRoleMap walks a check proof tree and returns a relation-name → role map.
  // Role is "fanout" for many-cardinality arrows, "recursive" for arrows that
  // hit a recursion sentinel, else "hop". On name collision, highest severity wins.
  function buildRoleMap(node) {
    var map = {};
    function walk(n) {
      if (!n || !n.kind) return;
      if (n.kind === "arrow") {
        var role = "hop";
        if (isManyCardinality(n.cardinality)) role = "fanout";
        else if (n.children && n.children[0] && n.children[0].kind === "recursive") role = "recursive";
        var prev = map[n.name];
        if (!prev || role === "fanout" || (role === "recursive" && prev === "hop")) {
          map[n.name] = role;
        }
      }
      if (n.children) n.children.forEach(walk);
      if (n.body) walk(n.body);
      if (n.left) walk(n.left);
      if (n.right) walk(n.right);
    }
    walk(node);
    return map;
  }

  // isManyCardinality returns true if cardinality is NOT one of the single-target
  // values ("ExactlyOne", "AtMostOne"). Mirrors Go isManyCardinality.
  function isManyCardinality(card) {
    return card !== "ExactlyOne" && card !== "AtMostOne";
  }

  // --- Search / filter -------------------------------------------------------

  function wireSearch(cy, input) {
    if (!input) return;
    input.addEventListener("input", function () {
      var q = input.value.trim().toLowerCase();
      cy.$(":selected").unselect();
      if (!q) {
        clearFade(cy);
        return;
      }
      var matches = cy.nodes().filter(function (n) {
        return (n.data("label") || "").toLowerCase().indexOf(q) !== -1;
      });
      // Keep matches, their ancestors (containing type) and descendants (facets).
      var keep = matches.union(matches.ancestors()).union(matches.descendants());
      cy.elements().addClass("faded");
      keep.removeClass("faded");
    });
  }

  // --- Detail panel ----------------------------------------------------------

  function resetDetails(details) {
    details.innerHTML = '<p class="empty">Nothing selected.</p>';
    details.removeAttribute("data-facet-id");
    details.removeAttribute("data-facet-type");
  }

  function showDetails(details, ele, cy) {
    var d = ele.data();
    // Only a facet node carries permissions the panel can highlight against; tag
    // the panel with its id/type so the delegated click listener can resolve them.
    var isFacet = ele.isNode() && !ele.hasClass("resource");
    if (isFacet) {
      details.setAttribute("data-facet-id", d.id);
      details.setAttribute("data-facet-type", d.typeName);
    } else {
      details.removeAttribute("data-facet-id");
      details.removeAttribute("data-facet-type");
    }
    if (ele.isNode()) {
      details.innerHTML = ele.hasClass("resource") ? renderType(d) : renderFacet(d, cy);
    } else {
      details.innerHTML = renderEdge(d);
    }
  }

  function header(kind, title) {
    return (
      '<h2><span class="badge">' +
      esc(kind) +
      "</span></h2><h1>" +
      esc(String(title)) +
      "</h1>"
    );
  }

  function renderType(d) {
    var rows =
      row("kind", d.kind) +
      row("type", d.typeName) +
      row("reporters", (d.reporters || []).join(", ") || "—") +
      row("common", d.hasCommon ? "yes" : "no");
    return header("Resource type", d.label) + table(rows);
  }

  function renderFacet(d, cy) {
    var kind = d.group === "common" ? "Common representation" : "Reporter facet";
    var rows = row("type", d.typeName);
    if (d.reporter) rows += row("reporter", d.reporter);
    if (d.extends) rows += row("extends", d.extends);
    var html = header(kind, d.label) + table(rows);
    html += section("Data fields", renderDataFields(d.dataFields));
    // Resolve each reference to a relation-or-permission badge so the panel shows
    // what a permission is built on before anything is clicked.
    var scope = cy ? relationScope(cy, d.id, d.typeName) : null;
    // A permission's read cost is resolved against a single reporter facet, so
    // only reporter facets get cost chips — a common permission has no one
    // reporter to ask the check against.
    var facet = d.group === "reporter" ? { typeName: d.typeName, reporter: d.reporter } : null;
    html += section("Permissions", renderPermissions(d.permissions, scope, facet));
    return html;
  }

  // costChip returns an at-a-glance read-cost badge for a permission, computed by
  // the WASM analyzer through the window.KesselCost provider the playground
  // installs after each compile. It is feature-detected: the static page has no
  // provider, so permissions render without chips there. The colour encodes the
  // dominant cost driver — fan-out (over a many-relation) is the loudest, then a
  // hierarchy/recursion walk, then constant-time.
  function costChip(facet, permName) {
    if (!facet || !facet.reporter || typeof window.KesselCost !== "function") return "";
    var res = window.KesselCost(facet.typeName + "." + facet.reporter + "#" + permName);
    if (!res || res.error || !res.cost) return "";
    var c = res.cost;
    var cls = "cheap";
    if (c.fanoutSites > 0) cls = "fanout";
    else if (c.recursive) cls = "depth";
    var tip =
      c.bigO +
      " · " + c.dispatchDepth + " hop(s)" +
      " · " + c.fanoutSites + " fan-out site(s)" +
      (c.recursive ? " · recursive" : "");
    return (
      ' <span class="badge cost ' +
      cls +
      '" title="' +
      esc(tip) +
      '">' +
      esc(c.bigO) +
      "</span>"
    );
  }

  // refKind classifies a reference name against the facet's scope for its badge:
  // a relation edge, another permission, or unknown (e.g. a cross-type target).
  function refKind(name, scope) {
    if (!scope) return "";
    if (scope.edgeByName[name]) return "rel";
    if (scope.permByName[name]) return "perm";
    return "";
  }

  function refBadge(name, scope) {
    var kind = refKind(name, scope);
    if (kind === "rel") return ' <span class="badge rel refkind">relation</span>';
    if (kind === "perm") return ' <span class="badge perm refkind">permission</span>';
    return "";
  }

  function renderEdge(d) {
    if (d.kind === "inherits") {
      return (
        header("Inheritance", "extends") +
        table(row("source", d.source) + row("target", d.target))
      );
    }
    if (d.kind === "shared") {
      return (
        header("Shared representation", "common → reporter") +
        table(row("source", d.source) + row("target", d.target))
      );
    }
    var rows =
      row("name", d.name) +
      row("cardinality", d.cardinality || "ExactlyOne") +
      row("scope", d.scope) +
      row("source", d.source) +
      row("target", d.target) +
      row("self", d.self ? "yes" : "no");
    if (d.sourceReporter) rows += row("source reporter", d.sourceReporter);
    if (d.targetReporter) rows += row("target reporter", d.targetReporter);
    return header("Relation", d.name) + table(rows);
  }

  // renderDataFields renders each field with its (recursive) data type.
  function renderDataFields(fields) {
    if (!fields || !fields.length) return null;
    return (
      '<div class="members">' +
      fields
        .map(function (f) {
          var req = f.required
            ? ' <span class="badge req">required</span>'
            : "";
          var desc = f.description
            ? '<div class="fdesc">' + esc(f.description) + "</div>"
            : "";
          return (
            '<div class="field"><span class="fname">' +
            esc(f.name) +
            "</span>" +
            req +
            ' — <span class="ftype">' +
            esc(formatType(f.type)) +
            "</span>" +
            desc +
            "</div>"
          );
        })
        .join("") +
      "</div>"
    );
  }

  // formatType renders a dataType (see GRAPH.md) as a compact readable string.
  function formatType(t) {
    if (!t || !t.kind) return "?";
    switch (t.kind) {
      case "text": {
        var attrs = [];
        if (t.minLength != null) attrs.push("minLength=" + t.minLength);
        if (t.maxLength != null) attrs.push("maxLength=" + t.maxLength);
        if (t.regex != null) attrs.push("regex=" + t.regex);
        return attrs.length ? "text(" + attrs.join(", ") + ")" : "text";
      }
      case "uuid":
        return "uuid";
      case "numeric_id": {
        var na = [];
        if (t.min != null) na.push("min=" + t.min);
        if (t.max != null) na.push("max=" + t.max);
        return na.length ? "numeric_id(" + na.join(", ") + ")" : "numeric_id";
      }
      case "boolean":
        return "boolean";
      case "date_time":
        return "date_time";
      case "enum":
        return "enum[" + (t.values || []).join(", ") + "]";
      case "nullable":
        return formatType(t.inner) + "?";
      case "composite":
        return "(" + (t.types || []).map(formatType).join(" | ") + ")";
      case "array":
        return "array<" + formatType(t.items) + ">";
      case "object": {
        var props = (t.properties || [])
          .map(function (p) {
            return p.name + ": " + formatType(p.type);
          })
          .join(", ");
        return "object{" + props + "}";
      }
      default:
        return t.kind;
    }
  }

  // renderPermissions renders each permission name and its rewrite tree. When a
  // scope is supplied, permission names and reference leaves become clickable
  // (data-perm / data-ref) and each leaf carries a relation/permission badge.
  // When a reporter `facet` is supplied and a cost provider is present, each
  // permission name also gets an at-a-glance read-cost chip (see costChip).
  function renderPermissions(perms, scope, facet) {
    if (!perms || !perms.length) return null;
    return (
      '<div class="members">' +
      perms
        .map(function (p) {
          return (
            '<div class="perm" data-perm="' +
            esc(p.name) +
            '"><div class="pname" data-perm="' +
            esc(p.name) +
            '">' +
            esc(p.name) +
            costChip(facet, p.name) +
            "</div>" +
            renderExpr(p.body, scope) +
            "</div>"
          );
        })
        .join("") +
      "</div>"
    );
  }

  // renderExpr renders a permission rewrite expression tree (and/or/unless and
  // reference/subreference leaves) as a nested, indented structure.
  function renderExpr(e, scope) {
    if (!e || !e.kind) return "";
    switch (e.kind) {
      case "reference":
        return (
          '<div class="expr"><span class="leaf" data-ref="' +
          esc(e.name) +
          '"><span class="arrow">→</span> ' +
          esc(e.name) +
          refBadge(e.name, scope) +
          "</span></div>"
        );
      case "subreference":
        return (
          '<div class="expr"><span class="leaf" data-ref="' +
          esc(e.name) +
          '" data-sub="' +
          esc(e.sub) +
          '"><span class="arrow">→</span> ' +
          esc(e.name) +
          ' <span class="arrow">▸</span> ' +
          esc(e.sub) +
          refBadge(e.name, scope) +
          "</span></div>"
        );
      case "and":
      case "or":
      case "unless":
        return (
          '<div class="expr"><span class="op">' +
          esc(e.kind) +
          "</span>" +
          renderExpr(e.left, scope) +
          renderExpr(e.right, scope) +
          "</div>"
        );
      default:
        return '<div class="expr"><span class="leaf">' + esc(e.kind) + "</span></div>";
    }
  }

  // --- Small HTML helpers ----------------------------------------------------

  function section(title, body) {
    if (!body) return "";
    return "<h2>" + esc(title) + "</h2>" + body;
  }

  function table(rows) {
    return '<table class="props">' + rows + "</table>";
  }

  function row(key, value) {
    var v =
      value === null || value === undefined || value === ""
        ? '<span class="empty">—</span>'
        : esc(String(value));
    return "<tr><th>" + esc(key) + "</th><td>" + v + "</td></tr>";
  }

  function esc(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  return { create: create, esc: esc, layoutOptions: layoutOptions };
})();
