const {
  Document, Packer, Paragraph, TextRun, HeadingLevel,
  AlignmentType, LevelFormat, TableOfContents, Header, Footer,
  PageNumber, BorderStyle, ExternalHyperlink
} = require('docx');
const fs = require('fs');

// ── Numbering config ──────────────────────────────────────────────────────────
const numbering = {
  config: [
    {
      reference: "bullets",
      levels: [{
        level: 0, format: LevelFormat.BULLET, text: "•", alignment: AlignmentType.LEFT,
        style: { paragraph: { indent: { left: 720, hanging: 360 } } }
      }, {
        level: 1, format: LevelFormat.BULLET, text: "◦", alignment: AlignmentType.LEFT,
        style: { paragraph: { indent: { left: 1080, hanging: 360 } } }
      }]
    },
    {
      reference: "numbers",
      levels: [{
        level: 0, format: LevelFormat.DECIMAL, text: "%1.", alignment: AlignmentType.LEFT,
        style: { paragraph: { indent: { left: 720, hanging: 360 } } }
      }]
    }
  ]
};

// ── Helpers ───────────────────────────────────────────────────────────────────
function h1(text) {
  return new Paragraph({ heading: HeadingLevel.HEADING_1, pageBreakBefore: true, children: [new TextRun(text)] });
}
function h2(text) {
  return new Paragraph({ heading: HeadingLevel.HEADING_2, children: [new TextRun(text)] });
}
function h3(text) {
  return new Paragraph({ heading: HeadingLevel.HEADING_3, children: [new TextRun(text)] });
}
function p(text, opts = {}) {
  return new Paragraph({
    spacing: { after: 160 },
    alignment: opts.center ? AlignmentType.CENTER : AlignmentType.LEFT,
    children: [new TextRun({ text, ...(opts.bold ? { bold: true } : {}), ...(opts.italic ? { italics: true } : {}) })]
  });
}
function bullet(text, level = 0) {
  return new Paragraph({
    numbering: { reference: "bullets", level },
    spacing: { after: 80 },
    children: [new TextRun(text)]
  });
}
function numbered(text) {
  return new Paragraph({
    numbering: { reference: "numbers", level: 0 },
    spacing: { after: 80 },
    children: [new TextRun(text)]
  });
}
function spacer() {
  return new Paragraph({ children: [new TextRun("")], spacing: { after: 120 } });
}
function divider() {
  return new Paragraph({
    border: { bottom: { style: BorderStyle.SINGLE, size: 4, color: "CCCCCC", space: 1 } },
    children: [new TextRun("")],
    spacing: { after: 240 }
  });
}
function callout(text) {
  return new Paragraph({
    indent: { left: 720 },
    border: { left: { style: BorderStyle.SINGLE, size: 12, color: "2E75B6", space: 4 } },
    spacing: { before: 120, after: 120 },
    children: [new TextRun({ text, italics: true, color: "333333" })]
  });
}

// ── Content ───────────────────────────────────────────────────────────────────
const children = [

  // ── Cover ──
  spacer(), spacer(),
  new Paragraph({
    heading: HeadingLevel.HEADING_1,
    alignment: AlignmentType.CENTER,
    spacing: { after: 240 },
    children: [new TextRun({ text: "AI와의 협업: 원칙에서 워크플로우로", bold: true, size: 48 })]
  }),
  new Paragraph({
    alignment: AlignmentType.CENTER,
    spacing: { after: 160 },
    children: [new TextRun({ text: "“만들어 줘”를 넘어서 — 지속 가능한 AI 협업 구조", italics: true, size: 28, color: "555555" })]
  }),
  spacer(), spacer(), divider(),

  // ── TOC ──
  new TableOfContents("목차", { hyperlink: true, headingStyleRange: "1-3" }),
  divider(),

  // ── 들어가며 ──
  h1("들어가며"),
  p("LLM 협업 방식을 구조화하려는 개발자를 위한 실용 가이드."),
  bullet("대상: Claude 사용 경험이 있는 개발자."),
  bullet("초점: 코드 생성 능력이 아닌 활용 원리와 구조."),
  bullet("목표: 워크플로우의 근거 원칙 이해와 자체 설계 능력."),
  spacer(),

  // ── Part 1 ──
  h1("Part 1. LLM을 이해하는 물리학"),
  p("AI 협업의 전제 — 시스템의 물리적 특성."),

  h2("1.1 토큰(Token) — AI가 세상을 읽는 단위"),
  bullet("LLM의 텍스트 처리 단위: 문자/단어가 아닌 토큰."),
  bullet("영어 “hello” = 1토큰, 한국어 “안녕하세요” = 4~6토큰."),
  bullet("원인: 영어 중심 어휘 사전. 영어는 큰 단위 묶음, 비영어는 작은 분할."),
  bullet("결과: 한국어는 동일 내용 전달 시 영어 대비 3~5배 토큰 소모."),

  h2("1.2 컨텍스트 윈도우와 열화 현상"),
  bullet("컨텍스트 윈도우: LLM이 한 번에 처리 가능한 텍스트 용량의 상한."),
  bullet("Lost in the Middle: 초반/후반 정보는 잘 참조되나, 중간부는 attention 희석으로 무시되는 구조."),
  bullet("긴 세션 후반의 지시 망각: 능력 저하가 아닌 구조적 제약."),

  h2("1.3 학습 데이터 편향 — 코드가 영어인 이유"),
  bullet("학습 데이터의 압도적 비중: 영어. GitHub, StackOverflow, 기술 문서 전반."),
  bullet("코드 자체도 영어: 변수명, 함수명, 주석, API, 에러 메시지 등 개발 생태계의 기반 언어."),
  bullet("영어 코드 작업에서 추론이 더 정확한 이유: 개념 간 연결이 영어 공간에서 더 촴촴한 구조."),
  callout("토큰 효율, 컨텍스트 열화, 학습 편향 — 세 가지 모두 영어 우위를 가리키는 물리적 특성."),
  spacer(),

  // ── Part 2 ──
  h1("Part 2. 언어 경계 원칙"),
  p("핵심은 AI가 소비하는 아티팩트의 언어."),

  h2("2.1 세 가지 층위"),
  bullet("사람의 사고 층 — 논의, 브레인스토밍, 피드백: 한국어."),
  bullet("경계 층 — 결정 사항의 문서화, AI에게 전달할 명세: 영어."),
  bullet("AI 소비 층 — 코드, 스펙, 티켓, 계획서, 문서: 영어."),

  h2("2.2 경계의 의미"),
  bullet("AI 처리 효율: 영어 아티팩트 소비 시 토큰 절약과 컨텍스트 활용률 향상."),
  bullet("문서화 강제 효과: 한국어 논의 → 영어 문서 전환 과정이 결정 명확성의 검증 장치."),
  spacer(),

  // ── Part 3 ──
  h1("Part 3. 핵심 명제 — 상태 외재화"),
  p("LLM의 근본 특성: 상태 없음(statelessness). 각 세션은 독립적."),

  h2("3.1 AI의 망각 — 버그가 아닌 전제"),
  bullet("망각을 결함으로 보는 시각: 반복 설명, 대화 무한 연장 등 비효율의 근원."),
  bullet("올바른 시각: 망각 전제 → 중요 정보 외부 기록의 강제. 인간 팀의 문서화와 동일한 원리."),

  h2("3.2 상태 외재화란"),
  bullet("AI의 매 세션 올바른 재진입을 위한 모든 중요 상태의 외부 문서 기록."),
  numbered("이해 상태의 외재화: mental-model, spec, ai-docs."),
  numbered("의도/결정의 외재화: 티켓(ticket)과 계획서(plan)."),
  callout("문서 = AI의 기억 대체. 이해 + 의도 외재화가 갖춰지면 매 세션 처음부터 시작하지 않는 구조."),
  spacer(),

  // ── Part 4 ──
  h1("Part 4. Skills, Agents, Subagents"),
  p("워크플로우를 구성하는 기술적 요소."),

  h2("4.1 Skills — 반복되는 워크플로우의 정의"),
  bullet("Skill: 특정 요청 유형에 대한 행동 패턴의 마크다운 정의."),
  bullet("예: /discuss — 탐색 대화 유도, 후속 스킬 연결의 프로토콜."),
  bullet("핵심: 특정 상황의 명시적 프로토콜. 팀 전체의 일관된 패턴 공유."),

  h2("4.2 Agents — 특화된 역할의 분리"),
  bullet("Agent: 특정 도메인 특화 프롬프트를 가진 Claude 인스턴스."),
  bullet("예: rust-api-lookup, mental-model-updater."),
  bullet("Skill과의 차이: Skill은 대화 흐름 정의, Agent는 독립적 작업 수행."),

  h2("4.3 Subagents — 컨텍스트 분산 구조"),
  bullet("Subagent: 메인 세션에서 별도 Claude 인스턴스를 생성하여 작업을 위임하는 패턴."),
  bullet("각 Subagent는 자신만의 깨끗한 컨텍스트 윈도우. 대규모 코드 탐색이 메인 컨텍스트를 오염시키지 않는 구조."),
  bullet("원칙: 비싼 읽기는 Subagent에게, 의사결정은 lean한 메인 에이전트에서."),
  spacer(),

  // ── Part 5 ──
  h1("Part 5. 워크플로우 체인"),
  p("원칙의 구체적 워크플로우 형성. 핵심: 아티팩트가 인터페이스."),
  bullet("각 단계는 이전 단계의 아티팩트를 입력으로, 다음 단계의 아티팩트를 출력으로 생성."),
  bullet("단계 간 독립성: 다른 컨텍스트 윈도우에서도 올바른 재개가 가능한 구조."),

  h2("5.1 /discuss — 발산의 공간"),
  bullet("아이디어 탐색과 방향 설정의 단계."),
  bullet("이 단계의 아티팩트: 없음. 출력: /write-ticket을 위한 명확한 방향."),

  h2("5.2 /write-ticket — 결정의 외재화"),
  bullet("티켓 = 의도를 잠그는 아티팩트."),
  bullet("담기는 내용: 목표/배경, 거부된 대안, 합의 접근, 제약/경계 조건, 구조 스케치."),
  bullet("품질 테스트: 대화 미참여 에이전트가 티켓만으로 다음 단계 진행 가능한 자기완결성."),
  bullet("상태 관리: 파일 이동 기반 (idea/ → todo/ → wip/ → done/)."),

  h2("5.3 /write-plan — 코드베이스와의 연결"),
  bullet("티켓 = 무엇을, 계획서 = 어떻게를 코드베이스에 맞춰 기술."),
  bullet("포함 내용: 변경 파일 목록, 각 변경의 계약(입력/출력/부작용), 통합 지점, 테스트 전략."),
  bullet("완성 후 코드 저장소에 커밋: 영구적 상태 외재화의 일부."),

  h2("5.4 실행 단계 — 세 가지 경로"),
  bullet("/execute-plan: 계획서를 계약으로 충실히 구현. 가정≠실제 시 정지 후 보고. 대규모 변경용."),
  bullet("/implement: 계획서가 방향/파일 목록만 제시하는 경우. 유연한 탐색 기반 구현."),
  bullet("/sprint: 단일 에이전트 경량 실행. 소규모/단순 작업용."),
  callout("모든 실행 경로 공통 — 완료 후 spec-updater와 mental-model-updater 서브에이전트 자동 실행 의무. Write-back loop의 기계적 강제 장치."),
  spacer(),

  // ── Part 6 ──
  h1("Part 6. 문서 관리 체계 — AI의 이해 상태 유지"),
  p("코드베이스에 대한 AI 이해의 외재화 체계."),

  h2("6.1 ai-docs 구조"),
  p("각 문서 유형의 상태 붕괴 속도(decay rate)와 소비자가 다른 구조."),
  bullet("_index.md: 세션 시작 시 첫 문서. 아키텍처 개요, 주요 스펙 링크, 빌드/테스트 명령어. 붕괴: 느림."),
  bullet("mental-model/: 코드 수정에 필요한 운영 지식. 암묵적 계약, 결합, 확장 지점. 붕괴: 중간."),
  bullet("spec/: 외부에서 바라본 기능 명세. “무엇을 하는가”만 기술. 붕괴: 느림."),
  bullet("tickets/: 결정 원장. 목표, 제약, 거부된 대안, 합의 접근. 붕괴: 매우 빠름(작성 시점에만 유효)."),
  bullet("plans/: 코드베이스 기반 구현 계약. 타임스탬프 커밋, 불변 아티팩트. 붕괴: 즉시(생성 직후 고정)."),

  h2("6.2 Write-back Loop — 상태 동기화의 의무"),
  bullet("구현 완료 후 문서 미갱신 시 코드베이스와의 괴리 축적."),
  bullet("mental-model-updater: 소스 변경이 기존 이해를 무효화한 부분을 갱신."),
  bullet("spec-updater: 사용자 가시적 기능 변경 시 스펙 동기화."),
  bullet("_index.md: 중요 아키텍처 변경이나 새로운 세션 노트."),
  bullet("티켓 Result: 실제 결과와 계획 대비 편차."),
  bullet("커밋 AI Context: 결정 근거, 기각된 대안, 사용자 지시를 커밋 메시지에 기록 — git history도 외재화 대상."),
  bullet("git log --oneline 금지: AI Context 섹션이 소실되어 미래 에이전트가 결정 이유를 읽지 못함."),
  callout("코드 변경 → AI 이해 변경의 동기화 의무. 동기화 실패 시 문서는 부채."),
  spacer(),

  // ── Part 7 ──
  h1("Part 7. Marathon — 토큰 경제의 구조적 해법"),
  p("단일 컨텍스트에 모든 탐색/수정이 쌓일 때의 문제: 설계 원칙 희석, 일관성 상실."),

  h2("7.1 팀 기반 위임 구조"),
  bullet("핵심: 메인 에이전트(Lead)는 코드를 직접 읽지 않는 구조."),
  bullet("Lead는 대화와 판단만 담당. 코드 읽기/쓰기는 신선한 컨텍스트의 Subagent에게 위임."),

  h2("7.2 팀 멤버 역할"),
  bullet("Planner: 코드베이스 탐색 후 구현 계획서 작성. Lead에게는 계획서 링크만 반환."),
  bullet("Implementer: 계획서와 대상 파일만 읽고 구현. diff 요약 반환."),
  bullet("Reviewer: diff만 읽고 코드 리뷰. 제한된 입력, 제한된 출력."),
  bullet("Clerk: 티켓 읽기/쓰기 담당. 문서 조작 특화."),
  bullet("Worker: 문서, 설정, 비코드 작업 처리."),

  h2("7.3 모델 선택의 경제학"),
  bullet("작업의 인지적 요구에 따른 모델 선택 원칙."),
  bullet("Explore: 저렴한 모델(Haiku)로 시작, 부족 시 에스컸레이션."),
  bullet("Implementer: Sonnet 기본. 새로운 아키텍처 설계 시에만 Opus."),
  callout("Marathon 핵심: Lead의 유일한 유한 자원은 컨텍스트 윈도우. 규칙이 모호할 때는 토큰당 더 나은 결정을 이끄는 해석을 선택."),
  spacer(),

  // ── Part 8 ──
  h1("Part 8. 핵심 원칙 3가지"),
  p("특정 도구 없이도 적용 가능한 세 가지 설계 원칙."),

  h2("원칙 1. 아티팩트 계약 — 컨텍스트 리셋을 안전하게"),
  bullet("모든 핸드오프 지점에서 자기완결적 아티팩트 생성의 의무."),
  bullet("품질 테스트: 이전 대화 미인지 에이전트가 이 아티팩트만으로 다음 단계 진행 가능 여부."),
  bullet("컨텍스트 리셋(세션 종료, 팀원 교체, 재개) 시에도 안전한 작업 계속의 보장."),

  h2("원칙 2. 결정과 탐색의 분리"),
  bullet("설계 결정: 티켓 수준. 소스 코드 없이 mental-model만 참조."),
  bullet("코드베이스 탐색: 계획서 수준. 명시적 탐색 후 계획 출력."),
  bullet("분리의 이점: 각 단계 인풋 명확화, 컨텍스트의 의도적 소비."),

  h2("원칙 3. 오케스트레이터를 Lean하게"),
  bullet("대화 컨텍스트 보유 에이전트: 인지 작업에 가장 비싼 곳. 조율과 의사결정만 담당."),
  bullet("대규모 입력 읽기는 신선한 컨텍스트의 Subagent에게 위임."),
  bullet("Subagent 경계를 넘는 것: 짧고 구조화된 brief. 이 brief가 외재화된 상태."),
  spacer(),

  // ── 부록 ──
  h1("부록. 추론 투명성 도구"),
  p("AI 추론 과정 자체를 외재화하는 보조 도구. 상태 외재화 원칙의 연장."),

  h2("A. /manual-think — 추론의 가시화"),
  bullet("용도: Extended thinking 비가용 환경에서 AI 추론 과정 보완."),
  bullet("<thinking> 태그로 추론 단계 명시 — 사용자 응답과 물리적 구분."),
  bullet("핵심 기법: 평가적 프레이밍 중립화 — 「X는 충분한가?」→「X의 강점과 약점은?」 확인 편향 차단."),
  bullet("사이클: Parse → Challenge(사용자 주장에 반례 탐색 의무) → Resolve → Decide."),
  bullet("언어 경계 원칙 연장: 추론은 영어 의무, 응답은 사용자 언어."),
  spacer(),

  h2("B. /monologue — 장세션 정합성 감사"),
  bullet("용도: 장세션 AI 가정의 가시화, 사용자-AI 이해 간격 추적."),
  bullet("Reading: 사용자 메시지의 중립화·분해된 영어 재진술 — 해석 단계와 응답 단계 분리."),
  bullet("가정-관찰 쌍: 행동 전 반증 가능한 가정 명시 → 결과 후 match / drift / abandon 분류."),
  bullet("Dropped: 기각된 대안을 이유와 함께 기록 — 맥락 소실 후 동일 후보 재검토 방지."),
  callout("match / drift / abandon — 사용자-AI 정합성 감사 어휘. 어느 단계에서 이해 간격이 벌어졌는지 추적 가능."),
  bullet("적합한 세션: 장세션, 복수 서브에이전트 위임, 아키텍처 탐색, 감사 필요 작업."),
  spacer(),

  // ── 마치며 ──
  h1("마치며"),
  p("이 문서는 하나의 구체적 구현. 다른 맥락에서는 다른 패턴이 최선일 수 있는 가능성."),
  bullet("AI는 상태 없는 시스템. 협업의 핵심: 상태 외재화 설계."),
  bullet("아티팩트가 컨텍스트 리셋을 견딘 수 있는 구조."),
  bullet("결정과 탐색의 분리."),
  bullet("오케스트레이터의 lean 유지."),
  spacer(),
  new Paragraph({
    spacing: { after: 80 },
    children: [new TextRun({ text: "참고 — 본 문서 스킬·에이전트 시스템 소스 코드 (오픈소스): ", size: 20, color: "555555" })]
  }),
  new Paragraph({
    spacing: { after: 0 },
    children: [new ExternalHyperlink({
      link: "https://github.com/kang-sw/devenv/tree/main/claude",
      children: [new TextRun({
        text: "github.com/kang-sw/devenv/tree/main/claude",
        style: "Hyperlink", size: 20
      })]
    })]
  }),
  spacer(),
  divider(),
  new Paragraph({
    alignment: AlignmentType.CENTER,
    spacing: { after: 0 },
    children: [new TextRun({ text: "— 끝 —", color: "888888", italics: true })]
  }),
];

// ── Document ──────────────────────────────────────────────────────────────────
const doc = new Document({
  numbering,
  styles: {
    default: {
      document: { run: { font: "Arial", size: 24 } }
    },
    paragraphStyles: [
      {
        id: "Heading1", name: "Heading 1", basedOn: "Normal", next: "Normal", quickFormat: true,
        run: { size: 36, bold: true, font: "Arial", color: "1F3864" },
        paragraph: { spacing: { before: 480, after: 240 }, outlineLevel: 0,
          border: { bottom: { style: BorderStyle.SINGLE, size: 4, color: "1F3864", space: 4 } } }
      },
      {
        id: "Heading2", name: "Heading 2", basedOn: "Normal", next: "Normal", quickFormat: true,
        run: { size: 28, bold: true, font: "Arial", color: "2E5496" },
        paragraph: { spacing: { before: 360, after: 180 }, outlineLevel: 1 }
      },
      {
        id: "Heading3", name: "Heading 3", basedOn: "Normal", next: "Normal", quickFormat: true,
        run: { size: 24, bold: true, font: "Arial", color: "375623" },
        paragraph: { spacing: { before: 240, after: 120 }, outlineLevel: 2 }
      },
    ]
  },
  sections: [{
    properties: {
      page: {
        size: { width: 11906, height: 16838 }, // A4
        margin: { top: 1440, right: 1440, bottom: 1440, left: 1440 }
      }
    },
    headers: {
      default: new Header({
        children: [new Paragraph({
          border: { bottom: { style: BorderStyle.SINGLE, size: 4, color: "CCCCCC", space: 4 } },
          children: [new TextRun({ text: "AI와의 협업: 원칙에서 워크플로우로", color: "888888", size: 18 })]
        })]
      })
    },
    footers: {
      default: new Footer({
        children: [new Paragraph({
          alignment: AlignmentType.RIGHT,
          children: [
            new TextRun({ text: "— ", color: "AAAAAA", size: 18 }),
            new TextRun({ children: [PageNumber.CURRENT], color: "AAAAAA", size: 18 }),
            new TextRun({ text: " —", color: "AAAAAA", size: 18 }),
          ]
        })]
      })
    },
    children,
  }]
});

Packer.toBuffer(doc).then(buffer => {
  fs.writeFileSync('/home/swkang/devenv/seminar-draft.docx', buffer);
  console.log('Done: /home/swkang/devenv/seminar-draft.docx');
});
