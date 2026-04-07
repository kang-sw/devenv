const {
  Document, Packer, Paragraph, TextRun, HeadingLevel,
  AlignmentType, LevelFormat, TableOfContents, Header, Footer,
  PageNumber, BorderStyle
} = require('docx');
const fs = require('fs');

// ── Numbering config ──────────────────────────────────────────────────────────
const numbering = {
  config: [
    {
      reference: "bullets",
      levels: [{
        level: 0, format: LevelFormat.BULLET, text: "\u2022", alignment: AlignmentType.LEFT,
        style: { paragraph: { indent: { left: 720, hanging: 360 } } }
      }, {
        level: 1, format: LevelFormat.BULLET, text: "\u25E6", alignment: AlignmentType.LEFT,
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
  return new Paragraph({ heading: HeadingLevel.HEADING_1, children: [new TextRun(text)] });
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
function pRuns(runs, opts = {}) {
  return new Paragraph({
    spacing: { after: 160 },
    children: runs.map(r =>
      typeof r === 'string'
        ? new TextRun(r)
        : new TextRun(r)
    )
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
    children: [new TextRun({ text: '"만들어 줘"를 넘어서 — 지속 가능한 AI 협업 구조 구축하기', italics: true, size: 28, color: "555555" })]
  }),
  spacer(), spacer(), divider(),

  // ── TOC ──
  new TableOfContents("목차", { hyperlink: true, headingStyleRange: "1-3" }),
  divider(),

  // ── 들어가며 ──
  h1("들어가며"),
  p("이 문서는 Claude를 비롯한 대형 언어 모델(LLM)과의 협업 방식을 한 단계 끌어올리고자 하는 개발자를 위한 실용적 가이드입니다. 대상 독자는 이미 Claude를 어느 정도 써본 경험이 있는 분들입니다. 따라서 \"AI가 코드를 생성할 수 있다\"는 사실 자체보다는, 그 능력을 일관성 있고 확장 가능한 방식으로 활용하는 원리와 구조에 초점을 맞춥니다."),
  p("이 문서의 목표는 특정 도구나 워크플로우를 그대로 복사하도록 설득하는 것이 아닙니다. 오히려 워크플로우가 어떤 원칙에서 비롯되는지를 이해하고, 여러분 각자의 맥락에 맞는 협업 구조를 스스로 설계할 수 있는 방향성을 제시하는 데 있습니다."),
  callout("\"길을 따라가는 것이 아니라, 길을 닦는 법을 이해하는 것.\""),
  spacer(),

  // ── Part 1 ──
  h1("Part 1. LLM을 이해하는 물리학"),
  p("효과적인 AI 협업을 위해서는 먼저 AI가 어떤 제약 속에서 작동하는지를 이해해야 합니다. 이 제약들은 버그가 아니라 시스템의 물리적 특성입니다. 좋은 협업 구조는 이 특성을 회피하는 것이 아니라 그것을 전제로 설계됩니다."),

  h2("1.1 토큰(Token) — AI가 세상을 읽는 단위"),
  p("LLM은 텍스트를 문자나 단어 단위로 처리하지 않습니다. 대신 \"토큰\"이라는 더 작은 단위로 처리합니다. 토큰은 대략 자주 등장하는 문자 조합 단위로 나뉘는데, 언어마다 토큰화 효율이 크게 다릅니다."),
  p("영어 단어 \"hello\"는 대개 토큰 1개에 해당합니다. 반면 한국어 \"안녕하세요\"는 동일한 개념임에도 4~6개의 토큰을 소모합니다. 이는 단순히 입력 비용의 차이가 아닙니다. 모델이 처리할 수 있는 컨텍스트 윈도우의 용량을 훨씬 빠르게 소모한다는 의미입니다."),
  p("이 차이가 발생하는 근본적인 이유는 모델의 어휘 사전(vocabulary)이 영어 중심으로 구성되어 있기 때문입니다. 모델이 학습할 때 가장 많이 노출된 언어가 영어이므로, 영어 토큰은 더 크고 의미 있는 단위로 묶이는 반면, 한국어나 다른 언어들은 더 잘게 쪼개져 처리됩니다."),
  callout("동일한 내용을 전달하는 데 한국어는 영어보다 3~5배 많은 토큰을 소모할 수 있습니다. 이는 컨텍스트 예산의 문제이자 모델의 이해 효율의 문제입니다."),

  h2("1.2 컨텍스트 윈도우와 열화 현상"),
  p("LLM은 한 번에 처리할 수 있는 텍스트 양에 제한이 있습니다. 이를 컨텍스트 윈도우라고 합니다. 현재의 모델들은 수십만 토큰에 달하는 큰 컨텍스트 윈도우를 지원하지만, 단순히 용량이 크다고 해서 모든 정보를 동등하게 처리하지는 않습니다."),
  p("\"Lost in the Middle\"이라 불리는 현상이 있습니다. 연구에 따르면 컨텍스트 초반과 후반에 위치한 정보는 잘 참조되지만, 중간 부분의 정보는 모델의 주의(attention)가 희석되어 사실상 무시되는 경향이 있습니다. 대화가 길어질수록, 앞서 했던 결정이나 합의 사항들이 점점 모델의 유효 컨텍스트 밖으로 밀려나게 됩니다."),
  p("실질적으로 이것이 의미하는 바는, 긴 대화 세션 후반부에서 모델이 \"dull\"해지거나 앞선 지시를 잊어버리는 현상이 일어난다는 것입니다. 이는 모델의 능력 저하가 아니라 구조적인 제약입니다."),

  h2("1.3 학습 데이터 편향 — 코드가 영어인 이유"),
  p("모델의 학습 데이터는 인터넷의 광대한 텍스트로 구성되어 있습니다. 이 데이터의 압도적인 비중이 영어입니다. 소프트웨어 문서, StackOverflow 답변, GitHub 코드, 기술 블로그 — 이 모든 것이 주로 영어로 쓰여 있습니다."),
  p("코드 자체도 사실상 영어입니다. 변수명, 함수명, 주석, API 이름, 에러 메시지 — 개발 생태계 전체가 영어를 기반으로 형성되어 있습니다. 따라서 모델은 영어로 된 코드 관련 작업에서 훨씬 더 정확하고 풍부한 추론을 합니다. 개념과 개념 사이의 연결이 영어 공간에서 더 촘촘하게 형성되어 있기 때문입니다."),
  p("이는 모델이 한국어를 이해하지 못한다는 의미가 아닙니다. 그러나 같은 내용을 영어로 표현했을 때, 모델이 관련 지식을 더 풍부하게 연결하고 더 정확한 결과물을 낼 가능성이 높다는 것을 의미합니다."),
  spacer(),

  // ── Part 2 ──
  h1("Part 2. 언어 경계 원칙"),
  p("Part 1에서 살펴본 물리적 제약을 종합하면 하나의 결론에 도달합니다: AI와의 협업에서 영어가 더 유리하다. 하지만 현실적으로 모든 개발자가 영어 네이티브가 될 수는 없습니다. 그렇다면 어떻게 해야 할까요?"),
  p("실용적인 해법은 언어를 \"어디서 사용하는가\"에 따라 구분하는 것입니다. 모든 것을 영어로 할 필요가 없습니다. 중요한 것은 AI가 소비하는 아티팩트의 언어입니다."),

  h2("2.1 세 가지 층위"),
  p("협업 언어를 다음 세 층위로 나누어 생각할 수 있습니다:"),
  bullet("사람의 사고 층 — 논의, 브레인스토밍, 피드백: 한국어"),
  bullet("경계 층 — 결정 사항의 문서화, AI에게 전달할 명세 작성: 영어"),
  bullet("AI 소비 층 — 코드, 스펙, 티켓, 계획서, 문서: 영어"),
  spacer(),
  p("이 구분에서 중요한 것은 \"경계 층\"입니다. 사람들 사이의 논의는 한국어로 자유롭게 진행합니다. 그러나 그 논의의 결론을 AI가 처리할 아티팩트(티켓, 계획서, 스펙 등)로 변환하는 순간, 언어는 영어로 전환됩니다."),

  h2("2.2 왜 이 경계가 중요한가"),
  p("이 언어 경계는 단순한 번역 문제가 아닙니다. 이것은 협업 구조의 아키텍처적 결정입니다. 경계를 의식하면 두 가지 이점이 생깁니다."),
  p("첫째, AI의 처리 효율이 높아집니다. AI가 영어 아티팩트를 소비할 때 토큰 예산을 아끼고, 더 풍부한 컨텍스트를 활용할 수 있습니다."),
  p("둘째, 문서화를 강제합니다. 한국어 논의에서 영어 문서로 전환하는 과정은 단순한 번역이 아니라 \"이 결정이 명확히 표현될 수 있는가\"를 검증하는 과정입니다. 모호하게 합의된 내용은 영어 문서로 만들기 어렵습니다. 이 저항감이 오히려 결정의 품질을 높입니다."),
  callout("\"지금 당신은 언어 경계를 어디에 두고 있습니까? 한국어로 요청하고 한국어로 결과물을 받는다면 — 경계가 없는 상태입니다.\""),
  spacer(),

  // ── Part 3 ──
  h1("Part 3. 핵심 명제 — 상태 외재화"),
  p("LLM의 물리적 특성에서 가장 근본적인 것은 \"상태 없음(statelessness)\"입니다. 각 세션은 독립적입니다. 지난 대화에서 무엇을 결정했든, 어떤 코드를 살펴봤든, 어떤 아키텍처를 논의했든 — 새 세션을 시작하면 모델은 그것을 모릅니다."),

  h2("3.1 AI는 망각한다 — 그리고 이것은 버그가 아니다"),
  p("대부분의 사용자는 이 망각을 극복해야 할 결함으로 봅니다. 그래서 매 세션마다 긴 배경 설명을 붙이거나, 대화를 최대한 길게 이어가거나, 같은 내용을 반복해서 설명하게 됩니다."),
  p("그러나 다른 관점이 있습니다. AI의 망각을 전제로 하면, 우리가 정말로 중요한 정보를 \"외부에 써놓는\" 습관을 강제적으로 갖게 됩니다. 인간 팀에서도 중요한 결정은 문서화해야 나중에 의미를 갖습니다. AI 협업은 이 원칙을 더 엄격하게 적용하도록 요구할 뿐입니다."),

  h2("3.2 상태 외재화란 무엇인가"),
  p("상태 외재화(State Externalization)란 AI가 매 세션마다 올바른 컨텍스트로 \"재진입\"할 수 있도록, 모든 중요한 상태를 외부 문서에 기록하는 실천입니다."),
  p("이 실천은 크게 두 갈래로 나뉩니다:"),
  numbered("이해 상태의 외재화: 코드베이스에 대한 AI의 이해를 문서로 유지합니다. mental-model, spec, ai-docs가 이 역할을 합니다. AI는 매 세션 시작 시 이 문서들을 읽고 올바른 이해 상태로 진입합니다."),
  numbered("의도와 결정의 외재화: 설계 결정, 거부된 대안, 합의된 방향을 문서로 기록합니다. 티켓(ticket)과 계획서(plan)가 이 역할을 합니다. AI는 이 문서들을 읽고 사람의 의도를 재구성합니다."),
  spacer(),
  callout("\"AI가 당신의 코드를 처음 보는 것처럼 행동한다면, 문제는 AI가 아니라 외재화된 상태가 없다는 것입니다.\""),
  p("이 두 가지 외재화가 탄탄하게 갖춰졌을 때, AI는 더 이상 매 세션마다 처음부터 시작하지 않습니다. 문서가 AI의 기억을 대체하는 것입니다."),
  spacer(),

  // ── Part 4 ──
  h1("Part 4. Skills, Agents, Subagents"),
  p("본론으로 들어가기 전에, 워크플로우를 구성하는 기술적 요소들을 간략히 살펴보겠습니다. Claude Code는 단순한 코드 생성 도구를 넘어, 스크립트화된 행동 패턴을 정의하고 실행할 수 있는 플랫폼을 제공합니다."),

  h2("4.1 Skills — 반복되는 워크플로우의 정의"),
  p("Skill은 Claude가 특정 유형의 요청을 받았을 때 따라야 할 행동 패턴을 마크다운 문서로 정의한 것입니다. 예를 들어, \"/discuss\"라는 스킬은 사용자가 아이디어를 탐색할 때 Claude가 어떤 방식으로 대화를 이끌고, 어떤 아티팩트를 생성하며, 어떤 후속 스킬로 연결할지를 정의합니다."),
  p("Skill의 핵심은 Claude 자체의 기본 행동을 바꾸는 것이 아니라, 특정 상황에서 따라야 할 프로토콜을 명시적으로 정의한다는 점입니다. 이를 통해 반복적인 워크플로우를 일관되게 실행하고, 팀 전체가 동일한 패턴을 공유할 수 있습니다."),

  h2("4.2 Agents — 특화된 역할의 분리"),
  p("Agent는 특정 도메인에 특화된 프롬프트 세트를 가진 Claude 인스턴스입니다. 예를 들어, \"rust-api-lookup\" 에이전트는 Rust 크레이트의 API 시그니처를 정확하게 조회하는 데 최적화되어 있습니다. \"mental-model-updater\"는 소스 코드 변경 후 mental-model 문서를 갱신하는 데 특화되어 있습니다."),
  p("Agent와 Skill의 차이는 범용성에 있습니다. Skill은 사람과의 대화 흐름을 정의하고, Agent는 특정 작업을 독립적으로 수행하도록 설계됩니다."),

  h2("4.3 Subagents — 컨텍스트를 분산하는 구조"),
  p("Subagent는 메인 Claude 세션에서 별도의 독립적인 Claude 인스턴스를 생성하여 특정 작업을 위임하는 패턴입니다. 핵심은 각 Subagent가 자신만의 깨끗한 컨텍스트 윈도우를 가지고 시작한다는 점입니다."),
  p("예를 들어, 메인 에이전트가 소스 코드 탐색을 Subagent에게 위임하면, 소스 코드 탐색에 필요한 대규모 컨텍스트 소모가 메인 에이전트의 컨텍스트를 오염시키지 않습니다. Subagent는 탐색을 완료하고 결과 요약만 메인 에이전트에게 반환합니다."),
  callout("Subagent 패턴은 \"컨텍스트를 아끼는 기술\"입니다. 비싼 읽기 작업은 신선한 컨텍스트를 가진 에이전트에게 위임하고, 의사결정은 lean한 메인 에이전트에서 합니다."),
  spacer(),

  // ── Part 5 ──
  h1("Part 5. 워크플로우 체인"),
  p("이제 원칙들이 어떻게 구체적인 워크플로우로 형성되는지 살펴보겠습니다. 이 워크플로우의 핵심 아이디어는 하나입니다: 아티팩트가 인터페이스다."),
  p("각 단계는 이전 단계의 아티팩트를 입력으로 받고, 다음 단계를 위한 아티팩트를 출력으로 생성합니다. 이 덕분에 각 단계는 서로 독립적이며, 다른 컨텍스트 윈도우에서도 올바르게 재개될 수 있습니다."),

  h2("5.1 /discuss — 발산의 공간"),
  p("모든 것은 탐색에서 시작합니다. /discuss 스킬은 아이디어를 자유롭게 탐색하고 방향을 잡는 단계입니다. 이 단계에서 Claude는 mental-model 문서를 참조하거나 Explore Subagent를 통해 코드베이스를 살펴보면서 제안, 반론, 위험 요소를 함께 검토합니다."),
  p("중요한 것은 /discuss 단계 자체는 아티팩트를 직접 생성하지 않는다는 점입니다. 이 단계의 목적은 다음 단계인 /write-ticket을 위한 명확한 방향을 도출하는 것입니다. 논의가 충분히 수렴되면, 그 결론이 티켓으로 외재화됩니다."),

  h2("5.2 /write-ticket — 결정의 외재화"),
  p("티켓은 \"의도를 잠그는\" 아티팩트입니다. 좋은 티켓은 다음 내용을 담습니다:"),
  bullet("달성하려는 목표와 그 배경"),
  bullet("거부된 대안과 거부 이유"),
  bullet("합의된 접근 방식"),
  bullet("알려진 제약과 경계 조건"),
  bullet("간략한 의사 코드 또는 구조 스케치"),
  spacer(),
  p("티켓의 자기완결성(self-containedness) 테스트는 엄격합니다: 이 대화에 참여하지 않았던 에이전트가 티켓만 읽고 다음 단계를 진행할 수 있는가? 이 질문에 \"예\"라고 답할 수 없다면, 티켓이 불완전한 것입니다."),
  p("티켓은 파일시스템 디렉토리 구조로 상태를 관리합니다: idea/ → todo/ → wip/ → done/. 상태 전환은 파일 이동입니다. 단순하지만 강력합니다."),

  h2("5.3 /write-plan — 코드베이스와의 연결"),
  p("티켓이 무엇을 할지를 결정한다면, 계획서는 어떻게 할지를 코드베이스의 실제 구조에 맞춰 기술합니다. 계획서는 티켓과 별개의 새로운 컨텍스트에서 작성됩니다 — 계획서 작성 에이전트는 티켓만을 입력으로 받고, 코드베이스를 직접 탐색하여 계획을 수립합니다."),
  p("계획서에는 다음 내용이 포함됩니다:"),
  bullet("변경할 구체적인 파일 목록"),
  bullet("각 변경의 계약(contract) — 입력, 출력, 부작용"),
  bullet("통합 지점과 의존성"),
  bullet("테스트 전략"),
  spacer(),
  p("계획서가 완성되면 코드 저장소에 커밋됩니다. 이는 계획서 자체가 영구적인 상태 외재화의 일부가 된다는 의미입니다."),

  h2("5.4 실행 단계 — 세 가지 선택"),
  p("계획이 준비되면 실행 단계로 넘어갑니다. 세 가지 경로가 있습니다:"),
  bullet("/execute-plan: 계획서를 계약으로 간주하고 충실하게 구현합니다. 계획의 가정이 실제 코드베이스와 다르면 멈추고 보고합니다. 대규모 변경에 적합합니다."),
  bullet("/implement: 계획서가 방향과 파일 목록만 제시하는 경우에 적합합니다. 좀 더 유연하게 코드베이스를 탐색하면서 구현합니다."),
  bullet("/sprint: 단일 에이전트가 유연하게 처리하는 경량 실행입니다. 소규모 변경이나 잘 정의된 단순 작업에 적합합니다."),
  spacer(),

  // ── Part 6 ──
  h1("Part 6. 문서 관리 체계 — AI의 이해 상태 유지"),
  p("워크플로우 체인이 \"의도와 결정\"을 외재화하는 구조라면, 문서 관리 체계는 \"코드베이스에 대한 AI의 이해\"를 외재화하는 구조입니다. 이 두 가지가 함께 작동할 때 비로소 완전한 상태 외재화가 달성됩니다."),

  h2("6.1 ai-docs 구조"),
  p("ai-docs 디렉토리는 AI가 세션마다 재진입하기 위해 읽어야 할 모든 문서의 저장소입니다. 각 문서 유형은 다른 \"상태 붕괴 속도(decay rate)\"와 다른 소비자를 가집니다."),
  bullet("_index.md: 세션 시작 시 첫 번째로 읽는 문서. 아키텍처 개요, 주요 스펙 링크, 빌드/테스트 명령어, 세션 간 노트를 담습니다. AI의 \"오리엔테이션\" 역할을 합니다."),
  bullet("mental-model/: 코드베이스를 수정하기 위해 알아야 하는 운영 지식. 공식 문서에는 없지만 모르면 버그를 만드는 암묵적 계약, 결합 관계, 확장 지점 등을 담습니다."),
  bullet("spec/: 외부에서 바라본 기능 명세. \"무엇을 하는가\"를 기술하지 \"어떻게 하는가\"는 기술하지 않습니다."),
  bullet("tickets/: 결정 원장(decision ledger). 각 티켓은 목표, 제약, 거부된 대안, 합의된 접근 방식을 담습니다."),
  bullet("plans/: 코드베이스 기반의 구현 계약. 타임스탬프가 찍혀 커밋됩니다. 한 번 작성되면 수정되지 않는 불변 아티팩트입니다."),

  h2("6.2 Write-back Loop — 상태 동기화의 의무"),
  p("문서 관리 체계의 가장 중요한 원칙은 Write-back Loop입니다. 어떤 구현이 완료된 후에도 관련 문서가 갱신되지 않으면, 문서는 점점 코드베이스와 괴리됩니다. 이 괴리가 쌓이면 AI는 잘못된 상태로 진입하고, 문서 체계 전체가 신뢰를 잃습니다."),
  p("따라서 모든 구현 스킬의 마지막 단계에는 반드시 다음 갱신이 포함됩니다:"),
  bullet("mental-model 문서 갱신 — 소스 변경이 기존 이해를 무효화한 부분 업데이트"),
  bullet("spec 문서 갱신 — 사용자 가시적 기능 변경이 있는 경우"),
  bullet("_index.md 갱신 — 중요한 아키텍처 변경이나 새로운 세션 노트"),
  bullet("티켓 Result 항목 추가 — 실제로 무슨 일이 일어났는지, 계획과의 편차는 무엇인지"),
  spacer(),
  p("이 갱신들은 선택 사항이 아닙니다. 구현의 마지막 단계로서 필수적으로 실행됩니다. 이를 자동화하기 위해 mental-model-updater, spec-updater 등의 전용 에이전트가 존재합니다."),
  callout("\"코드를 바꿨다면 AI가 그 코드에 대해 갖고 있는 이해도 바꿔야 합니다. 이 동기화가 무너지면 문서는 부채가 됩니다.\""),
  spacer(),

  // ── Part 7 ──
  h1("Part 7. Marathon — 토큰 경제의 구조적 해법"),
  p("지금까지 설명한 워크플로우는 단일 에이전트가 긴 세션 동안 작업한다고 가정합니다. 그러나 복잡한 기능 구현은 수십 번의 탐색, 수백 개의 파일 읽기, 반복적인 수정을 수반합니다. 이 모든 것이 하나의 컨텍스트 윈도우에 쌓이면 어떻게 될까요?"),
  p("세션 초반에 읽은 파일들이 \"lost in the middle\" 영역으로 밀려납니다. 초기에 합의한 설계 원칙들이 희석됩니다. 에이전트는 점점 느려지고 일관성을 잃습니다. 이것이 긴 구현 세션에서 흔히 경험하는 \"에이전트 피로\" 현상의 실제 원인입니다."),

  h2("7.1 팀 기반 위임 구조"),
  p("Marathon은 이 문제를 구조적으로 해결합니다. 핵심 아이디어는 단순합니다: 메인 에이전트(Lead)는 코드를 직접 읽지 않습니다."),
  p("Lead는 대화와 판단만 담당하며, 모든 코드 읽기/쓰기 작업은 신선한 컨텍스트를 가진 팀 멤버(Subagent)에게 위임됩니다. 팀 멤버는 작업을 완료하고 요약만 Lead에게 반환합니다. Lead는 방대한 소스 코드를 직접 소비하는 것이 아니라, 팀 멤버가 추출한 핵심 정보만 소비합니다."),

  h2("7.2 팀 멤버 역할"),
  bullet("Planner: 코드베이스를 깊이 탐색하고 구현 계획서를 작성합니다. 작업 완료 후 계획서 링크만 Lead에게 반환합니다."),
  bullet("Implementer: 계획서와 대상 파일만 읽고 구현합니다. diff 요약을 Lead에게 반환합니다."),
  bullet("Reviewer: diff만 읽고 코드 리뷰를 수행합니다. 제한된 입력, 제한된 출력."),
  bullet("Clerk: 티켓 읽기/쓰기를 담당합니다. 문서 조작에 특화."),
  bullet("Worker: 문서, 설정, 비코드 작업을 처리합니다."),
  spacer(),

  h2("7.3 모델 선택의 경제학"),
  p("Marathon은 작업의 인지적 요구에 따라 모델을 선택하는 원칙도 포함합니다. Explore 작업은 가장 저렴한 모델(Haiku)로 시작하고, 부족할 때만 더 강력한 모델로 에스컬레이션합니다. 구현자는 Sonnet을 기본으로 사용하고, 새로운 아키텍처 설계가 필요한 경우에만 Opus를 사용합니다."),
  p("이것은 단순한 비용 절감이 아닙니다. 각 호출을 그 인지적 요구에 맞게 적정 크기로 만드는 것입니다. 과도한 모델을 사용하면 시간이 낭비되고, 과소한 모델을 사용하면 품질이 저하됩니다."),
  callout("Marathon의 핵심 독트린: \"Lead의 유일한 유한 자원은 컨텍스트 윈도우입니다. 규칙이 모호할 때는 소비되는 토큰당 더 나은 결정을 이끌어내는 해석을 선택하십시오.\""),
  spacer(),

  // ── Part 8 ──
  h1("Part 8. 핵심 원칙 3가지 — 당신의 워크플로우 설계를 위해"),
  p("지금까지 설명한 구체적인 스킬들과 도구들을 제쳐두고, 이 시스템 전체가 구현하려는 원칙을 추출한다면 다음 세 가지로 정리됩니다. 이 원칙들은 특정 도구 없이도 적용 가능합니다."),

  h2("원칙 1. 아티팩트 계약 — 컨텍스트 리셋을 안전하게 만들어라"),
  p("워크플로우의 모든 핸드오프 지점은 자기완결적인 아티팩트를 생성해야 합니다. \"이전 대화를 모르는 에이전트가 이 아티팩트만 읽고 다음 단계를 계속할 수 있는가?\"가 아티팩트 품질의 테스트입니다."),
  p("이것은 단순한 문서화가 아닙니다. 컨텍스트 리셋(세션 종료, 팀원 교체, 며칠 후 재개 등)이 발생해도 작업이 안전하게 계속될 수 있도록 하는 아키텍처적 결정입니다."),
  p("실천: 티켓에는 결정 사항뿐 아니라 거부된 대안과 그 이유를 반드시 포함합니다. 계획서에는 파일 목록뿐 아니라 각 변경의 계약(입출력, 부작용)을 포함합니다."),

  h2("원칙 2. 결정과 탐색의 분리"),
  p("설계 결정과 코드베이스 탐색을 같은 컨텍스트에서 혼합하면 양쪽 다 품질이 낮아집니다. 설계 결정은 티켓 수준에서(소스 코드를 읽지 않고, mental-model 문서만 참조), 코드베이스 탐색은 계획서 수준에서(명시적 탐색, 작성된 계획 출력) 분리합니다."),
  p("이 분리를 통해 각 단계의 인풋이 명확해지고, 컨텍스트가 의도적으로 소비됩니다. 무엇을 결정하는 단계와 어떻게 구현하는 단계는 서로 다른 정보를 필요로 합니다."),

  h2("원칙 3. 오케스트레이터를 Lean하게 유지하라"),
  p("긴 세션에서 대화 컨텍스트를 보유한 에이전트는 인지 작업을 수행하기 가장 비싼 곳입니다. 이 에이전트는 조율자와 의사결정자로 남아야 하며, 대규모 입력을 읽는 작업은 신선한 컨텍스트의 Subagent에게 위임해야 합니다."),
  p("Subagent 경계를 넘는 것은 짧고 구조화된 \"brief\"(지시서)입니다. 이 brief가 외재화된 상태입니다. brief를 잘 쓰는 것이 효과적인 위임의 핵심입니다."),
  p("이 원칙은 비단 AI 협업에만 해당하지 않습니다. 인간 팀에서도 좋은 테크 리드는 모든 코드를 직접 읽지 않습니다. 팀원들이 잘 정의된 brief를 받아 독립적으로 작업하고, 요약을 보고하는 구조가 팀 전체의 컨텍스트를 효율적으로 유지합니다."),
  callout("이 세 원칙은 AI 협업의 발명이 아닙니다. 좋은 소프트웨어 팀이 이미 실천하는 것들입니다. AI는 이 원칙을 더 엄격하게 적용하도록 강제할 뿐입니다."),
  spacer(),

  // ── 마치며 ──
  h1("마치며"),
  p("이 문서에서 소개한 워크플로우와 도구들은 하나의 구체적인 구현입니다. 여러분의 팀과 프로젝트는 다른 맥락을 가지고 있으며, 동일한 패턴이 최선이 아닐 수 있습니다."),
  p("그러나 원칙은 다릅니다. AI는 상태가 없습니다. 따라서 협업의 핵심은 상태를 외재화하는 설계입니다. 아티팩트가 컨텍스트 리셋을 견딜 수 있어야 합니다. 결정과 탐색은 분리되어야 합니다. 오케스트레이터는 lean해야 합니다."),
  p("이 원칙들을 가지고 여러분만의 워크플로우를 설계해 보십시오. 특정 스킬이나 도구를 그대로 복사하는 것이 목표가 아닙니다. 여러분이 부딪히는 구체적인 문제 — 세션 간 컨텍스트 손실, 긴 세션에서의 품질 저하, 팀 내 AI 협업 패턴 공유 — 에 이 원칙들을 적용하여 자신만의 길을 닦으십시오."),
  callout("\"이 시스템이 어떻게 작동하는지가 아니라, 왜 이렇게 설계되었는지를 이해하는 것. 그것이 당신이 자신의 길을 닦을 수 있게 합니다.\""),
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
