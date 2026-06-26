#!/usr/bin/env node
// wsflow 워크플로우 세미나 v3 — 계열사 연구소(개발자) 대상 컨셉 발표
// 척추: LLM = f(context) → 에이전트는 노이즈 컬렉터 → 노이즈를 관리하는 세 레버
//        (문서화 · 언제 잘 되나 · 하네스) → wsflow란 무엇인가 → 사용법
// 실행: node 260616-wsflow-seminar-v3.js   →   wsflow-seminar-v3.pptx

const PptxGenJS = require("pptxgenjs");
const pptx = new PptxGenJS();
pptx.layout = "LAYOUT_WIDE";

const W = 13.33;
const H = 7.5;

// ── 색 의미론 (signal=cyan/green, noise=coral) ──────────────────────
const C = {
  bg:        "0D1B2A",   // page
  panel:     "13243B",   // card
  panelEdge: "27466A",   // card border
  codeBg:    "0B1626",   // context / code box
  gold:      "E8B23A",   // section number / top rule
  white:     "FFFFFF",
  mut:       "9DB2C6",   // muted body
  faint:     "6F8597",   // footer / least
  signal:    "5BC0DE",   // signal (cyan)
  signalGrn: "37C281",   // signal (green)
  noise:     "E8806B",   // noise (coral)
  noiseDeep: "E0573F",
  line:      "2FBF71",   // bright divider (slide 2)
};

const KR = "Malgun Gothic";
const MO = "Consolas";

// ── 헬퍼 ────────────────────────────────────────────────────────────
function S() {
  const s = pptx.addSlide();
  s.background = { color: C.bg };
  return s;
}

function topRule(s) {
  s.addShape(pptx.ShapeType.rect, { x: 0, y: 0, w: W, h: 0.07, fill: { color: C.gold }, line: { color: C.gold } });
}

function footer(s) {
  s.addText("wsflow 워크플로우 세미나   |   HB Solution", {
    x: 0.5, y: H - 0.36, w: W - 1, h: 0.24, fontSize: 9, color: C.faint, fontFace: KR,
  });
}

function header(s, num, section, title) {
  topRule(s);
  s.addText(
    [
      { text: num + "  ", options: { color: C.gold, bold: true } },
      { text: section, options: { color: C.white, bold: true } },
    ],
    { x: 0.5, y: 0.30, w: W - 1, h: 0.32, fontSize: 13, fontFace: KR },
  );
  s.addText(title, { x: 0.5, y: 0.66, w: W - 1, h: 0.78, fontSize: 30, bold: true, color: C.white, fontFace: KR });
  footer(s);
}

function bottomBar(s, runs) {
  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.5, y: H - 1.02, w: W - 1.0, h: 0.5, fill: { color: C.panel }, line: { color: C.panelEdge, pt: 1 }, rectRadius: 0.06,
  });
  s.addText(runs, { x: 0.74, y: H - 1.02, w: W - 1.48, h: 0.5, fontSize: 13, fontFace: KR, valign: "middle", color: C.mut });
}

// 세로 좌측 강조선이 있는 카드
function card(s, x, y, w, h, edge) {
  s.addShape(pptx.ShapeType.roundRect, { x, y, w, h, fill: { color: C.codeBg }, line: { color: C.panelEdge, pt: 1 }, rectRadius: 0.04 });
  s.addShape(pptx.ShapeType.rect, { x, y, w: 0.07, h, fill: { color: edge }, line: { color: edge } });
}

// mono chip
function chip(s, label, x, y, w, edge) {
  s.addShape(pptx.ShapeType.roundRect, { x, y, w, h: 0.42, fill: { color: C.panel }, line: { color: edge, pt: 1 }, rectRadius: 0.06 });
  s.addText(label, { x: x + 0.04, y, w: w - 0.08, h: 0.42, fontSize: 9.5, color: edge, fontFace: MO, align: "center", valign: "middle", wrap: false });
}

// ═══════════════════════════════════════════════════════════════════
// 1. 표지
// ═══════════════════════════════════════════════════════════════════
{
  const s = S();
  s.addShape(pptx.ShapeType.rect, { x: 0, y: 0, w: W, h: 0.16, fill: { color: C.gold }, line: { color: C.gold } });

  s.addText("wsflow", {
    x: 1.0, y: 1.65, w: W - 2.0, h: 1.4, fontSize: 66, bold: true, color: C.white, fontFace: KR, align: "center",
  });
  s.addText("AI 에이전트를 일하게 만드는 법", {
    x: 1.0, y: 3.05, w: W - 2.0, h: 0.6, fontSize: 22, color: C.signal, fontFace: KR, align: "center",
  });
  s.addText("노이즈 · 컨텍스트 · 워크플로우 하네스", {
    x: 1.0, y: 3.66, w: W - 2.0, h: 0.5, fontSize: 14, color: C.mut, fontFace: KR, align: "center",
  });

  s.addShape(pptx.ShapeType.rect, { x: 4.66, y: 4.5, w: 4.0, h: 0.03, fill: { color: C.line }, line: { color: C.line } });
  s.addText("HB Solution   |   2026", {
    x: 1.0, y: 4.66, w: W - 2.0, h: 0.4, fontSize: 12, color: C.faint, fontFace: KR, align: "center",
  });

  s.addShape(pptx.ShapeType.rect, { x: 0, y: H - 0.16, w: W, h: 0.16, fill: { color: C.gold }, line: { color: C.gold } });
}

// ═══════════════════════════════════════════════════════════════════
// 2. LLM = f(x)  (보내주신 슬라이드 복원)
// ═══════════════════════════════════════════════════════════════════
{
  const s = S();
  header(s, "01", "왜 wsflow인가", "LLM은 입력 컨텍스트에서 답을 고르는 함수다");

  // 밝은 그린 디바이더
  s.addShape(pptx.ShapeType.rect, { x: 0.5, y: 1.78, w: W - 1.0, h: 0.025, fill: { color: C.line }, line: { color: C.line } });

  // 수식 y = f(x)
  s.addText("y", { x: 3.2, y: 1.95, w: 1.6, h: 1.0, fontSize: 56, bold: true, color: C.signal, fontFace: MO, align: "center" });
  s.addText("=", { x: 5.0, y: 1.95, w: 1.2, h: 1.0, fontSize: 44, color: C.white, fontFace: MO, align: "center" });
  s.addText("f ( x )", { x: 6.2, y: 1.95, w: 3.9, h: 1.0, fontSize: 56, bold: true, color: C.white, fontFace: MO, align: "center" });

  s.addText("출력", { x: 3.2, y: 2.95, w: 1.6, h: 0.34, fontSize: 13, color: C.gold, fontFace: KR, align: "center" });
  s.addText("LLM", { x: 5.0, y: 2.95, w: 1.2, h: 0.34, fontSize: 13, color: C.faint, fontFace: KR, align: "center" });
  s.addText("입력 컨텍스트", { x: 6.2, y: 2.95, w: 3.9, h: 0.34, fontSize: 13, color: C.signal, fontFace: KR, align: "center" });

  // 정제된 입력 예시
  card(s, 0.5, 3.5, W - 1.0, 1.0, C.signal);
  s.addText([
    { text: "x:  ", options: { color: C.faint, fontFace: MO } },
    { text: '"나는 사과가 좋아서 어제 마트에서 사과를 ___"', options: { color: C.white, fontFace: KR } },
  ], { x: 0.78, y: 3.6, w: W - 1.5, h: 0.42, fontSize: 15, valign: "middle" });
  s.addText([
    { text: "y:  ", options: { color: C.faint, fontFace: MO } },
    { text: '"사왔다"', options: { color: C.signal, bold: true, fontFace: KR } },
  ], { x: 0.78, y: 4.02, w: W - 1.5, h: 0.42, fontSize: 15, valign: "middle" });

  // 노이즈(모순) 입력 예시
  card(s, 0.5, 4.66, W - 1.0, 1.0, C.noise);
  s.addText([
    { text: "x:  ", options: { color: C.faint, fontFace: MO } },
    { text: '"사과 사야 하는데, 어제 이미 샀고, 알레르기라 못 먹는데, 마트에서 사과를 ___"', options: { color: C.mut, fontFace: KR } },
  ], { x: 0.78, y: 4.76, w: W - 1.5, h: 0.42, fontSize: 14, valign: "middle" });
  s.addText([
    { text: "y:  ", options: { color: C.faint, fontFace: MO } },
    { text: '"사왔다?  /  안 샀다?  /  버렸다?  /  …"', options: { color: C.noise, bold: true, fontFace: KR } },
  ], { x: 0.78, y: 5.18, w: W - 1.5, h: 0.42, fontSize: 15, valign: "middle" });

  bottomBar(s, [
    { text: "입력 품질이 곧 출력 품질", options: { color: C.signal, bold: true } },
    { text: "   ·   노이즈 ↑  →  품질 ↓", options: { color: C.mut } },
  ]);
}

// ═══════════════════════════════════════════════════════════════════
// 3. 챗봇 vs 에이전트 = 노이즈 컬렉터  (보내주신 슬라이드 복원)
// ═══════════════════════════════════════════════════════════════════
{
  const s = S();
  topRule(s);
  s.addText(
    [
      { text: "01  ", options: { color: C.gold, bold: true } },
      { text: "왜 wsflow인가", options: { color: C.white, bold: true } },
    ],
    { x: 0.5, y: 0.30, w: W - 1, h: 0.32, fontSize: 13, fontFace: KR },
  );
  s.addText([
    { text: "챗봇은 쉬웠다   —   ", options: { color: C.white } },
    { text: "에이전트는 노이즈 컬렉터", options: { color: C.noise, underline: true } },
  ], { x: 0.5, y: 0.66, w: W - 1, h: 0.78, fontSize: 30, bold: true, fontFace: KR });
  footer(s);

  const colW = (W - 1.0 - 0.4) / 2;
  const lx = 0.5;
  const rx = 0.5 + colW + 0.4;
  const colY = 1.6;
  const colH = 4.4;
  const boxY = colY + 1.12;   // CONTEXT 박스 top
  const boxH = 2.55;          // CONTEXT 박스 height (3행 + 여백 수용)
  const chipY0 = boxY + 0.5;  // 첫 행 top
  const rowGap = 0.62;        // 행 간격
  const chipH = 0.44;
  const arrowY = colY + 3.84; // 박스 아래 화살표 라벨

  // 컬럼 + CONTEXT 박스 공통 골격
  function colShell(x, edge, title, sub, arrow) {
    s.addShape(pptx.ShapeType.roundRect, { x, y: colY, w: colW, h: colH, fill: { color: C.panel }, line: { color: edge, pt: 1 }, rectRadius: 0.03 });
    s.addText(title, { x: x + 0.3, y: colY + 0.22, w: colW - 0.6, h: 0.42, fontSize: 18, bold: true, color: edge, fontFace: KR });
    s.addText(sub, { x: x + 0.3, y: colY + 0.7, w: colW - 0.6, h: 0.36, fontSize: 12.5, color: C.mut, fontFace: KR });
    s.addShape(pptx.ShapeType.roundRect, { x: x + 0.3, y: boxY, w: colW - 0.6, h: boxH, fill: { color: C.codeBg }, line: { color: C.panelEdge, pt: 1 }, rectRadius: 0.03 });
    s.addText("CONTEXT", { x: x + 0.5, y: boxY + 0.14, w: colW - 1.0, h: 0.3, fontSize: 10, color: C.faint, fontFace: MO });
    s.addText("→  " + arrow, { x: x + 0.3, y: arrowY, w: colW - 0.6, h: 0.4, fontSize: 14, bold: true, color: edge, fontFace: KR });
  }

  // 챗봇 (정제) — 전폭 행 3개
  colShell(lx, C.signal, "챗봇 (Chatbot)", "사용자가 매 턴 컨텍스트를 직접 큐레이션", "정제된 입력");
  ["user_question", "prior_turn", "system_prompt"].forEach((t, i) => {
    const y = chipY0 + i * rowGap;
    s.addShape(pptx.ShapeType.roundRect, { x: lx + 0.5, y, w: colW - 1.0, h: chipH, fill: { color: C.panel }, line: { color: C.panelEdge, pt: 1 }, rectRadius: 0.05 });
    s.addText(t, { x: lx + 0.7, y, w: colW - 1.4, h: chipH, fontSize: 11.5, color: C.signal, fontFace: MO, valign: "middle" });
  });

  // 에이전트 (누적) — 4열 × 3행 칩
  colShell(rx, C.noise, "에이전트 (Agent)", "자율적으로 파일·도구 출력을 끌어옴", "노이즈 누적");
  const files = ["main.py", "config.yml", "old_log.txt", "README", "tests/…", ".env.bak", "utils.js", "grep_out", "notes.md", "deprecated/", "build.log", ".bak"];
  const cw2 = (colW - 1.0 - 0.22 * 3) / 4;
  files.forEach((f, i) => {
    const r = Math.floor(i / 4), c = i % 4;
    chip(s, f, rx + 0.5 + c * (cw2 + 0.22), chipY0 + r * rowGap, cw2, C.noise);
  });

  bottomBar(s, [
    { text: "노이즈가 덜 쌓이도록 하는 것", options: { color: C.signal, bold: true } },
    { text: "  —  에이전트 워크플로우의 최대 관심사", options: { color: C.mut } },
  ]);
}

// ═══════════════════════════════════════════════════════════════════
// 4. 레버1 — 문서화
// ═══════════════════════════════════════════════════════════════════
{
  const s = S();
  header(s, "02", "노이즈를 관리하는 세 레버", "레버 1 · 문서화 — 긁게 두지 말고, 큐레이션해서 건네라");

  s.addText("에이전트는 매 세션 백지에서 시작. 맥락을 글로 남기지 않으면 매번 처음부터 긁어 모음 → 노이즈.", {
    x: 0.5, y: 1.55, w: W - 1.0, h: 0.5, fontSize: 14, color: C.mut, fontFace: KR, lineSpacingMultiple: 1.2,
  });

  const colW = (W - 1.0 - 0.4) / 2;
  const lx = 0.5, rx = 0.5 + colW + 0.4, y = 2.2, h = 2.9;

  // 문서화 없음
  s.addShape(pptx.ShapeType.roundRect, { x: lx, y, w: colW, h, fill: { color: C.panel }, line: { color: C.noise, pt: 1 }, rectRadius: 0.03 });
  s.addText("문서 없는 에이전트", { x: lx + 0.3, y: y + 0.2, w: colW - 0.6, h: 0.4, fontSize: 16, bold: true, color: C.noise, fontFace: KR });
  ["코드베이스 전체를 매번 크롤링", '"이전에 왜 이렇게 결정했지?" → 알 수 없음', "세션이 끊기면 맥락이 증발", "사람마다 다른 방식으로 다시 지시"].forEach((t, i) => {
    s.addText("✗  " + t, { x: lx + 0.3, y: y + 0.74 + i * 0.5, w: colW - 0.6, h: 0.46, fontSize: 12.5, color: C.mut, fontFace: KR, valign: "middle" });
  });

  // 문서화 있음
  s.addShape(pptx.ShapeType.roundRect, { x: rx, y, w: colW, h, fill: { color: C.panel }, line: { color: C.signalGrn, pt: 1 }, rectRadius: 0.03 });
  s.addText("문서화된 프로젝트", { x: rx + 0.3, y: y + 0.2, w: colW - 0.6, h: 0.4, fontSize: 16, bold: true, color: C.signalGrn, fontFace: KR });
  ["결정·이유·계약을 미리 큐레이션해 주입", "커밋마다 '왜'가 함께 기록 (AI Context)", "세션을 넘어 지속되는 공유 작업기억", "누가 이어받아도 같은 맥락에서 시작"].forEach((t, i) => {
    s.addText("✓  " + t, { x: rx + 0.3, y: y + 0.74 + i * 0.5, w: colW - 0.6, h: 0.46, fontSize: 12.5, color: C.white, fontFace: KR, valign: "middle" });
  });

  bottomBar(s, [
    { text: "목표는 '맥락을 줄이기'가 아니라 ", options: { color: C.mut } },
    { text: "signal ↑ & noise ↓", options: { color: C.signal, bold: true } },
    { text: "  —  필요한 신호는 채우고, 군더더기는 제거", options: { color: C.mut } },
  ]);
}

// ═══════════════════════════════════════════════════════════════════
// 5. 레버2 — 에이전트는 언제 잘 동작하나
// ═══════════════════════════════════════════════════════════════════
{
  const s = S();
  header(s, "02", "노이즈를 관리하는 세 레버", "레버 2 · 에이전트는 언제 잘 동작하나");

  const colW = (W - 1.0 - 0.4) / 2;
  const lx = 0.5, rx = 0.5 + colW + 0.4, y = 1.7, h = 4.2;

  // 잘 됨
  s.addShape(pptx.ShapeType.roundRect, { x: lx, y, w: colW, h, fill: { color: C.panel }, line: { color: C.signalGrn, pt: 1 }, rectRadius: 0.03 });
  s.addText("잘 동작한다", { x: lx + 0.3, y: y + 0.22, w: colW - 0.6, h: 0.44, fontSize: 18, bold: true, color: C.signalGrn, fontFace: KR });
  [
    ["좁고 명확한 범위", "한 번에 한 모듈, 한 변경"],
    ["충분한 맥락", "관련 코드·결정·계약이 입력에 있음"],
    ["명확한 성공 기준", "무엇이 '완료'인지 정의됨"],
    ["검증 루프", "테스트·리뷰로 결과를 되돌려 확인"],
  ].forEach(([t, d], i) => {
    const yy = y + 0.78 + i * 0.82;
    s.addText("✓  " + t, { x: lx + 0.3, y: yy, w: colW - 0.6, h: 0.36, fontSize: 14, bold: true, color: C.white, fontFace: KR });
    s.addText(d, { x: lx + 0.62, y: yy + 0.36, w: colW - 0.9, h: 0.34, fontSize: 11.5, color: C.mut, fontFace: KR });
  });

  // 안 됨
  s.addShape(pptx.ShapeType.roundRect, { x: rx, y, w: colW, h, fill: { color: C.panel }, line: { color: C.noise, pt: 1 }, rectRadius: 0.03 });
  s.addText("흔들린다", { x: rx + 0.3, y: y + 0.22, w: colW - 0.6, h: 0.44, fontSize: 18, bold: true, color: C.noise, fontFace: KR });
  [
    ["모호한 지시", '"알아서 잘 해줘"'],
    ["거대한 범위", "한 번에 시스템 전체를 건드림"],
    ["맥락 부재", "관련 정보 없이 추측으로 채움"],
    ["정답 부재", "맞는지 확인할 방법이 없음"],
  ].forEach(([t, d], i) => {
    const yy = y + 0.78 + i * 0.82;
    s.addText("✗  " + t, { x: rx + 0.3, y: yy, w: colW - 0.6, h: 0.36, fontSize: 14, bold: true, color: C.mut, fontFace: KR });
    s.addText(d, { x: rx + 0.62, y: yy + 0.36, w: colW - 0.9, h: 0.34, fontSize: 11.5, color: C.faint, fontFace: KR });
  });

  bottomBar(s, [
    { text: "두 열의 차이 = ", options: { color: C.mut } },
    { text: "신호 대 노이즈 비율", options: { color: C.signal, bold: true } },
    { text: ".  워크플로우의 역할은 왼쪽 조건을 강제하는 것", options: { color: C.mut } },
  ]);
}

// ═══════════════════════════════════════════════════════════════════
// 6. 레버3 — 하네스 엔지니어링
// ═══════════════════════════════════════════════════════════════════
{
  const s = S();
  header(s, "02", "노이즈를 관리하는 세 레버", "레버 3 · 하네스 엔지니어링");

  s.addShape(pptx.ShapeType.roundRect, { x: 0.5, y: 1.6, w: W - 1.0, h: 0.72, fill: { color: C.codeBg }, line: { color: C.line, pt: 1 }, rectRadius: 0.05 });
  s.addText([
    { text: "모델을 더 키우는 것보다  ", options: { color: C.mut } },
    { text: "모델을 감싸는 '구조'", options: { color: C.line, bold: true } },
    { text: "  가 실사용 품질을 좌우", options: { color: C.mut } },
  ], { x: 0.7, y: 1.6, w: W - 1.4, h: 0.72, fontSize: 17, fontFace: KR, valign: "middle", align: "center" });

  s.addText("하네스(harness) = 모델 주위에서 맥락·도구·검증을 설계하는 층. 같은 모델도 하네스가 결과를 가름.", {
    x: 0.5, y: 2.5, w: W - 1.0, h: 0.5, fontSize: 13, color: C.mut, fontFace: KR, lineSpacingMultiple: 1.2,
  });

  const items = [
    ["무엇을 넣을까", "컨텍스트에 들어갈 신호를 고르고, 노이즈를 차단"],
    ["어떤 도구를", "에이전트가 쓸 행동(검색·편집·커밋)을 정의·제한"],
    ["검증 게이트", "완료를 주장하기 전에 테스트·리뷰로 되돌려 확인"],
    ["역할 분리", "논의 · 구현 · 리뷰를 분리, 단계별 맥락을 격리"],
  ];
  const cw = (W - 1.0 - 0.3 * 3) / 4;
  items.forEach(([t, d], i) => {
    const x = 0.5 + i * (cw + 0.3);
    card(s, x, 3.2, cw, 2.0, C.signal);
    s.addText(t, { x: x + 0.22, y: 3.4, w: cw - 0.36, h: 0.6, fontSize: 14, bold: true, color: C.signal, fontFace: KR, valign: "top" });
    s.addText(d, { x: x + 0.22, y: 4.0, w: cw - 0.36, h: 1.05, fontSize: 11.5, color: C.mut, fontFace: KR, lineSpacingMultiple: 1.25, valign: "top" });
  });

  bottomBar(s, [
    { text: "wsflow = 이 네 가지를 표준화한 ", options: { color: C.mut } },
    { text: "워크플로우 하네스", options: { color: C.signal, bold: true } },
  ]);
}

// ═══════════════════════════════════════════════════════════════════
// 7. wsflow란 무엇인가 (정체성 — change 아님)
// ═══════════════════════════════════════════════════════════════════
{
  const s = S();
  header(s, "03", "wsflow란 무엇인가", "필요한 절차를, 필요한 순간에, 어느 도구에든");

  s.addText("wsflow = 'AI 도구를 위한 플레이북 팩토리'. 정해진 절차를 한 번에 다 읽히지 않고, 행동하는 순간 필요한 것만 전달.", {
    x: 0.5, y: 1.55, w: W - 1.0, h: 0.56, fontSize: 14, color: C.mut, fontFace: KR, lineSpacingMultiple: 1.2,
  });

  const items = [
    ["플레이북 주입", "검증된 워크플로우를 그 순간 필요한 만큼만 에이전트에 전달 → 컨텍스트를 가볍게 유지(노이즈 ↓)"],
    ["하네스 중립", "Claude·Codex 등 어느 AI 도구에서든 같은 절차로 동작. 도구를 바꿔도 워크플로우는 그대로"],
    ["자동 기록", "결정·이유·작업 단위가 ai-docs/ 문서와 커밋에 쌓여 다음 세션·다음 사람에게 인계"],
  ];
  items.forEach(([t, d], i) => {
    const y = 2.3 + i * 1.1;
    card(s, 0.5, y, W - 1.0, 0.96, C.signal);
    s.addText(t, { x: 0.8, y: y + 0.08, w: 3.0, h: 0.8, fontSize: 15, bold: true, color: C.signal, fontFace: KR, valign: "middle" });
    s.addText(d, { x: 3.9, y: y + 0.08, w: W - 4.4, h: 0.8, fontSize: 12.5, color: C.white, fontFace: KR, valign: "middle", lineSpacingMultiple: 1.2 });
  });

  bottomBar(s, [
    { text: "'한 번에 다 읽히지 않는다'", options: { color: C.signal, bold: true } },
    { text: "  —  이것 자체가 앞서 말한 노이즈 최소화의 실천", options: { color: C.mut } },
  ]);
}

// ═══════════════════════════════════════════════════════════════════
// 8. 어떻게 쓰나
// ═══════════════════════════════════════════════════════════════════
{
  const s = S();
  header(s, "04", "어떻게 쓰나", "bootstrap → discuss ↔ proceed");

  s.addText("쓰는 법은 단순. 한 번 설정 후, 논의와 실행을 오가기.", {
    x: 0.5, y: 1.55, w: W - 1.0, h: 0.46, fontSize: 14, color: C.mut, fontFace: KR,
  });

  // bootstrap (1회)
  s.addShape(pptx.ShapeType.roundRect, { x: 0.5, y: 2.2, w: 3.4, h: 2.7, fill: { color: C.panel }, line: { color: C.gold, pt: 1 }, rectRadius: 0.04 });
  s.addText("bootstrap", { x: 0.5, y: 2.5, w: 3.4, h: 0.5, fontSize: 20, bold: true, color: C.gold, fontFace: MO, align: "center" });
  s.addText("최초 1회", { x: 0.5, y: 3.02, w: 3.4, h: 0.36, fontSize: 12, color: C.mut, fontFace: KR, align: "center" });
  s.addText("프로젝트에 워크플로우\n구조(ai-docs/)를 설치.\n이후 세션마다 자동 로드.", {
    x: 0.7, y: 3.5, w: 3.0, h: 1.2, fontSize: 12.5, color: C.white, fontFace: KR, align: "center", valign: "top", lineSpacingMultiple: 1.3,
  });

  // 화살표
  s.addText("→", { x: 4.0, y: 3.2, w: 0.7, h: 0.7, fontSize: 30, color: C.signal, fontFace: KR, align: "center", valign: "middle" });

  // discuss ↔ proceed 루프
  s.addShape(pptx.ShapeType.roundRect, { x: 4.8, y: 2.2, w: W - 0.5 - 4.8, h: 2.7, fill: { color: C.panel }, line: { color: C.signal, pt: 1 }, rectRadius: 0.04 });
  const loopW = W - 0.5 - 4.8;

  s.addShape(pptx.ShapeType.roundRect, { x: 5.2, y: 2.95, w: 3.0, h: 1.2, fill: { color: C.codeBg }, line: { color: C.signal, pt: 1 }, rectRadius: 0.06 });
  s.addText("discuss", { x: 5.2, y: 3.1, w: 3.0, h: 0.5, fontSize: 18, bold: true, color: C.signal, fontFace: MO, align: "center" });
  s.addText("방향 논의\n(코드 변경 없음)", { x: 5.2, y: 3.6, w: 3.0, h: 0.5, fontSize: 11.5, color: C.mut, fontFace: KR, align: "center", lineSpacingMultiple: 1.2 });

  s.addText("⇄", { x: 8.3, y: 3.2, w: 0.9, h: 0.7, fontSize: 30, color: C.white, fontFace: KR, align: "center", valign: "middle" });

  s.addShape(pptx.ShapeType.roundRect, { x: 9.3, y: 2.95, w: 3.0, h: 1.2, fill: { color: C.codeBg }, line: { color: C.signalGrn, pt: 1 }, rectRadius: 0.06 });
  s.addText("proceed", { x: 9.3, y: 3.1, w: 3.0, h: 0.5, fontSize: 18, bold: true, color: C.signalGrn, fontFace: MO, align: "center" });
  s.addText("실행 착수\n(구현 + 기록)", { x: 9.3, y: 3.6, w: 3.0, h: 0.5, fontSize: 11.5, color: C.mut, fontFace: KR, align: "center", lineSpacingMultiple: 1.2 });

  s.addText("논의가 무르익으면 proceed, 막히면 다시 discuss — 자연스럽게 오감.", {
    x: 4.8, y: 4.4, w: loopW, h: 0.4, fontSize: 11.5, color: C.faint, fontFace: KR, align: "center",
  });

  bottomBar(s, [
    { text: "모르면 그냥 질문.", options: { color: C.signal, bold: true } },
    { text: "  wsflow가 문서를 읽고 맥락을 들고 답함.", options: { color: C.mut } },
  ]);
}

// ═══════════════════════════════════════════════════════════════════
// 9. 마무리
// ═══════════════════════════════════════════════════════════════════
{
  const s = S();
  s.addShape(pptx.ShapeType.rect, { x: 0, y: 0, w: W, h: 0.16, fill: { color: C.gold }, line: { color: C.gold } });

  s.addText("좋은 에이전트는", { x: 1.0, y: 2.2, w: W - 2.0, h: 0.8, fontSize: 30, color: C.mut, fontFace: KR, align: "center" });
  s.addText("좋은 컨텍스트에서 나온다", { x: 1.0, y: 3.0, w: W - 2.0, h: 1.0, fontSize: 42, bold: true, color: C.white, fontFace: KR, align: "center" });

  s.addShape(pptx.ShapeType.rect, { x: 4.66, y: 4.3, w: 4.0, h: 0.03, fill: { color: C.line }, line: { color: C.line } });
  s.addText("노이즈를 덜어낸 만큼, 컨텍스트는 좋아진다.", {
    x: 1.0, y: 4.5, w: W - 2.0, h: 0.5, fontSize: 16, color: C.signal, fontFace: KR, align: "center", italic: true,
  });

  s.addText("wsflow 워크플로우 세미나   |   HB Solution", {
    x: 1.0, y: 5.6, w: W - 2.0, h: 0.4, fontSize: 12, color: C.faint, fontFace: KR, align: "center",
  });

  s.addShape(pptx.ShapeType.rect, { x: 0, y: H - 0.16, w: W, h: 0.16, fill: { color: C.gold }, line: { color: C.gold } });
}

// ── 출력 ────────────────────────────────────────────────────────────
pptx.writeFile({ fileName: "wsflow-seminar-v3.pptx" })
  .then(() => console.log("wsflow-seminar-v3.pptx 생성 완료 (슬라이드 9장)"))
  .catch(console.error);
