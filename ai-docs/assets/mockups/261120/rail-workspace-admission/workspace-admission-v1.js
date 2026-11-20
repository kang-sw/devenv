(function () {
  "use strict";

  const $ = (s, el = document) => el.querySelector(s);
  const esc = (v) => String(v ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const icon = (name, cls = "i") => `<svg class="${cls}" aria-hidden="true"><use href="#i-${name}"/></svg>`;

  function makeExplicitFixtures() {
    return [
      {
        id: "ws-devenv",
        name: "devenv",
        path: "~/devenv",
        repo: true,
        memoCount: 4,
        worktrees: [
          {
            branch: "main",
            folder: "루트",
            root: true,
            changes: 3,
            sessions: [
              { id: "d91f", label: "rail-ux-review", harness: "claude", working: true, starred: true, hosted: true, msg: "rail admission UX 정리 중", who: "", time: "2m", cur: true }
            ]
          },
          {
            branch: "feat/rail-admission",
            folder: "devenv-rail",
            root: false,
            changes: 12,
            sessions: [
              { id: "c40a", label: "rail-worker", harness: "codex", working: false, starred: false, hosted: true, msg: "bottom panel layout test", who: "", time: "9m", cur: false }
            ]
          }
        ]
      },
      {
        id: "ws-myapp",
        name: "my-app",
        path: "~/projects/my-app",
        repo: true,
        memoCount: 1,
        worktrees: [
          {
            branch: "develop",
            folder: "루트",
            root: true,
            changes: 1,
            sessions: [
              { id: "a73b", label: "ui-polish", harness: "agy", working: false, stopped: false, starred: false, hosted: false, msg: "responsive layout check", who: "", time: "14m", cur: false }
            ]
          }
        ]
      }
    ];
  }

  function makeJunkRoots(startNum, count, seqStart) {
    const harnesses = ["agy", "codex", "claude"];
    const out = [];
    for (let i = 0; i < count; i++) {
      const n = String(startNum + i).padStart(3, "0");
      const h = harnesses[(startNum + i) % harnesses.length];
      out.push({
        id: `unreg-junk-${seqStart + i}`,
        name: `TestUserStartedAgyActivity-${n}`,
        parentPath: "/private/var/folders/8x/9m2k4v/T/",
        fullPath: `/private/var/folders/8x/9m2k4v/T/TestUserStartedAgyActivity-${n}`,
        repo: false,
        sessions: [
          { id: `t${n}`, label: `agy-activity-${n}`, harness: h, time: `${2 + (i % 45)}m` }
        ]
      });
    }
    return out;
  }

  function buildScenarioData(name) {
    if (name === "fresh") {
      return {
        explicit: [],
        pending: [
          { id: "unreg-f1", name: "devenv", parentPath: "~/", fullPath: "~/devenv", repo: true, sessions: [{ id: "f101", label: "initial-setup", harness: "claude", time: "3m" }] },
          { id: "unreg-f2", name: "api-server", parentPath: "~/work/", fullPath: "~/work/api-server", repo: true, sessions: [{ id: "f102", label: "auth-middleware", harness: "codex", time: "12m" }] },
          ...makeJunkRoots(1, 1, 50),
          { id: "unreg-f4", name: "scratch", parentPath: "~/tmp/", fullPath: "~/tmp/scratch", repo: false, sessions: [{ id: "f104", label: "quick-check", harness: "agy", time: "28m" }] }
        ],
        rejected: [],
        nextJunkNum: 2,
        nextSeq: 60
      };
    }
    if (name === "normal") {
      return {
        explicit: makeExplicitFixtures(),
        pending: [
          { id: "unreg-n1", name: "side-project", parentPath: "~/workspace/", fullPath: "~/workspace/side-project", repo: true, sessions: [{ id: "n201", label: "scaffold-auth", harness: "agy", time: "4m" }, { id: "n202", label: "fix-lint", harness: "claude", time: "18m" }] },
          { id: "unreg-n2", name: "notes-vault", parentPath: "~/docs/", fullPath: "~/docs/notes-vault", repo: false, sessions: [{ id: "n203", label: "daily-notes", harness: "claude", time: "1h" }] },
          { id: "unreg-n3", name: "sandbox-cli", parentPath: "~/tmp/", fullPath: "~/tmp/sandbox-cli", repo: true, sessions: [{ id: "n204", label: "flag-parser", harness: "codex", time: "2h" }] }
        ],
        rejected: [
          { id: "rej-n1", name: "old-tmp-build", parentPath: "/private/var/folders/8x/9m2k4v/T/", fullPath: "/private/var/folders/8x/9m2k4v/T/old-tmp-build", repo: false, sessions: [{ id: "rn01", label: "tmp-run", harness: "agy", time: "1d" }] }
        ],
        nextJunkNum: 1,
        nextSeq: 70
      };
    }
    // flood (default): 2 explicit + 30 pending (2 realistic + 28 junk) + 3 rejected
    const junk2 = makeJunkRoots(1, 2, 10);
    const junkRest = makeJunkRoots(3, 26, 20);
    return {
      explicit: makeExplicitFixtures(),
      pending: [
        { id: "unreg-side", name: "side-project", parentPath: "~/workspace/", fullPath: "~/workspace/side-project", repo: true, sessions: [{ id: "s101", label: "scaffold-auth", harness: "agy", time: "3m" }, { id: "s102", label: "fix-lint", harness: "claude", time: "16m" }] },
        ...junk2,
        { id: "unreg-scratch", name: "scratch-notes", parentPath: "~/tmp/", fullPath: "~/tmp/scratch-notes", repo: false, sessions: [{ id: "s104", label: "review-261120", harness: "claude", time: "7m" }] },
        ...junkRest
      ],
      rejected: [
        { id: "rej-1", name: "TestUserStartedAgyActivity-000", parentPath: "/private/var/folders/8x/9m2k4v/T/", fullPath: "/private/var/folders/8x/9m2k4v/T/TestUserStartedAgyActivity-000", repo: false, sessions: [{ id: "r000", label: "agy-activity-000", harness: "agy", time: "3h" }] },
        { id: "rej-2", name: "tmp-playwright-smoke", parentPath: "/private/var/folders/8x/9m2k4v/T/", fullPath: "/private/var/folders/8x/9m2k4v/T/tmp-playwright-smoke", repo: false, sessions: [{ id: "r001", label: "pw-smoke", harness: "codex", time: "5h" }] },
        { id: "rej-3", name: "empty-sandbox-912", parentPath: "~/tmp/", fullPath: "~/tmp/empty-sandbox-912", repo: false, sessions: [{ id: "r002", label: "sandbox", harness: "pi", time: "1d" }] }
      ],
      nextJunkNum: 28,
      nextSeq: 100
    };
  }

  const state = {
    theme: "dark",
    scenario: "flood",
    compareBefore: false,
    unregOpen: false,
    unregRejOpen: false,
    filterText: "",
    expandedSessions: new Set(),
    collapsedExplicit: new Set(),
    drafts: new Map(), // id -> "approve" | "reject"
    data: buildScenarioData("flood"),
    toastTimer: 0
  };

  function renderSessionRow(s) {
    const cls = ["rs", s.cur ? "cur" : "", s.stopped ? "stopped" : ""].filter(Boolean).join(" ");
    const star = s.starred ? `<span class="hstar" title="고정됨">${icon("pin")}</span>` : "";
    const ring = s.working ? `<span class="hbr"></span>` : "";
    const hosted = s.hosted ? `<span class="hosted" title="호스트 세션">${icon("hosted")}</span>` : "";
    const msg = s.msg ? `<a class="drmsg" href="#/s/${esc(s.id)}"><span class="s2">${esc(s.msg)}</span></a>` : "";
    return `<div class="${cls}" data-rid="${esc(s.id)}">
      <span class="drgrip" aria-hidden="true">${icon("grip")}</span>
      <div class="drbody">
        <div class="s1">
          <a class="rl" href="#/s/${esc(s.id)}">
            <span class="hiw">
              <span class="hb" data-h="${esc(s.harness)}">${ring}</span>
              ${star}
            </span>
            <span class="rid lbl">${esc(s.label)}</span>
            ${hosted}
          </a>
          <a class="drtail" href="#/s/${esc(s.id)}" tabindex="-1">
            <span class="drslot"><time class="when t">${esc(s.time)}</time></span>
          </a>
        </div>
        ${msg}
      </div>
    </div>`;
  }

  function renderExplicitWorkspace(ws) {
    const collapsed = state.collapsedExplicit.has(ws.id);
    const liveCount = ws.worktrees.reduce((n, wt) => n + wt.sessions.filter((s) => !s.stopped).length, 0);
    const wtsHtml = ws.worktrees.map((wt) => {
      const rootMark = wt.root ? `<span class="wtroot" title="루트">${icon("repo")}</span>` : "";
      const gs = wt.changes ? `<span class="gs">±${wt.changes}</span>` : "";
      const sessHtml = wt.sessions.length ? `<div class="rsessions">${wt.sessions.map(renderSessionRow).join("")}</div>` : "";
      return `<div class="rwt">
        <div class="wthd" tabindex="0" role="link" title="${esc(ws.path)}">
          ${icon("branch")}
          <span class="br">${esc(wt.branch)}</span>
          ${rootMark}
          <span class="wf">· ${esc(wt.folder)}</span>
          <button type="button" class="rnew" title="새 세션" aria-label="새 세션">${icon("plus")}</button>
          ${gs}
        </div>
        ${sessHtml}
      </div>`;
    }).join("");

    return `<section class="rws${collapsed ? " col" : ""}" data-ws="${esc(ws.id)}">
      <div class="wsbar">
        <button type="button" class="wstog" data-tog-ws="${esc(ws.id)}" aria-expanded="${!collapsed}" title="${esc(ws.path)}">
          <span class="wsi">${icon(ws.repo ? "repo" : "wsfolder")}</span>
          <span class="nm">${esc(ws.name)}</span>
          <span class="pp">${esc(ws.path)}</span>
        </button>
        <button type="button" class="radd" title="워크트리 추가" aria-label="워크트리 추가">${icon("plus")}</button>
        <button type="button" class="rforget" data-forget-ws="${esc(ws.id)}" title="${esc(ws.name)} 목록에서 제거" aria-label="제거">${icon("x")}</button>
        <button type="button" class="memo-count" title="워크스페이스 메모">▤ <span>${ws.memoCount}</span></button>
        <span class="ct">${liveCount}</span>
      </div>
      <div class="wsbody">${wtsHtml}</div>
    </section>`;
  }

  function renderEmptyGuidance() {
    const pendCount = state.data.pending.length;
    const hint = pendCount > 0
      ? `<div class="rail-empty-hint">아래 <b>미등록 워크스페이스 (${pendCount})</b>를 열어 기존 세션 폴더를 바로 승인할 수 있습니다 ↓</div>`
      : "";
    return `<div class="rail-empty-guide">
      <div class="rail-empty-icon">${icon("wsfolder")}</div>
      <div class="rail-empty-title">등록된 워크스페이스가 없습니다</div>
      <p class="rail-empty-sub">작업 중인 Git 저장소나 폴더를 추가하면 여기에서 워크트리와 세션을 깔끔하게 관리합니다.</p>
      <button type="button" class="rail-empty-add" id="empty-add-ws">${icon("plus")}워크스페이스</button>
      ${hint}
    </div>`;
  }

  function getFilteredPending() {
    const q = state.filterText.trim().toLowerCase();
    if (!q) return state.data.pending;
    return state.data.pending.filter((item) =>
      item.name.toLowerCase().includes(q) || item.fullPath.toLowerCase().includes(q)
    );
  }

  function renderPendingRow(item) {
    const draft = state.drafts.get(item.id) || "";
    const modCls = draft === "approve" ? " m-ap" : draft === "reject" ? " m-rj" : "";
    const expanded = state.expandedSessions.has(item.id);
    const sessRows = expanded
      ? `<div class="unreg-sessions">${item.sessions.map((s) => `
          <a class="unreg-slink" href="#/s/${esc(s.id)}" data-open-session="${esc(s.id)}">
            <span class="hb sm" data-h="${esc(s.harness)}"></span>
            <span class="unreg-stitle">${esc(s.label)}</span>
            <span class="unreg-stime">${esc(s.time)}</span>
          </a>`).join("")}</div>`
      : "";
    return `<div class="rwt d-pend unreg-row${modCls}" data-unreg-id="${esc(item.id)}">
      <div class="unreg-row-main">
        <span class="tgs">
          <button type="button" class="tg ap" data-act="approve" data-id="${esc(item.id)}" aria-pressed="${draft === "approve"}" title="워크스페이스로 승인" aria-label="${esc(item.name)} 승인">${icon("check")}</button>
          <button type="button" class="tg rj" data-act="reject" data-id="${esc(item.id)}" aria-pressed="${draft === "reject"}" title="미등록 목록에서 거부" aria-label="${esc(item.name)} 거부">${icon("x")}</button>
        </span>
        <div class="unreg-info" title="${esc(item.fullPath)}">
          <div class="unreg-name">${esc(item.name)}</div>
          <div class="unreg-parent">${esc(item.parentPath)}</div>
        </div>
        <button type="button" class="unreg-scount" data-tog-sess="${esc(item.id)}" aria-expanded="${expanded}" title="세션 ${item.sessions.length}개 보기">${item.sessions.length}</button>
      </div>
      ${sessRows}
    </div>`;
  }

  function renderUnregisteredPanel() {
    const total = state.data.pending.length;
    const rejected = state.data.rejected;
    const filtered = getFilteredPending();
    const visibleAllRejected = filtered.length > 0 && filtered.every((it) => state.drafts.get(it.id) === "reject");
    const draftTotal = state.drafts.size;
    const draftPill = !state.unregOpen && draftTotal > 0 ? `<span class="unreg-draft-pill">선택 ${draftTotal}</span>` : "";
    const showFilter = total > 10;
    const filterHtml = showFilter ? `<div class="unreg-filter-wrap">
      ${icon("search")}
      <input type="search" class="unreg-filter" id="unreg-filter" placeholder="필터 (${total})..." value="${esc(state.filterText)}" aria-label="미등록 워크스페이스 필터">
    </div>` : "";
    const rowsHtml = filtered.length
      ? filtered.map(renderPendingRow).join("")
      : `<div class="unreg-empty-filter">${total ? "검색 결과가 없습니다" : "대기 중인 미등록 워크스페이스 없음"}</div>`;

    const rejHtml = rejected.length ? `<details class="rrej" id="unreg-rrej"${state.unregRejOpen ? " open" : ""}>
      <summary>${icon("x")}<span class="fl">거부됨 <b class="d-n">(${rejected.length})</b></span></summary>
      <div class="rrej-list">${rejected.map((r) => `<div class="rrej-row" data-rej-id="${esc(r.id)}">
        <div class="rrej-row-info" title="${esc(r.fullPath)}">
          <div class="rrej-row-name">${esc(r.name)}</div>
          <div class="rrej-row-path">${esc(r.parentPath)}</div>
        </div>
        <button type="button" class="admission-undo" data-undo-rej="${esc(r.id)}" title="다시 대기 목록으로 되돌리기">${icon("undo")}되돌리기</button>
      </div>`).join("")}</div>
    </details>` : "";

    return `<details class="rws-unreg" data-key="rail-unregistered-workspaces"${state.unregOpen ? " open" : ""}>
      <summary class="unreg-sum">
        ${icon("chev-right", "i unreg-chev")}
        <span class="unreg-dot${total === 0 ? " zero" : ""}"></span>
        <span class="unreg-lbl">미등록 워크스페이스</span>
        ${draftPill}
        <span class="unreg-count">${total}</span>
      </summary>
      <div class="unreg-body">
        <div class="unreg-toolbar">
          <span class="unreg-pend-stat">대기 <span class="pn">${total}</span></span>
          <button type="button" class="unreg-reject-all" id="unreg-reject-all" aria-pressed="${visibleAllRejected}"${filtered.length === 0 ? " disabled" : ""}>${visibleAllRejected ? "모두 거부 해제" : "모두 거부"}</button>
        </div>
        ${filterHtml}
        <div class="unreg-list" id="unreg-list">${rowsHtml}</div>
        ${rejHtml}
      </div>
    </details>`;
  }

  function renderLegacyFloodedRail() {
    const junkTop = state.data.pending.map((item) => {
      const sessHtml = item.sessions.map((s) => renderSessionRow({
        id: s.id, label: s.label, harness: s.harness, working: false, stopped: true, time: s.time
      })).join("");
      return `<section class="rws" data-ws="${esc(item.id)}">
        <div class="wsbar">
          <button type="button" class="wstog" title="${esc(item.fullPath)}">
            <span class="wsi">${icon(item.repo ? "repo" : "wsfolder")}</span>
            <span class="nm">${esc(item.name)}</span>
            <span class="pp">${esc(item.parentPath)}</span>
          </button>
          <span class="ct">${item.sessions.length}</span>
        </div>
        <div class="wsbody"><div class="rwt"><div class="rsessions">${sessHtml}</div></div></div>
      </section>`;
    }).join("");
    const explicitHtml = state.data.explicit.map(renderExplicitWorkspace).join("");
    return `<div class="rail-before-note">현재 방식: 미등록 ${state.data.pending.length}개가 상단 .rws를 점령해 실제 워크스페이스가 아래로 밀림</div>${junkTop}${explicitHtml}`;
  }

  function updateCapsuleAndHeader() {
    const rail = $("#rail");
    const cap = $("#rcap");
    const okEl = $("#cap-ok");
    const noEl = $("#cap-no");
    let ap = 0, rj = 0;
    for (const v of state.drafts.values()) {
      if (v === "approve") ap++;
      else if (v === "reject") rj++;
    }
    const showCap = !state.compareBefore && (ap + rj > 0);
    cap.hidden = !showCap;
    rail.classList.toggle("capon", showCap);
    okEl.hidden = ap === 0;
    okEl.textContent = `승인 ${ap}`;
    noEl.hidden = rj === 0;
    noEl.textContent = `거부 ${rj}`;

    const liveExplicit = state.data.explicit.reduce((sum, ws) =>
      sum + ws.worktrees.reduce((s2, wt) => s2 + wt.sessions.filter((s) => !s.stopped).length, 0), 0);
    const livePending = state.data.pending.reduce((sum, p) => sum + p.sessions.length, 0);
    $("#hdr-live-chip").textContent = `라이브 ${liveExplicit + livePending}`;
  }

  function updateInspector() {
    const pill = $("#insp-mode-pill");
    const title = $("#insp-title");
    const desc = $("#insp-desc");
    const metrics = $("#insp-metrics");
    if (state.compareBefore) {
      pill.textContent = "Before (Current Flooded)";
      pill.classList.add("before");
      title.textContent = "Unbounded Top-Level Flood";
      desc.innerHTML = "Every unregistered session root renders as a top-level <code>.rws</code> section, burying real explicit workspaces.";
    } else {
      pill.textContent = "Redesign (v1)";
      pill.classList.remove("before");
      title.textContent = "Unregistered Workspace Admission";
      desc.innerHTML = "Explicit workspaces stay clean at the top of <code>#rlist</code>. All unregistered roots collapse into one quiet 32px footer bar until expanded.";
    }
    metrics.innerHTML = [
      ["Top-level .rws sections", state.compareBefore ? state.data.explicit.length + state.data.pending.length : state.data.explicit.length],
      ["Unregistered pending", state.data.pending.length],
      ["Rejected archive", state.data.rejected.length],
      ["Drafted (Approve / Reject)", `${[...state.drafts.values()].filter((v) => v === "approve").length} / ${[...state.drafts.values()].filter((v) => v === "reject").length}`]
    ].map(([k, v]) => `<li><span>${esc(k)}</span><b>${esc(v)}</b></li>`).join("");
  }

  function render() {
    document.documentElement.setAttribute("data-theme", state.theme);
    document.querySelectorAll("[data-theme-btn]").forEach((b) => {
      b.setAttribute("aria-pressed", String(b.dataset.themeBtn === state.theme));
    });
    $("#ctrl-scenario").value = state.scenario;
    $("#ctrl-compare").setAttribute("aria-pressed", String(state.compareBefore));

    const rlist = $("#rlist");
    const prevScroll = rlist.scrollTop;
    const unregEl = rlist.querySelector(".unreg-list");
    const prevUnregScroll = unregEl ? unregEl.scrollTop : 0;
    const filterWasFocused = document.activeElement && document.activeElement.id === "unreg-filter";
    const selStart = filterWasFocused ? document.activeElement.selectionStart : null;

    if (state.compareBefore) {
      rlist.innerHTML = renderLegacyFloodedRail();
    } else {
      const explicitHtml = state.data.explicit.length
        ? state.data.explicit.map(renderExplicitWorkspace).join("")
        : renderEmptyGuidance();
      rlist.innerHTML = explicitHtml + renderUnregisteredPanel();
    }

    rlist.scrollTop = prevScroll;
    const nextUnregEl = rlist.querySelector(".unreg-list");
    if (nextUnregEl) nextUnregEl.scrollTop = prevUnregScroll;
    if (filterWasFocused) {
      const inp = $("#unreg-filter");
      if (inp) {
        inp.focus();
        if (selStart !== null) inp.setSelectionRange(selStart, selStart);
      }
    }
    updateCapsuleAndHeader();
    updateInspector();
  }

  function showToast(msg, sub = "", ok = false) {
    const el = $("#rtoast");
    el.className = `rtoast on${ok ? " ok" : ""}`;
    el.innerHTML = `${esc(msg)}${sub ? `<small>${esc(sub)}</small>` : ""}`;
    clearTimeout(state.toastTimer);
    state.toastTimer = setTimeout(() => el.classList.remove("on"), 2800);
  }

  function applyDrafts() {
    let apCount = 0, rjCount = 0;
    const keepPending = [];
    for (const item of state.data.pending) {
      const d = state.drafts.get(item.id);
      if (d === "approve") {
        apCount++;
        state.data.explicit.push({
          id: `ws-${item.id}`,
          name: item.name,
          path: item.fullPath,
          repo: item.repo,
          memoCount: 0,
          worktrees: [{
            branch: "main",
            folder: "루트",
            root: true,
            changes: 0,
            sessions: item.sessions.map((s) => ({ ...s, working: false, stopped: false, hosted: false, msg: "" }))
          }]
        });
      } else if (d === "reject") {
        rjCount++;
        state.data.rejected.unshift(item);
      } else {
        keepPending.push(item);
      }
    }
    state.data.pending = keepPending;
    state.drafts.clear();
    render();
    showToast(`적용 완료: 승인 ${apCount} · 거부 ${rjCount}`, apCount ? "승인된 워크스페이스가 상단 목록에 추가되었습니다." : "거부된 항목은 아래 거부됨 목록으로 이동했습니다.", true);
  }

  // Controls bar & rail interactions
  document.addEventListener("click", (e) => {
    const themeBtn = e.target.closest("[data-theme-btn]");
    if (themeBtn) { state.theme = themeBtn.dataset.themeBtn; render(); return; }

    if (e.target.closest("#ctrl-compare")) { state.compareBefore = !state.compareBefore; render(); return; }
    if (e.target.closest("#ctrl-sim-junk")) {
      const added = makeJunkRoots(state.data.nextJunkNum, 10, state.data.nextSeq);
      state.data.nextJunkNum += 10;
      state.data.nextSeq += 10;
      state.data.pending.push(...added);
      render();
      showToast(`미등록 워크스페이스 +10 추가됨 (총 ${state.data.pending.length}개)`, state.unregOpen ? "" : "패널은 접힌 상태로 유지됩니다.", true);
      return;
    }
    if (e.target.closest("#insp-reset")) {
      state.data = buildScenarioData(state.scenario);
      state.unregOpen = false;
      state.unregRejOpen = false;
      state.filterText = "";
      state.expandedSessions.clear();
      state.drafts.clear();
      render();
      return;
    }

    const sumEl = e.target.closest("details.rws-unreg > summary.unreg-sum");
    if (sumEl) { e.preventDefault(); state.unregOpen = !state.unregOpen; render(); return; }

    const rejSum = e.target.closest("details#unreg-rrej > summary");
    if (rejSum) { e.preventDefault(); state.unregRejOpen = !state.unregRejOpen; render(); return; }

    const wsTog = e.target.closest("[data-tog-ws]");
    if (wsTog) {
      const id = wsTog.dataset.togWs;
      if (state.collapsedExplicit.has(id)) state.collapsedExplicit.delete(id);
      else state.collapsedExplicit.add(id);
      render();
      return;
    }

    const forgetBtn = e.target.closest("[data-forget-ws]");
    if (forgetBtn) {
      e.stopPropagation();
      const id = forgetBtn.dataset.forgetWs;
      state.data.explicit = state.data.explicit.filter((w) => w.id !== id);
      render();
      showToast("워크스페이스를 목록에서 제거했습니다", "디스크 파일은 삭제되지 않습니다.");
      return;
    }

    if (e.target.closest("#unreg-reject-all")) {
      const visible = getFilteredPending();
      const allRejected = visible.length > 0 && visible.every((it) => state.drafts.get(it.id) === "reject");
      for (const it of visible) {
        if (allRejected) state.drafts.delete(it.id);
        else state.drafts.set(it.id, "reject");
      }
      render();
      return;
    }

    const tgBtn = e.target.closest(".tg[data-act]");
    if (tgBtn) {
      const id = tgBtn.dataset.id;
      const act = tgBtn.dataset.act;
      if (state.drafts.get(id) === act) state.drafts.delete(id);
      else state.drafts.set(id, act);
      render();
      return;
    }

    const scBtn = e.target.closest("[data-tog-sess]");
    if (scBtn) {
      const id = scBtn.dataset.togSess;
      if (state.expandedSessions.has(id)) state.expandedSessions.delete(id);
      else state.expandedSessions.add(id);
      render();
      return;
    }

    const slink = e.target.closest("[data-open-session]");
    if (slink) {
      e.preventDefault();
      showToast(`세션 열기: /s/${slink.dataset.openSession}`, "미등록 상태에서도 세션 기록을 바로 확인할 수 있습니다.", true);
      return;
    }

    const undoBtn = e.target.closest("[data-undo-rej]");
    if (undoBtn) {
      const id = undoBtn.dataset.undoRej;
      const idx = state.data.rejected.findIndex((r) => r.id === id);
      if (idx >= 0) {
        const [item] = state.data.rejected.splice(idx, 1);
        state.data.pending.unshift(item);
        render();
        showToast(`대기 목록으로 복원됨: ${item.name}`, "", true);
      }
      return;
    }

    if (e.target.closest("#cap-clear")) { state.drafts.clear(); render(); return; }
    if (e.target.closest("#cap-apply")) { applyDrafts(); return; }
    if (e.target.closest("#empty-add-ws") || e.target.closest("#hdr-add-ws")) {
      showToast("워크스페이스 추가 다이얼로그", "경로나 Git 저장소를 선택해 명시적으로 등록합니다.");
    }
  });

  document.addEventListener("input", (e) => {
    if (e.target && e.target.id === "unreg-filter") {
      state.filterText = e.target.value;
      render();
    }
  });

  $("#ctrl-scenario").addEventListener("change", (e) => {
    state.scenario = e.target.value;
    state.data = buildScenarioData(state.scenario);
    state.unregOpen = false;
    state.unregRejOpen = false;
    state.filterText = "";
    state.expandedSessions.clear();
    state.drafts.clear();
    render();
  });

  window.__railMockup = {
    setState(patch) {
      if (patch.scenario && patch.scenario !== state.scenario) {
        state.scenario = patch.scenario;
        state.data = buildScenarioData(state.scenario);
      }
      if (patch.theme) state.theme = patch.theme;
      if (typeof patch.compareBefore === "boolean") state.compareBefore = patch.compareBefore;
      if (typeof patch.unregOpen === "boolean") state.unregOpen = patch.unregOpen;
      if (typeof patch.unregRejOpen === "boolean") state.unregRejOpen = patch.unregRejOpen;
      if (typeof patch.filterText === "string") state.filterText = patch.filterText;
      if (Array.isArray(patch.expandedSessions)) state.expandedSessions = new Set(patch.expandedSessions);
      if (patch.drafts) state.drafts = new Map(Object.entries(patch.drafts));
      render();
    },
    getMetrics() {
      const rail = $("#rail");
      const rlist = $("#rlist");
      const unregList = $("#unreg-list");
      return {
        railWidth: rail.getBoundingClientRect().width,
        railHeight: rail.getBoundingClientRect().height,
        railScrollWidth: rail.scrollWidth,
        railClientWidth: rail.clientWidth,
        rlistScrollWidth: rlist.scrollWidth,
        rlistClientWidth: rlist.clientWidth,
        unregScrollWidth: unregList ? unregList.scrollWidth : 0,
        unregClientWidth: unregList ? unregList.clientWidth : 0,
        unregOpen: Boolean($("details.rws-unreg")?.open),
        capHidden: $("#rcap").hidden
      };
    }
  };

  render();
})();
