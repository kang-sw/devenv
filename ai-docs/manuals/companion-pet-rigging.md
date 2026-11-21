---
summary: Living production spec for the Werkschaft lead companion (desktop pet) — pose families, engine-shared props, 12-scenario catalog, look-at stack, and same-canvas cutout rigging pipeline
---

# Werkschaft Lead Companion: Scenario Spec & Paper-Doll Cutout Rigging Manual

Living specification and asset-production manual for the opt-in Werkschaft lead companion (desktop pet) engine. Revise this manual directly as Stage 1–3 asset design and live rigging details evolve. For exploratory prototyping data and rejected approaches, see research ticket `261121-research-werkschaft-lead-companion-rig`.

## Production Workflow (Engine-First Order)

Asset production follows three ordered stages so every character pack plugs cleanly into one reusable engine:

1. **Stage 1 — Concept & Static SD Reference Lock (Complete)**
   - **Step 1-A (Character Concept — Locked):** Lock character silhouette, palette, and personality via clean **Front + Back full-body turnaround illustrations** (no text boxes or cluttered sheet chrome). Locked default mascot turnaround: `ai-docs/assets/companion-pet/mascot-stage1a-turnaround.png` (see `## Locked Default Mascot (Stage 1-A & 1-B)` below).
   - **Step 1-B (Target SD Style, Pose References & Engine-Shared Props — Locked):** Translate the approved character into final target **compact ~2.0-head SD/chibi** static references (`1:1` square canvas, ~44% head-to-height ratio, compact vertical silhouette `aspect ~0.60`, warm thick cocoa-brown outer outlines, glossy 2-step cel shading + soft cheek blush, simplified mitten paws, lace-free chunky sneakers, clear arm/leg cutout gaps) for every required/selected pose family plus engine-shared props. Never down-scale a non-SD illustration into an SD look.
2. **Stage 2 — Scenario, Pose-Family, Prop & Socket Contract (Locked)**
   - Governed by this manual: declares the canonical v1 scenarios, pose families with automatic fallbacks, engine-shared interactive props, attachment sockets, expressions, and passive gaze rules.
3. **Stage 3 — Same-Canvas Cutout Dissection Pipeline (Active)**
   - Run same-canvas image-to-image layer dissection and deterministic post-processing against each approved Stage 1 reference, packing uniform lossless-alpha WebP layers into a `pack.json` bundle.

## Locked Default Mascot (Stage 1-A & 1-B)

- **Canonical Turnaround (Stage 1-A):** `ai-docs/assets/companion-pet/mascot-stage1a-turnaround.png` (`1536 × 1024`, Front + Back full-body turnaround, IP-clean original Werkschaft mascot).
- **Canonical Standing SD Master (Stage 1-B — Family A):** `ai-docs/assets/companion-pet/mascot-stage1b-standing-sd.png` (`1024 × 1024`, pure `#FFFFFF` background, ~44% head-to-height compact 2.0-head proportion, `aspect ~0.60`, warm thick cocoa-brown outline, glossy 2-step cel shading + soft cheek blush, rounded mitten paws, simplified lace-free chunky sneakers, paper-doll cutout arm/leg separation). Use this image as the visual style/proportion anchor for all pose families, engine-shared props, and BYO local packs.
- **Locked Stage 1-B Pose References (`1024 × 1024`, pure `#FFFFFF`):**
  - `W. Side-Walk` (3/4-side energetic walk facing right, flipped via `scaleX(±1)`): `ai-docs/assets/companion-pet/mascot-stage1b-pose-w-sidewalk.png`
  - `B. Typing / Desk` (seated forward lean, right-way-around Werkschaft keyboard with rear pentagon bezel facing viewer and golden spacebar nearest her wrists, focused tongue-out expression): `ai-docs/assets/companion-pet/mascot-stage1b-pose-b-typing.png`
  - `C. Perched / Sit` (front seated dangling legs on invisible ledge): `ai-docs/assets/companion-pet/mascot-stage1b-pose-c-perched.png`
  - `D. Dangle` (limp scruff-carried kitten posture — half-lidded deadpan eyes, shrugged collar, drooping mitten paws, vertical relaxed dangling legs, `aspect ~0.53`): `ai-docs/assets/companion-pet/mascot-stage1b-pose-d-dangle.png`
- **Locked Engine-Shared Props (`1024 × 1024`, pure `#FFFFFF`):**
  - `prop-drone`: `ai-docs/assets/companion-pet/prop-drone.png` (compact ivory/navy mini-drone with amber rotor and glowing pixel visor)
  - `prop-mail`: `ai-docs/assets/companion-pet/prop-mail.png` (chunky ivory envelope with vermilion wax seal + amber pentagon stamp)
  - `prop-grab`: `ai-docs/assets/companion-pet/prop-grab.png` (side-by-side Open / Gripped workshop mini crane claw, cleanly separated at center)
- **Palette Harmony:** Deep midnight navy (`#1E2233`), warm cream ivory (`#F5F2EA`), vermilion-coral (`#E2582C`), and golden amber (`#F0A500`) paired with coral-peach hair and amber-to-magenta sparkling eyes.
- **Silhouette & Outfit Anchors:**
  - Voluminous wavy coral-peach twin-tails, curved top ahoge, two small amber side hairpins on the left bangs (viewer's right), cheeky fang grin.
  - Deep navy twin hair bows featuring a **golden-amber pentagon keystone gem with a subtle vermilion chevron notch on top** and dangling gold diamond-drop charms (consistent front and back).
  - Off-shoulder cropped cream-ivory hoodie with golden-amber hood lining, golden drawstrings, **circular open-ring gold zipper pull**, deep navy shoulder trim, double vermilion-coral sleeve stripes, navy/amber ribbed cuffs, and a **clean ivory hoodie back**.
  - Dark midnight-navy inner crop top, suspender straps with gold buckles, cuffed dark navy cargo shorts with amber pocket trim, and a **left-thigh dark navy strap carrying a golden-amber pentagon keystone + dangling circular open metallic ring** (matched on the same leg in front and back views).
  - Dark navy knee-high socks with amber + vermilion double top stripes and chunky cream/navy/amber/vermilion streetwear sneakers.
- **CI Integration Rule:** Geometric Werkschaft brand motifs (open rings, pentagon keystone, chevron notch, 4-color palette) are integrated organically as streetwear hardware and trims — never pasted as a literal dark rounded-square app-icon badge.

## Pose Families & Fallback Matrix

Packs must provide **Family A** and may provide **W**, **B**, **C**, and **D** for full expressiveness. Every optional family degrades gracefully when omitted.

| Family | Stage 1 Reference | Articulated Parts & Sockets | Requirement & Fallback |
|---|---|---|---|
| **A. Standing** | Front / slight-3/4 full-body SD | `tail-l/r` (or back hair), `leg-l/r`, `torso-stand`, `arm-l/r-idle`, swappable arms (`arm-point`, `arm-raised`, optional `arm-headrub`), sockets `socket:hand` and `socket:scruff` | **Required** (baseline for every pack) |
| **W. Side-Walk** | 3/4-side walking SD (single direction, flipped via `scaleX(±1)`) | `side-tail-back/front`, `side-leg-back/front`, `side-arm-back/front`, `side-torso`, `side-head` (default expression + blink) | **Recommended** (falls back to Family A hop-trot) |
| **B. Typing / Desk** | Seated/leaning-forward keyboard SD | `torso-typing` (forward lean + shoulder-heave pivot), `upper-arm-l/r`, separated **`forearm-l/r` (below-elbow + hand)**, `prop-keyboard` | **Recommended** (falls back to Family A + keyboard prop) |
| **C. Perched / Sit** | Seated lower-body add-on (reuses Family A upper body & head) | `legs-seated` (dangling legs over composer/card edges) | **Optional** (falls back to Family A) |
| **D. Dangle (Pick-Up)** | Pulled-collar dangling body add-on (reuses Family A head) | `torso-dangle` (collar pulled upward) + flailing/kicking `arm-dangle`, `leg-dangle` | **Optional** (falls back to Family A + `arm-raised` velocity pendulum) |

### Horizontal Flip Rule (`scaleX(±1)`)
- Author `W. Side-Walk` facing one direction only (e.g., right) and mirror horizontally when walking left.
- Keep speech bubbles, comic text (`!?`, `Ouch!`, `Pop!`), badges, and debug pins **outside** the flipped rig container (or counter-flipped via `scaleX(var(--facing))`) so typography never mirrors.

## Engine-Shared Interactive Props

Engine-shared props are shipped once with the engine so every character pack gets Werkschaft-native choreography without duplicating prop art. Individual packs may still override any prop skin.

| Prop ID | Visual Design | Runtime Behavior |
|---|---|---|
| **`prop-drone`** | Unified chibi subagent mini-drone (compact propeller/thruster + glowing visor eye; LED tint auto-mapped to subagent harness/tier) | Spawns in `socket:hand` on `agent.spawn`, tossed along an upward Bezier arc into the `hud-roster` landmark with a pop-in burst (`Pop!`). On subagent completion, pops back out of `hud-roster` to report or drop `prop-mail`. |
| **`prop-mail`** | Crisp SD envelope / paper-plane with sender-type wax seal/stamp tint (child report, peer lead, system nudge) | Streaks in from the edge opposite the companion's facing direction to bonk the back of her head (2-frame hitstop + recoil squash + bounce). Burst coalescing: multiple mails arriving within ~1 s merge into a rapid tap / count badge (`×N`) rather than queuing long stalls. |
| **`prop-grab`** | Pick-up prop set (workshop mini crane claw, pinched gloved hand, or clothespin; open / gripped / release frames) | Latches onto `socket:scruff` when the user drags the companion (`drag_dangle`) and releases with a crisp snap/puff on drop. |

## Always-On Passive Layers

### Gaze / Look-At Priority Stack
Eye tracking is an always-on passive layer rather than a one-off scenario:
1. **Event override (short-lived):** Flying `prop-drone` arc, incoming `prop-mail`, or `attention_point` target landmark.
2. **Active work target:** Typing caret position inside the composer (`perched_watch`), horizontal code-scan sweep (`working_tatata`).
3. **Ambient default — live cursor tracking:** Smooth spring-damped pupils plus subtle head micro-tilt toward the mouse pointer, blended with natural blink cadence and occasional brief idle glances so gaze feels organic rather than robotic.

### Long-Idle Substate (`idle_doze`)
- During extended inactivity on `perched_watch` or `idle_roam`, lower the sliding half-lids smoothly (`lid: 0.85 -> 1.0`) with gentle nodding and `zZZ` comic FX — zero extra static art required.

## Canonical v1 Scenario Catalog (12 Scenarios)

| # | Scenario ID | Trigger (Local / Sidecar) | Pose / Prop | Expression & Gaze | Motion & FX |
|---|---|---|---|---|---|
| 1 | `idle_roam` | Idle / landmark travel | **W (`scaleX(±1)`) while moving <-> A when stopped** | `neutral_smug` · half-lids · travel direction -> cursor | 3/4-side walk/hop locomotion, front landing squash, 2nd-order tail spring lag |
| 2 | `perched_watch` | User typing in composer / extended idle | **C (Sit)** / A | Curious / half-lids · **typing caret `look-at`** | Perched on `composer-top`, dangling legs, subtle rhythm nods; transitions to `idle_doze` on long idle |
| 3 | `working_tatata` | Streaming / tool call / active workers | **B (Typing)** | `focused` · **left-right code-scan gaze** | **Alternating below-elbow `forearm-l/r` strike + shoulder-heave bob** + keyboard sparks |
| 4 | `thinking_ponder` | Lead / worker thinking | **B** or **A** | Half-lidded ponder · upward eye roll | Chin-rest / slow sway + `…` or `?` FX |
| 5 | `attention_point` | Open whiteboard ask / approval prompt | **W travel -> A + `arm-point`** | `neutral_smug` · alternate card <-> user gaze | Side-walk beside target landmark, turn front, point directly at card + `!` |
| 6 | `startled_poke` | User click-poke / sudden warning | **A + `arm-raised`** | `flustered` · jittering pinpoint pupils | Hop in place + squash-and-stretch + arm flail + `!?` |
| 7 | `drag_dangle` | Pointer drag & drop | **D (fallback A + `arm-raised`) + `prop-grab` (`socket:scruff`)** | `flustered` / `wince` | Grab prop lifts collar; drag velocity (`vx`) drives leg/tail pendulum; drop snaps to nearest landmark or floor with landing squash |
| 8 | `gloom_fail` | Build/test/turn failure / worker error | **A** (slumped shoulders) | `gloom` · heavy half-lids · slow drift gaze | Torso slump + slow sigh cycle + spinning `🌀` vortex |
| 9 | `celebrate_done` | Commit / merge / test PASS / done | **A + `arm-raised`** | `happy` · crescent/sparkle eyes | Double hop + both arms raised + star/sparkle burst |
| 10 | `subagent_dispatch` | Subagent spawn (`agent.spawn`) | **A (`socket:hand`) + `prop-drone`** | `neutral_smug` · **track drone arc upward** | Brief crouch wind-up holding drone, flick-toss upward -> drone arcs to `hud-roster` and pops in (`Pop!`) |
| 11 | `subagent_return` | Subagent turn completed | **A + `prop-drone`** | `neutral_smug` / `happy` · **track descending drone** | Drone pops out of `hud-roster`, zips down with green/check visor (or drops `prop-mail` chaining into `mailbox_bonk`), nods and docks |
| 12 | `mailbox_bonk` | Incoming mailbox event | **A + `arm-headrub` (fallback `arm-raised`) + `prop-mail`** | **`wince` (`>_<`, fallback `flustered`)** · glance at mail | Envelope streaks into back of head -> 2-frame hitstop + recoil jolt + head-rub + impact star FX (coalesces rapid bursts) |

## Expression Set & Shared-Head Registration

All front-facing families (**A**, **B**, **C**, **D**) share one anchored `head-blank` + `head-bangs-fg` pair so mood swaps produce **zero hair or accessory silhouette jitter**:

- **5 Base Expressions:** `neutral_smug`, `focused`, `flustered`, `gloom`, `happy`.
- **1 Lightweight Overlay:** `wince` (`>_<` ouch eye lines drawn over `head-blank`; falls back to `flustered` if omitted).
- **Side-Walk Head (`W`):** Single lightweight 3/4-side `side-head` (neutral expression + blink) to keep pack authoring lean.

## Same-Canvas Paper-Doll Cutout & Packaging Pipeline

### 1. Same-Canvas Image-to-Image Dissection
Starting from an approved Stage 1 `1:1` SD static reference (`1024–1254 px`), edit the **exact same canvas** for every layer, instructing the generator to erase all other body parts and cleanly inpaint hidden joints/overlaps:
- Back accessories / hair (`tails` or `back-hair`) with rounded top anchor bulbs.
- Lower body (`legs` / `legs-seated`) with cleanly rounded hip tops so skirt motion never exposes flat cuts.
- Core torso (`torso-stand`, `torso-typing`, `torso-dangle`) with shoulder/neck sockets cleanly inpainted and stray sleeve lobes removed.
- Articulated arms (`arm-l/r-idle`, `arm-point`, `arm-raised`, optional `arm-headrub`; for `B. Typing`, separate `upper-arm-l/r` and below-elbow `forearm-l/r`).
- `head-blank` (full hair, accessories, ears, chin outline, cheek blush; completely blank eyes/mouth/brows) plus facial variants aligned to `head-blank`.

### 2. Deterministic Post-Processing & Compression
1. Strip low-alpha fringe noise (`alpha < 12 -> 0`).
2. Split paired limbs along their vertical midline/valley (`tail-l/r`, `arm-l/r`, `leg-l/r`, `forearm-l/r`).
3. Register alternate expression heads to `head-blank` via upper-hair alpha correlation `(dx, dy)` and composite facial detail strictly inside `head-blank`'s `face_skin` flood-fill mask.
4. Extract `head-bangs-fg` by punching `face_skin` and lower jaw outline out of `head-blank`, keeping inner bang strokes intact so live eyes/half-lids slide behind real hand-drawn bangs.
5. Erase static pupil dots from raster expression sockets strictly below the top lash stroke so live pupils move without nicking the lash line.
6. Resize all registered layers to a uniform pack canvas (`440 × 440` or `512 × 512`) and encode lossless-alpha WebP (`cwebp -q 92 -alpha_q 100 -m 6`; prototype baseline was ~110 KB for 13 parts, 2× Retina crisp at `72–236 px`).

### 3. Runtime Bone Stack (Back to Front)
```text
root (ground position, jump arc, facing scaleX, landing squash-and-stretch)
  ├── tail-l / tail-r       (2nd-order spring-damper lagging head & velocity)
  ├── leg-l / leg-r         (hip pivots; walk/hop/sit/dangle velocity pendulum)
  ├── torso                 (waist pivot; breathing, shoulder-heave pivot; swappable by family)
  ├── arm-l / arm-r         (shoulder pivots + forearm-l/r child bones for B. Typing; socket:hand)
  ├── head                  (neck pivot; spring bob & look-at micro-tilt; socket:scruff)
  │     ├── head-blank      (shared base hair/head/skin)
  │     ├── face-raster     (optional expression blush/gloom/sweat inside face mask)
  │     ├── eyes-svg        (sclera + clipped live irises + sliding half-lids / wince)
  │     ├── mouth-layer     (swappable raster/SVG mouth; speech bounce)
  │     ├── head-bangs-fg   (hand-drawn bangs masking upper lid edges)
  │     └── brows-svg       (anime eyebrows drawn over bangs)
  └── prop / comic-fx       (outside flipped root for text; prop-drone, prop-mail, prop-grab, sparks)
```

### 4. Known Live-Rigging Pitfalls (Check During Stage 3 Tuning)
1. **Sclera-to-skin edge bleed under tilted half-lids:** Join the lower/side sclera outline cleanly into the lash bar and pad sliding-lid mask overlap so inner eye corners never show skin erosion.
2. **Interior cutout drop-shadow halo:** Apply paper-doll drop-shadows strictly to outer articulated silhouettes (`tails`, `legs`, `torso`, `arms`, `head-blank`), never to interior facial layers (`mouth`, `face-raster`, `bangs-fg`).
3. **No forced pose contortion:** Use dedicated pose-family swaps (`W. Side-Walk`, `B. Typing`, optional `C`/`D`) rather than twisting upright idle arms/legs into unnatural angles.
4. **Kinetic typing articulation:** Separate below-elbow `forearm-l/r` bones and add subtle torso/shoulder heave plus synchronized keyboard bounce; tune spring curves live.
5. **Flip-safe overlays:** Mount speech bubbles, comic text, and directional icons outside the `scaleX(±1)` container or counter-flip them explicitly.

## Architecture & Repository Hygiene Summary

- **Top-level game-actor stage:** `position: fixed; inset: 0; pointer-events: none` viewport overlay (hitbox `pointer-events: auto`) operating over named dashboard landmarks (`composer-top`, `whiteboard-ask`, `hud-roster`, `chat-edge`, `rail-active`, `viewport-floor`).
- **Two-speed AI:** Local 60 fps 0-token physics/utility/behavior tree + optional delta-cached sidecar narrative director + pluggable lead-only `ws/emotion`.
- **Lead-only opt-in knobs (T0, default `off`):** `dashboard.companion_mode` (`off | visual | full`), `dashboard.companion_director` (`rules | sidecar | lead-tool`), `dashboard.companion_pack` (`auto | <pack-id>`).
- **IP & repo boundary:** The public repository ships **only** the engine, pack schema, IP-clean original default assets, and the reproducible same-canvas cutout pack recipe. Third-party or community packs live strictly outside git in local user packs (`~/.ws/companions/<pack-id>/pack.json`).
