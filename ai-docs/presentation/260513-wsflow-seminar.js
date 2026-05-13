#!/usr/bin/env node
// wsflow 워크플로우 세미나 슬라이드 생성 스크립트
// 실행: node make-seminar.js
// 출력: wsflow-seminar.pptx

const PptxGenJS = require("pptxgenjs");
const pptx = new PptxGenJS();

// ── 색상 팔레트 ────────────────────────────────────────────────
const C = {
  navy:    "1B2F5A",   // 진네이비 (배경/헤더)
  navyMid: "2C4A7C",   // 중간 네이비
  sky:     "5BB4DC",   // 스카이블루 (강조)
  skyDim:  "3A8DB5",   // 진한 스카이블루
  white:   "FFFFFF",
  offWhite:"EBF4FA",   // 연한 배경
  gray:    "B0BEC5",
  darkGray:"546E7A",
  black:   "0D1B2A",
  codeBg:  "0F1E36",   // 코드블록 배경
};

pptx.layout = "LAYOUT_WIDE"; // 16:9, 33.867cm x 19.05cm
const W = 13.33; // inches (LAYOUT_WIDE)
const H = 7.5;

// ── 공통 헬퍼 ─────────────────────────────────────────────────
function addSlide(opts = {}) {
  const s = pptx.addSlide();
  const bg = opts.dark ? C.navy : C.white;
  s.background = { color: bg };
  return s;
}

function header(s, title, opts = {}) {
  const dark = opts.dark !== false;
  const bgColor = dark ? C.navyMid : C.offWhite;
  s.addShape(pptx.ShapeType.rect, {
    x: 0, y: 0, w: W, h: 0.72,
    fill: { color: bgColor },
    line: { color: bgColor },
  });
  s.addText(title, {
    x: 0.3, y: 0.06, w: W - 0.6, h: 0.6,
    fontSize: 16, bold: true,
    color: dark ? C.white : C.navy,
    fontFace: "Malgun Gothic",
  });
  // 하단 구분선
  s.addShape(pptx.ShapeType.rect, {
    x: 0, y: 0.72, w: W, h: 0.04,
    fill: { color: C.sky },
    line: { color: C.sky },
  });
}

function footerBar(s) {
  s.addShape(pptx.ShapeType.rect, {
    x: 0, y: H - 0.28, w: W, h: 0.28,
    fill: { color: C.navy },
    line: { color: C.navy },
  });
  s.addText("wsflow 워크플로우 세미나  |  HB Solution", {
    x: 0.3, y: H - 0.28, w: W * 0.6, h: 0.28,
    fontSize: 7.5, color: C.gray, fontFace: "Malgun Gothic",
  });
}

function body(s, text, opts = {}) {
  s.addText(text, {
    x: opts.x ?? 0.35,
    y: opts.y ?? 0.9,
    w: opts.w ?? W - 0.7,
    h: opts.h ?? H - 1.3,
    fontSize: opts.fontSize ?? 11,
    color: opts.color ?? C.black,
    fontFace: "Malgun Gothic",
    valign: opts.valign ?? "top",
    wrap: true,
    bullet: opts.bullet ?? false,
    lineSpacingMultiple: opts.lineSpacing ?? 1.25,
    bold: opts.bold ?? false,
    ...opts.extra,
  });
}

function codeBox(s, code, opts = {}) {
  s.addShape(pptx.ShapeType.rect, {
    x: opts.x ?? 0.35, y: opts.y ?? 1.1,
    w: opts.w ?? W - 0.7, h: opts.h ?? 1.0,
    fill: { color: C.codeBg },
    line: { color: C.navyMid, pt: 1 },
    radius: 3,
  });
  s.addText(code, {
    x: (opts.x ?? 0.35) + 0.15,
    y: (opts.y ?? 1.1) + 0.06,
    w: (opts.w ?? W - 0.7) - 0.3,
    h: (opts.h ?? 1.0) - 0.12,
    fontSize: opts.fontSize ?? 10,
    fontFace: "Cascadia Code",
    color: C.sky,
    valign: "top",
    wrap: true,
    lineSpacingMultiple: 1.3,
  });
}

function pill(s, label, x, y, w, opts = {}) {
  const bg = opts.bg ?? C.sky;
  s.addShape(pptx.ShapeType.roundRect, {
    x, y, w, h: opts.h ?? 0.38,
    fill: { color: bg },
    line: { color: bg },
    arcSize: 50,
  });
  s.addText(label, {
    x, y, w, h: opts.h ?? 0.38,
    fontSize: opts.fontSize ?? 11,
    color: opts.color ?? C.white,
    bold: true,
    align: "center",
    valign: "middle",
    fontFace: "Malgun Gothic",
  });
}

function dividerLabel(s, text, y) {
  s.addShape(pptx.ShapeType.rect, {
    x: 0, y, w: W, h: 0.38,
    fill: { color: C.offWhite },
    line: { color: C.offWhite },
  });
  s.addText(text, {
    x: 0.35, y, w: W - 0.7, h: 0.38,
    fontSize: 11, bold: true, color: C.navy,
    fontFace: "Malgun Gothic", valign: "middle",
  });
}

function arrow(s, x, y) {
  s.addText("→", {
    x, y, w: 0.4, h: 0.38,
    fontSize: 16, color: C.sky, align: "center", valign: "middle",
    fontFace: "Malgun Gothic",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 1: 표지
// ═══════════════════════════════════════════════════════════════
{
  const s = pptx.addSlide();
  s.background = { color: C.navy };

  // 상단 스카이블루 띠
  s.addShape(pptx.ShapeType.rect, {
    x: 0, y: 0, w: W, h: 0.18,
    fill: { color: C.sky }, line: { color: C.sky },
  });

  // 메인 타이틀
  s.addText("wsflow 워크플로우", {
    x: 1.0, y: 1.8, w: 11.3, h: 1.2,
    fontSize: 48, bold: true, color: C.white,
    fontFace: "Malgun Gothic", align: "center",
  });

  // 서브타이틀
  s.addText("AI 보조 개발 워크플로우 도입 가이드", {
    x: 1.0, y: 3.1, w: 11.3, h: 0.6,
    fontSize: 20, color: C.sky,
    fontFace: "Malgun Gothic", align: "center",
  });

  // 구분선
  s.addShape(pptx.ShapeType.rect, {
    x: 4.0, y: 3.85, w: 5.3, h: 0.04,
    fill: { color: C.navyMid }, line: { color: C.navyMid },
  });

  // 날짜 / 회사
  s.addText("HB Solution  |  2026", {
    x: 1.0, y: 4.0, w: 11.3, h: 0.45,
    fontSize: 13, color: C.gray,
    fontFace: "Malgun Gothic", align: "center",
  });

  // 하단 바
  s.addShape(pptx.ShapeType.rect, {
    x: 0, y: H - 0.28, w: W, h: 0.28,
    fill: { color: C.sky }, line: { color: C.sky },
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 2: 목차
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "목차", { dark: false });
  footerBar(s);

  const sections = [
    ["01", "wsflow란?",                   "AI 워크플로우 레이어의 필요성과 개념"],
    ["02", "설치",                     "Claude Code / Codex 플러그인 설치"],
    ["03", "핵심 개념",               "Skill · Ticket · Spec · Mental Model · Git"],
    ["04", "실전",                     "discuss, proceed, /compact 습관"],
    ["05", "wsflow에게 물어보기",          "셀프서비스 학습법"],
    ["06", "레거시 프로젝트 온보딩",  "bootstrap → forge-spec → forge-mental-model"],
    ["07", "치트시트",                 "커맨드 및 플로우 요약"],
    ["08", "팀 협업",                  "GitLab MR 협업 루프 · wsflow:lead-review"],
  ];

  sections.forEach(([num, title, desc], i) => {
    const y = 0.95 + i * 0.67;
    // 번호 pill
    pill(s, num, 0.35, y, 0.5, { h: 0.38, fontSize: 10, bg: C.navy });
    // 섹션 제목
    s.addText(title, {
      x: 0.95, y, w: 2.8, h: 0.38,
      fontSize: 12, bold: true, color: C.navy,
      fontFace: "Malgun Gothic", valign: "middle",
    });
    // 설명
    s.addText(desc, {
      x: 3.85, y, w: 9.1, h: 0.38,
      fontSize: 10, color: C.darkGray,
      fontFace: "Malgun Gothic", valign: "middle",
    });
    // 구분선 (마지막 제외)
    if (i < sections.length - 1) {
      s.addShape(pptx.ShapeType.rect, {
        x: 0.35, y: y + 0.38, w: W - 0.7, h: 0.01,
        fill: { color: C.offWhite }, line: { color: C.offWhite },
      });
    }
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 3: wsflow란? — 문제 정의
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide({ dark: true });
  s.background = { color: C.navy };
  header(s, "01  wsflow란?  —  왜 필요한가", { dark: true });
  footerBar(s);

  s.addText("AI에게 매번 설명하는 반복 작업", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.45,
    fontSize: 15, bold: true, color: C.sky,
    fontFace: "Malgun Gothic",
  });

  const problems = [
    "새 대화를 열 때마다 프로젝트 맥락을 처음부터 설명해야 한다",
    "\"이전에 논의한 방향\"이 다음 세션에서는 없어진다",
    "팀원마다 AI에게 다른 방식으로 지시해 결과물 품질이 제각각이다",
    "어디까지 구현했는지, 어떤 결정을 왜 내렸는지 추적이 어렵다",
  ];

  problems.forEach((p, i) => {
    const y = 1.45 + i * 0.72;
    s.addShape(pptx.ShapeType.rect, {
      x: 0.35, y, w: W - 0.7, h: 0.58,
      fill: { color: C.navyMid }, line: { color: C.navyMid }, radius: 3,
    });
    s.addText(`✗  ${p}`, {
      x: 0.55, y: y + 0.05, w: W - 1.0, h: 0.48,
      fontSize: 11, color: C.white,
      fontFace: "Malgun Gothic", valign: "middle",
    });
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 4: wsflow란? — 솔루션
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "01  wsflow란?  —  솔루션 개념", { dark: false });
  footerBar(s);

  s.addText("wsflow = 프로젝트에 묶인 구조화된 메모리 + 워크플로우 파이프라인", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.45,
    fontSize: 13, bold: true, color: C.navy,
    fontFace: "Malgun Gothic",
  });

  const points = [
    ["프로젝트 메모리", "ai-docs/_index.md 가 세션 시작마다 자동 로드 — 맥락 재설명 불필요"],
    ["구조화된 흐름", "discuss → proceed → implement → commit 파이프라인이 고정 — 누가 써도 일관된 결과"],
    ["추적 가능한 결정", "모든 커밋에 ## AI Context 포함 — 왜 이렇게 만들었는지 git log로 확인"],
    ["팀 공유 규칙",    "AGENTS.md 한 파일에 팀 전체 AI 행동 규칙 — 개인 설정 없이 저장소 단위로 통일"],
  ];

  points.forEach(([title, desc], i) => {
    const y = 1.45 + i * 1.1;
    s.addShape(pptx.ShapeType.rect, {
      x: 0.35, y, w: 0.06, h: 0.85,
      fill: { color: C.sky }, line: { color: C.sky },
    });
    s.addText(title, {
      x: 0.55, y, w: W - 0.9, h: 0.35,
      fontSize: 12, bold: true, color: C.navy,
      fontFace: "Malgun Gothic",
    });
    s.addText(desc, {
      x: 0.55, y: y + 0.34, w: W - 0.9, h: 0.48,
      fontSize: 10.5, color: C.darkGray,
      fontFace: "Malgun Gothic",
    });
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 5: wsflow란? — 구조 다이어그램
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "01  wsflow란?  —  구조", { dark: false });
  footerBar(s);

  // 레이어 박스들 (위에서 아래로)
  const layers = [
    { label: "개발자 (You)", sub: "/wsflow:lead-discuss  /wsflow:lead-proceed  …", bg: C.sky, fg: C.white },
    { label: "wsflow 플러그인", sub: "Skill 라우팅 · 파이프라인 · 프로젝트 메모리 (ai-docs/)", bg: C.navy, fg: C.white },
    { label: "Claude Code  /  Codex", sub: "AI 백엔드 (LLM 실행 환경)", bg: C.navyMid, fg: C.gray },
  ];

  layers.forEach((l, i) => {
    const y = 1.1 + i * 1.6;
    s.addShape(pptx.ShapeType.roundRect, {
      x: 1.5, y, w: 10.3, h: 1.3,
      fill: { color: l.bg }, line: { color: l.bg }, arcSize: 6,
    });
    s.addText(l.label, {
      x: 1.7, y: y + 0.12, w: 9.9, h: 0.42,
      fontSize: 14, bold: true, color: l.fg,
      fontFace: "Malgun Gothic", align: "center",
    });
    s.addText(l.sub, {
      x: 1.7, y: y + 0.55, w: 9.9, h: 0.55,
      fontSize: 9.5, color: i === 0 ? C.white : (i === 1 ? C.sky : C.gray),
      fontFace: "Malgun Gothic", align: "center",
    });
    // 아래 화살표 (마지막 제외)
    if (i < layers.length - 1) {
      s.addText("↕", {
        x: 6.1, y: y + 1.3, w: 1.1, h: 0.3,
        fontSize: 14, color: C.sky, align: "center",
        fontFace: "Malgun Gothic",
      });
    }
  });

  s.addText("유저는 wsflow 스킬만 호출합니다. 내부 파이프라인은 wsflow가 처리합니다.", {
    x: 0.35, y: 6.75, w: W - 0.7, h: 0.35,
    fontSize: 9.5, color: C.darkGray, align: "center",
    fontFace: "Malgun Gothic",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 6: wsflow란? — 가치 요약
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide({ dark: true });
  s.background = { color: C.navy };
  header(s, "01  wsflow란?  —  한 줄 요약", { dark: true });
  footerBar(s);

  s.addText('"세션이 끊겨도 맥락이 남고,\n누가 써도 같은 품질이 나오는\nAI 개발 환경"', {
    x: 1.0, y: 1.3, w: W - 2.0, h: 3.2,
    fontSize: 26, bold: true, color: C.white,
    fontFace: "Malgun Gothic", align: "center", valign: "middle",
    lineSpacingMultiple: 1.5,
  });

  s.addShape(pptx.ShapeType.rect, {
    x: 3.0, y: 4.7, w: 7.3, h: 0.04,
    fill: { color: C.sky }, line: { color: C.sky },
  });

  s.addText("wsflow는 Claude Code / Codex 위에 얹히는 플러그인입니다. AI 모델 자체는 그대로 사용합니다.", {
    x: 0.35, y: 4.85, w: W - 0.7, h: 0.38,
    fontSize: 10, color: C.gray, align: "center",
    fontFace: "Malgun Gothic",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 7: 설치 — Claude Code
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "02  설치  —  Claude Code (표준)", { dark: false });
  footerBar(s);

  s.addText("마켓플레이스 등록 후 플러그인 설치 — 두 단계", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.38,
    fontSize: 12, bold: true, color: C.navy, fontFace: "Malgun Gothic",
  });

  // Step 1
  dividerLabel(s, "Step 1  —  마켓플레이스 등록 (최초 1회)", 1.38);
  codeBox(s, "claude plugin marketplace add kang-sw/devenv", {
    x: 0.35, y: 1.82, w: W - 0.7, h: 0.62,
  });

  // Step 2
  dividerLabel(s, "Step 2  —  플러그인 설치", 2.58);
  codeBox(s, "claude plugin install wsflow@kang-sw-devenv", {
    x: 0.35, y: 3.02, w: W - 0.7, h: 0.62,
  });

  // 업그레이드
  dividerLabel(s, "업그레이드 (신규 버전 배포 후)", 3.78);
  codeBox(s, "claude plugin upgrade wsflow@kang-sw-devenv", {
    x: 0.35, y: 4.22, w: W - 0.7, h: 0.62,
  });

  s.addText("설치 후 Claude Code를 재시작하면 /wsflow:lead-* 스킬이 활성화됩니다.", {
    x: 0.35, y: 5.0, w: W - 0.7, h: 0.35,
    fontSize: 9.5, color: C.darkGray, fontFace: "Malgun Gothic",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 8: 설치 — Codex (보너스)
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "02  설치  —  Codex (보너스)", { dark: false });
  footerBar(s);

  s.addText("Codex CLI 또는 UI에서 설치 가능합니다.", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.38,
    fontSize: 12, bold: true, color: C.navy, fontFace: "Malgun Gothic",
  });

  dividerLabel(s, "CLI 방식", 1.38);
  codeBox(s, "codex plugin marketplace add kang-sw/devenv", {
    x: 0.35, y: 1.82, w: W - 0.7, h: 0.62,
  });

  dividerLabel(s, "UI 방식 (Codex 앱)", 2.58);
  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: 3.02, w: W - 0.7, h: 1.05,
    fill: { color: C.offWhite }, line: { color: C.sky, pt: 1 }, arcSize: 4,
  });
  s.addText(
    "Codex 실행  →  /plugins  →  마켓플레이스에서 kang-sw/devenv 검색  →  wsflow 설치",
    {
      x: 0.55, y: 3.12, w: W - 0.9, h: 0.85,
      fontSize: 11, color: C.navy, fontFace: "Malgun Gothic", valign: "middle",
    }
  );

  s.addText("※ 이후 내용은 모두 Claude Code 기준으로 설명합니다.", {
    x: 0.35, y: 4.3, w: W - 0.7, h: 0.35,
    fontSize: 9.5, color: C.darkGray, fontFace: "Malgun Gothic",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 9: 핵심 개념 — 개요
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "03  핵심 개념  —  개요", { dark: false });
  footerBar(s);

  const concepts = [
    { icon: "⚡", name: "Skill",         desc: "AI에게 내리는 구조화된 명령 단위. /wsflow:lead-discuss 등" },
    { icon: "🎫", name: "Ticket",        desc: "작업 단위. 상태(idea→todo→ready→done)로 진행 관리" },
    { icon: "📋", name: "Spec",          desc: "기능의 외부 동작 계약서. 구현과 분리된 행동 기술" },
    { icon: "🧠", name: "Mental Model",  desc: "수정 시 알아야 할 모듈 계약·결합·주의사항" },
    { icon: "📁", name: "ai-docs/",      desc: "프로젝트 메모리 폴더. 세션 시작마다 AI가 자동 로드" },
    { icon: "📝", name: "Git & Commit",  desc: "커밋마다 ## AI Context 포함 — 의사결정 추적" },
  ];

  concepts.forEach((c, i) => {
    const col = i % 2;
    const row = Math.floor(i / 2);
    const x = 0.35 + col * 6.5;
    const y = 1.0 + row * 1.75;

    s.addShape(pptx.ShapeType.roundRect, {
      x, y, w: 6.2, h: 1.55,
      fill: { color: C.offWhite }, line: { color: C.sky, pt: 1 }, arcSize: 4,
    });
    s.addText(c.icon, {
      x: x + 0.15, y: y + 0.1, w: 0.7, h: 1.25,
      fontSize: 22, align: "center", valign: "middle", fontFace: "Segoe UI Emoji",
    });
    s.addText(c.name, {
      x: x + 0.9, y: y + 0.12, w: 5.1, h: 0.42,
      fontSize: 12, bold: true, color: C.navy, fontFace: "Malgun Gothic",
    });
    s.addText(c.desc, {
      x: x + 0.9, y: y + 0.54, w: 5.1, h: 0.85,
      fontSize: 10, color: C.darkGray, fontFace: "Malgun Gothic",
      lineSpacingMultiple: 1.2,
    });
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 10: Skill
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "03  핵심 개념  —  Skill", { dark: false });
  footerBar(s);

  s.addText(
    "Skill은 AI에게 내리는 구조화된 명령 단위입니다.\n" +
    "Claude Code에서 /wsflow:lead-<이름> 형식으로 호출합니다.",
    {
      x: 0.35, y: 0.88, w: W - 0.7, h: 0.62,
      fontSize: 11, color: C.navy, fontFace: "Malgun Gothic", lineSpacingMultiple: 1.4,
    }
  );

  dividerLabel(s, "주요 스킬 목록", 1.6);

  const skills = [
    ["wsflow:lead-discuss",            "방향 논의. 코드 변경 없이 설계·범위·리스크를 탐색"],
    ["wsflow:lead-proceed",            "실행 진입. 파이프라인을 자동 체이닝하여 구현까지 진행"],
    ["wsflow:lead-implement",          "실제 코드 구현 실행 (proceed가 자동 호출)"],
    ["wsflow:lead-bootstrap",          "신규/레거시 프로젝트에 wsflow 워크플로우 구조 초기 설정"],
    ["wsflow:lead-forge-spec",         "코드베이스를 분석해 spec 문서 일괄 생성"],
    ["wsflow:lead-forge-mental-model", "도메인별 mental model 문서 일괄 생성"],
  ];

  skills.forEach(([name, desc], i) => {
    const y = 2.04 + i * 0.64;
    s.addText(name, {
      x: 0.35, y, w: 4.3, h: 0.5,
      fontSize: 10, fontFace: "Cascadia Code", color: C.skyDim, valign: "middle",
    });
    s.addText(desc, {
      x: 4.75, y, w: 8.2, h: 0.5,
      fontSize: 10, fontFace: "Malgun Gothic", color: C.darkGray, valign: "middle",
    });
    if (i < skills.length - 1) {
      s.addShape(pptx.ShapeType.rect, {
        x: 0.35, y: y + 0.5, w: W - 0.7, h: 0.01,
        fill: { color: C.offWhite }, line: { color: C.offWhite },
      });
    }
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 11: Ticket
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "03  핵심 개념  —  Ticket", { dark: false });
  footerBar(s);

  s.addText(
    "Ticket은 작업 단위입니다. 파일 시스템의 디렉터리가 곧 상태입니다.\n" +
    "stem(파일명)이 ID이며, 이동해도 stem은 변하지 않습니다.",
    {
      x: 0.35, y: 0.88, w: W - 0.7, h: 0.62,
      fontSize: 11, color: C.navy, fontFace: "Malgun Gothic", lineSpacingMultiple: 1.4,
    }
  );

  // 상태 흐름 다이어그램
  const statuses = ["idea/", "todo/", "ready/", ".done/"];
  const colors   = [C.gray, C.darkGray, C.sky, C.navy];
  const sx = [0.5, 3.3, 6.1, 8.9];

  statuses.forEach((st, i) => {
    s.addShape(pptx.ShapeType.roundRect, {
      x: sx[i], y: 1.7, w: 2.5, h: 0.75,
      fill: { color: colors[i] }, line: { color: colors[i] }, arcSize: 8,
    });
    s.addText(st, {
      x: sx[i], y: 1.7, w: 2.5, h: 0.75,
      fontSize: 13, bold: true, color: C.white,
      fontFace: "Cascadia Code", align: "center", valign: "middle",
    });
    if (i < statuses.length - 1) {
      arrow(s, sx[i] + 2.5, 1.78);
    }
  });

  // 설명
  dividerLabel(s, "티켓 파일 구조", 2.72);
  codeBox(s,
    "ai-docs/tickets/ready/260512-add-auth-module.md\n\n" +
    "# Add Auth Module\n" +
    "## Phases\n" +
    "### Phase 1: JWT 검증 구현\n" +
    "### Result\n" +
    "  jwt.go 추가, middleware 연결 완료",
    { x: 0.35, y: 3.16, w: W - 0.7, h: 2.35, fontSize: 9.5 }
  );

  s.addText("티켓은 wsflow:lead-discuss 종료 시 자동 생성/업데이트됩니다.", {
    x: 0.35, y: 5.7, w: W - 0.7, h: 0.32,
    fontSize: 9.5, color: C.darkGray, fontFace: "Malgun Gothic",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 12: Spec
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "03  핵심 개념  —  Spec", { dark: false });
  footerBar(s);

  s.addText(
    "Spec은 기능의 외부 동작 계약서입니다.\n" +
    "구현 방법이 아닌 '어떻게 동작해야 하는가'를 기술합니다.",
    {
      x: 0.35, y: 0.88, w: W - 0.7, h: 0.62,
      fontSize: 11, color: C.navy, fontFace: "Malgun Gothic", lineSpacingMultiple: 1.4,
    }
  );

  dividerLabel(s, "Spec 파일 예시  —  ai-docs/spec/auth.md", 1.6);
  codeBox(s,
    "## JWT 검증 {#260512-jwt-verify}\n" +
    "토큰이 만료되었을 때 401을 반환한다.\n\n" +
    "## 🚧 OAuth 연동 {#260512-oauth-link}\n" +
    "GitHub OAuth로 로그인할 수 있다.  ← 계획 중",
    { x: 0.35, y: 2.04, w: W - 0.7, h: 2.0, fontSize: 9.5 }
  );

  const rules = [
    ["{#날짜-slug}", "각 항목마다 안정적인 앵커 부여 — 티켓·커밋에서 참조"],
    ["🚧 마커",      "구현 예정 항목 표시. 완료되면 마커 제거"],
    ["ai-docs/spec/", "모든 spec 파일의 위치. wsflow가 자동 인덱싱"],
  ];

  rules.forEach(([term, desc], i) => {
    const y = 4.2 + i * 0.65;
    s.addText(term, {
      x: 0.35, y, w: 2.2, h: 0.52,
      fontSize: 10, fontFace: "Cascadia Code", color: C.sky, valign: "middle",
    });
    s.addText(desc, {
      x: 2.65, y, w: W - 3.0, h: 0.52,
      fontSize: 10, fontFace: "Malgun Gothic", color: C.darkGray, valign: "middle",
    });
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 13: Mental Model
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "03  핵심 개념  —  Mental Model", { dark: false });
  footerBar(s);

  s.addText(
    "Mental Model은 코드를 수정할 때 알아야 할 운영 지식입니다.\n" +
    "'이걸 바꾸면 저것도 바꿔야 한다'는 내용이 여기에 들어갑니다.",
    {
      x: 0.35, y: 0.88, w: W - 0.7, h: 0.62,
      fontSize: 11, color: C.navy, fontFace: "Malgun Gothic", lineSpacingMultiple: 1.4,
    }
  );

  dividerLabel(s, "포함되는 내용", 1.6);

  const items = [
    ["모듈 계약",   "이 모듈을 수정할 때 반드시 함께 변경해야 하는 다른 모듈"],
    ["결합 지점",   "공유 상태, 초기화 순서, 이벤트 순서 등 숨겨진 의존성"],
    ["확장 포인트", "플러그인 레지스트리, 열거형 추가 규칙 등"],
    ["자주 하는 실수", "놓치면 조용히 오작동하는 케이스들"],
  ];

  items.forEach(([title, desc], i) => {
    const y = 2.04 + i * 0.82;
    s.addShape(pptx.ShapeType.roundRect, {
      x: 0.35, y, w: W - 0.7, h: 0.68,
      fill: { color: C.offWhite }, line: { color: C.sky, pt: 1 }, arcSize: 4,
    });
    s.addText(title, {
      x: 0.55, y: y + 0.06, w: 2.8, h: 0.52,
      fontSize: 11, bold: true, color: C.navy, fontFace: "Malgun Gothic", valign: "middle",
    });
    s.addText(desc, {
      x: 3.4, y: y + 0.06, w: W - 3.8, h: 0.52,
      fontSize: 10.5, color: C.darkGray, fontFace: "Malgun Gothic", valign: "middle",
    });
  });

  s.addText(
    "Spec이 '무엇을 하는가'라면, Mental Model은 '어떻게 안전하게 바꾸는가'입니다.",
    {
      x: 0.35, y: 5.42, w: W - 0.7, h: 0.35,
      fontSize: 9.5, color: C.darkGray, fontFace: "Malgun Gothic",
    }
  );
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 14: ai-docs/ 구조
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "03  핵심 개념  —  ai-docs/ 구조", { dark: false });
  footerBar(s);

  s.addText("프로젝트 루트의 ai-docs/ 폴더가 wsflow의 프로젝트 메모리입니다.", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.38,
    fontSize: 11, color: C.navy, fontFace: "Malgun Gothic",
  });

  codeBox(s,
    "ai-docs/\n" +
    "  _index.md          ← 세션 시작마다 AI가 읽는 핵심 메모 (적극적으로 정리)\n" +
    "  _index.local.md    ← 머신 로컬 메모 (.gitignore)\n" +
    "  tickets/           ← 티켓 (idea/ todo/ ready/ .done/ .dropped/)\n" +
    "  spec/              ← 기능 계약서\n" +
    "  mental-model/      ← 도메인별 수정 가이드\n" +
    "  ref/               ← 정적 참고 자료\n" +
    "  WORKFLOW.md        ← wsflow 가이드 (bootstrap이 자동 복사)",
    { x: 0.35, y: 1.38, w: W - 0.7, h: 2.9, fontSize: 9.5 }
  );

  s.addText("_index.md는 완료된 작업을 적극적으로 제거해 간결하게 유지합니다.\n완료 이력은 git log에 남아 있습니다.", {
    x: 0.35, y: 4.42, w: W - 0.7, h: 0.55,
    fontSize: 10, color: C.darkGray, fontFace: "Malgun Gothic", lineSpacingMultiple: 1.4,
  });

  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: 5.1, w: W - 0.7, h: 0.52,
    fill: { color: C.offWhite }, line: { color: C.sky, pt: 1 }, arcSize: 4,
  });
  s.addText("wsflow:lead-bootstrap 실행 시 이 폴더 구조가 자동으로 생성됩니다.", {
    x: 0.55, y: 5.1, w: W - 1.0, h: 0.52,
    fontSize: 10, color: C.navy, fontFace: "Malgun Gothic", valign: "middle",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 15: Git & Commit
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "03  핵심 개념  —  Git & Commit", { dark: false });
  footerBar(s);

  s.addText(
    "wsflow는 작업 단위마다 커밋을 자동 생성합니다.\n커밋 메시지에 ## AI Context 섹션을 포함해 의사결정을 추적합니다.",
    {
      x: 0.35, y: 0.88, w: W - 0.7, h: 0.62,
      fontSize: 11, color: C.navy, fontFace: "Malgun Gothic", lineSpacingMultiple: 1.4,
    }
  );

  dividerLabel(s, "커밋 메시지 형식", 1.6);
  codeBox(s,
    "feat(auth): JWT 검증 미들웨어 추가\n\n" +
    "토큰 만료 시 401 반환, 서명 검증 실패 시 403 반환\n\n" +
    "## AI Context\n" +
    "- RS256 대신 HS256 선택: 서버 간 키 공유 불필요\n" +
    "- 기존 middleware 체이닝 방식 유지 (breaking change 없음)\n\n" +
    "## Spec\n" +
    "- 260512-jwt-verify",
    { x: 0.35, y: 2.04, w: W - 0.7, h: 2.55, fontSize: 9.5 }
  );

  const rules = [
    "type(scope): summary  —  Conventional Commits 형식",
    "## AI Context  —  왜 이 방식을 선택했는지, 기각한 대안",
    "## Spec  —  변경된 spec 앵커 (동작 변경이 있을 때)",
  ];
  rules.forEach((r, i) => {
    s.addText(`• ${r}`, {
      x: 0.35, y: 4.74 + i * 0.38, w: W - 0.7, h: 0.35,
      fontSize: 10, color: C.darkGray, fontFace: "Malgun Gothic",
    });
  });

  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: 5.95, w: W - 0.7, h: 0.52,
    fill: { color: C.offWhite }, line: { color: C.sky, pt: 1 }, arcSize: 4,
  });
  s.addText("💡  AI Context 섹션이 MR 리뷰어에게 의도를 전달하는 재료입니다.  wsflow로 작업하면 협업 인프라가 자동으로 쌓입니다.", {
    x: 0.55, y: 5.95, w: W - 1.0, h: 0.52,
    fontSize: 10, color: C.navy, fontFace: "Malgun Gothic", valign: "middle",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 16: 실전 — discuss
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide({ dark: true });
  s.background = { color: C.navy };
  header(s, "04  실전  —  워크플로우 예시", { dark: true });
  footerBar(s);

  s.addText("작업 전에 논의를 시작하는 스킬입니다. 코드는 건드리지 않고, 방향·범위·리스크를 탐색합니다.", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.3,
    fontSize: 10.5, color: C.sky, fontFace: "Malgun Gothic",
  });

  // 대화 + 프로세스 블록
  // type: "user" | "wsflow" | "process"
  const conv = [
    ["user", "/wsflow:lead-discuss  지금 모터 컨트롤러 제품을 바꾸려고 하는데 어느 부분을 건드려야 하나요?", null],
    ["wsflow",   "motor_controller/의 VendorAdapter가 연결 지점입니다. 새 벤더 추가 vs. 인터페이스 교체 두 방향을 검토할 수 있습니다.", null],
    ["user", "그러면 새 벤더보다 인터페이스를 새로 추가하자는 말인가요?", null],
    ["wsflow",   "맞습니다. 인터페이스 분리 시 기존 코드 수정 없이 신규 벤더를 추가할 수 있습니다.", null],
    ["user", "좋아요, 그러면 내용을 티켓으로 정리해 주세요.", "→ wsflow가 티켓 자동 작성"],
    ["wsflow",   "티켓 생성 완료. /wsflow:lead-proceed로 구현을 시작할 수 있습니다.", null],
    ["user", "/wsflow:lead-proceed  구현 시작해주세요", "→ 파이프라인 자동 실행"],
    // proceed 이후 wsflow 내부 처리 흐름
    ["process", null, null],
    ["user", "Merge해도 좋습니다.", "→ 커밋 후 main 머지"],
  ];

  // 프로세스 단계 텍스트
  const processSteps = [
    "ticket → ready 로 올리는 중 ...",
    "관련 spec 업데이트 중 ...",
    "구현 라우트:  write-skeleton → write-code → review (correctness · fit · test)",
    "구현 완료.  spec 및 mental-model 갱신 중 ...",
    "구현 절차가 끝났습니다.  유저 승인 대기 중.",
  ];
  const processH = 0.14 + processSteps.length * 0.27;

  const bW    = 9.6;
  const noteW = W - bW - 0.35 - 0.35 - 0.1;
  const userX = W - bW - 0.35;
  const wsX   = 0.35;

  let cy = 1.26;

  conv.forEach(([type, text, note]) => {
    if (type === "process") {
      // ── 프로세스 로그 박스 ──────────────────────────────
      s.addShape(pptx.ShapeType.roundRect, {
        x: 0.35, y: cy, w: W - 0.7, h: processH,
        fill: { color: C.codeBg }, line: { color: C.navyMid, pt: 1 }, arcSize: 4,
      });
      // 좌측 스카이블루 액센트 바
      s.addShape(pptx.ShapeType.rect, {
        x: 0.35, y: cy, w: 0.05, h: processH,
        fill: { color: C.sky }, line: { color: C.sky },
      });
      processSteps.forEach((step, si) => {
        const isLast = si === processSteps.length - 1;
        s.addText(`●  ${step}`, {
          x: 0.55, y: cy + 0.07 + si * 0.27, w: W - 1.0, h: 0.25,
          fontSize: 9, fontFace: "Malgun Gothic",
          color: isLast ? C.sky : C.gray,
          valign: "middle",
        });
      });
      cy += processH + 0.07;
      return;
    }

    // ── 채팅 버블 ──────────────────────────────────────────
    const isUser = type === "user";
    const x      = isUser ? userX : wsX;
    const bg     = isUser ? C.navyMid : C.skyDim;
    const lines  = Math.ceil(text.length / 72);
    const bh     = 0.1 + lines * 0.26;

    s.addShape(pptx.ShapeType.roundRect, {
      x, y: cy, w: bW, h: bh,
      fill: { color: bg }, line: { color: bg }, arcSize: 14,
    });
    s.addText(text, {
      x: x + 0.14, y: cy, w: bW - 0.28, h: bh,
      fontSize: 9.5, color: C.white, fontFace: "Malgun Gothic",
      valign: "middle", lineSpacingMultiple: 1.15,
    });

    if (note) {
      const nx = isUser ? 0.35 : wsX + bW + 0.1;
      s.addText(note, {
        x: nx, y: cy, w: noteW, h: bh,
        fontSize: 8.5, color: C.sky, fontFace: "Malgun Gothic",
        valign: "middle",
      });
    }

    cy += bh + 0.07;
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 17: 실전 — discuss
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "04  실전  —  discuss", { dark: false });
  footerBar(s);

  // 핵심 원칙 강조
  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.62,
    fill: { color: C.offWhite }, line: { color: C.sky, pt: 2 }, arcSize: 4,
  });
  s.addText("항상 행동 착수 직전까지 고삐를 매세요.", {
    x: 0.55, y: 0.88, w: W - 1.1, h: 0.62,
    fontSize: 15, bold: true, color: C.navy,
    fontFace: "Malgun Gothic", valign: "middle", align: "center",
  });

  // ❌ / ✓ 대비 (좌/우 2분할)
  const colW  = (W - 1.05) / 2;
  const colY  = 1.60;
  const colH  = 4.0;
  const padX  = 0.22;   // 내부 좌우 여백
  const padT  = 0.22;   // 내부 상단 여백
  const padB  = 0.20;   // 내부 하단 여백
  const codeH = 0.68;
  const codeGap = 0.16;
  const titleH = 0.36;
  // 코드박스 시작 y (제목 아래 0.20 gap)
  const codeStart = padT + titleH + 0.20;

  // ❌ 왼쪽
  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: colY, w: colW, h: colH,
    fill: { color: "FFF0F0" }, line: { color: "E57373", pt: 1 }, arcSize: 4,
  });
  s.addText("❌  명령형", {
    x: 0.35 + padX, y: colY + padT, w: colW - padX * 2, h: titleH,
    fontSize: 12, bold: true, color: "C62828", fontFace: "Malgun Gothic",
  });
  const badExamples = [
    "\"모터 컨트롤러 인터페이스를 교체해줘\"",
    "\"이 부분 리팩터링해줘\"",
    "\"새 API 엔드포인트 추가해줘\"",
  ];
  badExamples.forEach((ex, i) => {
    codeBox(s, ex, {
      x: 0.35 + padX, y: colY + codeStart + i * (codeH + codeGap),
      w: colW - padX * 2, h: codeH, fontSize: 9.5,
    });
  });
  const badTextY = colY + colH - padB - 0.42;
  s.addText("AI가 즉시 구현을 시작합니다.\n리스크·대안 탐색 없이 방향이 고정됩니다.", {
    x: 0.35 + padX, y: badTextY, w: colW - padX * 2, h: 0.42,
    fontSize: 9.5, color: "C62828", fontFace: "Malgun Gothic",
    lineSpacingMultiple: 1.3,
  });

  // ✓ 오른쪽
  const rx = 0.35 + colW + 0.35;
  s.addShape(pptx.ShapeType.roundRect, {
    x: rx, y: colY, w: colW, h: colH,
    fill: { color: "F0F8FF" }, line: { color: C.sky, pt: 1 }, arcSize: 4,
  });
  s.addText("✓  질문형  (권장)", {
    x: rx + padX, y: colY + padT, w: colW - padX * 2, h: titleH,
    fontSize: 12, bold: true, color: C.skyDim, fontFace: "Malgun Gothic",
  });
  const goodExamples = [
    "\"인터페이스 교체 방향, 어떻게 생각해요?\"",
    "\"이렇게 리팩터링하면 어떤 리스크가 있을까요?\"",
    "\"이 방향이 맞는지 의견 듣고 싶어요\"",
  ];
  goodExamples.forEach((ex, i) => {
    codeBox(s, ex, {
      x: rx + padX, y: colY + codeStart + i * (codeH + codeGap),
      w: colW - padX * 2, h: codeH, fontSize: 9.5,
    });
  });
  const goodTextY = colY + colH - padB - 0.42;
  s.addText("wsflow가 리스크·대안·연결 지점을 먼저 짚어줍니다.\n방향 확정 후 proceed로 넘깁니다.", {
    x: rx + padX, y: goodTextY, w: colW - padX * 2, h: 0.42,
    fontSize: 9.5, color: C.skyDim, fontFace: "Malgun Gothic",
    lineSpacingMultiple: 1.3,
  });

  // 하단 원칙 박스
  const footY = colY + colH + 0.18;
  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: footY, w: W - 0.7, h: 0.48,
    fill: { color: C.navy }, line: { color: C.navy }, arcSize: 4,
  });
  s.addText(
    "discuss는 코드를 건드리지 않습니다.  실행은 오직 proceed 호출로만 시작됩니다.",
    {
      x: 0.55, y: footY, w: W - 1.1, h: 0.48,
      fontSize: 10.5, color: C.white, fontFace: "Malgun Gothic",
      valign: "middle", align: "center",
    }
  );
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 18 (구: 17): 실전 — proceed
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "04  실전  —  proceed", { dark: false });
  footerBar(s);

  s.addText(
    "proceed는 구현 진입을 자동으로 라우팅합니다.\n" +
    "티켓 경로나 간단한 설명을 주면 파이프라인을 체이닝해 구현까지 진행합니다.",
    {
      x: 0.35, y: 0.88, w: W - 0.7, h: 0.62,
      fontSize: 11, color: C.navy, fontFace: "Malgun Gothic", lineSpacingMultiple: 1.4,
    }
  );

  dividerLabel(s, "자동 파이프라인", 1.6);

  // 파이프라인 다이어그램
  const pipeline = ["write-spec", "write-ticket", "implement", "commit"];
  const pipeX = [0.55, 3.3, 6.05, 8.8];
  pipeline.forEach((p, i) => {
    const isKey = i === 2;
    s.addShape(pptx.ShapeType.roundRect, {
      x: pipeX[i], y: 2.08, w: 2.3, h: 0.72,
      fill: { color: isKey ? C.sky : C.offWhite },
      line: { color: isKey ? C.sky : C.navyMid, pt: 1 },
      arcSize: 8,
    });
    s.addText(p, {
      x: pipeX[i], y: 2.08, w: 2.3, h: 0.72,
      fontSize: 9.5, fontFace: "Cascadia Code",
      color: isKey ? C.white : C.navy,
      align: "center", valign: "middle",
    });
    if (i < pipeline.length - 1) {
      s.addText("→", {
        x: pipeX[i] + 2.3, y: 2.12, w: 0.55, h: 0.62,
        fontSize: 14, color: C.sky, align: "center", valign: "middle",
        fontFace: "Malgun Gothic",
      });
    }
  });

  s.addText("필요 없는 단계는 자동으로 skip 됩니다.", {
    x: 0.35, y: 2.9, w: W - 0.7, h: 0.32,
    fontSize: 9.5, color: C.darkGray, fontFace: "Malgun Gothic",
  });

  dividerLabel(s, "사용 예시", 3.3);
  codeBox(s,
    "# 티켓 경로 지정\n" +
    "/wsflow:lead-proceed ai-docs/tickets/ready/260512-add-auth.md\n\n" +
    "# 인라인 설명 (티켓 없을 때)\n" +
    "/wsflow:lead-proceed  JWT 검증 미들웨어를 추가해줘",
    { x: 0.35, y: 3.74, w: W - 0.7, h: 1.7, fontSize: 9.5 }
  );

  s.addText("proceed 한 번으로 spec 작성 → 티켓 생성 → 코드 구현 → 커밋까지 완료됩니다.", {
    x: 0.35, y: 5.6, w: W - 0.7, h: 0.32,
    fontSize: 9.5, color: C.darkGray, fontFace: "Malgun Gothic",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 18: 실전 — /compact 습관
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "04  실전  —  /compact 습관", { dark: false });
  footerBar(s);

  s.addText("작업 하나가 끝날 때마다 /compact를 실행하세요.", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.42,
    fontSize: 13, bold: true, color: C.navy, fontFace: "Malgun Gothic",
  });

  codeBox(s, "/compact", { x: 0.35, y: 1.42, w: 4.0, h: 0.62, fontSize: 14 });

  s.addText("(Claude Code 내장 커맨드)", {
    x: 4.5, y: 1.52, w: 5.0, h: 0.42,
    fontSize: 10, color: C.darkGray, fontFace: "Malgun Gothic", valign: "middle",
  });

  dividerLabel(s, "왜 필요한가", 2.2);

  const reasons = [
    ["컨텍스트 노이즈 제거",
     "대화가 길어질수록 초반의 시행착오, 취소된 시도, 중간 결과물이 AI 컨텍스트에 쌓입니다.\n" +
     "/compact는 이를 핵심 요약으로 압축해 다음 작업에 깨끗한 컨텍스트를 제공합니다."],
    ["비용·속도 개선",
     "컨텍스트 크기가 작을수록 응답이 빠르고 토큰 비용이 줄어듭니다."],
    ["wsflow와의 궁합",
     "wsflow가 세션 시작마다 _index.md를 로드하므로, compact 후에도 프로젝트 맥락은 유지됩니다."],
  ];

  reasons.forEach(([title, desc], i) => {
    const y = 2.64 + i * 1.3;
    s.addShape(pptx.ShapeType.rect, {
      x: 0.35, y, w: 0.06, h: 1.1,
      fill: { color: C.sky }, line: { color: C.sky },
    });
    s.addText(title, {
      x: 0.55, y, w: W - 0.9, h: 0.38,
      fontSize: 11, bold: true, color: C.navy, fontFace: "Malgun Gothic",
    });
    s.addText(desc, {
      x: 0.55, y: y + 0.38, w: W - 0.9, h: 0.72,
      fontSize: 10, color: C.darkGray, fontFace: "Malgun Gothic", lineSpacingMultiple: 1.3,
    });
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 19: 실전 — 전체 플로우
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide({ dark: true });
  s.background = { color: C.navy };
  header(s, "04  실전  —  전체 워크플로우", { dark: true });
  footerBar(s);

  // 플로우 행 렌더 헬퍼 (overflow 방지: arrow 폭 포함해 bw 계산)
  function flowRow(items, y, bh, highlightIdx) {
    const arrowW = 0.28;
    const n = items.length;
    const totalW = W - 0.7;
    const bw = (totalW - arrowW * (n - 1)) / n;
    items.forEach((label, i) => {
      const x = 0.35 + i * (bw + arrowW);
      const isHL = i === highlightIdx;
      s.addShape(pptx.ShapeType.roundRect, {
        x, y, w: bw, h: bh,
        fill: { color: isHL ? C.skyDim : C.navyMid },
        line: { color: C.sky, pt: 1 },
        arcSize: 8,
      });
      s.addText(label, {
        x, y, w: bw, h: bh,
        fontSize: 9.5, fontFace: "Cascadia Code",
        color: C.white, align: "center", valign: "middle",
      });
      if (i < n - 1) {
        s.addText("→", {
          x: x + bw, y, w: arrowW, h: bh,
          fontSize: 11, color: C.sky, align: "center", valign: "middle",
          fontFace: "Malgun Gothic",
        });
      }
    });
  }

  const bh = 0.58;
  const labelH = 0.32;
  const gap = 0.12;

  // 패턴 A
  s.addText("패턴 A  —  논의 후 구현 (권장)", {
    x: 0.35, y: 0.86, w: W - 0.7, h: labelH,
    fontSize: 11, bold: true, color: C.sky, fontFace: "Malgun Gothic",
  });
  flowRow(["discuss", "proceed", "implement", "commit", "/compact"], 0.86 + labelH + 0.06, bh, 4);

  // 패턴 B
  const yB = 0.86 + labelH + 0.06 + bh + gap;
  s.addText("패턴 B  —  직접 구현 (소규모 작업)", {
    x: 0.35, y: yB, w: W - 0.7, h: labelH,
    fontSize: 11, bold: true, color: C.sky, fontFace: "Malgun Gothic",
  });
  flowRow(["proceed <설명>", "implement", "commit", "/compact"], yB + labelH + 0.06, bh, 3);

  // 패턴 C
  const yC = yB + labelH + 0.06 + bh + 0.2;
  s.addText("패턴 C  —  MR 리뷰 (메인테이너)", {
    x: 0.35, y: yC, w: W - 0.7, h: labelH,
    fontSize: 11, bold: true, color: C.sky, fontFace: "Malgun Gothic",
  });
  flowRow(["lead-review <브랜치>", "LGTM / FIX / OPEN", "머지 (LGTM 시)"], yC + labelH + 0.06, bh, 1);

  const yNote = yC + labelH + 0.06 + bh + 0.2;
  s.addText("개인 작업은 proceed · /compact, MR 리뷰는 lead-review 하나로 진행합니다.", {
    x: 0.35, y: yNote, w: W - 0.7, h: 0.32,
    fontSize: 10, color: C.gray, fontFace: "Malgun Gothic", align: "center",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 20: ai-docs 사용 철학
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "05  wsflow에게 물어보기  —  ai-docs 사용 방식", { dark: false });
  footerBar(s);

  // 핵심 메시지
  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.58,
    fill: { color: C.navy }, line: { color: C.navy }, arcSize: 4,
  });
  s.addText("ai-docs는 사람이 직접 읽는 문서가 아닙니다.  AI가 읽고 전달합니다.", {
    x: 0.55, y: 0.88, w: W - 1.1, h: 0.58,
    fontSize: 13, bold: true, color: C.white,
    fontFace: "Malgun Gothic", valign: "middle", align: "center",
  });

  // 2단 설명
  const colW2 = (W - 1.05) / 2;

  // 왼쪽: 영어로 작성하는 이유
  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: 1.62, w: colW2, h: 2.8,
    fill: { color: C.offWhite }, line: { color: C.sky, pt: 1 }, arcSize: 4,
  });
  s.addText("왜 영어로 작성하나요?", {
    x: 0.55, y: 1.76, w: colW2 - 0.4, h: 0.38,
    fontSize: 12, bold: true, color: C.navy, fontFace: "Malgun Gothic",
  });
  const whyEng = [
    ["토큰 효율", "같은 정보를 영어로 쓰면 한국어 대비 토큰 수가 절반 이하. AI 처리 비용·속도에 직결됩니다."],
    ["AI 이해도", "LLM은 영어 데이터로 학습됩니다. 기술 개념·코드 설명의 정확도가 높습니다."],
  ];
  whyEng.forEach(([title, desc], i) => {
    const y = 2.28 + i * 1.0;
    s.addShape(pptx.ShapeType.rect, {
      x: 0.55, y, w: 0.05, h: 0.75,
      fill: { color: C.sky }, line: { color: C.sky },
    });
    s.addText(title, {
      x: 0.72, y, w: colW2 - 0.58, h: 0.32,
      fontSize: 11, bold: true, color: C.navy, fontFace: "Malgun Gothic",
    });
    s.addText(desc, {
      x: 0.72, y: y + 0.33, w: colW2 - 0.58, h: 0.45,
      fontSize: 9.5, color: C.darkGray, fontFace: "Malgun Gothic",
      lineSpacingMultiple: 1.25,
    });
  });

  // 오른쪽: 조회 방법
  const rx2 = 0.35 + colW2 + 0.35;
  s.addShape(pptx.ShapeType.roundRect, {
    x: rx2, y: 1.62, w: colW2, h: 2.8,
    fill: { color: C.offWhite }, line: { color: C.sky, pt: 1 }, arcSize: 4,
  });
  s.addText("그럼 어떻게 조회하나요?", {
    x: rx2 + 0.2, y: 1.76, w: colW2 - 0.4, h: 0.38,
    fontSize: 12, bold: true, color: C.navy, fontFace: "Malgun Gothic",
  });
  s.addText("AI에게 물어보면 됩니다.\nAI가 해당 문서를 직접 읽고 설명해줍니다.\n파일을 직접 열 필요가 없습니다.", {
    x: rx2 + 0.2, y: 2.22, w: colW2 - 0.4, h: 0.75,
    fontSize: 10.5, color: C.darkGray, fontFace: "Malgun Gothic",
    lineSpacingMultiple: 1.4,
  });
  codeBox(s, '"auth 모듈 구조가 어떻게 돼 있어?"', {
    x: rx2 + 0.2, y: 3.05, w: colW2 - 0.4, h: 0.58, fontSize: 9.5,
  });
  s.addText("→ AI가 ai-docs/mental-model/auth.md를 읽고 설명", {
    x: rx2 + 0.2, y: 3.68, w: colW2 - 0.4, h: 0.32,
    fontSize: 9.5, color: C.darkGray, fontFace: "Malgun Gothic",
  });

  // 하단 요약
  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: 4.6, w: W - 0.7, h: 1.0,
    fill: { color: C.offWhite }, line: { color: C.sky, pt: 1 }, arcSize: 4,
  });
  const summary = [
    ["✍  작성", "wsflow가 자동으로 담당 (forge-spec, forge-mental-model 등)"],
    ["🔍  조회", "AI에게 질문으로 위임  —  직접 파일을 열어 읽는 것은 authoring 시에만"],
  ];
  summary.forEach(([label, desc], i) => {
    const sy = 4.72 + i * 0.38;
    s.addText(label, {
      x: 0.6, y: sy, w: 1.1, h: 0.32,
      fontSize: 11, bold: true, color: C.navy, fontFace: "Malgun Gothic",
    });
    s.addText(desc, {
      x: 1.8, y: sy, w: W - 2.2, h: 0.32,
      fontSize: 10.5, color: C.darkGray, fontFace: "Malgun Gothic", valign: "middle",
    });
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 21: wsflow에게 물어보기
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "05  wsflow에게 물어보기", { dark: false });
  footerBar(s);

  s.addText(
    "어느 프로젝트에서든 AI에게 물어보면 wsflow 워크플로우를 설명받을 수 있습니다.",
    {
      x: 0.35, y: 0.88, w: W - 0.7, h: 0.42,
      fontSize: 11, color: C.navy, fontFace: "Malgun Gothic",
    }
  );

  dividerLabel(s, "방법 1  —  전체 구조 파악 (처음 접할 때)", 1.42);
  codeBox(s, '"wsflow workflow에 대해 설명해주세요"', {
    x: 0.35, y: 1.86, w: W - 0.7, h: 0.62,
  });
  s.addText("→ AI가 ai-docs/WORKFLOW.md와 _index.md를 참고해 전체 구조를 설명합니다.", {
    x: 0.35, y: 2.54, w: W - 0.7, h: 0.35,
    fontSize: 10, color: C.darkGray, fontFace: "Malgun Gothic",
  });

  dividerLabel(s, "방법 2  —  핀포인트 질문 (실전 권장)", 3.0);

  const exs = [
    '"forge-spec이 ticket이랑 어떻게 연결돼?"',
    '"proceed를 쓸 때 ticket이 없으면 어떻게 해?"',
    '"mental model은 언제 업데이트해야 해?"',
  ];
  exs.forEach((e, i) => {
    codeBox(s, e, { x: 0.35, y: 3.44 + i * 0.8, w: W - 0.7, h: 0.65, fontSize: 10 });
  });

  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: 5.88, w: W - 0.7, h: 0.48,
    fill: { color: C.offWhite }, line: { color: C.sky, pt: 1 }, arcSize: 4,
  });
  s.addText(
    "💡  모르면 그냥 물어보면 됩니다. wsflow 자체가 맥락을 들고 있습니다.",
    {
      x: 0.55, y: 5.88, w: W - 1.0, h: 0.48,
      fontSize: 11, bold: true, color: C.navy,
      fontFace: "Malgun Gothic", valign: "middle",
    }
  );
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 21: 레거시 온보딩 개요
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide({ dark: true });
  s.background = { color: C.navy };
  header(s, "06  레거시 프로젝트 온보딩  —  개요", { dark: true });
  footerBar(s);

  s.addText("기존 프로젝트에 wsflow를 도입할 때의 3단계 흐름입니다.", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.3,
    fontSize: 11, color: C.sky, fontFace: "Malgun Gothic",
  });

  const onb = [
    {
      cmd: "wsflow:lead-bootstrap",
      title: "워크플로우 구조 초기화",
      desc: "AGENTS.md, ai-docs/ 폴더 구조, WORKFLOW.md를 생성합니다. 기존 CLAUDE.md가 있으면 자동으로 마이그레이션합니다.",
    },
    {
      cmd: "wsflow:lead-forge-spec",
      title: "Spec 일괄 생성",
      desc: "코드베이스·티켓·커밋 히스토리를 분석해 도메인별 spec 문서를 작성합니다. 각 항목을 구현 완료 / 계획 중으로 분류합니다.",
    },
    {
      cmd: "wsflow:lead-forge-mental-model",
      title: "Mental Model 일괄 생성",
      desc: "모듈 계약·결합 지점·주의사항을 도메인별로 정리해 mental-model 문서를 작성합니다. 작성 후 코드베이스와 대조 검증합니다.",
    },
  ];

  const rowH  = 1.6;
  const rowGap = 0.2;
  const startY = 1.28;

  onb.forEach((o, i) => {
    const y = startY + i * (rowH + rowGap);

    // 번호 원
    s.addShape(pptx.ShapeType.ellipse, {
      x: 0.35, y: y + (rowH - 0.52) / 2, w: 0.52, h: 0.52,
      fill: { color: C.sky }, line: { color: C.sky },
    });
    s.addText(`${i + 1}`, {
      x: 0.35, y: y + (rowH - 0.52) / 2, w: 0.52, h: 0.52,
      fontSize: 13, bold: true, color: C.white,
      fontFace: "Malgun Gothic", align: "center", valign: "middle",
    });

    // 카드 박스
    s.addShape(pptx.ShapeType.roundRect, {
      x: 1.05, y, w: W - 1.4, h: rowH,
      fill: { color: C.navyMid }, line: { color: C.sky, pt: 1 }, arcSize: 4,
    });

    // 커맨드
    s.addText(`/wsflow:lead-${o.cmd.replace("wsflow:lead-", "")}`, {
      x: 1.2, y: y + 0.14, w: 5.5, h: 0.3,
      fontSize: 10, fontFace: "Cascadia Code", color: C.sky, valign: "middle",
    });

    // 제목
    s.addText(o.title, {
      x: 1.2, y: y + 0.46, w: W - 1.7, h: 0.34,
      fontSize: 12, bold: true, color: C.white, fontFace: "Malgun Gothic",
    });

    // 설명
    s.addText(o.desc, {
      x: 1.2, y: y + 0.84, w: W - 1.7, h: 0.65,
      fontSize: 10, color: C.gray, fontFace: "Malgun Gothic",
      lineSpacingMultiple: 1.3, valign: "top",
    });

    // 아래 화살표 (마지막 제외)
    if (i < onb.length - 1) {
      s.addText("↓", {
        x: 0.35, y: y + rowH + 0.02, w: 0.52, h: rowGap,
        fontSize: 14, color: C.sky, align: "center", valign: "middle",
        fontFace: "Malgun Gothic",
      });
    }
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 22: bootstrap 상세
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "06  레거시 온보딩  —  Step 1: bootstrap", { dark: false });
  footerBar(s);

  s.addText(
    "AGENTS.md와 ai-docs/ 구조를 생성합니다.\n" +
    "이미 있는 경우 버전 태그를 보고 자동으로 업그레이드·마이그레이션합니다.",
    {
      x: 0.35, y: 0.88, w: W - 0.7, h: 0.62,
      fontSize: 11, color: C.navy, fontFace: "Malgun Gothic", lineSpacingMultiple: 1.4,
    }
  );

  codeBox(s, "/wsflow:lead-bootstrap", { x: 0.35, y: 1.6, w: 5.0, h: 0.62 });

  dividerLabel(s, "동작 감지 모드", 2.38);

  const modes = [
    ["fresh",         "아무 파일도 없음",          "전체 구조 신규 생성"],
    ["upgrade",       "AGENTS.md에 버전 태그 있음", "누락 마이그레이션만 적용"],
    ["adopt",         "버전 태그 없는 AGENTS.md",   "전체 마이그레이션 적용 후 upgrade"],
    ["claude-migrate","CLAUDE.md만 있음",           "CLAUDE.md → AGENTS.md 변환"],
  ];

  modes.forEach(([mode, cond, action], i) => {
    const y = 2.82 + i * 0.78;
    pill(s, mode, 0.35, y + 0.06, 1.65, { h: 0.34, fontSize: 9, bg: C.navy });
    s.addText(cond, {
      x: 2.1, y: y + 0.06, w: 4.8, h: 0.34,
      fontSize: 10, color: C.darkGray, fontFace: "Malgun Gothic", valign: "middle",
    });
    s.addText(`→  ${action}`, {
      x: 7.0, y: y + 0.06, w: 5.95, h: 0.34,
      fontSize: 10, color: C.navy, fontFace: "Malgun Gothic", valign: "middle",
    });
  });

  s.addText("데모에서 직접 확인합니다.", {
    x: 0.35, y: 6.0, w: W - 0.7, h: 0.35,
    fontSize: 10, bold: true, color: C.sky, fontFace: "Malgun Gothic",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 23: forge-spec & forge-mental-model
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "06  레거시 온보딩  —  Step 2 & 3", { dark: false });
  footerBar(s);

  // forge-spec
  s.addText("Step 2  —  forge-spec", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.38,
    fontSize: 12, bold: true, color: C.navy, fontFace: "Malgun Gothic",
  });
  codeBox(s, "/wsflow:lead-forge-spec", { x: 0.35, y: 1.32, w: 5.5, h: 0.58 });
  s.addText(
    "코드베이스·티켓·커밋을 분석 → 도메인 후보 제안 → 사용자 확인 → spec 문서 작성\n" +
    "각 항목을 '구현 완료 / 계획 중'으로 분류합니다. 모호하면 사용자에게 확인을 구합니다.",
    {
      x: 0.35, y: 2.0, w: W - 0.7, h: 0.72,
      fontSize: 10, color: C.darkGray, fontFace: "Malgun Gothic", lineSpacingMultiple: 1.35,
    }
  );

  // 구분선
  s.addShape(pptx.ShapeType.rect, {
    x: 0.35, y: 2.85, w: W - 0.7, h: 0.03,
    fill: { color: C.offWhite }, line: { color: C.offWhite },
  });

  // forge-mental-model
  s.addText("Step 3  —  forge-mental-model", {
    x: 0.35, y: 3.0, w: W - 0.7, h: 0.38,
    fontSize: 12, bold: true, color: C.navy, fontFace: "Malgun Gothic",
  });
  codeBox(s, "/wsflow:lead-forge-mental-model", { x: 0.35, y: 3.44, w: 6.5, h: 0.58 });
  s.addText(
    "모듈 구조·결합 지점·취약 영역을 서베이 → 도메인 후보 확인 → 도메인별 문서 작성\n" +
    "작성 후 코드베이스와 대조해 자동 검증(오류·누락·구식 내용 표시)합니다.",
    {
      x: 0.35, y: 4.12, w: W - 0.7, h: 0.72,
      fontSize: 10, color: C.darkGray, fontFace: "Malgun Gothic", lineSpacingMultiple: 1.35,
    }
  );

  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: 5.0, w: W - 0.7, h: 0.55,
    fill: { color: C.offWhite }, line: { color: C.sky, pt: 1 }, arcSize: 4,
  });
  s.addText(
    "두 스킬 모두 중간에 사용자 확인을 거칩니다. 자동으로 모든 것이 작성되지는 않습니다.",
    {
      x: 0.55, y: 5.0, w: W - 1.0, h: 0.55,
      fontSize: 10, color: C.navy, fontFace: "Malgun Gothic", valign: "middle",
    }
  );

  s.addText("데모에서 직접 확인합니다.", {
    x: 0.35, y: 5.7, w: W - 0.7, h: 0.35,
    fontSize: 10, bold: true, color: C.sky, fontFace: "Malgun Gothic",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 24: 치트시트 — 커맨드
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "07  치트시트  —  커맨드 일람", { dark: false });
  footerBar(s);

  const cmds = [
    // [커맨드, 설명, 카테고리 색]
    ["/wsflow:lead-discuss",            "방향 논의. 코드 미수정",                           C.navy],
    ["/wsflow:lead-proceed <target>",   "파이프라인 자동 실행 (spec→ticket→implement→commit)", C.sky],
    ["/compact",                        "컨텍스트 압축. 작업 후 습관화",                     C.skyDim],
    ["/wsflow:lead-review [브랜치]",    "MR/PR 리뷰 → LGTM / NEEDS FIX / OPEN 판정",        C.sky],
    ["/wsflow:lead-bootstrap",          "프로젝트 wsflow 구조 초기화/업그레이드",            C.navy],
    ["/wsflow:lead-forge-spec",         "코드베이스 분석 → spec 일괄 생성",                  C.navy],
    ["/wsflow:lead-forge-mental-model", "코드베이스 분석 → mental model 일괄 생성",          C.navy],
    ["/wsflow:lead-write-ticket",       "티켓 작성/업데이트",                                C.darkGray],
    ["/wsflow:lead-update-spec",        "기존 spec 업데이트",                                C.darkGray],
  ];

  cmds.forEach(([cmd, desc, color], i) => {
    const y = 0.9 + i * 0.59;
    s.addText(cmd, {
      x: 0.35, y, w: 5.2, h: 0.48,
      fontSize: 9.5, fontFace: "Cascadia Code", color, valign: "middle",
    });
    s.addText(desc, {
      x: 5.65, y, w: 7.3, h: 0.48,
      fontSize: 10, fontFace: "Malgun Gothic", color: C.darkGray, valign: "middle",
    });
    if (i < cmds.length - 1) {
      s.addShape(pptx.ShapeType.rect, {
        x: 0.35, y: y + 0.48, w: W - 0.7, h: 0.01,
        fill: { color: C.offWhite }, line: { color: C.offWhite },
      });
    }
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 25: 치트시트 — 플로우 요약
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide({ dark: true });
  s.background = { color: C.navy };
  header(s, "07  치트시트  —  플로우 요약", { dark: true });
  footerBar(s);

  const flows = [
    { label: "일반 작업",      flow: ["discuss", "proceed", "commit", "/compact"] },
    { label: "소규모 작업",    flow: ["proceed <설명>", "commit", "/compact"] },
    { label: "레거시 온보딩",  flow: ["bootstrap", "forge-spec", "forge-mental-model"] },
  ];

  const rowH = 0.62;
  const rowGap = 1.38;
  flows.forEach((f, ri) => {
    const y = 0.95 + ri * rowGap;
    s.addText(f.label, {
      x: 0.35, y, w: 2.2, h: rowH,
      fontSize: 10.5, bold: true, color: C.sky,
      fontFace: "Malgun Gothic", valign: "middle",
    });
    // arrow 폭 포함해 bw 계산 (overflow 방지)
    const arrowW = 0.3;
    const n = f.flow.length;
    const availW = W - 2.65 - 0.35;  // label 오른쪽부터 우측 여백까지
    const bw = (availW - arrowW * (n - 1)) / n;
    f.flow.forEach((step, si) => {
      const x = 2.65 + si * (bw + arrowW);
      s.addShape(pptx.ShapeType.roundRect, {
        x, y, w: bw, h: rowH,
        fill: { color: C.navyMid }, line: { color: C.sky, pt: 1 }, arcSize: 8,
      });
      s.addText(step, {
        x, y, w: bw, h: rowH,
        fontSize: 9, fontFace: "Cascadia Code", color: C.white,
        align: "center", valign: "middle",
      });
      if (si < n - 1) {
        s.addText("→", {
          x: x + bw, y, w: arrowW, h: rowH,
          fontSize: 11, color: C.sky, align: "center", valign: "middle",
          fontFace: "Malgun Gothic",
        });
      }
    });
  });

  s.addShape(pptx.ShapeType.rect, {
    x: 0, y: 7.08, w: W, h: 0.04,
    fill: { color: C.sky }, line: { color: C.sky },
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 26: 팀 협업 — 역할과 루프
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide({ dark: true });
  s.background = { color: C.navy };
  header(s, "08  팀 협업  —  역할과 협업 루프", { dark: true });
  footerBar(s);

  s.addText("wsflow 개인 작업 규율이 팀 협업 인프라가 됩니다.  AI Context 커밋이 MR 리뷰의 재료입니다.", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.35,
    fontSize: 10.5, color: C.sky, fontFace: "Malgun Gothic",
  });

  const roles = [
    {
      title: "컨트리뷰터",
      icon: "👩‍💻",
      steps: [
        "브랜치 생성",
        "wsflow:lead-discuss  →  proceed 로 구현",
        "AI Context 커밋 자동 포함",
        "MR 생성 (커밋이 리뷰 재료)",
      ],
    },
    {
      title: "메인테이너",
      icon: "🔍",
      steps: [
        "wsflow:lead-review 실행",
        "브랜치 fetch + 의도 파악 + 정합성 검토",
        "LGTM → 머지",
        "NEEDS FIX → 로컬 수정 또는 코멘트",
      ],
    },
  ];

  const cW = (W - 1.05) / 2;
  roles.forEach((r, ri) => {
    const x = 0.35 + ri * (cW + 0.35);
    s.addShape(pptx.ShapeType.roundRect, {
      x, y: 1.32, w: cW, h: 5.0,
      fill: { color: C.navyMid }, line: { color: C.sky, pt: 1 }, arcSize: 4,
    });
    s.addText(r.icon, {
      x, y: 1.44, w: cW, h: 0.55,
      fontSize: 20, align: "center", fontFace: "Segoe UI Emoji",
    });
    s.addText(r.title, {
      x: x + 0.2, y: 2.0, w: cW - 0.4, h: 0.38,
      fontSize: 13, bold: true, color: C.white,
      fontFace: "Malgun Gothic", align: "center",
    });
    r.steps.forEach((step, si) => {
      s.addShape(pptx.ShapeType.roundRect, {
        x: x + 0.2, y: 2.48 + si * 0.86, w: cW - 0.4, h: 0.72,
        fill: { color: C.codeBg }, line: { color: C.navyMid }, arcSize: 4,
      });
      s.addText(step, {
        x: x + 0.35, y: 2.48 + si * 0.86, w: cW - 0.7, h: 0.72,
        fontSize: 9.5, color: C.white, fontFace: "Malgun Gothic",
        valign: "middle", lineSpacingMultiple: 1.2,
      });
    });
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 27: 팀 협업 — 컨트리뷰터 플로우
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "08  팀 협업  —  컨트리뷰터 플로우", { dark: false });
  footerBar(s);

  s.addText("브랜치를 따서 wsflow로 작업하면 MR 리뷰 재료가 자동으로 만들어집니다.", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.35,
    fontSize: 11, color: C.navy, fontFace: "Malgun Gothic",
  });

  const steps = [
    { num: "1", title: "브랜치 생성",       cmd: "git checkout -b feat/my-feature",              desc: null },
    { num: "2", title: "wsflow로 구현",     cmd: "/wsflow:lead-discuss  →  /wsflow:lead-proceed", desc: "커밋마다 AI Context 섹션 자동 포함" },
    { num: "3", title: "MR 생성",           cmd: "glab mr create  또는  GitLab Web UI",           desc: "AI Context 커밋들이 리뷰어의 의도 파악 재료가 됩니다" },
    { num: "4", title: "FIX 수신 시 수정", cmd: "/wsflow:lead-discuss  →  /wsflow:lead-implement", desc: "피드백을 컨텍스트로 전달해 수정" },
  ];

  steps.forEach((st, i) => {
    const y = 1.32 + i * 1.32;
    s.addShape(pptx.ShapeType.ellipse, {
      x: 0.35, y: y + 0.2, w: 0.46, h: 0.46,
      fill: { color: C.sky }, line: { color: C.sky },
    });
    s.addText(st.num, {
      x: 0.35, y: y + 0.2, w: 0.46, h: 0.46,
      fontSize: 12, bold: true, color: C.white,
      fontFace: "Malgun Gothic", align: "center", valign: "middle",
    });
    s.addShape(pptx.ShapeType.roundRect, {
      x: 0.98, y, w: W - 1.33, h: 1.14,
      fill: { color: C.offWhite }, line: { color: C.sky, pt: 1 }, arcSize: 4,
    });
    s.addText(st.title, {
      x: 1.14, y: y + 0.08, w: W - 1.6, h: 0.28,
      fontSize: 11, bold: true, color: C.navy, fontFace: "Malgun Gothic",
    });
    s.addText(st.cmd, {
      x: 1.14, y: y + 0.40, w: W - 1.6, h: 0.28,
      fontSize: 9.5, fontFace: "Cascadia Code", color: C.skyDim,
    });
    if (st.desc) {
      s.addText(`→  ${st.desc}`, {
        x: 1.14, y: y + 0.74, w: W - 1.6, h: 0.28,
        fontSize: 9, color: C.darkGray, fontFace: "Malgun Gothic",
      });
    }
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 28: 팀 협업 — 메인테이너 플로우
// ═══════════════════════════════════════════════════════════════
{
  const s = addSlide();
  header(s, "08  팀 협업  —  메인테이너 플로우 (wsflow:lead-review)", { dark: false });
  footerBar(s);

  s.addText("wsflow:lead-review 하나로 MR 리뷰 전 과정을 진행합니다.", {
    x: 0.35, y: 0.88, w: W - 0.7, h: 0.35,
    fontSize: 11, color: C.navy, fontFace: "Malgun Gothic",
  });

  dividerLabel(s, "실행", 1.32);
  codeBox(s, "/wsflow:lead-review [브랜치명]  —  최초 실행 시 _review.local.md 환경 설정 인터뷰 (1회)", {
    x: 0.35, y: 1.76, w: W - 0.7, h: 0.62,
  });

  dividerLabel(s, "판정 결과", 2.52);

  const verdicts = [
    { label: "LGTM",       color: C.sky,    desc: "머지 진행 (설정에 따라 자동 또는 수동 승인)" },
    { label: "NEEDS FIX",  color: C.skyDim, desc: "로컬 수정: wsflow:lead-discuss 로 피드백 컨텍스트 유지\n또는 컨트리뷰터에게 코멘트 전달 (설정된 코멘트 방식 사용)" },
    { label: "OPEN",       color: "8A6FBF", desc: "추가 논의 필요 — wsflow:lead-discuss 로 진입해 방향 결정" },
  ];

  verdicts.forEach((v, i) => {
    const y = 2.96 + i * 1.1;
    s.addShape(pptx.ShapeType.roundRect, {
      x: 0.35, y, w: 1.55, h: 0.88,
      fill: { color: v.color }, line: { color: v.color }, arcSize: 8,
    });
    s.addText(v.label, {
      x: 0.35, y, w: 1.55, h: 0.88,
      fontSize: 9.5, bold: true, color: C.white,
      fontFace: "Malgun Gothic", align: "center", valign: "middle",
    });
    s.addShape(pptx.ShapeType.roundRect, {
      x: 2.1, y: y + 0.08, w: W - 2.45, h: 0.72,
      fill: { color: C.offWhite }, line: { color: C.offWhite }, arcSize: 4,
    });
    s.addText(v.desc, {
      x: 2.25, y: y + 0.08, w: W - 2.75, h: 0.72,
      fontSize: 10, color: C.darkGray, fontFace: "Malgun Gothic",
      valign: "middle", lineSpacingMultiple: 1.3,
    });
  });

  s.addShape(pptx.ShapeType.roundRect, {
    x: 0.35, y: 6.3, w: W - 0.7, h: 0.42,
    fill: { color: C.navy }, line: { color: C.navy }, arcSize: 4,
  });
  s.addText("원격 접근 방법(glab, API token, git fetch 등)은 _review.local.md 에 저장됩니다.  팀별 환경에 맞게 1회 설정.", {
    x: 0.55, y: 6.3, w: W - 1.0, h: 0.42,
    fontSize: 9.5, color: C.gray, fontFace: "Malgun Gothic", valign: "middle",
  });
}

// ═══════════════════════════════════════════════════════════════
// 슬라이드 29: 마무리
// ═══════════════════════════════════════════════════════════════
{
  const s = pptx.addSlide();
  s.background = { color: C.navy };

  s.addShape(pptx.ShapeType.rect, {
    x: 0, y: 0, w: W, h: 0.18,
    fill: { color: C.sky }, line: { color: C.sky },
  });

  s.addText("Q & A", {
    x: 1.0, y: 1.5, w: W - 2.0, h: 1.5,
    fontSize: 56, bold: true, color: C.white,
    fontFace: "Malgun Gothic", align: "center",
  });

  s.addText("데모 및 질의응답", {
    x: 1.0, y: 3.1, w: W - 2.0, h: 0.6,
    fontSize: 20, color: C.sky,
    fontFace: "Malgun Gothic", align: "center",
  });

  s.addShape(pptx.ShapeType.rect, {
    x: 4.0, y: 3.85, w: 5.3, h: 0.04,
    fill: { color: C.navyMid }, line: { color: C.navyMid },
  });

  s.addText("github.com/kang-sw/devenv", {
    x: 1.0, y: 4.0, w: W - 2.0, h: 0.42,
    fontSize: 12, color: C.gray,
    fontFace: "Malgun Gothic", align: "center",
  });

  s.addShape(pptx.ShapeType.rect, {
    x: 0, y: H - 0.28, w: W, h: 0.28,
    fill: { color: C.sky }, line: { color: C.sky },
  });
}

// ── 저장 ──────────────────────────────────────────────────────
pptx.writeFile({ fileName: "wsflow-seminar.pptx" })
  .then(() => console.log("생성 완료: wsflow-seminar.pptx"))
  .catch(e => { console.error(e); process.exit(1); });
