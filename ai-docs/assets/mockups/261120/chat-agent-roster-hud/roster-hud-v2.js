(() => {
  const MAX_DEPTH = 4;

  const STATE_META = {
    thinking:   { label: "Thinking",    ko: "사고 중" },
    tool:       { label: "Tool Call",   ko: "도구 실행" },
    holding:    { label: "Waiting Kids", ko: "하위 대기" },
    idle:       { label: "Idle (alive)", ko: "대기 중" },
    permission: { label: "Permission",  ko: "승인 대기" },
    failed:     { label: "Failed",      ko: "실패" },
  };

  const HARNESS_LABEL = { claude: "Cl", codex: "Co", agy: "Ag", pi: "Pi" };
  const HARNESS_NAME = { claude: "Claude", codex: "Codex", agy: "Antigravity", pi: "Pi" };

  const SCENARIOS = {
    heavy: {
      completedBase: 2,
      peers: 20,
      roots: [
        {
          id: "s-101", label: "parser-worker", harness: "codex", state: "holding", process: true,
          model: "gpt-5.4", effort: "high", elapsed: "2m 14s", task: "Waiting on ast-scanner & schema-diff",
          children: [
            {
              id: "s-102", label: "ast-scanner", harness: "claude", state: "holding", process: true,
              model: "sonnet-4.6", effort: "medium", elapsed: "1m 48s", task: "Aggregating symbol table slices",
              children: [
                { id: "s-103", label: "sym-extract", harness: "agy", state: "tool", process: true, model: "gemini-4-h", effort: "high", elapsed: "41s", task: "rg -n 'export (type|interface)' internal/" },
                { id: "s-104", label: "type-bounds", harness: "pi", state: "thinking", process: true, model: "o3-mini", effort: "medium", elapsed: "29s", task: "Resolving generic constraint graph" },
              ],
            },
            { id: "s-105", label: "schema-diff", harness: "codex", state: "tool", process: true, model: "gpt-5.4", effort: "high", elapsed: "1m 05s", task: "apply_patch internal/schema/v2.go" },
            { id: "s-106", label: "cache-warm", harness: "pi", state: "idle", process: false, model: "gpt-5.4", effort: "low", elapsed: "3m 02s", task: "Closed after cache warmup" },
          ],
        },
        {
          id: "s-201", label: "graph-auditor", harness: "claude", state: "holding", process: true,
          model: "opus-4.6", effort: "high", elapsed: "2m 09s", task: "Waiting on cycle-detect & import-lint",
          children: [
            { id: "s-202", label: "cycle-detect", harness: "claude", state: "permission", process: true, model: "sonnet-4.6", effort: "medium", elapsed: "1m 18s", task: "Needs approval: git diff --stat origin/develop" },
            {
              id: "s-203", label: "import-lint", harness: "pi", state: "holding", process: true,
              model: "gpt-5.4", effort: "low", elapsed: "44s", task: "Waiting on pkg-edges scan",
              children: [
                { id: "s-204", label: "pkg-edges", harness: "codex", state: "tool", process: true, model: "gpt-5.4", effort: "medium", elapsed: "19s", task: "go list -deps ./internal/..." },
              ],
            },
          ],
        },
        {
          id: "s-301", label: "test-runner", harness: "agy", state: "holding", process: true,
          model: "gemini-4-h", effort: "high", elapsed: "1m 55s", task: "Collecting test matrix results",
          children: [
            { id: "s-302", label: "unit-matrix", harness: "codex", state: "tool", process: true, model: "gpt-5.4", effort: "medium", elapsed: "58s", task: "go test ./internal/symindex -run TestResolve" },
            { id: "s-303", label: "e2e-smoke", harness: "claude", state: "failed", process: true, model: "sonnet-4.6", effort: "medium", elapsed: "37s", task: "exit 1: fixture port 4318 already in use" },
            { id: "s-304", label: "bench-closed", harness: "agy", state: "idle", process: false, model: "gemini-4-h", effort: "low", elapsed: "4m 10s", task: "Process exited cleanly" },
          ],
        },
      ],
    },
    states: {
      completedBase: 2,
      peers: 8,
      forceShowAliveIdle: true,
      showLegend: true,
      roots: [
        {
          id: "st-hold", label: "lead-coordinator", harness: "claude", state: "holding", process: true,
          model: "opus-4.6", effort: "high", elapsed: "1m 32s", task: "Alive & waiting on 4 harness children",
          children: [
            { id: "st-cl", label: "claude-think", harness: "claude", state: "thinking", process: true, model: "sonnet-4.6", effort: "high", elapsed: "24s", task: "Drafting migration strategy" },
            { id: "st-co", label: "codex-tool", harness: "codex", state: "tool", process: true, model: "gpt-5.4", effort: "high", elapsed: "18s", task: "go test ./internal/wsdashboard/..." },
            { id: "st-ag", label: "agy-perm", harness: "agy", state: "permission", process: true, model: "gemini-4-h", effort: "medium", elapsed: "49s", task: "Approval requested: run_command rm -rf dist" },
            { id: "st-pi", label: "pi-fail", harness: "pi", state: "failed", process: true, model: "o3-mini", effort: "medium", elapsed: "12s", task: "panic: nil pointer in tree walker" },
          ],
        },
        { id: "st-idle", label: "sleeper-worker", harness: "codex", state: "idle", process: true, model: "gpt-5.4", effort: "low", elapsed: "6m 40s", task: "Alive-idle (Process=true), waiting on mailbox" },
      ],
    },
    max4: {
      completedBase: 5,
      peers: 24,
      roots: [
        {
          id: "m4-r1", label: "arch-lead", harness: "claude", state: "holding", process: true,
          model: "opus-4.6", effort: "high", elapsed: "3m 41s", task: "Coordinating 4-level deep refactor tree",
          children: [
            { id: "m4-2a", label: "doc-sync", harness: "agy", state: "thinking", process: true, model: "gemini-4-h", effort: "medium", elapsed: "45s", task: "Updating API doc anchors" },
            {
              id: "m4-2b", label: "core-split", harness: "codex", state: "holding", process: true,
              model: "gpt-5.4", effort: "high", elapsed: "2m 50s", task: "Waiting on depth-3 workers",
              children: [
                {
                  id: "m4-3a", label: "ast-rewriter", harness: "agy", state: "holding", process: true,
                  model: "gemini-4-h", effort: "high", elapsed: "1m 39s", task: "Waiting on depth-4 leaf + deeper spawns",
                  children: [
                    {
                      id: "m4-4a", label: "leaf-patcher", harness: "pi", state: "tool", process: true,
                      model: "o3-mini", effort: "medium", elapsed: "33s", task: "Patching node imports (+2 hidden depth-5 children)",
                      children: [
                        { id: "m4-5a", label: "depth5-a", harness: "codex", state: "thinking", process: true, model: "gpt-5.4", effort: "low", elapsed: "14s", task: "Depth 5 helper A" },
                        { id: "m4-5b", label: "depth5-b", harness: "claude", state: "tool", process: true, model: "sonnet-4.6", effort: "low", elapsed: "11s", task: "Depth 5 helper B" },
                      ],
                    },
                  ],
                },
                { id: "m4-3b", label: "type-check", harness: "claude", state: "permission", process: true, model: "sonnet-4.6", effort: "medium", elapsed: "54s", task: "Needs approval for write_file" },
              ],
            },
            { id: "m4-2c", label: "warm-pool", harness: "pi", state: "idle", process: true, model: "o3-mini", effort: "low", elapsed: "5m 12s", task: "Alive-idle (Process=true) standby worker" },
            { id: "m4-c1", label: "closed-scan", harness: "codex", state: "idle", process: false, model: "gpt-5.4", effort: "low", elapsed: "6m 02s", task: "Asleep/closed — collapsed into 완료" },
          ],
        },
        {
          id: "m4-r2", label: "fanout-hub", harness: "codex", state: "holding", process: true,
          model: "gpt-5.4", effort: "high", elapsed: "2m 18s", task: "Wide 4-way fanout across shards",
          children: [
            { id: "m4-f1", label: "shard-01", harness: "claude", state: "tool", process: true, model: "sonnet-4.6", effort: "medium", elapsed: "42s", task: "Scanning shard 01" },
            { id: "m4-f2", label: "shard-02", harness: "agy", state: "thinking", process: true, model: "gemini-4-h", effort: "medium", elapsed: "38s", task: "Analyzing shard 02" },
            { id: "m4-f3", label: "shard-03", harness: "codex", state: "idle", process: true, model: "gpt-5.4", effort: "low", elapsed: "3m 05s", task: "Alive-idle (Process=true) shard worker" },
            { id: "m4-f4", label: "shard-04", harness: "pi", state: "failed", process: true, model: "o3-mini", effort: "low", elapsed: "21s", task: "Timeout on shard 04" },
          ],
        },
        { id: "m4-r3", label: "lint-fast", harness: "agy", state: "thinking", process: true, model: "gemini-4-h", effort: "low", elapsed: "27s", task: "Running vet checks" },
        { id: "m4-r4", label: "old-cache", harness: "pi", state: "idle", process: true, model: "o3-mini", effort: "low", elapsed: "8m 00s", task: "Alive-idle (Process=true) — awaiting prompt" },
        { id: "m4-c2", label: "closed-prep", harness: "claude", state: "idle", process: false, model: "sonnet-4.6", effort: "low", elapsed: "9m 15s", task: "Closed — collapsed into 완료" },
      ],
    },
    fanout: {
      completedBase: 9,
      peers: 31,
      roots: Array.from({ length: 14 }, (_, idx) => {
        const harnesses = ["claude", "codex", "agy", "pi"];
        const states = ["thinking", "tool", "tool", "thinking", "permission", "tool", "thinking", "failed", "tool", "thinking", "tool", "idle", "thinking", "tool"];
        return {
          id: `fo-${String(idx + 1).padStart(2, "0")}`,
          label: `shard-worker-${String(idx + 1).padStart(2, "0")}`,
          harness: harnesses[idx % harnesses.length],
          state: states[idx],
          process: true,
          model: idx % 2 === 0 ? "gpt-5.4" : "gemini-4-h",
          effort: idx % 3 === 0 ? "high" : "medium",
          elapsed: `${12 + idx * 7}s`,
          task: `Parallel shard task #${idx + 1}`,
        };
      }),
    },
  };

  const state = {
    theme: "dark",
    scenario: "heavy",
    idleMode: "alive", // (42)=c default: "alive" shows Process=true idle dimmed; "active" shows active subtrees only
    width: 620,
    compareOld: false,
    popOpen: false,
    hoveredId: null,
    pinnedHoverId: null,
    showFootprint: false,
  };

  const $ = (id) => document.getElementById(id);
  const rootEl = document.documentElement;
  const convoShell = $("convo-shell");
  const hudView = $("hud-view");
  const hudCanvas = $("hud-canvas");
  const hudRoots = $("hud-roots");
  const hudLines = $("hud-lines");
  const hudSummary = $("hud-summary-text");
  const hudFootprint = $("hud-footprint");
  const hudExpandBtn = $("hud-expand-btn");
  const agcompactOld = $("agcompact-old");
  const agpop = $("agpop");
  const agpopChips = $("agpop-chips");
  const agpopTree = $("agpop-tree");
  const hudTooltip = $("hud-tooltip");
  const hudToast = $("hud-toast");
  const hudLegend = $("hud-legend");
  const hdrRunChip = $("hdr-run-chip");

  let nodeRegistry = new Map();
  let parentEdges = [];
  let toastTimer = null;

  function countDescendants(node) {
    if (!node.children || !node.children.length) return 0;
    return node.children.reduce((acc, c) => acc + 1 + countDescendants(c), 0);
  }

  function isNodeShown(node, idleMode) {
    if (node.state !== "idle") return true;
    return idleMode === "alive" && node.process === true;
  }

  function hasShownSubtree(node, idleMode) {
    if (isNodeShown(node, idleMode)) return true;
    return (node.children || []).some((c) => hasShownSubtree(c, idleMode));
  }

  function countCollapsedIdle(nodes, idleMode) {
    let n = 0;
    for (const node of nodes || []) {
      if (node.state === "idle" && !hasShownSubtree(node, idleMode)) n += 1;
      n += countCollapsedIdle(node.children, idleMode);
    }
    return n;
  }

  function buildVisibleTree(nodes, depth, idleMode, chain) {
    const out = [];
    for (const raw of nodes || []) {
      if (!hasShownSubtree(raw, idleMode)) continue;
      const nextChain = [...chain, raw.label];
      let kids = [];
      let overflow = 0;
      if (depth < MAX_DEPTH) {
        kids = buildVisibleTree(raw.children, depth + 1, idleMode, nextChain);
      } else {
        overflow = countDescendants(raw);
      }
      out.push({ ...raw, depth, chain: chain.slice(), overflow, kids });
    }
    return out;
  }

  function countTreeNodes(nodes, onlyRunning) {
    let total = 0;
    for (const n of nodes) {
      if (!onlyRunning || n.state !== "idle") total += 1;
      total += countTreeNodes(n.kids, onlyRunning);
    }
    return total;
  }

  function renderNodeEl(node) {
    const wrap = document.createElement("div");
    wrap.className = "tree-node";

    const slot = document.createElement("div");
    slot.className = "node-slot";

    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "hud-node";
    btn.dataset.id = node.id;
    btn.dataset.state = node.state;
    btn.setAttribute("aria-label", `${node.label} (${HARNESS_NAME[node.harness]}, ${STATE_META[node.state].label})`);

    const ring = document.createElement("span");
    ring.className = "hud-ring";
    const emb = document.createElement("span");
    emb.className = "hud-emblem";
    emb.dataset.h = node.harness;
    emb.textContent = HARNESS_LABEL[node.harness] || "??";

    btn.append(ring, emb);
    if (node.state === "permission") {
      const b = document.createElement("span");
      b.className = "hud-badge perm";
      b.textContent = "!";
      btn.appendChild(b);
    } else if (node.state === "failed") {
      const b = document.createElement("span");
      b.className = "hud-badge fail";
      b.textContent = "×";
      btn.appendChild(b);
    }

    slot.appendChild(btn);
    if (node.overflow > 0) {
      const ov = document.createElement("span");
      ov.className = "hud-overflow";
      ov.textContent = `+${node.overflow}`;
      slot.appendChild(ov);
    }
    wrap.appendChild(slot);

    nodeRegistry.set(node.id, { data: node, el: btn });

    if (node.kids.length > 0) {
      const kidsEl = document.createElement("div");
      kidsEl.className = "tree-kids";
      for (const k of node.kids) {
        parentEdges.push([node.id, k.id]);
        kidsEl.appendChild(renderNodeEl(k));
      }
      wrap.appendChild(kidsEl);
    }
    return wrap;
  }

  function drawConnectors() {
    if (state.compareOld || hudView.hidden) return;
    const cRect = hudCanvas.getBoundingClientRect();
    if (!cRect.width || !cRect.height) return;
    hudLines.setAttribute("viewBox", `0 0 ${cRect.width} ${cRect.height}`);
    hudLines.setAttribute("width", String(cRect.width));
    hudLines.setAttribute("height", String(cRect.height));

    const byParent = new Map();
    for (const [pid, cid] of parentEdges) {
      if (!byParent.has(pid)) byParent.set(pid, []);
      byParent.get(pid).push(cid);
    }

    const parts = [];
    for (const [pid, cids] of byParent.entries()) {
      const pEntry = nodeRegistry.get(pid);
      if (!pEntry) continue;
      const pr = pEntry.el.getBoundingClientRect();
      const px = Math.round(pr.left - cRect.left + pr.width / 2) + 0.5;
      const py = Math.round(pr.bottom - cRect.top);
      const childCoords = cids.map((cid) => {
        const cr = nodeRegistry.get(cid).el.getBoundingClientRect();
        return {
          cx: Math.round(cr.left - cRect.left + cr.width / 2) + 0.5,
          cy: Math.round(cr.top - cRect.top),
        };
      });
      const minCy = Math.min(...childCoords.map((c) => c.cy));
      const midY = Math.round((py + minCy) / 2) + 0.5;
      if (childCoords.length === 1) {
        const c = childCoords[0];
        if (Math.abs(px - c.cx) < 1) parts.push(`M${px},${py}V${c.cy}`);
        else parts.push(`M${px},${py}V${midY}H${c.cx}V${c.cy}`);
      } else {
        const minX = Math.min(...childCoords.map((c) => c.cx));
        const maxX = Math.max(...childCoords.map((c) => c.cx));
        parts.push(`M${px},${py}V${midY}`);
        parts.push(`M${minX},${midY}H${maxX}`);
        for (const c of childCoords) parts.push(`M${c.cx},${midY}V${c.cy}`);
      }
    }
    hudLines.innerHTML = `<path d="${parts.join(" ")}" />`;
  }

  function positionTooltip(btn) {
    const shellRect = convoShell.getBoundingClientRect();
    const btnRect = btn.getBoundingClientRect();
    const ttRect = hudTooltip.getBoundingClientRect();
    const pad = 8;
    let left = btnRect.left - shellRect.left + btnRect.width / 2 - ttRect.width / 2;
    left = Math.max(pad, Math.min(left, shellRect.width - ttRect.width - pad));
    let top = btnRect.bottom - shellRect.top + 8;
    if (top + ttRect.height > shellRect.height - pad) {
      top = Math.max(pad, btnRect.top - shellRect.top - ttRect.height - 8);
    }
    hudTooltip.style.left = `${Math.round(left)}px`;
    hudTooltip.style.top = `${Math.round(top)}px`;
  }

  function showTooltip(id) {
    const entry = nodeRegistry.get(id);
    if (!entry) return;
    const { data, el } = entry;
    state.hoveredId = id;
    for (const { el: b } of nodeRegistry.values()) b.classList.toggle("is-hovered", b === el);
    const chainText = data.chain.length ? `${data.chain.join(" › ")} › ${data.label}` : `coordinator › ${data.label}`;
    const overflowNote = data.overflow > 0 ? ` · +${data.overflow} deeper` : "";
    hudTooltip.innerHTML = `
      <div class="tt-head">
        <span class="hb sm" data-h="${data.harness}"></span>
        <span class="tt-title">${data.label}</span>
        <span class="tt-pill" data-state="${data.state}">${STATE_META[data.state].label}</span>
      </div>
      <div class="tt-meta"><code>${data.model}</code><span>${data.effort}</span><span>· ${data.elapsed}${overflowNote}</span></div>
      <div class="tt-chain">${chainText}</div>
      <div class="tt-task">${data.task}</div>
      <div class="tt-hint">클릭: 세션 이동 /s/${data.id}</div>
    `;
    hudTooltip.hidden = false;
    positionTooltip(el);
  }

  function hideTooltip() {
    if (state.pinnedHoverId) { showTooltip(state.pinnedHoverId); return; }
    state.hoveredId = null;
    for (const { el: b } of nodeRegistry.values()) b.classList.remove("is-hovered");
    hudTooltip.hidden = true;
  }

  function showToast(msg) {
    hudToast.textContent = msg;
    hudToast.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { hudToast.hidden = true; }, 2200);
  }

  function renderLegend(visible) {
    hudLegend.hidden = !visible;
    if (!visible) return;
    hudLegend.innerHTML = `
      <div>
        <div class="lg-title">Outer Ring States (6)</div>
        <div class="lg-states">
          <span class="lg-item"><span class="lg-swatch thinking"></span><span>Thinking</span></span>
          <span class="lg-item"><span class="lg-swatch tool"></span><span>Tool Call</span></span>
          <span class="lg-item"><span class="lg-swatch holding"></span><span>Waiting Kids</span></span>
          <span class="lg-item"><span class="lg-swatch idle"></span><span>Idle (alive)</span></span>
          <span class="lg-item"><span class="lg-swatch permission"></span><span>Permission (!)</span></span>
          <span class="lg-item"><span class="lg-swatch failed"></span><span>Failed (×)</span></span>
        </div>
      </div>
      <div class="lg-harnesses">
        <div class="lg-title">Harnesses (4)</div>
        <span class="lg-hrow"><span class="hb sm" data-h="claude"></span><span>Claude</span></span>
        <span class="lg-hrow"><span class="hb sm" data-h="codex"></span><span>Codex</span></span>
        <span class="lg-hrow"><span class="hb sm" data-h="agy"></span><span>Antigravity</span></span>
        <span class="lg-hrow"><span class="hb sm" data-h="pi"></span><span>Pi</span></span>
      </div>
    `;
  }

  function flattenVisible(nodes, out = []) {
    for (const n of nodes) { out.push(n); flattenVisible(n.kids, out); }
    return out;
  }

  function renderOldCompact(tree, summaryText) {
    const flat = flattenVisible(tree);
    const rowsHtml = flat.map((n) => `
      <div class="agrow-old">
        <span class="hb sm" data-h="${n.harness}"></span>
        <span class="agname">${n.label}</span>
        <span class="agphase ${n.state}">${STATE_META[n.state].ko}</span>
        <span class="agelapsed">${n.elapsed}</span>
      </div>
    `).join("");
    agcompactOld.innerHTML = `
      <div class="agcompact-old-head">
        <span>Before: #agcompact (320px 세로 리스트)</span>
        <span>${flat.length}행</span>
      </div>
      ${rowsHtml}
      <div class="agsummary-old">${summaryText}</div>
    `;
  }

  function renderPopTree(nodes) {
    return nodes.map((n) => `
      <div class="pop-node">
        <div class="pop-row" data-pop-id="${n.id}">
          <span class="hb sm" data-h="${n.harness}"></span>
          <span class="pop-name">${n.label}</span>
          <span class="tt-pill" data-state="${n.state}">${STATE_META[n.state].label}</span>
          <span class="pop-meta">${n.model} · ${n.elapsed}</span>
        </div>
        ${n.kids.length ? `<div class="pop-kids">${renderPopTree(n.kids)}</div>` : ""}
      </div>
    `).join("");
  }

  function renderAll() {
    rootEl.setAttribute("data-theme", state.theme);
    convoShell.setAttribute("data-width", String(state.width));

    const sc = SCENARIOS[state.scenario];
    const effectiveIdleMode = sc.forceShowAliveIdle ? "alive" : state.idleMode;
    const tree = buildVisibleTree(sc.roots, 1, effectiveIdleMode, []);
    const collapsedIdle = countCollapsedIdle(sc.roots, effectiveIdleMode);
    const summaryText = `완료 ${sc.completedBase + collapsedIdle} · 피어 ${sc.peers}`;
    const activeCount = countTreeNodes(tree, true);
    hdrRunChip.textContent = `실행 ${activeCount}`;
    hudSummary.textContent = summaryText;

    nodeRegistry = new Map();
    parentEdges = [];
    hudRoots.innerHTML = "";
    for (const r of tree) hudRoots.appendChild(renderNodeEl(r));

    renderLegend(Boolean(sc.showLegend) && !state.compareOld);
    renderOldCompact(tree, summaryText);

    hudView.hidden = state.compareOld;
    agcompactOld.hidden = !state.compareOld;
    agpop.hidden = !state.popOpen;
    hudExpandBtn.setAttribute("aria-expanded", String(state.popOpen));

    const totalCount = countTreeNodes(tree, false);
    agpopChips.innerHTML = `<span class="chip crun-chip">실행 ${activeCount}</span><span class="chip">표시 ${totalCount}</span>`;
    agpopTree.innerHTML = `${renderPopTree(tree)}<div class="pop-peers"><span>${summaryText}</span><span>클릭 시 세션 이동</span></div>`;

    requestAnimationFrame(() => {
      drawConnectors();
      if (state.compareOld && hudView.hidden) {
        const oldR = agcompactOld.getBoundingClientRect();
        hudFootprint.hidden = true;
        agcompactOld.querySelector(".agcompact-old-head span:last-child").textContent = `${Math.round(oldR.width)}×${Math.round(oldR.height)}px`;
      } else {
        const newR = hudView.getBoundingClientRect();
        hudFootprint.textContent = `${Math.round(newR.width)}×${Math.round(newR.height)}px`;
        hudFootprint.hidden = !state.showFootprint;
      }
      if (state.hoveredId && nodeRegistry.has(state.hoveredId)) showTooltip(state.hoveredId);
      else if (state.pinnedHoverId && nodeRegistry.has(state.pinnedHoverId)) showTooltip(state.pinnedHoverId);
      else hideTooltip();
    });
  }

  function syncControls() {
    document.querySelectorAll("[data-theme-btn]").forEach((b) => {
      b.setAttribute("aria-pressed", String(b.dataset.themeBtn === state.theme));
    });
    document.querySelectorAll("[data-width-btn]").forEach((b) => {
      b.setAttribute("aria-pressed", String(Number(b.dataset.widthBtn) === state.width));
    });
    document.querySelectorAll("[data-idle-btn]").forEach((b) => {
      b.setAttribute("aria-pressed", String(b.dataset.idleBtn === state.idleMode));
    });
    $("ctrl-scenario").value = state.scenario;
    const cmpBtn = $("ctrl-compare");
    cmpBtn.setAttribute("aria-pressed", String(state.compareOld));
    cmpBtn.textContent = state.compareOld ? "Back to New HUD" : "Compare: Before (#agcompact)";
  }

  document.querySelectorAll("[data-theme-btn]").forEach((b) => {
    b.addEventListener("click", () => { state.theme = b.dataset.themeBtn; syncControls(); renderAll(); });
  });
  document.querySelectorAll("[data-width-btn]").forEach((b) => {
    b.addEventListener("click", () => { state.width = Number(b.dataset.widthBtn); syncControls(); renderAll(); });
  });
  document.querySelectorAll("[data-idle-btn]").forEach((b) => {
    b.addEventListener("click", () => { state.idleMode = b.dataset.idleBtn; syncControls(); renderAll(); });
  });
  $("ctrl-scenario").addEventListener("change", (e) => {
    state.scenario = e.target.value;
    state.pinnedHoverId = null;
    state.hoveredId = null;
    syncControls();
    renderAll();
  });
  $("ctrl-compare").addEventListener("click", () => {
    state.compareOld = !state.compareOld;
    state.showFootprint = state.compareOld;
    state.popOpen = false;
    state.pinnedHoverId = null;
    state.hoveredId = null;
    syncControls();
    renderAll();
  });

  hudRoots.addEventListener("mouseover", (e) => {
    const btn = e.target.closest(".hud-node");
    if (btn) showTooltip(btn.dataset.id);
  });
  hudRoots.addEventListener("mouseout", (e) => {
    const btn = e.target.closest(".hud-node");
    if (btn && !btn.contains(e.relatedTarget)) hideTooltip();
  });
  hudRoots.addEventListener("focusin", (e) => {
    const btn = e.target.closest(".hud-node");
    if (btn) showTooltip(btn.dataset.id);
  });
  hudRoots.addEventListener("focusout", () => hideTooltip());
  hudRoots.addEventListener("click", (e) => {
    const btn = e.target.closest(".hud-node");
    if (btn) showToast(`Navigate → /s/${btn.dataset.id}`);
  });

  hudExpandBtn.addEventListener("click", () => {
    state.popOpen = !state.popOpen;
    hideTooltip();
    renderAll();
  });
  $("agpop-close").addEventListener("click", () => {
    state.popOpen = false;
    renderAll();
  });
  agpopTree.addEventListener("click", (e) => {
    const row = e.target.closest("[data-pop-id]");
    if (row) showToast(`Navigate → /s/${row.dataset.popId}`);
  });

  new ResizeObserver(() => drawConnectors()).observe(hudCanvas);
  window.addEventListener("resize", () => {
    drawConnectors();
    if (state.hoveredId && nodeRegistry.has(state.hoveredId)) showTooltip(state.hoveredId);
  });

  const params = new URLSearchParams(location.search);
  if (params.get("theme") === "light" || params.get("theme") === "dark") state.theme = params.get("theme");
  if (params.get("scenario") && SCENARIOS[params.get("scenario")]) state.scenario = params.get("scenario");
  if (params.get("idle") === "active" || params.get("idle") === "alive") state.idleMode = params.get("idle");
  if (params.get("width") === "420") state.width = 420;
  if (params.get("hover")) state.pinnedHoverId = params.get("hover");

  syncControls();
  renderAll();
})();
