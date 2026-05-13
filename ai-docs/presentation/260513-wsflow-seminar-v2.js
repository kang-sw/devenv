#!/usr/bin/env node
// wsflow 워크플로우 세미나 v2 — 빠른 시작 가이드
// 실행: node 260513-wsflow-seminar-v2.js
// 출력: wsflow-seminar-v2.pptx

const PptxGenJS = require("pptxgenjs");
const pptx = new PptxGenJS();
pptx.layout = "LAYOUT_WIDE";

const W = 13.33;
const H = 7.5;

const C = {
  navy:    "1B2F5A",
  navyMid: "2C4A7C",
  sky:     "5BB4DC",
  skyDim:  "3A8DB5",
  white:   "FFFFFF",
  offWhite:"EBF4FA",
  userBg:  "EAF4FF",
  gray:    "B0BEC5",
  darkGray:"546E7A",
  black:   "0D1B2A",
  codeBg:  "0F1E36",
  green:   "2E7D52",
  amber:   "8A6F10",
  purple:  "5E35A0",
};

// ── 헬퍼 ───────────────────────────────────────────────────────────

function S(dark = false) {
  const s = pptx.addSlide();
  s.background = { color: dark ? C.navy : C.white };
  return s;
}

function hdr(s, title, dark = false) {
  const bg = dark ? C.navyMid : C.offWhite;
  s.addShape(pptx.ShapeType.rect, { x:0, y:0, w:W, h:0.72, fill:{color:bg}, line:{color:bg} });
  s.addText(title, {
    x:0.38, y:0.06, w:W-0.76, h:0.6,
    fontSize:18, bold:true, color: dark ? C.white : C.navy, fontFace:"Malgun Gothic",
  });
  s.addShape(pptx.ShapeType.rect, { x:0, y:0.72, w:W, h:0.05, fill:{color:C.sky}, line:{color:C.sky} });
}

function ftr(s) {
  s.addShape(pptx.ShapeType.rect, { x:0, y:H-0.28, w:W, h:0.28, fill:{color:C.navy}, line:{color:C.navy} });
  s.addText("wsflow 워크플로우 세미나  |  HB Solution", {
    x:0.35, y:H-0.28, w:W*0.6, h:0.28, fontSize:8, color:C.gray, fontFace:"Malgun Gothic",
  });
}

function code(s, text, opts = {}) {
  const x = opts.x ?? 0.38, y = opts.y ?? 1.1;
  const w = opts.w ?? W-0.76, h = opts.h ?? 0.9;
  s.addShape(pptx.ShapeType.rect, { x, y, w, h, fill:{color:C.codeBg}, line:{color:C.navyMid, pt:1}, radius:3 });
  s.addText(text, {
    x:x+0.2, y:y+0.1, w:w-0.4, h:h-0.2,
    fontSize: opts.fs ?? 14, fontFace:"Cascadia Code", color:C.sky,
    valign:"top", wrap:true, lineSpacingMultiple:1.3,
  });
}

function pill(s, label, x, y, w, bg, fg) {
  const h = 0.44;
  s.addShape(pptx.ShapeType.roundRect, { x, y, w, h, fill:{color:bg??C.sky}, line:{color:bg??C.sky}, arcSize:50 });
  s.addText(label, { x, y, w, h, fontSize:13, bold:true, color:fg??C.white, fontFace:"Malgun Gothic", align:"center", valign:"middle" });
}

// 말풍선 (side: "user" | "bot")
function bubble(s, text, side, y, h) {
  const isUser = side === "user";
  const x  = isUser ? 0.38 : 3.0;
  const bw = W - 3.38;     // 약 9.95"
  const bg = isUser ? C.userBg : C.navyMid;
  const tc = isUser ? C.black : C.white;
  const lc = isUser ? C.skyDim : C.sky;
  s.addShape(pptx.ShapeType.roundRect, { x, y, w:bw, h, fill:{color:bg}, line:{color:lc, pt:1}, arcSize:6 });
  s.addText(isUser ? "사용자" : "wsflow", {
    x:x+0.2, y:y+0.1, w:3.5, h:0.3,
    fontSize:9.5, color: isUser ? C.skyDim : C.sky, fontFace:"Malgun Gothic", bold:true,
  });
  s.addText(text, {
    x:x+0.2, y:y+0.42, w:bw-0.4, h:h-0.54,
    fontSize:16, color:tc, fontFace:"Malgun Gothic", valign:"top", wrap:true, lineSpacingMultiple:1.35,
  });
}

// 디테일 배지 (▶ 상세)
function detailBadge(s) {
  s.addShape(pptx.ShapeType.roundRect, {
    x:W-1.28, y:0.14, w:0.98, h:0.3, fill:{color:C.skyDim}, line:{color:C.skyDim}, arcSize:30,
  });
  s.addText("▶ 상세", {
    x:W-1.28, y:0.14, w:0.98, h:0.3,
    fontSize:8.5, bold:true, color:C.white, fontFace:"Malgun Gothic", align:"center", valign:"middle",
  });
}

// 화살표 (→)
function arr(s, x, y) {
  s.addText("→", { x, y, w:0.38, h:0.44, fontSize:18, color:C.sky, align:"center", valign:"middle", fontFace:"Malgun Gothic" });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 1: 표지
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S(true);
  s.addShape(pptx.ShapeType.rect, { x:0, y:0, w:W, h:0.18, fill:{color:C.sky}, line:{color:C.sky} });

  s.addText("wsflow", {
    x:1.0, y:1.6, w:W-2.0, h:1.4,
    fontSize:64, bold:true, color:C.white, fontFace:"Malgun Gothic", align:"center",
  });
  s.addText("AI 보조 개발 워크플로우 — 빠른 시작 가이드", {
    x:1.0, y:3.1, w:W-2.0, h:0.65,
    fontSize:20, color:C.sky, fontFace:"Malgun Gothic", align:"center",
  });
  s.addShape(pptx.ShapeType.rect, { x:3.8, y:3.9, w:5.7, h:0.04, fill:{color:C.navyMid}, line:{color:C.navyMid} });
  s.addText("HB Solution  |  2026", {
    x:1.0, y:4.05, w:W-2.0, h:0.45,
    fontSize:13, color:C.gray, fontFace:"Malgun Gothic", align:"center",
  });
  s.addShape(pptx.ShapeType.rect, { x:0, y:H-0.3, w:W, h:0.3, fill:{color:C.sky}, line:{color:C.sky} });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 2: 목차
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "목차");
  ftr(s);

  const secs = [
    ["00", "왜 wsflow인가",         "문제 정의 및 솔루션 개념"],
    ["01", "설치",                  "Claude Code / Codex 플러그인 설치"],
    ["02", "기본 워크플로우",        "discuss → 티켓 → proceed 실전 루프"],
    ["02.5","모르면 물어보세요",      "셀프서비스 학습 — wsflow에게 직접 질문"],
    ["03", "레거시 온보딩",          "bootstrap → forge-spec → forge-mental-model"],
    ["04", "협업: 컨트리뷰터",       "기능 브랜치 → MR 생성"],
    ["05", "협업: 메인테이너",       "lead-review → 판정 → 머지"],
  ];

  secs.forEach(([num, title, desc], i) => {
    const y = 0.95 + i * 0.77;
    pill(s, num, 0.38, y, 0.72, C.navy);
    s.addText(title, { x:1.2, y, w:3.2, h:0.44, fontSize:13, bold:true, color:C.navy, fontFace:"Malgun Gothic", valign:"middle" });
    s.addText(desc,  { x:4.5, y, w:8.5, h:0.44, fontSize:11, color:C.darkGray, fontFace:"Malgun Gothic", valign:"middle" });
    if (i < secs.length-1) {
      s.addShape(pptx.ShapeType.rect, { x:0.38, y:y+0.44, w:W-0.76, h:0.01, fill:{color:C.offWhite}, line:{color:C.offWhite} });
    }
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 3: 왜 wsflow인가
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S(true);
  hdr(s, "00  왜 wsflow인가", true);
  ftr(s);

  const pairs = [
    { prob: "세션마다 프로젝트 맥락을 처음부터 설명",
      sol:  "세션 기억 유지",    desc: "ai-docs/_index.md 자동 로드" },
    { prob: "\"이전에 논의한 방향\"이 다음 대화에서 사라짐",
      sol:  "결정 기록 보존",    desc: "커밋마다 AI Context 자동 포함" },
    { prob: "팀원마다 AI에게 다른 방식으로 지시",
      sol:  "워크플로우 통일",   desc: "discuss → proceed 파이프라인" },
    { prob: "결정 이유를 나중에 추적하기 어려움",
      sol:  "팀 규칙 공유",      desc: "AGENTS.md 한 파일로 팀 통일" },
    { prob: "프로젝트마다 문서·이슈 관리 방식이 달라 인수인계 부담",
      sol:  "문서화 규격 통일",  desc: "티켓·스펙·커밋 양식 표준화" },
  ];

  // 컬럼 레이블 (박스 없음)
  s.addText("지금의 불편함", {
    x:0.38, y:0.90, w:5.5, h:0.28,
    fontSize:11, bold:true, color:"EF9A9A", fontFace:"Malgun Gothic", align:"center",
  });
  s.addText("wsflow가 바꾸는 것", {
    x:6.58, y:0.90, w:6.37, h:0.28,
    fontSize:11, bold:true, color:C.sky, fontFace:"Malgun Gothic", align:"center",
  });

  const rh = 1.00, vgap = 0.14;
  pairs.forEach((p, i) => {
    const y = 1.28 + i * (rh + vgap);

    // 문제 카드
    s.addShape(pptx.ShapeType.roundRect, {
      x:0.38, y, w:5.5, h:rh,
      fill:{color:"161B2E"}, line:{color:"E57373", pt:1}, arcSize:2,
    });
    s.addText("✗", {
      x:0.50, y:y+0.1, w:0.38, h:rh-0.2,
      fontSize:15, bold:true, color:"E57373",
      fontFace:"Malgun Gothic", align:"center", valign:"middle",
    });
    s.addText(p.prob, {
      x:0.94, y:y+0.1, w:4.80, h:rh-0.2,
      fontSize:13, color:"FFCDD2", fontFace:"Malgun Gothic",
      valign:"middle", wrap:true, lineSpacingMultiple:1.3,
    });

    // 화살표
    s.addText("→", {
      x:5.93, y:y+0.25, w:0.60, h:0.50,
      fontSize:16, color:C.sky, align:"center", valign:"middle", fontFace:"Malgun Gothic",
    });

    // 솔루션 카드
    s.addShape(pptx.ShapeType.roundRect, {
      x:6.58, y, w:6.37, h:rh,
      fill:{color:C.navyMid}, line:{color:C.sky, pt:1}, arcSize:2,
    });
    s.addText("✓", {
      x:6.70, y:y+0.1, w:0.38, h:rh-0.2,
      fontSize:15, bold:true, color:C.sky,
      fontFace:"Malgun Gothic", align:"center", valign:"middle",
    });
    s.addText(p.sol, {
      x:7.14, y:y+0.10, w:5.67, h:0.36,
      fontSize:14, bold:true, color:C.sky, fontFace:"Malgun Gothic",
    });
    s.addText(p.desc, {
      x:7.14, y:y+0.50, w:5.67, h:0.38,
      fontSize:12, color:C.gray, fontFace:"Malgun Gothic",
    });
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 4: 설치 — Claude Code
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "01  설치  —  Claude Code");
  ftr(s);

  s.addText("마켓플레이스 등록 후 플러그인 설치  —  두 단계", {
    x:0.38, y:0.9, w:W-0.76, h:0.44, fontSize:16, bold:true, color:C.navy, fontFace:"Malgun Gothic",
  });

  s.addText("Step 1  —  마켓플레이스 등록 (최초 1회)", {
    x:0.38, y:1.52, w:W-0.76, h:0.36, fontSize:13, bold:true, color:C.darkGray, fontFace:"Malgun Gothic",
  });
  code(s, "claude plugin marketplace add kang-sw/devenv", { y:1.94, h:0.72 });

  s.addText("Step 2  —  플러그인 설치", {
    x:0.38, y:2.84, w:W-0.76, h:0.36, fontSize:13, bold:true, color:C.darkGray, fontFace:"Malgun Gothic",
  });
  code(s, "claude plugin install wsflow@kang-sw-devenv", { y:3.26, h:0.72 });

  s.addText("업그레이드  —  신규 버전 배포 후", {
    x:0.38, y:4.16, w:W-0.76, h:0.36, fontSize:13, bold:true, color:C.darkGray, fontFace:"Malgun Gothic",
  });
  code(s, "claude plugin upgrade wsflow@kang-sw-devenv", { y:4.58, h:0.72 });

  s.addShape(pptx.ShapeType.roundRect, {
    x:0.38, y:5.52, w:W-0.76, h:0.52,
    fill:{color:C.offWhite}, line:{color:C.sky, pt:1}, arcSize:2,
  });
  s.addText("설치 후 Claude Code 재시작 → /wsflow:lead-* 스킬 활성화", {
    x:0.58, y:5.52, w:W-1.16, h:0.52,
    fontSize:13, color:C.navy, fontFace:"Malgun Gothic", valign:"middle",
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 5: 설치 — Codex
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "01  설치  —  Codex");
  ftr(s);

  s.addText("CLI 또는 UI 방식으로 설치합니다.", {
    x:0.38, y:0.9, w:W-0.76, h:0.44, fontSize:16, bold:true, color:C.navy, fontFace:"Malgun Gothic",
  });

  s.addText("CLI 방식", {
    x:0.38, y:1.52, w:W-0.76, h:0.36, fontSize:13, bold:true, color:C.darkGray, fontFace:"Malgun Gothic",
  });
  code(s, "codex plugin marketplace add kang-sw/devenv", { y:1.94, h:0.72 });

  s.addText("UI 방식 (Codex 앱)", {
    x:0.38, y:2.84, w:W-0.76, h:0.36, fontSize:13, bold:true, color:C.darkGray, fontFace:"Malgun Gothic",
  });
  s.addShape(pptx.ShapeType.roundRect, {
    x:0.38, y:3.26, w:W-0.76, h:0.88,
    fill:{color:C.offWhite}, line:{color:C.sky, pt:1}, arcSize:2,
  });
  s.addText("Codex 실행  →  /plugins  →  kang-sw/devenv 검색  →  wsflow 설치", {
    x:0.58, y:3.26, w:W-1.16, h:0.88,
    fontSize:14, color:C.navy, fontFace:"Malgun Gothic", valign:"middle",
  });

  s.addShape(pptx.ShapeType.roundRect, {
    x:0.38, y:4.38, w:W-0.76, h:0.52,
    fill:{color:C.navyMid}, line:{color:C.navyMid}, arcSize:2,
  });
  s.addText("이후 내용은 모두 Claude Code 기준으로 설명합니다.", {
    x:0.58, y:4.38, w:W-1.16, h:0.52,
    fontSize:13, color:C.gray, fontFace:"Malgun Gothic", valign:"middle",
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 6: 기본 워크플로우 — 전체 흐름
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S(true);
  hdr(s, "02  기본 워크플로우  —  전체 흐름", true);
  ftr(s);

  // 큰 흐름 다이어그램
  const steps = [
    { label: "discuss",   sub: "방향 논의",      color: C.sky },
    { label: "티켓 생성",  sub: '"정리해주세요"', color: C.skyDim },
    { label: "proceed",   sub: "실행 착수",       color: C.navyMid },
    { label: "commit",    sub: "AI Context",      color: C.purple },
    { label: "/compact",  sub: "컨텍스트 정리",   color: C.darkGray },
  ];

  const bw  = (W - 0.76 - 0.32 * 4) / 5;  // 박스 폭 (화살표 0.32" 포함)
  const bh  = 1.7;
  const by  = 1.7;

  steps.forEach((st, i) => {
    const x = 0.38 + i * (bw + 0.32);
    const isLight = (st.color === C.sky || st.color === C.skyDim);
    s.addShape(pptx.ShapeType.roundRect, {
      x, y:by, w:bw, h:bh,
      fill:{color:st.color}, line:{color:st.color}, arcSize:4,
    });
    s.addText(st.label, {
      x, y:by+0.35, w:bw, h:0.7,
      fontSize:16, bold:true, color: isLight ? C.navy : C.white,
      fontFace:"Cascadia Code", align:"center", valign:"middle",
    });
    s.addText(st.sub, {
      x, y:by+1.1, w:bw, h:0.44,
      fontSize:11, color: isLight ? C.navyMid : C.gray,
      fontFace:"Malgun Gothic", align:"center",
    });
    if (i < steps.length-1) {
      s.addText("→", {
        x:x+bw, y:by+0.55, w:0.32, h:0.5,
        fontSize:18, color:C.sky, align:"center", valign:"middle", fontFace:"Malgun Gothic",
      });
    }
  });

  s.addText('"세션이 끊겨도 맥락이 남고, 누가 써도 같은 흐름으로 진행됩니다."', {
    x:0.38, y:4.9, w:W-0.76, h:0.52,
    fontSize:15, color:C.gray, fontFace:"Malgun Gothic", align:"center", italic:true,
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 7: 기본 워크플로우 — discuss
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "02  기본 워크플로우  —  discuss");
  ftr(s);

  s.addShape(pptx.ShapeType.roundRect, {
    x:0.38, y:0.9, w:W-0.76, h:0.6,
    fill:{color:C.navy}, line:{color:C.navy}, arcSize:2,
  });
  s.addText("항상 행동 착수 직전까지 고삐를 매세요.", {
    x:0.58, y:0.9, w:W-1.16, h:0.6,
    fontSize:18, bold:true, color:C.white, fontFace:"Malgun Gothic", valign:"middle", align:"center",
  });

  const cw = (W - 1.14) / 2;
  // 왼쪽 ❌
  s.addShape(pptx.ShapeType.roundRect, {
    x:0.38, y:1.65, w:cw, h:4.5,
    fill:{color:"FFF0F0"}, line:{color:"E57373", pt:1}, arcSize:2,
  });
  s.addText("❌  명령형", { x:0.58, y:1.78, w:cw-0.4, h:0.42, fontSize:14, bold:true, color:"C62828", fontFace:"Malgun Gothic" });
  [
    '"모터 컨트롤러 인터페이스 교체해줘"',
    '"이 부분 리팩터링해줘"',
    '"새 API 엔드포인트 추가해줘"',
  ].forEach((e, i) => code(s, e, { x:0.58, y:2.3+i*1.02, w:cw-0.4, h:0.78, fs:11 }));
  s.addText("AI가 즉시 구현을 시작합니다.\n리스크·대안 탐색 없이 방향이 고정됩니다.", {
    x:0.58, y:5.38, w:cw-0.4, h:0.64,
    fontSize:11, color:"C62828", fontFace:"Malgun Gothic", lineSpacingMultiple:1.3,
  });

  // 오른쪽 ✓
  const rx = 0.38 + cw + 0.38;
  s.addShape(pptx.ShapeType.roundRect, {
    x:rx, y:1.65, w:cw, h:4.5,
    fill:{color:"F0F8FF"}, line:{color:C.sky, pt:1}, arcSize:2,
  });
  s.addText("✓  질문형 (권장)", { x:rx+0.2, y:1.78, w:cw-0.4, h:0.42, fontSize:14, bold:true, color:C.skyDim, fontFace:"Malgun Gothic" });
  [
    '"인터페이스 교체 방향, 어떻게 생각해요?"',
    '"이렇게 리팩터링하면 어떤 리스크가 있을까요?"',
    '"이 방향이 맞는지 의견 듣고 싶어요"',
  ].forEach((e, i) => code(s, e, { x:rx+0.2, y:2.3+i*1.02, w:cw-0.4, h:0.78, fs:11 }));
  s.addText("wsflow가 리스크·대안·연결 지점을 먼저 짚어줍니다.\n방향 확정 후 proceed로 넘깁니다.", {
    x:rx+0.2, y:5.38, w:cw-0.4, h:0.64,
    fontSize:11, color:C.skyDim, fontFace:"Malgun Gothic", lineSpacingMultiple:1.3,
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 8: 기본 워크플로우 — 대화체 (전체 루프)
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "02  기본 워크플로우  —  실제 대화 흐름 (전체 루프)");
  ftr(s);

  const bw = W - 3.18;
  const gap = 0.08;
  const gateColor = "3D9E5A";

  function cBub(side, y, h, text, isGate = false) {
    const isUser = side === "user";
    const x = isUser ? 0.38 : 2.8;
    const bg = isUser ? C.userBg : C.navyMid;
    const tc = isUser ? C.black : C.white;
    const lc = isGate ? gateColor : (isUser ? C.skyDim : C.sky);
    s.addShape(pptx.ShapeType.roundRect, {x, y, w:bw, h, fill:{color:bg}, line:{color:lc, pt:isGate?2:1}, arcSize:6});
    s.addText(isUser ? "사용자" : (isGate ? "wsflow  ·  merge gate" : "wsflow"), {
      x:x+0.18, y:y+0.07, w:4.5, h:0.20,
      fontSize:8.5, bold:true, color:isGate ? gateColor : (isUser ? C.skyDim : C.sky), fontFace:"Malgun Gothic",
    });
    s.addText(text, {
      x:x+0.18, y:y+0.28, w:bw-0.36, h:h-0.34,
      fontSize:13, color:tc, fontFace:"Malgun Gothic", valign:"top", wrap:true, lineSpacingMultiple:1.25,
    });
  }

  let y = 0.90;

  cBub("user", y, 0.64, "인터페이스 교체 방향, 어떻게 생각해요?");
  y += 0.64 + gap;

  cBub("bot", y, 0.88, "참조 모듈 3개 발견. 직접 교체 시 런타임 오류 리스크 있어요.\n어댑터 패턴으로 점진적 전환을 권장합니다.");
  y += 0.88 + gap;

  cBub("user", y, 0.64, "좋아요. 티켓으로 정리해주세요.");
  y += 0.64 + gap;

  cBub("bot", y, 0.64, "티켓 생성 완료. 어댑터 도입 → 레거시 제거 순서로 구현 시작합니다...");
  y += 0.64 + gap;

  cBub("bot", y, 0.64, "구현 완료.  병합할까요?", true);
  y += 0.64 + gap;

  cBub("user", y, 0.88, "UIAdapter.reset()이 빠진 것 같아요.\n추가해주세요.");
  y += 0.88 + gap;

  cBub("bot", y, 0.64, "UIAdapter.reset() 추가 완료.  병합할까요?", true);
  y += 0.64 + gap;

  cBub("user", y, 0.64, "확인, 병합해주세요.");
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 9: 기본 워크플로우 — 티켓
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "02  기본 워크플로우  —  티켓");
  ftr(s);

  s.addText('"티켓으로 정리해주세요"  한 마디면 됩니다.', {
    x:0.38, y:0.9, w:W-0.76, h:0.5, fontSize:20, bold:true, color:C.navy, fontFace:"Malgun Gothic",
  });

  s.addText("discuss 종료 시 wsflow가 자동으로 티켓 파일을 생성하고 ai-docs/tickets/에 저장합니다.", {
    x:0.38, y:1.52, w:W-0.76, h:0.44, fontSize:14, color:C.darkGray, fontFace:"Malgun Gothic",
  });

  code(s,
    "ai-docs/tickets/ready/260513-refactor-interface.md\n\n" +
    "# Refactor Interface\n" +
    "## Phases\n" +
    "### Phase 1: 어댑터 도입\n" +
    "### Phase 2: 레거시 제거",
    { y:2.1, h:2.3, fs:13 });

  // 상태 흐름
  const stats = ["idea/", "todo/", "ready/", ".done/"];
  const cols  = [C.gray, C.darkGray, C.sky, C.navy];
  const sw = (W - 0.76 - 0.28 * 3) / 4;
  stats.forEach((st, i) => {
    const x = 0.38 + i * (sw + 0.28);
    s.addShape(pptx.ShapeType.roundRect, { x, y:4.7, w:sw, h:0.6, fill:{color:cols[i]}, line:{color:cols[i]}, arcSize:8 });
    s.addText(st, { x, y:4.7, w:sw, h:0.6, fontSize:13, bold:true, color:C.white, fontFace:"Cascadia Code", align:"center", valign:"middle" });
    if (i < stats.length-1) s.addText("→", { x:x+sw, y:4.78, w:0.28, h:0.44, fontSize:14, color:C.sky, align:"center", valign:"middle", fontFace:"Malgun Gothic" });
  });

  s.addText("discuss에서 proceed 진입 시 wsflow가 자동으로 ready/ 상태로 승격합니다.", {
    x:0.38, y:5.52, w:W-0.76, h:0.38, fontSize:12, color:C.darkGray, fontFace:"Malgun Gothic",
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 10: 기본 워크플로우 — proceed + /compact
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "02  기본 워크플로우  —  proceed + /compact");
  ftr(s);

  const cw = (W - 1.14) / 2;

  // proceed
  s.addShape(pptx.ShapeType.roundRect, {
    x:0.38, y:0.9, w:cw, h:5.52,
    fill:{color:C.offWhite}, line:{color:C.sky, pt:1}, arcSize:1,
  });
  s.addText("proceed", { x:0.58, y:1.02, w:cw-0.4, h:0.5, fontSize:18, bold:true, color:C.navy, fontFace:"Cascadia Code" });
  s.addText("구현을 시작하는 진입 명령입니다.", { x:0.58, y:1.56, w:cw-0.4, h:0.38, fontSize:13, color:C.darkGray, fontFace:"Malgun Gothic" });
  code(s, "/wsflow:lead-proceed", { x:0.58, y:2.06, w:cw-0.4, h:0.62, fs:13 });
  code(s, "/wsflow:lead-proceed 로그인 모듈 추가", { x:0.58, y:2.82, w:cw-0.4, h:0.78, fs:12 });
  [
    "티켓이 있으면 → 해당 티켓으로 구현",
    "티켓이 없으면 → 자동 생성 후 구현",
    "논의 중이면 → 티켓 승격 후 구현",
    "구현 완료 → spec · 문서 자동 업데이트",
  ].forEach((t, i) => {
    s.addText("▸  " + t, {
      x:0.58, y:3.76 + i*0.66, w:cw-0.4, h:0.56,
      fontSize:12, color:C.navy, fontFace:"Malgun Gothic", valign:"middle",
    });
  });

  // /compact
  const rx = 0.38 + cw + 0.38;
  s.addShape(pptx.ShapeType.roundRect, {
    x:rx, y:0.9, w:cw, h:5.52,
    fill:{color:C.offWhite}, line:{color:C.sky, pt:1}, arcSize:1,
  });
  s.addText("/compact", { x:rx+0.2, y:1.02, w:cw-0.4, h:0.5, fontSize:18, bold:true, color:C.navy, fontFace:"Cascadia Code" });
  s.addText("컨텍스트 창이 길어질 때 압축합니다.", { x:rx+0.2, y:1.56, w:cw-0.4, h:0.38, fontSize:13, color:C.darkGray, fontFace:"Malgun Gothic" });
  code(s, "/compact", { x:rx+0.2, y:2.06, w:cw-0.4, h:0.62, fs:13 });

  [
    "컨텍스트 창이 가득 차면 응답 품질이 떨어집니다",
    "/compact로 핵심만 압축해서 계속 진행",
    "결정 맥락과 작업 상태는 유지됩니다",
    "다음 세션 재개 시에도 이전 요약이 인계됩니다",
  ].forEach((t, i) => {
    s.addText("▸  " + t, {
      x:rx+0.2, y:2.86 + i*0.62, w:cw-0.4, h:0.5,
      fontSize:12, color:C.navy, fontFace:"Malgun Gothic", valign:"middle",
    });
  });

  s.addShape(pptx.ShapeType.roundRect, {
    x:rx+0.2, y:5.42, w:cw-0.4, h:0.48,
    fill:{color:C.sky}, line:{color:C.sky}, arcSize:2,
  });
  s.addText("습관: 큰 작업 단위 종료 후 항상 /compact", {
    x:rx+0.2, y:5.42, w:cw-0.4, h:0.48,
    fontSize:12, bold:true, color:C.navy, fontFace:"Malgun Gothic", valign:"middle", align:"center",
  });
  s.addText("세션 재시작 시 ai-docs/_index.md를 자동으로 다시 읽습니다.", {
    x:rx+0.2, y:6.00, w:cw-0.4, h:0.32,
    fontSize:10, color:"8FA8B8", fontFace:"Malgun Gothic", valign:"middle", align:"center",
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 11: 기본 워크플로우 — 자동으로 쌓이는 기록들
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "02  기본 워크플로우  —  자동으로 쌓이는 기록들");
  ftr(s);

  s.addText("wsflow로 작업하면 아래 항목들이 자동으로 쌓입니다.", {
    x:0.38, y:0.9, w:W-0.76, h:0.44,
    fontSize:16, color:C.navy, fontFace:"Malgun Gothic",
  });

  const items = [
    { icon:"📁", name:"ai-docs/",      color:C.sky,    desc:"_index.md · tickets/ · spec/ · mental-model/\n다음 세션 시작 시 AI가 자동으로 읽어 맥락을 복원합니다." },
    { icon:"🎫", name:"티켓 파일",      color:C.skyDim, desc:"ai-docs/tickets/ 아래 Markdown 파일로 작업 단위 기록\n상태(idea → todo → ready → .done)는 디렉터리로 관리됩니다." },
    { icon:"📝", name:"AI Context 커밋",color:C.navy,   desc:"커밋마다 ## AI Context 섹션이 자동 포함\n\"왜 이렇게 만들었는가\"를 git log로 언제든 추적 가능합니다." },
    { icon:"📋", name:"spec / mental-model", color:C.navyMid, desc:"bootstrap / forge-* 실행 시 생성\n동작 계약서와 수정 가이드가 도메인별로 저장됩니다." },
  ];

  items.forEach((it, i) => {
    const y = 1.48 + i * 1.32;
    s.addShape(pptx.ShapeType.roundRect, {
      x:0.38, y, w:W-0.76, h:1.2,
      fill:{color:C.offWhite}, line:{color:it.color, pt:1}, arcSize:2,
    });
    s.addText(it.icon, { x:0.52, y:y+0.1, w:0.8, h:1.0, fontSize:24, align:"center", valign:"middle", fontFace:"Segoe UI Emoji" });
    s.addText(it.name, { x:1.4, y:y+0.1, w:3.0, h:0.42, fontSize:14, bold:true, color:C.navy, fontFace:"Malgun Gothic" });
    s.addText(it.desc, { x:1.4, y:y+0.54, w:W-1.9, h:0.6, fontSize:11, color:C.darkGray, fontFace:"Malgun Gothic", lineSpacingMultiple:1.3 });
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 12: 기본 워크플로우 ▶ 디테일
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "02  기본 워크플로우  —  내부 라우팅 상세");
  ftr(s);
  detailBadge(s);

  s.addText("사용자가 discuss / proceed를 호출하면 wsflow가 내부적으로 다음 스킬들을 라우팅합니다.", {
    x:0.38, y:0.9, w:W-0.76, h:0.44, fontSize:13, color:C.darkGray, fontFace:"Malgun Gothic",
  });

  const routes = [
    { trigger:"discuss",  chain:["lead-discuss", "lead-write-spec?", "lead-write-ticket?"],          note:"코드 변경 없음. 논의 결과를 spec·티켓으로 캡처." },
    { trigger:"proceed",  chain:["lead-proceed", "lead-write-ticket?", "lead-implement"],             note:"라우터 역할. 실행 슬라이스 선택 후 implement에 위임." },
    { trigger:"implement",chain:["lead-edit", "or lead-write-code", "update-spec", "commit"],         note:"실제 코드 수정. 완료 후 docs 파이프라인 자동 실행." },
  ];

  routes.forEach((r, i) => {
    const y = 1.48 + i * 1.68;
    s.addShape(pptx.ShapeType.rect, {
      x:0.38, y, w:0.06, h:1.5,
      fill:{color:C.sky}, line:{color:C.sky},
    });
    s.addText(r.trigger, { x:0.58, y:y+0.06, w:2.2, h:0.4, fontSize:14, bold:true, color:C.navy, fontFace:"Cascadia Code" });
    s.addText(r.note, { x:0.58, y:y+0.5, w:W-1.0, h:0.36, fontSize:11, color:C.darkGray, fontFace:"Malgun Gothic" });
    // 체인 pills
    r.chain.forEach((c, ci) => {
      const px = 0.58 + ci * 2.95;
      s.addShape(pptx.ShapeType.roundRect, { x:px, y:y+0.96, w:2.72, h:0.38, fill:{color:C.navy}, line:{color:C.sky, pt:1}, arcSize:6 });
      s.addText(c, { x:px, y:y+0.96, w:2.72, h:0.38, fontSize:9.5, color:C.sky, fontFace:"Cascadia Code", align:"center", valign:"middle" });
      if (ci < r.chain.length-1) s.addText("→", { x:px+2.72, y:y+0.96, w:0.23, h:0.38, fontSize:11, color:C.gray, align:"center", valign:"middle", fontFace:"Malgun Gothic" });
    });
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 13: 모르면 물어보세요 — 개요
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "02.5  모르면 물어보세요");
  ftr(s);

  s.addShape(pptx.ShapeType.roundRect, {
    x:0.38, y:0.9, w:W-0.76, h:0.68,
    fill:{color:C.navy}, line:{color:C.navy}, arcSize:2,
  });
  s.addText("모르면 그냥 물어보면 됩니다. wsflow 자체가 맥락을 들고 있습니다.", {
    x:0.58, y:0.9, w:W-1.16, h:0.68,
    fontSize:18, bold:true, color:C.white, fontFace:"Malgun Gothic", valign:"middle", align:"center",
  });

  const qs = [
    { type:"워크플로우 질문",  ex:'"proceed는 언제 쓰는 건가요?"',          ans:"wsflow가 WORKFLOW.md와 _index.md를 읽고 설명해줍니다." },
    { type:"프로젝트 질문",    ex:'"auth 모듈 구조가 어떻게 돼 있어?"',      ans:"wsflow가 mental-model 문서를 읽고 설명해줍니다." },
    { type:"작업 전 확인",     ex:'"이 리팩터링 방향 맞나요?"',              ans:"discuss가 spec·코드를 조회해 검증 후 의견을 줍니다." },
  ];

  qs.forEach((q, i) => {
    const y = 1.76 + i * 1.5;
    s.addShape(pptx.ShapeType.roundRect, {
      x:0.38, y, w:W-0.76, h:1.32,
      fill:{color:C.offWhite}, line:{color:C.sky, pt:1}, arcSize:2,
    });
    pill(s, q.type, 0.58, y+0.14, 2.2, C.sky);
    code(s, q.ex, { x:3.0, y:y+0.08, w:10.0, h:0.6, fs:12 });
    s.addText("→ " + q.ans, {
      x:0.58, y:y+0.84, w:W-1.16, h:0.36,
      fontSize:11, color:C.darkGray, fontFace:"Malgun Gothic",
    });
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 14: 모르면 물어보세요 — 대화체
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "02.5  모르면 물어보세요  —  실제 대화");
  ftr(s);

  const gap = 0.14;
  let y = 0.92;

  bubble(s, "proceed는 언제 쓰는 건가요?", "user", y, 0.92);
  y += 0.92 + gap;

  bubble(s,
    "논의가 끝나고 구현을 시작하고 싶을 때요.\n티켓 생성부터 구현까지 파이프라인을 자동으로 연결해줍니다.",
    "bot", y, 1.28);
  y += 1.28 + gap;

  bubble(s, "bootstrap은 한 번만 하면 되나요?", "user", y, 0.92);
  y += 0.92 + gap;

  bubble(s,
    "맞아요. 프로젝트 루트에서 한 번만 실행하면\nai-docs/ 구조가 생기고 이후 세션마다 자동으로 로드됩니다.",
    "bot", y, 1.28);
  y += 1.28 + gap;

  s.addShape(pptx.ShapeType.roundRect, {
    x:0.38, y, w:W-0.76, h:0.52,
    fill:{color:C.offWhite}, line:{color:C.sky, pt:1}, arcSize:2,
  });
  s.addText("파일을 직접 열 필요 없습니다. AI에게 물어보면 AI가 문서를 읽고 설명합니다.", {
    x:0.58, y, w:W-1.16, h:0.52,
    fontSize:12, color:C.navy, fontFace:"Malgun Gothic", valign:"middle",
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 15: 레거시 온보딩 — 3단계
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S(true);
  hdr(s, "03  레거시 온보딩  —  3단계", true);
  ftr(s);

  s.addText("기존 프로젝트에 wsflow를 도입할 때의 순서입니다.", {
    x:0.38, y:0.9, w:W-0.76, h:0.38, fontSize:14, color:C.gray, fontFace:"Malgun Gothic",
  });

  const steps = [
    { n:"1", cmd:"lead-bootstrap",          title:"워크플로우 구조 초기화", desc:"AGENTS.md, ai-docs/ 폴더, WORKFLOW.md 생성.\n기존 CLAUDE.md가 있으면 자동 마이그레이션합니다." },
    { n:"2", cmd:"lead-forge-spec",          title:"Spec 일괄 생성",        desc:"코드베이스·커밋 히스토리를 분석해 도메인별 spec 작성.\n구현 완료 / 계획 중으로 분류합니다." },
    { n:"3", cmd:"lead-forge-mental-model",  title:"Mental Model 일괄 생성", desc:"모듈 계약·결합 지점·주의사항을 도메인별로 정리.\n수정 시 알아야 할 운영 지식을 문서화합니다." },
  ];

  steps.forEach((st, i) => {
    const y = 1.42 + i * 1.7;

    // 번호
    s.addShape(pptx.ShapeType.ellipse, { x:0.38, y:y+0.5, w:0.6, h:0.6, fill:{color:C.sky}, line:{color:C.sky} });
    s.addText(st.n, { x:0.38, y:y+0.5, w:0.6, h:0.6, fontSize:16, bold:true, color:C.navy, fontFace:"Malgun Gothic", align:"center", valign:"middle" });

    // 화살표 (마지막 제외)
    if (i < steps.length-1) {
      s.addText("↓", { x:0.52, y:y+1.1, w:0.32, h:0.62, fontSize:16, color:C.sky, align:"center", valign:"middle", fontFace:"Malgun Gothic" });
    }

    // 카드
    s.addShape(pptx.ShapeType.roundRect, { x:1.18, y, w:W-1.56, h:1.54, fill:{color:C.navyMid}, line:{color:C.sky, pt:1}, arcSize:4 });
    code(s, "/wsflow:" + st.cmd, { x:1.38, y:y+0.1, w:5.5, h:0.52, fs:12 });
    s.addText(st.title, { x:1.38, y:y+0.66, w:W-1.9, h:0.38, fontSize:14, bold:true, color:C.white, fontFace:"Malgun Gothic" });
    s.addText(st.desc,  { x:1.38, y:y+1.06, w:W-1.9, h:0.44, fontSize:11, color:C.gray, fontFace:"Malgun Gothic", lineSpacingMultiple:1.3 });
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 16: 레거시 온보딩 — 완료 후 상태
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "03  레거시 온보딩  —  완료 후 상태");
  ftr(s);

  s.addText("3단계 완료 후 이제 기본 워크플로우를 그대로 사용할 수 있습니다.", {
    x:0.38, y:0.9, w:W-0.76, h:0.44, fontSize:16, bold:true, color:C.navy, fontFace:"Malgun Gothic",
  });

  code(s,
    "ai-docs/\n" +
    "  _index.md            ← 세션 시작마다 AI가 읽는 프로젝트 메모\n" +
    "  tickets/             ← 작업 단위 파일들\n" +
    "  spec/                ← 기능 동작 계약서 (forge-spec이 생성)\n" +
    "  mental-model/        ← 도메인별 수정 가이드 (forge-mental-model이 생성)\n" +
    "  WORKFLOW.md          ← wsflow 사용 가이드 (bootstrap이 복사)",
    { y:1.5, h:2.6, fs:13 });

  const nexts = [
    "AI가 이 구조를 읽어 프로젝트 맥락을 자동 복원합니다",
    "이후 작업은 discuss → proceed 루프로 진행하면 됩니다",
    "spec·mental-model은 구현 후 wsflow가 자동 업데이트합니다",
  ];
  nexts.forEach((t, i) => {
    s.addShape(pptx.ShapeType.roundRect, {
      x:0.38, y:4.3 + i*0.7, w:W-0.76, h:0.58,
      fill:{color:C.offWhite}, line:{color:C.sky, pt:1}, arcSize:2,
    });
    s.addText("✓  " + t, {
      x:0.58, y:4.3 + i*0.7, w:W-1.16, h:0.58,
      fontSize:14, color:C.navy, fontFace:"Malgun Gothic", valign:"middle",
    });
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 17: 협업: 컨트리뷰터 — 플로우
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "04  협업: 컨트리뷰터  —  플로우");
  ftr(s);

  // 레인 헤더
  const laneW = (W - 0.76) / 2;
  const laneColors = [C.offWhite, "EFF4FF"];

  ["wsflow 구간", "일반 git 구간"].forEach((label, i) => {
    const x = 0.38 + i * laneW;
    s.addShape(pptx.ShapeType.rect, { x, y:0.88, w:laneW-0.1, h:0.42, fill:{color: i===0 ? C.sky : C.navyMid}, line:{color: i===0 ? C.sky : C.navyMid} });
    s.addText(label, { x, y:0.88, w:laneW-0.1, h:0.42, fontSize:13, bold:true, color:C.white, fontFace:"Malgun Gothic", align:"center", valign:"middle" });
  });

  // 왼쪽: wsflow 구간
  const wsSteps = [
    ["기능 브랜치 생성", "git checkout -b feature/xxx"],
    ["discuss + proceed", "/wsflow:lead-discuss"],
    ["implement + commit", "AI Context 자동 포함"],
  ];
  wsSteps.forEach(([title, sub], i) => {
    const y = 1.46 + i * 1.56;
    s.addShape(pptx.ShapeType.roundRect, { x:0.38, y, w:laneW-0.28, h:1.32, fill:{color:C.offWhite}, line:{color:C.sky, pt:1}, arcSize:4 });
    s.addText(title, { x:0.58, y:y+0.12, w:laneW-0.68, h:0.44, fontSize:14, bold:true, color:C.navy, fontFace:"Malgun Gothic" });
    code(s, sub, { x:0.58, y:y+0.6, w:laneW-0.68, h:0.56, fs:11 });
    if (i < wsSteps.length-1) {
      s.addText("↓", { x:0.38 + laneW/2 - 0.2, y:y+1.32, w:0.4, h:0.26, fontSize:14, color:C.sky, align:"center", valign:"middle", fontFace:"Malgun Gothic" });
    }
  });

  // 오른쪽: 일반 git 구간
  const gitSteps = [
    ["브랜치 push", "git push origin feature/xxx"],
    ["GitLab MR 생성", "직접 웹에서 생성 (fork 불필요)"],
  ];
  const rx = 0.38 + laneW;
  gitSteps.forEach(([title, sub], i) => {
    const y = 1.46 + i * 1.56;
    s.addShape(pptx.ShapeType.roundRect, { x:rx+0.1, y, w:laneW-0.48, h:1.32, fill:{color:"EFF4FF"}, line:{color:C.navyMid, pt:1}, arcSize:4 });
    s.addText(title, { x:rx+0.3, y:y+0.12, w:laneW-0.88, h:0.44, fontSize:14, bold:true, color:C.navy, fontFace:"Malgun Gothic" });
    code(s, sub, { x:rx+0.3, y:y+0.6, w:laneW-0.88, h:0.56, fs:11 });
    if (i < gitSteps.length-1) {
      s.addText("↓", { x:rx + laneW/2 - 0.2, y:y+1.32, w:0.4, h:0.26, fontSize:14, color:C.navyMid, align:"center", valign:"middle", fontFace:"Malgun Gothic" });
    }
  });

  s.addText("AI Context 커밋들이 MR description의 재료가 됩니다.", {
    x:0.38, y:6.3, w:W-0.76, h:0.36, fontSize:12, color:C.darkGray, fontFace:"Malgun Gothic", align:"center",
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 18: 협업: 컨트리뷰터 — FIX 수신 루프
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "04  협업: 컨트리뷰터  —  FIX 수신 시");
  ftr(s);

  s.addText("메인테이너로부터 NEEDS FIX 코멘트를 받으면:", {
    x:0.38, y:0.9, w:W-0.76, h:0.44, fontSize:16, bold:true, color:C.navy, fontFace:"Malgun Gothic",
  });

  const loop = [
    { n:"1", label:"GitLab MR 코멘트 확인",        sub:"메인테이너가 붙여넣은 리뷰 내용 확인" },
    { n:"2", label:"discuss로 수정 방향 논의",       sub:"/wsflow:lead-discuss [수정 내용 설명]" },
    { n:"3", label:"proceed로 수정 구현",            sub:"/wsflow:lead-proceed" },
    { n:"4", label:"브랜치 push",                   sub:"git push origin feature/xxx" },
  ];

  loop.forEach((st, i) => {
    const y = 1.5 + i * 1.18;
    // 번호 원
    s.addShape(pptx.ShapeType.ellipse, { x:0.38, y:y+0.2, w:0.56, h:0.56, fill:{color:C.sky}, line:{color:C.sky} });
    s.addText(st.n, { x:0.38, y:y+0.2, w:0.56, h:0.56, fontSize:16, bold:true, color:C.navy, fontFace:"Malgun Gothic", align:"center", valign:"middle" });
    if (i < loop.length-1) s.addText("↓", { x:0.52, y:y+0.76, w:0.28, h:0.44, fontSize:14, color:C.sky, align:"center", valign:"middle", fontFace:"Malgun Gothic" });

    s.addShape(pptx.ShapeType.roundRect, { x:1.14, y, w:W-1.52, h:0.98, fill:{color:C.offWhite}, line:{color:C.sky, pt:1}, arcSize:4 });
    s.addText(st.label, { x:1.34, y:y+0.1, w:6.0, h:0.4, fontSize:15, bold:true, color:C.navy, fontFace:"Malgun Gothic" });
    s.addText(st.sub,   { x:1.34, y:y+0.52, w:W-1.9, h:0.36, fontSize:12, color:C.darkGray, fontFace:"Cascadia Code" });
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 19: 협업: 컨트리뷰터 ▶ 디테일
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "04  협업: 컨트리뷰터  —  상세 흐름");
  ftr(s);
  detailBadge(s);

  s.addText("wsflow가 개입하는 구간과 일반 git 동작 구간을 구분합니다.", {
    x:0.38, y:0.9, w:W-0.76, h:0.38, fontSize:13, color:C.darkGray, fontFace:"Malgun Gothic",
  });

  const rows = [
    { step:"git checkout -b feature/xxx", tag:"일반 git", color:C.navyMid, note:"wsflow 개입 없음. 평소 브랜치 작업과 동일." },
    { step:"discuss → ticket → proceed",  tag:"wsflow",  color:C.sky,    note:"discuss: 설계 논의. proceed: 라우터. implement: 코드 수정 + spec/MM 업데이트 + commit." },
    { step:"git push origin feature/xxx", tag:"일반 git", color:C.navyMid, note:"wsflow 개입 없음. implement가 완료한 커밋들을 올립니다." },
    { step:"GitLab MR 생성",              tag:"일반 git", color:C.navyMid, note:"웹 UI에서 직접 생성. fork 없이 단일 레포 MR." },
    { step:"NEEDS FIX 수신 → 재구현",     tag:"wsflow",  color:C.sky,    note:"discuss로 수정 방향 논의 후 proceed로 재구현. push로 MR 자동 갱신." },
  ];

  rows.forEach((r, i) => {
    const y = 1.4 + i * 1.02;
    s.addShape(pptx.ShapeType.roundRect, { x:0.38, y, w:W-0.76, h:0.88, fill:{color:C.offWhite}, line:{color:r.color, pt:1}, arcSize:4 });
    pill(s, r.tag, 0.58, y+0.22, 1.4, r.color);
    code(s, r.step, { x:2.12, y:y+0.1, w:4.5, h:0.58, fs:11 });
    s.addText(r.note, { x:6.82, y:y+0.1, w:W-7.2, h:0.7, fontSize:11, color:C.darkGray, fontFace:"Malgun Gothic", lineSpacingMultiple:1.3, valign:"middle" });
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 20: 협업: 메인테이너 — 최초 설정
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "05  협업: 메인테이너  —  최초 설정 (1회)");
  ftr(s);

  s.addShape(pptx.ShapeType.roundRect, {
    x:0.38, y:0.9, w:W-0.76, h:0.6,
    fill:{color:C.sky}, line:{color:C.sky}, arcSize:2,
  });
  s.addText("lead-review 첫 실행 시 설정을 자동으로 안내합니다. 이후에는 설정 불필요.", {
    x:0.58, y:0.9, w:W-1.16, h:0.6,
    fontSize:15, bold:true, color:C.navy, fontFace:"Malgun Gothic", valign:"middle", align:"center",
  });

  code(s, "/wsflow:lead-review", { y:1.68, h:0.62 });

  s.addText("설정 질문 (ai-docs/_review.local.md 자동 생성):", {
    x:0.38, y:2.48, w:W-0.76, h:0.38, fontSize:14, bold:true, color:C.navy, fontFace:"Malgun Gothic",
  });

  const qs = [
    ["Remote 접근 방식",    "git fetch  /  glab  /  API token",  "내부망 GitLab → git fetch 선택"],
    ["Branch 패턴",         "feature/, fix/, TICKET-[0-9]+",       "필터링할 브랜치 접두사 (선택)"],
    ["Comment Method",     "GitLab Web UI  /  glab mr note",     "Web UI → /copy 후 수동 붙여넣기"],
    ["Merge 방식",          "local merge → push  /  web approve", "팀 승인 방식에 따라 선택"],
  ];

  qs.forEach(([q, opt, hint], i) => {
    const y = 3.0 + i * 0.84;
    s.addShape(pptx.ShapeType.roundRect, { x:0.38, y, w:W-0.76, h:0.72, fill:{color:C.offWhite}, line:{color:C.sky, pt:1}, arcSize:4 });
    s.addText(q,    { x:0.58, y:y+0.06, w:2.8, h:0.58, fontSize:12, bold:true, color:C.navy, fontFace:"Malgun Gothic", valign:"middle" });
    s.addText(opt,  { x:3.5,  y:y+0.06, w:4.8, h:0.58, fontSize:11, color:C.darkGray, fontFace:"Cascadia Code", valign:"middle" });
    s.addText("← " + hint, { x:8.4, y:y+0.06, w:4.8, h:0.58, fontSize:10, color:C.skyDim, fontFace:"Malgun Gothic", valign:"middle" });
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 21: 협업: 메인테이너 — 리뷰 실행
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "05  협업: 메인테이너  —  리뷰 실행");
  ftr(s);

  code(s, "/wsflow:lead-review feature/xxx\n→ fetch + checkout + 리뷰 자동 실행", { y:0.9, h:0.96, fs:14 });

  s.addText("판정별 액션", {
    x:0.38, y:2.0, w:W-0.76, h:0.38,
    fontSize:15, bold:true, color:C.navy, fontFace:"Malgun Gothic",
  });

  const verdicts = [
    { v:"LGTM",       col:C.sky,    actions:["설정된 머지 시퀀스 자동 실행"] },
    { v:"NEEDS FIX",  col:C.skyDim, actions:["/copy → GitLab MR 코멘트 붙여넣기", "또는: 메인테이너가 직접 수정 후 머지"] },
    { v:"OPEN",       col:C.purple, actions:["의도 불명확 → lead-discuss로 논의 후 재판정"] },
    { v:"BLOCKED",    col:"C62828", actions:["차단 경로 발견 (예: .env) → MR에 알림"] },
  ];

  verdicts.forEach((vd, i) => {
    const y = 2.52 + i * 1.06;
    s.addShape(pptx.ShapeType.roundRect, { x:0.38, y, w:W-0.76, h:0.92, fill:{color:C.offWhite}, line:{color:vd.col, pt:2}, arcSize:4 });
    pill(s, vd.v, 0.58, y+0.22, 2.0, vd.col);
    vd.actions.forEach((a, ai) => {
      s.addText((vd.actions.length>1 ? ["①","②"][ai]+" " : "") + a, {
        x:2.82, y:y+0.12 + ai*0.38, w:W-3.2, h:0.38,
        fontSize:13, color:C.navy, fontFace:"Malgun Gothic", valign:"middle",
      });
    });
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 22: 협업: 메인테이너 ▶ 디테일
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S();
  hdr(s, "05  협업: 메인테이너  —  내부 동작 상세");
  ftr(s);
  detailBadge(s);

  s.addText("lead-review invoke 흐름:", {
    x:0.38, y:0.9, w:W-0.76, h:0.38, fontSize:14, bold:true, color:C.navy, fontFace:"Malgun Gothic",
  });

  const steps = [
    { n:"1", t:"config 로드",     d:"ai-docs/_review.local.md 읽기. 없으면 setup 인터뷰 실행." },
    { n:"2", t:"브랜치 확인",     d:"인자 없으면 Remote 설정으로 브랜치 목록 조회 → 선택." },
    { n:"3", t:"준비",            d:"현재 브랜치 기록. Remote 방식으로 fetch. 대상 브랜치 checkout." },
    { n:"4", t:"BLOCKED 검사",   d:"Blocked Paths 설정 있으면 diff에서 먼저 검사. 발견 시 즉시 중단." },
    { n:"5", t:"리뷰 실행",       d:"intent → alignment → risk 순서로 단계 실행. Large diff 시 subagent 병렬 처리." },
    { n:"6", t:"판정",            d:"LGTM / NEEDS FIX / OPEN / BLOCKED → 각 처리 경로 실행." },
  ];

  steps.forEach((st, i) => {
    const y = 1.38 + i * 0.86;
    s.addShape(pptx.ShapeType.roundRect, { x:0.38, y, w:W-0.76, h:0.74, fill:{color:C.offWhite}, line:{color:C.sky, pt:1}, arcSize:4 });
    s.addShape(pptx.ShapeType.ellipse, { x:0.52, y:y+0.12, w:0.5, h:0.5, fill:{color:C.sky}, line:{color:C.sky} });
    s.addText(st.n, { x:0.52, y:y+0.12, w:0.5, h:0.5, fontSize:13, bold:true, color:C.navy, fontFace:"Malgun Gothic", align:"center", valign:"middle" });
    s.addText(st.t, { x:1.18, y:y+0.06, w:2.8, h:0.38, fontSize:13, bold:true, color:C.navy, fontFace:"Malgun Gothic", valign:"middle" });
    s.addText(st.d, { x:1.18, y:y+0.4,  w:W-1.6, h:0.3, fontSize:11, color:C.darkGray, fontFace:"Malgun Gothic", valign:"middle" });
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 23: 치트시트
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S(true);
  hdr(s, "치트시트", true);
  ftr(s);

  const cmds = [
    ["/wsflow:lead-discuss [주제]",           "방향 논의. 코드 변경 없음",              C.sky],
    ["/wsflow:lead-proceed [설명/티켓경로]",   "구현 진입. 파이프라인 자동 연결",         C.sky],
    ["/wsflow:lead-bootstrap",                "프로젝트 초기 구조 설정 (1회)",           C.skyDim],
    ["/wsflow:lead-forge-spec",               "코드베이스 분석 → spec 일괄 생성",        C.skyDim],
    ["/wsflow:lead-forge-mental-model",       "도메인별 mental model 일괄 생성",         C.skyDim],
    ["/wsflow:lead-review [브랜치]",           "MR 리뷰 → LGTM / NEEDS FIX / OPEN",    C.purple],
    ["/compact",                              "컨텍스트 창 압축 — 큰 작업 후 습관화",    C.skyDim],
    ["(질문을 그냥 입력)",                     "wsflow가 문서를 읽고 답변",               C.darkGray],
  ];

  const rh = (H - 0.77 - 0.28 - 0.9) / cmds.length;
  cmds.forEach(([cmd, desc, col], i) => {
    const y = 0.9 + i * rh;
    s.addShape(pptx.ShapeType.rect, { x:0.12, y, w:0.18, h:rh-0.06, fill:{color:col}, line:{color:col} });
    s.addText(cmd, { x:0.38, y, w:5.4, h:rh-0.06, fontSize:11, fontFace:"Cascadia Code", color:C.sky, valign:"middle" });
    s.addText(desc, { x:5.9, y, w:W-6.3, h:rh-0.06, fontSize:12, fontFace:"Malgun Gothic", color:C.offWhite, valign:"middle" });
    if (i < cmds.length-1) {
      s.addShape(pptx.ShapeType.rect, { x:0.38, y:y+rh-0.08, w:W-0.38, h:0.02, fill:{color:"1E3560"}, line:{color:"1E3560"} });
    }
  });
}

// ═══════════════════════════════════════════════════════════════════════
// 슬라이드 24: 마무리
// ═══════════════════════════════════════════════════════════════════════
{
  const s = S(true);
  s.addShape(pptx.ShapeType.rect, { x:0, y:0, w:W, h:0.18, fill:{color:C.sky}, line:{color:C.sky} });

  s.addText("오늘 시작하기", {
    x:1.0, y:1.2, w:W-2.0, h:1.0,
    fontSize:42, bold:true, color:C.white, fontFace:"Malgun Gothic", align:"center",
  });

  const steps = [
    ["1", "설치",            "claude plugin install wsflow@kang-sw-devenv"],
    ["2", "bootstrap",       "/wsflow:lead-bootstrap  (기존 프로젝트)"],
    ["3", "첫 논의",          "/wsflow:lead-discuss [궁금한 것]"],
  ];

  steps.forEach(([n, title, cmd], i) => {
    const x = 0.38 + i * (W-0.76)/3 + (i > 0 ? 0.14 : 0);
    const bw = (W-0.76)/3 - 0.14;
    const y = 2.6;
    s.addShape(pptx.ShapeType.ellipse, { x:x + bw/2 - 0.35, y, w:0.7, h:0.7, fill:{color:C.sky}, line:{color:C.sky} });
    s.addText(n, { x:x + bw/2 - 0.35, y, w:0.7, h:0.7, fontSize:20, bold:true, color:C.navy, fontFace:"Malgun Gothic", align:"center", valign:"middle" });
    s.addText(title, { x, y:3.5, w:bw, h:0.48, fontSize:16, bold:true, color:C.white, fontFace:"Malgun Gothic", align:"center" });
    code(s, cmd, { x, y:4.1, w:bw, h:0.72, fs:10 });
  });

  s.addText("모르면 그냥 물어보세요.  wsflow가 안내합니다.", {
    x:0.38, y:5.3, w:W-0.76, h:0.44,
    fontSize:16, color:C.gray, fontFace:"Malgun Gothic", align:"center", italic:true,
  });

  s.addShape(pptx.ShapeType.rect, { x:0, y:H-0.3, w:W, h:0.3, fill:{color:C.sky}, line:{color:C.sky} });
}

// ── 출력 ──────────────────────────────────────────────────────────────
pptx.writeFile({ fileName: "wsflow-seminar-v2.pptx" })
  .then(() => console.log("wsflow-seminar-v2.pptx 생성 완료 (슬라이드 24장)"))
  .catch(console.error);
