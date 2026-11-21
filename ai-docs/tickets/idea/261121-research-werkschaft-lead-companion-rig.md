---
title: "Werkschaft 리드 컴패니언(데스크톱 펫): 종이인형 컷아웃 리깅 파이프라인 및 최상위 게임 액터 아키텍처 리서치"
related:
  261110-epic-ws-dashboard-agent-host: host surface for lead companion overlay
---

# Werkschaft Lead Companion (Desktop Pet): Layered Paper-Doll Cutout Rigging Pipeline and Top-Level Game-Actor Architecture

## Background

During exploratory prototyping for an opt-in Werkschaft dashboard companion (a cute chibi character accompanying **lead sessions only**, disabled by default), we tested two visual rigging approaches — pure hand-coded SVG rigs across three harnesses vs. **layered paper-doll cutout sprite rigging** — alongside a top-level game-actor overlay architecture driven by a two-speed decision model.

This research ticket preserves the empirical findings, the reproducible same-canvas image-to-layer cutout pipeline, the agreed three-stage asset/scenario workflow, and the live rigging details to watch out for when moving to real asset production and engine implementation.

## Prototyping Findings: Pure SVG Rig vs. Layered Paper-Doll Cutout

### 1. Three-Harness Pure SVG Rig Showdown
- Three large-tier agents independently authored animatable, CSS-driven pure SVG rigs (`50–71 KB` each) with pivoted bone groups (`root`, `head`, `twin-tails`, `arms`, `legs`), swappable expression `<defs>`, and continuous clipped pupil-rolling/blinking animations.
- **Result:** Technically solid and cleanly animated, but hand-traced SVG primitives failed to preserve the organic hand-drawn line weight, shading warmth, and comedic charm of the reference chibi illustration.

### 2. Same-Canvas Layered Paper-Doll Dissection Prototype
- Instead of hand-tracing vectors or generating disconnected parts from scratch, we started from a single static chibi reference (`1254 × 1254`) and used **same-canvas image-to-image editing** to isolate overlapping anatomical layers while inpainting hidden overlap areas:
  - Back layer: `layer-tails.png` (twin-tails with rounded top anchor bulbs)
  - Lower body: `layer-legs.png` (thighs rounded cleanly at hip joint)
  - Core body: `layer-torso.png` (outfit & skirt)
  - Upper limbs: `layer-arms.png` (bent arms with rounded shoulder joints)
  - Head & expressions: `layer-head-blank.png` (clean blank face with hair, ribbons, hairpins, blush), plus `layer-head-smug.png`, `layer-head-typing.png`, `layer-head-flustered.png`, `layer-head-gloom.png`.
- **Empirical alignment & footprint:**
  - `head-blank` vs. `head-smug` aligned at `(dx=0, dy=0)`; `head-typing` and `head-flustered` at `dx=-1`; `head-gloom` at `dx=-7`.
  - Automated post-processing cleanly separated paired parts (`tail-l/r`, `leg-l/r`, `arm-l/r`), flood-erased ghost sleeves outside the suspender lines on `torso`, extracted a foreground bangs cutout (`head-bangs-fg`) by flood-filling the face skin interior on `head-blank`, and composited alternate facial details (`flustered` blush/sweat/mouth, `gloom` vertical shadow/mouth/sweat) strictly inside the shared `head-blank` face mask — eliminating hair/ribbon contour jitter completely when switching moods.
  - Resizing all 13 transparent parts to `440 × 440` and encoding with `cwebp -q 92 -alpha_q 100 -m 6` yielded **109,606 bytes total (~110 KB)** while remaining 2× Retina crisp at `72–236 px` on dark and light themes.
- **Sliding Half-Lids + Live Pupils Behind Raster Bangs:**
  - Sandwiching an aligned SVG eye layer between `head-blank` (back) and `head-bangs-fg` (front) allowed continuous cursor-tracking/rolling irises and skin-colored sliding upper half-lids (`0%` open -> `44%` half-lidded -> `100%` blink) to tuck naturally behind the real hand-drawn raster bangs, with anime eyebrows rendered on top of `head-bangs-fg`.

## Reproducible Same-Canvas Cutout Pipeline (Reference Recipe)

Preserve this pipeline for both official IP-safe packs and user-authored local packs:

1. **Source reference freeze:** Start from one approved, clean-background static SD reference per required base pose (`1:1` square canvas, e.g., `1024–1254 px`). Keep line weight uniform and clear of background props.
2. **Same-canvas image-to-image layer dissection:** Generate every layer by editing the **exact same reference image** at the same canvas size, prompting the model to erase all other parts and cleanly inpaint hidden joints/overlaps:
   - `tails` (or back hair/accessories behind head & torso)
   - `legs` (hips rounded cleanly so skirt swing does not expose flat cut edges)
   - `torso` (sleeveless/core body with shoulder/neck attachment zones inpainted)
   - `arms` (or scenario-specific upper-arm + forearm/hand layers; see Rigging Detail Notes)
   - `head-blank` (full hair, accessories, ears, chin outline, cheek blush — completely blank eyes/mouth/brows)
   - Expression/face variants (edited from `head-blank` or the base head so hair silhouette stays anchored)
3. **Deterministic programmatic post-processing (Python / Pillow + NumPy + `cwebp`):**
   - Strip low-alpha fringe noise (`alpha < 12 -> 0`).
   - Split paired limbs along their vertical valley/midline (`tail-l/r`, `arm-l/r`, `leg-l/r`).
   - Clean residual bleed on `torso` (e.g., flood-erase unwanted outer sleeve lobes while protecting core outfit outlines).
   - Register expression heads against `head-blank` via upper-hair alpha correlation `(dx, dy)` and transfer only the face interior mask (`face_skin` flood-fill from center of `head-blank`) so hair/accessories never shift between moods.
   - Extract `head-bangs-fg` by punching `face_skin` and lower jaw outline out of `head-blank`, preserving the dark inner bang strokes so eyes/eyelids slide cleanly underneath.
   - Erase static pupil dots from raster expression sockets (e.g., `flustered`, `gloom`) strictly below the upper lash line so live pupils animate cleanly without nicking the top lid stroke.
   - Resize all registered layers to a uniform pack canvas (e.g., `440 × 440` or `512 × 512`) and compress to lossless-alpha WebP (`cwebp -q 92 -alpha_q 100 -m 6`).
4. **Runtime bone stack (back to front, shared canvas coordinates):**
   ```text
   root (ground position, jump arc, facing scaleX, landing squash-and-stretch)
     ├── tail-l / tail-r       (2nd-order spring-damper lagging head & velocity)
     ├── leg-l / leg-r         (hip pivots; alternating walk/hop steps)
     ├── torso                 (waist pivot; breathing & subtle tilt; swappable per scenario)
     ├── arm-l / arm-r         (shoulder pivots + optional forearm child bone; swappable per scenario)
     ├── head                  (neck pivot; spring bob & look-at tilt)
     │     ├── head-blank      (shared base hair/head/skin)
     │     ├── face-raster     (optional scenario base: blush hatch, gloom stripes, sweat)
     │     ├── eyes-svg        (sclera + clipped live irises + sliding half-lids)
     │     ├── mouth-layer     (swappable raster/SVG mouth; speech bounce)
     │     ├── head-bangs-fg   (hand-drawn bangs masking upper lid edges)
     │     └── brows-svg       (anime eyebrows drawn over bangs)
     └── comic-fx              (floating scenario props: sparks, keyboard, !?, gloom vortex)
   ```

## Three-Stage Production Workflow (Engine-First Ordering)

Because we are building a reusable companion **engine** rather than a single hard-coded sprite, asset creation and rigging follow three ordered stages:

1. **Stage 1 — Lock actual SD static reference assets:** Produce and approve the final static SD character style & proportions first. (Down-scaling an existing chibi illustration into a smaller SD style altered its aesthetic appeal; the real target SD style must be locked before dissecting layers.)
2. **Stage 2 — Define canonical engine scenarios:** Before dissecting parts, enumerate the engine's scenario catalog (idle/roam, intense coding/typing `tatata`, attention/pointing at an open ask, startled/poke, failure/gloom, celebration/done, etc.) and declare which body/limb/pose asset swaps each scenario requires.
3. **Stage 3 — Apply the same-canvas cutout pipeline per scenario asset:** Run the same-canvas dissection and registration pipeline against the scenario-selected reference assets from Stages 1–2, packing them into the uniform pack format.

## Live Rigging Detail Notes & Known Prototype Gotchas

Captured from zoomed visual inspection of the prototype so they are addressed during live engine/asset tuning (not over-engineered prematurely):

1. **Sclera-to-skin edge bleed under tilted half-lids:** When an SVG sclera is clipped and overlapped by a rotated skin-colored half-lid rectangle, tiny slivers of skin or anti-aliased edge gaps can erode the inner corner of the eye white under the thick lash bar. Ensure the sclera's lower/side outline joins cleanly into the lash bar and pad the sliding lid mask geometry with clean overlap/anti-alias margins.
2. **Cutout drop-shadow bleed on interior face patches:** Applying a blanket `drop-shadow` filter to every `.part img` also casts a muddy grey shadow under floating interior patches like the mouth cutout (`face-smug-mouth`). Restrict paper-doll drop-shadows strictly to outer articulated silhouettes (`tails`, `legs`, `torso`, `arms`, `head-blank`), never to inner facial overlays (`mouth`, `face-raster`, `bangs-fg`).
3. **Scenario-specific body & limb swaps (no forced pose reuse):** Rotating hands-on-hips idle arms inward for keyboard typing (`tatata`) looks unnatural. Scenarios with distinct body language (typing, pointing at a UI landmark, cheering, slumped gloom) should swap dedicated pose/body/arm assets rather than stretching one idle arm cutout.
4. **Fine-grained articulation for kinetic scenarios (e.g., keyboard `tatata`):** Expressive typing needs articulation beyond a single rigid shoulder-pivoted arm — specifically separate **forearm / below-elbow** animation elements plus subtle **shoulder shrug/heave** (`torso`/shoulder motion) and synchronized head/keyboard bounce. Keep exact curves and amplitudes tunable live in the engine.
5. **Facing flip vs. overlay text/props:** Flipping the character root with `scaleX(-1)` mirrors child text or directional icons unless counter-flipped (`scaleX(var(--facing))`) or mounted outside the flipped rig layer.

## Companion Engine Architecture (Settled Direction & Draft Sketches)

### 1. Top-Level Viewport Game-Actor Overlay
- Mount the companion in a top-level viewport overlay (`position: fixed; inset: 0; pointer-events: none;`, with only the character hitbox `pointer-events: auto`), **not** confined inside the composer box.
- The dashboard exposes named UI landmarks and obstacle rectangles (modals/popovers) plus a lightweight actor state snapshot so an external controller or local AI can direct the companion anywhere on screen like a game character:
  ```json
  {
    "actor": {
      "pos": { "x": 583, "y": 371 },
      "anchor": "composer-top",
      "facing": 1,
      "pose": "idle",
      "mood": "smug",
      "idle_sec": 12.4,
      "last_event": "user_typing"
    },
    "landmarks": [
      { "id": "composer-top", "rect": [220, 740, 520, 64] },
      { "id": "whiteboard-ask", "rect": [860, 120, 340, 180] },
      { "id": "hud-roster", "rect": [24, 16, 260, 44] },
      { "id": "chat-edge", "rect": [300, 420, 480, 120] }
    ]
  }
  ```

### 2. Two-Speed Game AI Decision Model
- **Fast Path — Local 60 fps 0-Token Utility / Behavior-Tree AI:**
  - Runs in the browser with zero token cost: paper-doll second-order spring physics, smooth `look-at` (typing caret, scroll direction/velocity, mouse pointer), obstacle avoidance, idle wandering/perching, click-poke reaction, and instant dashboard HUD state reflection (`thinking`, `tool`, `permission`, `failed`, `idle`).
- **Slow Path — Lightweight Delta-Cached Sidecar Narrative Director:**
  - Keeps lead session context clean and avoids expensive lead-model tool-call round-trips.
  - Consumes a static cached system/persona/choreography prompt + append-only lead conversation deltas + actor state snapshot + visible landmarks, emitting high-level game commands:
    ```json
    {
      "move_to": "whiteboard-ask",
      "pose": "point-at",
      "mood": "smug",
      "quip": "Ask (22) still waiting!"
    }
    ```
- **Optional Direct Lead Tool (`ws/emotion`):** Kept pluggable as an alternative director mode (`lead-tool`, lead-only via `isLeadOnlyTool`) when the user wants the lead itself to drive reactions directly.

### 3. Opt-In Configuration (`lead-tune` / `dashboard-config-manifest.json`, T0, Default `off`)
Draft configuration knobs (T0 transparent additions; lead sessions only; off by default):
```json
{
  "dashboard.companion_mode": "off | visual | full",
  "dashboard.companion_director": "rules | sidecar | lead-tool",
  "dashboard.companion_pack": "auto | <pack-id>"
}
```
- `visual`: animated game actor + optional short speech quips; zero change to lead chat prose.
- `full`: also injects persona speaking-style guidance into the lead's `User preferences` with a strict guardrail (chat responses only — never code, commits, tickets, or worker prompts).
- `auto`: resolves a pack by session harness (`agy`, `claude`, `codex`, `pi`) plus optional `model_match` pattern from local user packs under `~/.ws/companions/<pack-id>/pack.json`.

### 4. Public Repository IP Boundary
- Public repository ships **only** the companion engine, pack schema, IP-safe original default assets (or BYO starter guide), and the reproducible same-canvas cutout pack generation recipe.
- Third-party or community anime characters (including the local prototyping character used during this exploration) live strictly in local user packs (`~/.ws/companions/`) and are never committed to the public repository.

## Outcome Ledger

### Verified Findings
- Three independent pure-SVG character rigs (`50–71 KB`, CSS-driven) animated smoothly in headless Chromium, but hand-coded SVG geometry did not preserve the organic hand-drawn charm of the chibi reference.
- Same-canvas image-to-image layer dissection from a single `1254 × 1254` chibi reference produced usable transparent layers with near-zero registration drift (`0–1 px` on `smug`/`typing`/`flustered`, `7 px` horizontal shift on `gloom` easily corrected by upper-hair alpha correlation).
- Programmatic post-processing (vertical midline splitting, suspender-guarded sleeve flood-erase on `torso`, `face_skin` flood-fill to extract `head-bangs-fg` and lock all moods onto `head-blank`) eliminated mood-switch silhouette jitter and compressed 13 `440 × 440` transparent WebP layers to **~110 KB total** (`109,606 B`), rendering crisply at `72–236 px` on both dark and light backgrounds.
- Sandwiching live SVG eyes and sliding skin-tone half-lids between `head-blank` and `head-bangs-fg` achieved a clean half-lidded look, smooth blinking, and live cursor tracking while keeping hand-drawn bangs naturally on top.
- Zoomed inspection of the live prototype identified concrete rigging pitfalls to solve during real production: slight sclera-vs-skin lid edge erosion at inner eye corners, unwanted drop-shadow halos on interior mouth cutouts, and unnatural posture when reusing idle hands-on-hips arms for keyboard typing.

### Confirmed Decisions
- **Visual rigging approach:** Adopt layered paper-doll cutout sprite rigging (same-canvas dissected raster WebP layers + live sliding half-lids/pupils behind `bangs-fg` + decoupled 2D spring bones) as the primary visual pipeline.
- **Three-stage engine-first production order:** Execute in order: `(1)` lock actual static SD reference assets -> `(2)` define the engine's canonical scenario catalog -> `(3)` run the same-canvas cutout dissection pipeline per scenario-selected asset.
- **Scenario-specific body/limb swaps:** Swap body/arm/pose assets per scenario (e.g., dedicated typing body/arms with below-elbow forearm articulation and shoulder bounce) instead of contorting a single idle arm pose.
- **Top-level game-actor stage:** Mount the companion as a top-level viewport overlay receiving external landmark/target positions and reporting actor state, rather than trapping it inside the composer.
- **Two-speed decision architecture:** Combine a local 60 fps 0-token physics/behavior layer with an optional lightweight sidecar narrative director (plus pluggable lead-only `ws/emotion`).
- **Lead-only opt-in posture & IP boundary:** Gate behind `lead-tune` / `dashboard-config-manifest.json` knobs defaulting to `off` (T0), support per-harness/model auto-binding via local `~/.ws/companions/` packs, and keep third-party character assets out of the public repository.

### Proposals
- Draft `dashboard.companion_mode`, `dashboard.companion_director`, and `dashboard.companion_pack` key names and enum values in `dashboard-config-manifest.json`.
- Draft actor state snapshot JSON (`pos`, `anchor`, `facing`, `pose`, `mood`, `idle_sec`, `last_event`) and sidecar command JSON (`move_to`, `pose`, `mood`, `quip`).
- Draft runtime bone stack (`tail-l/r`, `leg-l/r`, `torso`, `arm-l/r` + forearm child, `head` with `head-blank`/`face-raster`/`eyes-svg`/`mouth`/`head-bangs-fg`/`brows-svg`, `comic-fx`).
- Pack layout under `~/.ws/companions/<pack-id>/pack.json` supporting both layered WebP cutout packs and optional SVG rigs.

### Open Questions
- **Canonical scenario catalog (Stage 2):** Exact minimal v1 scenario set (e.g., `idle/roam`, `typing/tatata`, `point-at-ask`, `flustered/poke`, `gloom/error`, `celebrate/done`) and which scenarios share vs. swap torso/arm/forearm layers.
- **Target SD static reference style (Stage 1):** Exact SD head-to-body ratio, line weight, and IP-safe original default character design(s) for the public repo.
- **Sidecar director runtime budget:** Model tier, delta batching interval, rate limits, and prompt-caching mechanics so sidecar cost and latency stay negligible.
- **Live rigging polish:** Exact eyelid mask/outline construction to prevent sclera edge erosion, plus forearm + shoulder spring tuning for keyboard `tatata`.
- **`ws/emotion` vs. sidecar precedence:** Arbitration rules if both `sidecar` and `lead-tool` signals are enabled or switched mid-session.

### Rejected Alternatives
- **Pure hand-coded SVG character rigs as primary art style:** Rejected because hand-traced SVG primitives lost the organic hand-drawn chibi appeal of the reference art despite clean bone/eye animation.
- **Drawing disconnected cutout parts from scratch on separate sheets:** Rejected in favor of same-canvas image-to-image dissection from a single static reference to preserve exact proportions, shading, and registration.
- **Confining the companion strictly inside the composer input container:** Rejected in favor of a top-level viewport game-actor overlay that can roam, perch on UI landmarks, and point at asks or HUD events.
- **Reusing a single idle arms-on-hips cutout for all actions (such as keyboard typing):** Rejected because rotated idle arms read awkwardly during typing; distinct scenarios need dedicated body/arm/forearm pose assets.
