const pptxgen = require("pptxgenjs");

// ─── Theme: Midnight Indigo ──────────────────────────────────────────
const C = {
  bg:      "0B0F19",
  surface: "141B2D",
  accent:  "818CF8",  // soft indigo
  bright:  "38BDF8",  // sky blue
  warm:    "F59E0B",  // amber
  alert:   "FB7185",  // rose
  good:    "4ADE80",  // green
  text:    "E8ECF4",
  muted:   "7C8DB0",
  dim:     "2A3352",
  white:   "FFFFFF",
  black:   "0B0F19",
};
const F = { h: "Trebuchet MS", b: "Calibri", code: "Consolas" };
const W = 10, H = 5.625; // 16:9

const pres = new pptxgen();
pres.layout = "LAYOUT_16x9";
pres.author = "Kang SW";
pres.title = "그림이 선명하면, AI는 만든다";

// ─── Helpers ─────────────────────────────────────────────────────────
function slide(bg) {
  const s = pres.addSlide();
  s.background = { color: bg || C.bg };
  return s;
}

// Full-screen statement slide
function statement(text, opts = {}) {
  const s = slide(opts.bg || C.surface);
  // Decorative top bar
  s.addShape(pres.shapes.RECTANGLE, { x: 0, y: 0, w: W, h: 0.06, fill: { color: opts.barColor || C.accent } });
  s.addText(text, {
    x: 1.0, y: 0.8, w: 8.0, h: 4.0,
    fontFace: F.h, fontSize: opts.fontSize || 36, color: opts.color || C.text,
    bold: true, valign: "middle", align: opts.align || "left",
    margin: 0, lineSpacingMultiple: 1.4,
  });
  if (opts.sub) {
    s.addText(opts.sub, {
      x: 1.0, y: 4.5, w: 8.0, h: 0.6,
      fontFace: F.b, fontSize: 14, color: C.muted, margin: 0, italic: true,
    });
  }
  return s;
}

function partLabel(s, num, title) {
  s.addText(`Part ${num}`, {
    x: 0.8, y: 0.25, w: 2, h: 0.35,
    fontFace: F.b, fontSize: 12, color: C.accent, bold: true, margin: 0,
  });
  s.addShape(pres.shapes.RECTANGLE, { x: 0.8, y: 0.6, w: 0.6, h: 0.03, fill: { color: C.accent } });
  if (title) {
    s.addText(title, {
      x: 0.8, y: 0.7, w: 8.4, h: 0.5,
      fontFace: F.h, fontSize: 26, color: C.text, bold: true, margin: 0,
    });
  }
}

function title(s, text, opts = {}) {
  s.addText(text, {
    x: opts.x || 0.8, y: opts.y || 0.3, w: opts.w || 8.4, h: 0.5,
    fontFace: F.h, fontSize: opts.size || 28, color: C.text, bold: true, margin: 0,
  });
}

function muted(s, text, x, y, w, h, opts = {}) {
  s.addText(text, {
    x, y, w, h: h || 0.35,
    fontFace: F.b, fontSize: opts.size || 14, color: opts.color || C.muted, margin: 0,
    ...(opts.italic !== false ? { italic: true } : {}),
  });
}

function box(s, x, y, w, h, color) {
  s.addShape(pres.shapes.RECTANGLE, {
    x, y, w, h, fill: { color: color || C.surface },
  });
}

function accentBox(s, text, x, y, w, h, opts = {}) {
  s.addShape(pres.shapes.RECTANGLE, {
    x, y, w, h,
    fill: { color: opts.bg || C.accent },
  });
  s.addText(text, {
    x, y, w, h,
    fontFace: F.h, fontSize: opts.size || 18, color: opts.color || C.white,
    bold: true, align: "center", valign: "middle", margin: 0.2,
  });
}

function code(s, text, x, y, w, h, opts = {}) {
  box(s, x, y, w, h, "0D1117");
  s.addText(text, {
    x: x + 0.2, y: y + 0.1, w: w - 0.4, h: h - 0.2,
    fontFace: F.code, fontSize: opts.size || 13, color: "E6EDF3",
    valign: "top", margin: 0, lineSpacingMultiple: 1.35,
  });
}

function bullets(s, items, x, y, w, h, opts = {}) {
  s.addText(items.map((t, i) => ({
    text: typeof t === "string" ? t : t.text,
    options: {
      bullet: true, breakLine: i < items.length - 1,
      fontSize: opts.size || 16, color: (typeof t === "object" && t.color) || opts.color || C.text,
      ...(typeof t === "object" && t.bold ? { bold: true } : {}),
    },
  })), {
    x, y, w, h,
    fontFace: F.b, valign: "top", margin: 0, paraSpaceAfter: opts.space || 8,
  });
}

function tbl(s, headers, rows, x, y, w, opts = {}) {
  const colW = opts.colW || headers.map(() => w / headers.length);
  const hdr = headers.map(h => ({
    text: h, options: {
      bold: true, color: C.white, fill: { color: C.dim },
      fontFace: F.b, fontSize: 13, align: "center", valign: "middle",
    }
  }));
  const data = rows.map(row =>
    row.map((cell, i) => ({
      text: cell, options: {
        color: C.text, fill: { color: C.surface },
        fontFace: F.b, fontSize: opts.cellSize || 13,
        align: i === 0 ? "left" : "center", valign: "middle",
      }
    }))
  );
  s.addTable([hdr, ...data], {
    x, y, w, colW, border: { pt: 0.5, color: C.dim }, rowH: opts.rowH || 0.5,
  });
}

function bigNum(s, num, label, x, y, numColor) {
  s.addText(num, {
    x, y, w: 2.5, h: 1.2,
    fontFace: F.h, fontSize: 64, color: numColor || C.bright, bold: true, align: "center", margin: 0,
  });
  s.addText(label, {
    x, y: y + 1.1, w: 2.5, h: 0.35,
    fontFace: F.b, fontSize: 13, color: C.muted, align: "center", margin: 0,
  });
}

// =====================================================================
// SLIDES
// =====================================================================

// ── 1. COVER ─────────────────────────────────────────────────────────
{
  const s = slide();
  // Gradient-ish layered bg
  box(s, 0, 0, W, 2.6, C.surface);
  s.addShape(pres.shapes.RECTANGLE, { x: 0, y: 2.5, w: W, h: 0.12, fill: { color: C.accent } });

  s.addText("그림이 선명하면,\nAI는 만든다.", {
    x: 0.8, y: 0.4, w: 8.4, h: 2.0,
    fontFace: F.h, fontSize: 48, color: C.white, bold: true, margin: 0,
    lineSpacingMultiple: 1.2,
  });

  s.addText("AI 시대, 개발자의 새로운 역할", {
    x: 0.8, y: 3.2, w: 8.4, h: 0.5,
    fontFace: F.b, fontSize: 20, color: C.muted, margin: 0,
  });
}

// ── 2. HOOK: 숫자 + 테제 ─────────────────────────────────────────────
{
  const s = slide();
  partLabel(s, 1, "왜 지금 이 이야기를 하는가");

  bigNum(s, "1인", "개발자", 0.5, 1.4, C.bright);
  bigNum(s, "10일", "실투입", 3.75, 1.4, C.bright);
  bigNum(s, "MVP", "완성", 7.0, 1.4, C.bright);

  s.addShape(pres.shapes.RECTANGLE, { x: 0.8, y: 3.3, w: 8.4, h: 0.015, fill: { color: C.dim } });

  bullets(s, [
    "사내 AI 학습 플랫폼 — 바닥부터 1인 개발",
    "그래프 노드 기반 웹 플랫폼, 비동기 스케줄링 전면 재구현",
    "SI 업체에서 특급 개발자 여러 명이 필요한 규모",
  ], 0.8, 3.5, 8.4, 1.8, { size: 14, color: C.muted });
}

// ── 3. 테제 선언 ─────────────────────────────────────────────────────
{
  const s = slide(C.surface);
  s.addText('"이거 빠르게 만들어줘"', {
    x: 0.8, y: 0.5, w: 8.4, h: 0.7,
    fontFace: F.h, fontSize: 28, color: C.warm, bold: true, margin: 0,
  });

  const arrows = ["→ 실행 속도가 빠른 코드?", "→ 개발을 빨리 끝내라는 뜻?", "→ 대충이라도 빨리?"];
  s.addText(arrows.map((t, i) => ({
    text: t, options: { breakLine: i < 2, fontSize: 18, color: C.muted },
  })), { x: 1.2, y: 1.5, w: 7, h: 1.5, fontFace: F.b, margin: 0, paraSpaceAfter: 6 });

  s.addText("같은 한 마디가 세 가지로 읽힌다.", {
    x: 0.8, y: 3.1, w: 8.4, h: 0.5,
    fontFace: F.b, fontSize: 18, color: C.text, margin: 0,
  });

  accentBox(s, "머릿속 그림이 선명하면, AI는 만들 수 있다.", 0.8, 3.9, 8.4, 0.8, { size: 24 });

  muted(s, '이 세미나는 그 "명료성"이 무엇인지에 대한 이야기입니다.', 0.8, 4.9, 8.4, 0.4);
}

// ── 4. Chatbot vs Agent ──────────────────────────────────────────────
{
  const s = slide();
  partLabel(s, 2, "Chatbot에서 Agent로");

  // Pipeline diagram at top
  code(s, "사람 → [맥락 복사] → AI → [결과 텍스트] → 사람 → [붙여넣기]", 0.8, 1.3, 8.4, 0.55, { size: 12 });

  // Two columns
  const colY = 2.2, colH = 2.8;
  box(s, 0.5, colY, 4.3, colH, C.surface);
  s.addShape(pres.shapes.RECTANGLE, { x: 0.5, y: colY, w: 4.3, h: 0.05, fill: { color: C.muted } });
  s.addText("Chatbot", { x: 0.5, y: colY + 0.15, w: 4.3, h: 0.4, fontFace: F.h, fontSize: 18, color: C.muted, bold: true, align: "center", margin: 0 });
  bullets(s, [
    "사람이 맥락을 골라서 복사",
    "AI가 결과 텍스트 생성",
    "사람이 결과를 골라서 붙여넣기",
  ], 0.8, colY + 0.7, 3.7, 1.6, { size: 14, color: C.muted });

  box(s, 5.2, colY, 4.3, colH, C.surface);
  s.addShape(pres.shapes.RECTANGLE, { x: 5.2, y: colY, w: 4.3, h: 0.05, fill: { color: C.bright } });
  s.addText("Agent", { x: 5.2, y: colY + 0.15, w: 4.3, h: 0.4, fontFace: F.h, fontSize: 18, color: C.bright, bold: true, align: "center", margin: 0 });
  bullets(s, [
    "사람은 의도만 전달",
    "AI가 파일 탐색, 코드 검색, 명령 실행",
    "사람은 승인 / 거부 / 수정 요청",
  ], 5.5, colY + 0.7, 3.7, 1.6, { size: 14 });

  s.addText('사람은 "무엇을", AI는 "어떻게".', {
    x: 0.8, y: 5.15, w: 8.4, h: 0.35,
    fontFace: F.h, fontSize: 16, color: C.accent, bold: true, margin: 0,
  });
}

// ── 5. Claude Code ───────────────────────────────────────────────────
{
  const s = slide();
  title(s, "Claude Code: 터미널 위의 Agent");

  bullets(s, [
    "claude 명령 하나로 시작",
    "AI가 명령을 제안 → 사람이 승인/거부 → 실행",
    "파일 읽기, 검색, 빌드, 테스트 — 전부 AI가 수행",
  ], 0.8, 1.2, 8.4, 1.8, { size: 18 });

  accentBox(s, "터미널을 배우라는 게 아닙니다.\nAI가 터미널을 대신 씁니다.", 0.8, 3.3, 8.4, 1.1, { size: 22 });

  muted(s, "다만 ls, cd, git 정도는 알아야 승인/거부 판단이 됩니다.", 0.8, 4.7, 8.4, 0.4, { size: 13 });
}

// ── 6. 보안 (merged) ─────────────────────────────────────────────────
{
  const s = slide();
  title(s, "보안 — 구분하면 쓸 수 있다");

  tbl(s,
    ["영역", "보호 수준"],
    [
      ["범용 로직 (CRUD, 유틸리티, 인프라)", "낮음"],
      ["도메인 특화 알고리즘 (비전 검사, 고객별 처리)", "보호 대상"],
      ["고객사 민감 데이터 (도면, 사양, 라인 정보)", "절대 보호"],
    ],
    0.8, 1.1, 8.4, { colW: [5.4, 3.0], rowH: 0.5 }
  );

  s.addShape(pres.shapes.RECTANGLE, { x: 0.8, y: 3.2, w: 8.4, h: 0.015, fill: { color: C.dim } });

  accentBox(s, "지금 할 수 있는 것: 개인 프로젝트로 워크플로우를 익혀두기", 0.8, 3.5, 8.4, 0.6, { size: 16, bg: C.warm, color: C.black });

  bullets(s, [
    "엔터프라이즈 계약 (데이터 학습 금지, SOC2 인증)",
    "온프레미스 / 로컬 모델 (성능은 떨어지지만 개선 중)",
  ], 0.8, 4.4, 8.4, 1.0, { size: 13, color: C.muted });
}

// ── 7. Token ─────────────────────────────────────────────────────────
{
  const s = slide();
  partLabel(s, 3, "AI의 연료를 이해하자");

  s.addText("Token = AI가 텍스트를 처리하는 최소 단위", {
    x: 0.8, y: 1.2, w: 8.4, h: 0.4,
    fontFace: F.b, fontSize: 16, color: C.muted, margin: 0,
  });

  tbl(s, ["", "영어", "한국어"],
    [
      ["1단어", "1~2 토큰", "2~3 토큰"],
      ["같은 문장", "기준", "2~3배 소모"],
    ],
    0.8, 1.8, 8.4, { colW: [2.5, 2.95, 2.95], rowH: 0.5 }
  );

  tbl(s, ["한국어", "영어"],
    [
      ["모션 컨트롤러의 직접 조작에 의한 장비 제어", "Direct equipment control via motion controller"],
      ["비전 검사 결과에 따른 불량 판정 기준을 변경한다", "Update defect criteria based on vision inspection results"],
    ],
    0.8, 3.3, 8.4, { colW: [4.2, 4.2], rowH: 0.55 }
  );
}

// ── 8. Context Window ────────────────────────────────────────────────
{
  const s = slide();
  title(s, "Context Window: AI의 책상");

  s.addText([
    { text: "프로젝트 파일 + 대화 내용 + AI의 사고", options: { fontSize: 18, color: C.text, breakLine: true } },
    { text: "= 전부 이 책상 위에 올라간다", options: { fontSize: 18, color: C.muted, breakLine: true } },
    { text: "", options: { fontSize: 10, breakLine: true } },
    { text: "한국어로 채우면 → 같은 책상에 절반의 자료만.", options: { fontSize: 18, color: C.warm, bold: true } },
  ], { x: 0.8, y: 1.2, w: 8.4, h: 2.0, fontFace: F.b, margin: 0, paraSpaceAfter: 4 });

  accentBox(s, "정보의 양이 아니라 정보의 밀도가 품질을 결정한다.", 0.8, 3.8, 8.4, 0.8, { size: 20 });
}

// ── 9. 영어 워크플로우 (merged) ──────────────────────────────────────
{
  const s = slide();
  title(s, "역할이 나뉘면, 언어도 나뉜다");

  tbl(s, ["사람", "AI"],
    [
      ["논의, 방향, 의사결정", "코드, 문서, 커밋"],
      ["구조와 의도", "디테일"],
      ["한국어", "영어"],
    ],
    0.8, 1.1, 5.0, { colW: [2.5, 2.5], rowH: 0.45 }
  );

  // Right side: workflow
  const steps = [
    { n: "1", t: "한국어로 의도를 전달" },
    { n: "2", t: "AI는 영어로 산출물 작성" },
    { n: "3", t: '"한국어로 핵심만 요약해줘"' },
    { n: "4", t: "AI가 즉시 다이제스트 제공" },
  ];
  steps.forEach((st, i) => {
    const yp = 1.1 + i * 0.65;
    s.addShape(pres.shapes.OVAL, {
      x: 6.2, y: yp, w: 0.4, h: 0.4, fill: { color: C.accent },
    });
    s.addText(st.n, {
      x: 6.2, y: yp, w: 0.4, h: 0.4,
      fontFace: F.h, fontSize: 14, color: C.white, bold: true, align: "center", valign: "middle", margin: 0,
    });
    s.addText(st.t, {
      x: 6.8, y: yp, w: 2.8, h: 0.4,
      fontFace: F.b, fontSize: 13, color: i === 2 ? C.warm : C.text, valign: "middle", margin: 0,
    });
  });

  accentBox(s, "같은 컨텍스트에 2~3배 많은 정보", 0.8, 4.0, 8.4, 0.55, { size: 16, bg: C.warm, color: C.black });

  muted(s, "영어로 일하라는 게 아닙니다. AI가 영어로 일하는 겁니다.", 0.8, 4.75, 8.4, 0.35, { size: 13 });
}

// ── 10. 비용 ─────────────────────────────────────────────────────────
{
  const s = slide();
  title(s, "비용의 현실");

  // Two big price cards
  box(s, 0.5, 1.1, 4.3, 2.3, C.surface);
  s.addShape(pres.shapes.RECTANGLE, { x: 0.5, y: 1.1, w: 4.3, h: 0.04, fill: { color: C.bright } });
  s.addText("시작", { x: 0.5, y: 1.25, w: 4.3, h: 0.3, fontFace: F.b, fontSize: 14, color: C.bright, bold: true, align: "center", margin: 0 });
  s.addText("$20", { x: 0.5, y: 1.6, w: 4.3, h: 1.0, fontFace: F.h, fontSize: 56, color: C.text, bold: true, align: "center", margin: 0 });
  s.addText("/월  —  학습, 일상적 보조", { x: 0.5, y: 2.7, w: 4.3, h: 0.3, fontFace: F.b, fontSize: 12, color: C.muted, align: "center", margin: 0 });

  box(s, 5.2, 1.1, 4.3, 2.3, C.surface);
  s.addShape(pres.shapes.RECTANGLE, { x: 5.2, y: 1.1, w: 4.3, h: 0.04, fill: { color: C.warm } });
  s.addText("도달점", { x: 5.2, y: 1.25, w: 4.3, h: 0.3, fontFace: F.b, fontSize: 14, color: C.warm, bold: true, align: "center", margin: 0 });
  s.addText("$200", { x: 5.2, y: 1.6, w: 4.3, h: 1.0, fontFace: F.h, fontSize: 56, color: C.text, bold: true, align: "center", margin: 0 });
  s.addText("/월  —  본업의 생산성을 바꾸는 수준", { x: 5.2, y: 2.7, w: 4.3, h: 0.3, fontFace: F.b, fontSize: 12, color: C.muted, align: "center", margin: 0 });

  s.addText("$200/월 = 하루 $7 = 커피 두 잔 → 1인 1개월 MVP", {
    x: 0.8, y: 3.7, w: 8.4, h: 0.4,
    fontFace: F.b, fontSize: 16, color: C.text, margin: 0,
  });

  accentBox(s, "써봐야 안다. 시작은 $20.", 0.8, 4.3, 8.4, 0.55, { size: 18 });

  muted(s, "이 비용 구조는 지속되지 않을 수 있다. 지금이 기회.", 0.8, 5.05, 8.4, 0.3, { size: 12 });
}

// ── 11. 인터랙션 A ───────────────────────────────────────────────────
{
  const s = slide("111827");
  s.addText("잠깐,\n여쭤봅니다.", {
    x: 0.8, y: 0.4, w: 8.4, h: 1.4,
    fontFace: F.h, fontSize: 44, color: C.warm, bold: true, margin: 0,
  });

  const qs = [
    "AI가 생성한 코드를 그대로 쓴 적 있나요?",
    "AI가 틀린 답을 줬던 경험이 있나요?",
  ];
  qs.forEach((q, i) => {
    const yp = 2.3 + i * 1.4;
    box(s, 0.8, yp, 8.4, 1.1, C.surface);
    s.addText(`${i + 1}`, {
      x: 1.1, y: yp + 0.15, w: 0.7, h: 0.7,
      fontFace: F.h, fontSize: 36, color: C.warm, bold: true, margin: 0,
    });
    s.addText(q, {
      x: 2.0, y: yp, w: 7.0, h: 1.1,
      fontFace: F.h, fontSize: 22, color: C.text, valign: "middle", margin: 0,
    });
  });
}

// ── 12. 두 개의 축 ───────────────────────────────────────────────────
{
  const s = slide();
  partLabel(s, 4, "개발자의 역할이 바뀌고 있다");

  const mx = 1.5, my = 1.3, mw = 7, mh = 3.6;
  // Cross
  s.addShape(pres.shapes.LINE, { x: mx + mw / 2, y: my, w: 0, h: mh, line: { color: C.dim, width: 1 } });
  s.addShape(pres.shapes.LINE, { x: mx, y: my + mh / 2, w: mw, h: 0, line: { color: C.dim, width: 1 } });

  // Labels
  muted(s, "손으로 개발", mx - 0.2, my + mh / 2 - 0.15, 1.6, 0.3, { size: 11, italic: false });
  muted(s, "AI에게 위임", mx + mw - 1.3, my + mh / 2 - 0.15, 1.4, 0.3, { size: 11, italic: false });
  muted(s, "이해 없음", mx + mw / 2 - 0.6, my - 0.05, 1.2, 0.25, { size: 11, italic: false });
  muted(s, "구조 이해", mx + mw / 2 - 0.6, my + mh - 0.2, 1.2, 0.25, { size: 11, italic: false });

  // Danger (top-right)
  s.addShape(pres.shapes.RECTANGLE, {
    x: mx + mw / 2 + 0.2, y: my + 0.2, w: mw / 2 - 0.4, h: mh / 2 - 0.4,
    fill: { color: "7F1D1D", transparency: 70 },
  });
  s.addText("위험 지대\nAI에 의존 + 구조 모름", {
    x: mx + mw / 2 + 0.2, y: my + 0.2, w: mw / 2 - 0.4, h: mh / 2 - 0.4,
    fontFace: F.b, fontSize: 14, color: C.alert, align: "center", valign: "middle", margin: 0,
  });

  // Goal (bottom-right)
  s.addShape(pres.shapes.RECTANGLE, {
    x: mx + mw / 2 + 0.2, y: my + mh / 2 + 0.2, w: mw / 2 - 0.4, h: mh / 2 - 0.4,
    fill: { color: "064E3B", transparency: 70 },
  });
  s.addText("목표 지대\nAI에게 맡김 + 구조 꿰뚫음", {
    x: mx + mw / 2 + 0.2, y: my + mh / 2 + 0.2, w: mw / 2 - 0.4, h: mh / 2 - 0.4,
    fontFace: F.b, fontSize: 14, color: C.good, align: "center", valign: "middle", margin: 0,
  });
}

// ── 13. 추상화 + 비결정성 (merged) ───────────────────────────────────
{
  const s = slide();
  title(s, "추상화의 역사 — 그리고 단절");

  // Staircase
  const labels = ["어셈블리", "C", "고급 언어", "프레임워크", "???"];
  const colors = [C.muted, C.muted, C.muted, C.bright, C.warm];
  labels.forEach((l, i) => {
    const sx = 0.6 + i * 1.85, sy = 2.8 - i * 0.35, sw = 1.6, sh = 0.5;
    box(s, sx, sy, sw, sh, C.surface);
    s.addShape(pres.shapes.RECTANGLE, { x: sx, y: sy, w: sw, h: sh, line: { color: colors[i], width: 1.5 }, fill: { color: C.surface } });
    s.addText(l, { x: sx, y: sy, w: sw, h: sh, fontFace: F.h, fontSize: 13, color: colors[i], bold: true, align: "center", valign: "middle", margin: 0 });
    if (i < 4) s.addText("→", { x: sx + sw, y: sy, w: 0.25, h: sh, fontFace: F.b, fontSize: 14, color: C.dim, align: "center", valign: "middle", margin: 0 });
  });

  // Key difference below
  box(s, 0.5, 3.5, 4.2, 1.1, C.surface);
  s.addText("기존: 결정적\n같은 코드 → 같은 결과", { x: 0.7, y: 3.6, w: 3.8, h: 0.9, fontFace: F.b, fontSize: 15, color: C.bright, align: "center", valign: "middle", margin: 0 });

  box(s, 5.3, 3.5, 4.2, 1.1, C.surface);
  s.addText("AI: 비결정적\n같은 프롬프트 → 다른 결과", { x: 5.5, y: 3.6, w: 3.8, h: 0.9, fontFace: F.b, fontSize: 15, color: C.warm, align: "center", valign: "middle", margin: 0 });

  s.addText("도구가 아니라, 사람에게 일을 맡기는 것.", {
    x: 0.8, y: 4.85, w: 8.4, h: 0.4, fontFace: F.h, fontSize: 16, color: C.accent, bold: true, margin: 0,
  });
}

// ── 14. 부하 직원 ────────────────────────────────────────────────────
statement("우리가 얻은 건 새로운 도구가 아니라,\n똑똑하고, 순종적이고, 실행력이 강하고,\n잘 넘겨짚는 부하 직원.", {
  fontSize: 30, barColor: C.warm,
  sub: '"네 맞습니다" 하면 의심하라. 지시는 명확하게. 산출물은 리뷰하라.',
});

// ── 15. 인지 강화의 함정 ─────────────────────────────────────────────
{
  const s = slide();
  partLabel(s, 5, "AI는 당신 편이 아니다");

  // Two cards
  box(s, 0.5, 1.3, 4.3, 1.6, C.surface);
  s.addShape(pres.shapes.RECTANGLE, { x: 0.5, y: 1.3, w: 4.3, h: 0.04, fill: { color: C.good } });
  s.addText("당신이 옳으면\n→ 더 옳은 방향으로 가속", { x: 0.7, y: 1.5, w: 3.9, h: 1.2, fontFace: F.b, fontSize: 18, color: C.text, valign: "middle", margin: 0 });

  box(s, 5.2, 1.3, 4.3, 1.6, C.surface);
  s.addShape(pres.shapes.RECTANGLE, { x: 5.2, y: 1.3, w: 4.3, h: 0.04, fill: { color: C.alert } });
  s.addText("당신이 틀리면\n→ 더 틀린 방향으로 가속", { x: 5.4, y: 1.5, w: 3.9, h: 1.2, fontFace: F.b, fontSize: 18, color: C.text, valign: "middle", margin: 0 });

  accentBox(s, '"네, 맞습니다"는 AI의 가장 위험한 대답이다.', 0.8, 3.3, 8.4, 0.8, { size: 22, bg: C.alert });

  // Solution
  tbl(s, ["나쁜 질문", "좋은 질문"],
    [['"이거 맞아?"', '"여기서 뭐가 틀릴 수 있어?"']],
    0.8, 4.4, 8.4, { colW: [4.2, 4.2], rowH: 0.5 }
  );

  muted(s, "교차 검증: 다른 세션, 다른 모델에게 같은 산출물을 리뷰시킨다.", 0.8, 5.1, 8.4, 0.3, { size: 12 });
}

// ── 16. 테스트 가능성 (condensed) ────────────────────────────────────
{
  const s = slide();
  title(s, "테스트 가능성의 스펙트럼");

  const levels = [
    { lbl: "Level 1", color: C.good, code: "int add(int a, int b) { return a + b; }", note: "AI 혼자 검증 가능" },
    { lbl: "Level 2", color: C.warm, code: "void move_motor_to(float x, float y);", note: "mock 설계는 사람이" },
    { lbl: "Level 3", color: C.alert, code: "float read_sensor(int index);", note: "사람만 검증 가능" },
  ];

  levels.forEach((lv, i) => {
    const yp = 1.1 + i * 1.3;
    // Level badge
    s.addShape(pres.shapes.RECTANGLE, { x: 0.8, y: yp, w: 1.2, h: 0.35, fill: { color: lv.color } });
    s.addText(lv.lbl, { x: 0.8, y: yp, w: 1.2, h: 0.35, fontFace: F.h, fontSize: 12, color: C.black, bold: true, align: "center", valign: "middle", margin: 0 });
    // Code
    code(s, lv.code, 0.8, yp + 0.45, 5.5, 0.55, { size: 11 });
    // Note
    s.addText("→ " + lv.note, { x: 6.5, y: yp + 0.45, w: 3.2, h: 0.55, fontFace: F.b, fontSize: 13, color: C.muted, valign: "middle", margin: 0 });
  });

  accentBox(s, "Level 3→2→1: 이 경계를 밀어내는 것이 개발자의 일이다.", 0.8, 4.6, 8.4, 0.6, { size: 16 });
}

// ── 17. 인터랙션 B-1 ─────────────────────────────────────────────────
{
  const s = slide("111827");
  s.addText("AI가 작성한 코드입니다.\n테스트는 전부 통과합니다.", {
    x: 0.8, y: 0.3, w: 8.4, h: 0.9,
    fontFace: F.h, fontSize: 24, color: C.text, bold: true, margin: 0,
  });

  code(s, `int add(int a, int b);  // 선언

assert(add(1, 2) == 3);       // PASS!
assert(add(2, 1) == 3);       // PASS!
assert(add(2, 1) == add(1, 2));  // PASS!
assert(add(4, -1) == 3);      // PASS!`, 0.8, 1.5, 8.4, 2.2, { size: 15 });

  accentBox(s, "4개 테스트, 전부 PASS.\n이 코드를 믿으시겠습니까?", 0.8, 4.0, 8.4, 1.1, { size: 26, bg: C.warm, color: C.black });
}

// ── 18. 인터랙션 B-2 ─────────────────────────────────────────────────
{
  const s = slide("111827");
  s.addText("구현을 봅시다.", {
    x: 0.8, y: 0.5, w: 8.4, h: 0.6,
    fontFace: F.h, fontSize: 32, color: C.text, bold: true, margin: 0,
  });

  code(s, `int add(int a, int b) {
    return 3;  // ← ???
}`, 0.8, 1.5, 8.4, 1.6, { size: 22 });

  s.addText("테스트가 통과했습니다.\n그런데 맞습니까?", {
    x: 0.8, y: 3.5, w: 8.4, h: 1.0,
    fontFace: F.h, fontSize: 30, color: C.alert, bold: true, margin: 0,
  });
}

// ── 19. 기만적 안전망 + TDD (merged) ─────────────────────────────────
{
  const s = slide();
  title(s, "테스트의 의도를 사람이 잡는다");

  bullets(s, [
    "테스트가 있다 ≠ 안전하다",
    'AI는 "테스트를 통과시키는 것"에 최적화할 수 있다',
    "사람이 봐야 하는 건 코드가 아니라 테스트의 의도",
  ], 0.8, 1.0, 8.4, 1.5, { size: 16 });

  // Two approach cards
  box(s, 0.5, 2.7, 4.3, 2.3, C.surface);
  s.addShape(pres.shapes.RECTANGLE, { x: 0.5, y: 2.7, w: 4.3, h: 0.04, fill: { color: C.accent } });
  s.addText("TDD: 테스트 먼저, 구현 나중", { x: 0.7, y: 2.85, w: 3.9, h: 0.35, fontFace: F.h, fontSize: 14, color: C.accent, bold: true, margin: 0 });
  s.addText([
    { text: "1. 테스트 의도를 사람이 정의", options: { breakLine: true, fontSize: 14, color: C.text } },
    { text: "2. 구현은 AI에게", options: { breakLine: true, fontSize: 14, color: C.text } },
    { text: "3. 통과하면 다음, 실패하면 수정 지시", options: { fontSize: 14, color: C.text } },
  ], { x: 0.8, y: 3.3, w: 3.7, h: 1.3, fontFace: F.b, margin: 0, paraSpaceAfter: 6 });

  box(s, 5.2, 2.7, 4.3, 2.3, C.surface);
  s.addShape(pres.shapes.RECTANGLE, { x: 5.2, y: 2.7, w: 4.3, h: 0.04, fill: { color: C.warm } });
  s.addText("내 말로 다시 설명하기", { x: 5.4, y: 2.85, w: 3.9, h: 0.35, fontFace: F.h, fontSize: 14, color: C.warm, bold: true, margin: 0 });
  s.addText([
    { text: '"그러니까 이건 이런 거지?"', options: { breakLine: true, fontSize: 14, color: C.text, bold: true } },
    { text: "이 한 마디가 검증.", options: { breakLine: true, fontSize: 14, color: C.text } },
    { text: "꾸준히 하면 공유된 인지 지도가 만들어진다.", options: { fontSize: 13, color: C.muted } },
  ], { x: 5.4, y: 3.3, w: 3.8, h: 1.3, fontFace: F.b, margin: 0, paraSpaceAfter: 6 });
}

// ── 20. 실전 워크스루 ────────────────────────────────────────────────
{
  const s = slide();
  partLabel(s, 6, "실전 — 이렇게 일합니다");

  const steps = [
    '"이런 기능이 필요하다" — 대화의 시작',
    "AI가 생성한 프로젝트 구조 지도",
    "구현 과정 — 핵심 대화만 발췌",
    "완성된 기능을 실행해서 보여준다",
  ];

  steps.forEach((t, i) => {
    const yp = 1.3 + i * 0.95;
    box(s, 0.8, yp, 8.4, 0.75, C.surface);
    s.addShape(pres.shapes.OVAL, { x: 1.0, y: yp + 0.12, w: 0.45, h: 0.45, fill: { color: C.accent } });
    s.addText(`${i + 1}`, { x: 1.0, y: yp + 0.12, w: 0.45, h: 0.45, fontFace: F.h, fontSize: 16, color: C.white, bold: true, align: "center", valign: "middle", margin: 0 });
    s.addText(t, { x: 1.7, y: yp, w: 7.2, h: 0.75, fontFace: F.b, fontSize: 16, color: C.text, valign: "middle", margin: 0 });
  });

  muted(s, "청중이 보는 건 코드가 아니라 협업 과정.", 0.8, 5.1, 8.4, 0.3, { size: 13 });
}

// ── 21. CLAUDE.md + 맥락 관리 ────────────────────────────────────────
{
  const s = slide();
  title(s, "CLAUDE.md = 프로젝트의 매뉴얼");

  code(s, `# Project Summary
측정 데이터 관리 API. Flask + SQLite.

# Architecture Rules
- 모든 엔드포인트는 /api/ 접두사
- 테스트는 pytest로 작성`, 0.8, 1.0, 5.0, 2.0, { size: 13 });

  // Right side: context tips
  s.addText("맥락 관리 원칙", {
    x: 6.2, y: 1.0, w: 3.5, h: 0.35,
    fontFace: F.h, fontSize: 15, color: C.accent, bold: true, margin: 0,
  });
  bullets(s, [
    "관련 없는 파일을 열지 않기",
    "큰 작업은 세션 분리",
    "코드베이스를 통째로 던지지 않는다",
    "문서로 맥락을 정제",
  ], 6.2, 1.5, 3.5, 2.0, { size: 13, color: C.muted });

  accentBox(s, "AI의 작업 메모리를 아끼는 것도 명료성의 일부다.", 0.8, 3.4, 8.4, 0.55, { size: 16 });
}

// ── 22. 좋은 지시 vs 나쁜 지시 ───────────────────────────────────────
{
  const s = slide();
  title(s, "좋은 지시 vs 나쁜 지시");

  tbl(s,
    ["나쁜 지시", "좋은 지시"],
    [
      ['"이거 고쳐줘"', '"반환값이 null일 때 크래시. null 체크 추가해줘"'],
      ['"더 좋게 만들어줘"', '"이 루프가 O(n²). 해시맵으로 O(n)으로 바꿔줘"'],
      ['"테스트 짜줘"', '"경계값 케이스 3개에 대한 유닛 테스트 작성해줘"'],
    ],
    0.8, 1.1, 8.4, { colW: [2.8, 5.6], rowH: 0.65 }
  );

  accentBox(s, "의도 + 맥락 + 구체적 기대 결과 = 좋은 지시", 0.8, 3.6, 8.4, 0.7, { size: 20 });
}

// ── 23. 시작 경로 ────────────────────────────────────────────────────
{
  const s = slide();
  partLabel(s, 7, "나만의 워크플로우를 만들어라");

  tbl(s, ["시점", "할 일"],
    [
      ["오늘", "CLAUDE.md를 만들어 본다"],
      ["한 달 후", "반복 패턴을 발견하면 Skill로 추출"],
      ["석 달 후", "나만의 작업 패턴을 자각하고 설계"],
    ],
    0.8, 1.2, 8.4, { colW: [2.0, 6.4], rowH: 0.55 }
  );

  s.addShape(pres.shapes.RECTANGLE, { x: 0.8, y: 3.1, w: 8.4, h: 0.015, fill: { color: C.dim } });

  bullets(s, [
    { text: "Skill = 반복 지시를 파일로 정의한 것", color: C.muted },
    "처음부터 만들지 않는다. 반복이 보일 때.",
    '남의 Skill을 복사하지 않는다. 맥락이 다르면 워크플로우도 다르다.',
  ], 0.8, 3.3, 8.4, 1.6, { size: 15 });
}

// ── 24. 전통적 워크플로우 + 성장 방향 (merged) ───────────────────────
{
  const s = slide();
  title(s, "1인 커버리지가 넓어졌다");

  s.addText([
    { text: "브랜치, PR, 코드 리뷰 = 인간의 역량 한계를 전제한 구조", options: { fontSize: 16, color: C.muted, breakLine: true } },
    { text: "", options: { fontSize: 8, breakLine: true } },
    { text: "AI 시대: 협업의 성격이 바뀌었다.", options: { fontSize: 18, color: C.text, bold: true } },
  ], { x: 0.8, y: 1.0, w: 8.4, h: 1.2, fontFace: F.b, margin: 0, paraSpaceAfter: 4 });

  bullets(s, [
    "이전에 팀이 필요했던 규모를 1인이 다룰 수 있다",
    '"네가 짠 코드"의 리뷰 → "네가 AI에게 시킨 코드"의 리뷰',
  ], 0.8, 2.4, 8.4, 1.0, { size: 16 });

  s.addShape(pres.shapes.RECTANGLE, { x: 0.8, y: 3.6, w: 8.4, h: 0.015, fill: { color: C.dim } });

  // Skill & Agent cards
  box(s, 0.5, 3.8, 4.3, 1.2, C.surface);
  s.addText("Skill", { x: 0.5, y: 3.9, w: 4.3, h: 0.3, fontFace: F.h, fontSize: 16, color: C.accent, bold: true, align: "center", margin: 0 });
  s.addText("반복 작업 패턴을 정의한 것", { x: 0.7, y: 4.3, w: 3.9, h: 0.4, fontFace: F.b, fontSize: 13, color: C.muted, align: "center", margin: 0 });

  box(s, 5.2, 3.8, 4.3, 1.2, C.surface);
  s.addText("Agent", { x: 5.2, y: 3.9, w: 4.3, h: 0.3, fontFace: F.h, fontSize: 16, color: C.warm, bold: true, align: "center", margin: 0 });
  s.addText("전문 영역을 담당하는 하위 AI", { x: 5.4, y: 4.3, w: 3.9, h: 0.4, fontFace: F.b, fontSize: 13, color: C.muted, align: "center", margin: 0 });
}

// ── 25. 시작하기 ─────────────────────────────────────────────────────
{
  const s = slide();
  title(s, "시작하기");

  code(s, `winget install Anthropic.ClaudeCode    # Windows
brew install claude-code               # Mac`, 0.8, 1.0, 8.4, 0.7, { size: 14 });

  const steps = [
    "빈 폴더를 만든다",
    'CLAUDE.md에 "이 프로젝트는 ___이다"를 한 줄 적는다',
    '"___를 만들어줘. 테스트도 작성해줘"',
    "AI가 코드를 쓰고 테스트를 돌리는 과정을 지켜본다",
  ];
  steps.forEach((t, i) => {
    const yp = 2.0 + i * 0.65;
    s.addShape(pres.shapes.OVAL, { x: 0.8, y: yp, w: 0.4, h: 0.4, fill: { color: C.accent } });
    s.addText(`${i + 1}`, { x: 0.8, y: yp, w: 0.4, h: 0.4, fontFace: F.h, fontSize: 14, color: C.white, bold: true, align: "center", valign: "middle", margin: 0 });
    s.addText(t, { x: 1.4, y: yp, w: 8, h: 0.4, fontFace: F.b, fontSize: 15, color: C.text, valign: "middle", margin: 0 });
  });

  accentBox(s, "기존 코드베이스에 바로 던지지 마세요.\n빈 폴더에서 작은 것부터.", 0.8, 4.5, 8.4, 0.8, { size: 18, bg: C.warm, color: C.black });
}

// ── 26. 핵심 메시지 (CLOSING) ────────────────────────────────────────
{
  const s = slide(C.accent);
  s.addText("도구는 준비되어 있다.", {
    x: 1.0, y: 1.0, w: 8.0, h: 1.0,
    fontFace: F.h, fontSize: 36, color: C.white, bold: true, align: "center", margin: 0,
  });
  s.addText("문제는 우리가 무엇을 원하는지\n얼마나 선명히 그릴 수 있느냐다.", {
    x: 1.0, y: 2.2, w: 8.0, h: 1.5,
    fontFace: F.h, fontSize: 36, color: C.white, bold: true, align: "center", margin: 0,
    lineSpacingMultiple: 1.3,
  });
  s.addShape(pres.shapes.RECTANGLE, { x: 4.0, y: 3.8, w: 2.0, h: 0.04, fill: { color: C.warm } });
  s.addText("후속 자료: CLAUDE.template.md + 첫 체험 가이드 (PPT 부록)", {
    x: 1.0, y: 4.3, w: 8.0, h: 0.5,
    fontFace: F.b, fontSize: 13, color: "C7D2FE", align: "center", margin: 0,
  });
}

// ── 27. 부록 표지 ────────────────────────────────────────────────────
{
  const s = slide(C.surface);
  s.addShape(pres.shapes.RECTANGLE, { x: 0, y: 0, w: 0.06, h: H, fill: { color: C.muted } });
  s.addText("부록", { x: 0.8, y: 1.6, w: 8, h: 0.5, fontFace: F.b, fontSize: 18, color: C.muted, margin: 0 });
  s.addText("첫 체험 가이드", { x: 0.8, y: 2.1, w: 8, h: 1.0, fontFace: F.h, fontSize: 38, color: C.text, bold: true, margin: 0 });
}

// ── 28. 부록: 사전 준비 + 절차 ───────────────────────────────────────
{
  const s = slide();
  title(s, "사전 준비 & 절차 요약");

  code(s, `# 설치
winget install Anthropic.ClaudeCode    # Windows
brew install claude-code               # Mac
winget install Python.Python.3.12      # Python (Windows)

# 시작
mkdir my-first-project && cd my-first-project
claude`, 0.8, 0.9, 8.4, 2.0, { size: 12 });

  tbl(s, ["Step", "뭘 하나", "예시"],
    [
      ["1. 물어봐라", "의도 전달", '"CRUD API를 Python으로. 구조 제안해줘"'],
      ["2. 계획해라", "순서 잡기", '"구현 순서를 잡아줘"'],
      ["3. 시켜라", "구현", '"1번부터 구현. 테스트도 같이"'],
      ["4. 정리해라", "메모리 확보", "/compact"],
      ["5. 확인해라", "검증", '"서버 실행해줘" → 브라우저 확인'],
    ],
    0.8, 3.1, 8.4, { colW: [1.4, 1.5, 5.5], rowH: 0.42, cellSize: 12 }
  );
}

// ── 29. 부록: 팁 + 주의사항 ──────────────────────────────────────────
{
  const s = slide();
  title(s, "팁 & 주의사항");

  s.addText("팁", { x: 0.8, y: 1.0, w: 2, h: 0.35, fontFace: F.h, fontSize: 16, color: C.bright, bold: true, margin: 0 });
  bullets(s, [
    '모르는 용어 → 바로 물어본다: "Flask가 뭐야?"',
    "빈 폴더이므로 승인해도 된다. 망가질 게 없다.",
    "대화 20회 이상 → /compact",
    '"목록이 최신순으로 정렬됐으면 좋겠어"',
  ], 0.8, 1.4, 8.4, 1.6, { size: 14, color: C.muted });

  s.addShape(pres.shapes.RECTANGLE, { x: 0.8, y: 3.1, w: 8.4, h: 0.015, fill: { color: C.dim } });

  s.addText("주의", { x: 0.8, y: 3.3, w: 2, h: 0.35, fontFace: F.h, fontSize: 16, color: C.alert, bold: true, margin: 0 });

  const warns = [
    { text: "기존 업무 코드에 바로 쓰지 말 것.", color: C.alert },
    { text: "$20 플랜은 할당량이 있다. 작게 시작.", color: C.warm },
    { text: '"이거 맞아?"가 아니라 "여기서 뭐가 틀릴 수 있어?"', color: C.accent },
  ];
  warns.forEach((w, i) => {
    box(s, 0.8, 3.7 + i * 0.6, 8.4, 0.5, C.surface);
    s.addText(w.text, { x: 1.0, y: 3.7 + i * 0.6, w: 8.0, h: 0.5, fontFace: F.b, fontSize: 14, color: w.color, valign: "middle", margin: 0 });
  });
}

// ── Write ────────────────────────────────────────────────────────────
const outPath = __dirname + "/seminar.pptx";
pres.writeFile({ fileName: outPath }).then(() => {
  console.log("Generated:", outPath, "— Slides:", pres.slides.length);
}).catch(err => console.error("Error:", err));
