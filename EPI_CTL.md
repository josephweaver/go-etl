# EPISTEMIC CONTROL AUDIT PROTOCOL

After each significant coding session, create a markdown record in:

```text
epi_ctl/YYYYMMDD.md
```

where `YYYYMMDD` is today's date so files sort chronologically.

Example:

```text
epi_ctl/20260602.md
```

---

# Purpose

The goal of this audit is to measure and preserve epistemic control over the codebase while using AI-assisted software development.

The human operator is intentionally using controlled friction:

* design discussions,
* TARGET_STATE.md,
* manual code integration,
* test execution,
* and review cycles

to maintain an internal predictive model of the system.

This audit is intended to empirically assess whether that understanding is actually being preserved.

Be fair, but cut no slack.

Do not inflate scores.
Do not reward vague answers.
Do not assume understanding from confidence.
Do not substitute implementation success for comprehension.

The standard is:

> Could the human meaningfully reason about, modify, debug, navigate, and partially reconstruct this system with ordinary reference to the codebase, but without depending on AI to explain the architecture?

---

# Audit Procedure

1. Before creating a new audit, check whether previous audits contain pending retention reviews.

   Inspect recent `epi_ctl/*.md` files for:

   ```text
   Retention Follow-Up Required: Yes
   ```

   If the current date is near a scheduled review window, perform the retention review before or alongside the new immediate audit.

   Retention reviews are first-class audits and should not be skipped merely because a new coding session occurred.

2. Review:

   * TARGET_STATE.md
   * relevant git diff
   * tests added or modified
   * architectural discussions if available
   * coding-session metrics where available

3. Generate a focused epistemic audit.

4. Ask questions interactively.

5. Force prediction before explanation.

6. Evaluate answers strictly.

7. Produce:

   * rubric scores
   * session metrics
   * observed weaknesses
   * epistemic drift indicators
   * recommendations

8. For same-day audits, determine whether the development slice is significant enough to justify longitudinal follow-up.

   If follow-up is recommended, record:

   ```text
   Retention Follow-Up Required: Yes
   ```

   and include:

   ```text
   Scheduled Reviews:
   - T+3
   - T+14
   - T+180
   ```

   Otherwise record:

   ```text
   Retention Follow-Up Required: No
   ```

9. Save the completed audit.

   Same-day audits use:

   ```text
   epi_ctl/YYYYMMDD.md
   ```

   Retention reviews use the retention-chain file naming rules below.

---

# Core Principle

This audit is NOT measuring:

* syntax memorization
* exact implementation recall
* trivia

It IS measuring:

* predictive understanding
* causal reasoning
* architectural coherence
* debugging capability
* reconstruction ability
* operational control

---

# Longitudinal Review Schedule

Epistemic control should be measured at multiple time horizons because immediate understanding and retained understanding are different phenomena.

Use these review checkpoints:

```text
T      = Immediate review (same day)
T+3    = Consolidation review (~3 days later)
T+14   = Retention review (~14 days later)
T+180  = Long-term ownership review (~6 months later)
```

T measures immediate comprehension after development.

T+3 measures consolidation after sleep, reflection, and normal work.

T+14 measures medium-term retention after implementation details begin to decay.

T+180 measures long-term ownership and maintainability.

Retention reviews are the primary evidence for durable ownership.

Immediate reviews measure comprehension.

Delayed reviews measure retention.

Long-term reviews measure ownership.

## Review Philosophy

* Immediate scores are not sufficient evidence of ownership.
* Strong ownership should remain visible after implementation details fade.
* Strategic Understanding and Operational Control are expected to decay more slowly than Implementation Recall.
* Long-term ownership is primarily demonstrated through navigation, modification planning, debugging, and architectural reasoning.

The ultimate goal is not immediate recall.

The ultimate goal is durable ownership.

A human who cannot remember exact implementation details but can successfully navigate, debug, and extend the system months later may still possess strong epistemic control.

## Retention Chain

A retention chain is the linked set of audits that measure the same original development session across time.

When a T audit is created, the auditor must determine whether the development slice is significant enough to justify longitudinal follow-up.

Criteria may include:

* large architectural changes
* new abstractions
* substantial AI-generated implementation
* major refactors
* high surprise penalty
* low Operational Control score
* low Strategic Understanding score
* novel workflows or experiments

If follow-up is recommended, the original audit must record:

```text
Retention Follow-Up Required: Yes
```

and:

```text
Scheduled Reviews:
- T+3
- T+14
- T+180
```

Otherwise record:

```text
Retention Follow-Up Required: No
```

## Retention Review File Names

Original audit:

```text
epi_ctl/20260624.md
```

Follow-up reviews:

```text
epi_ctl/20260624_T3.md
epi_ctl/20260624_T14.md
epi_ctl/20260624_T180.md
```

The date portion always refers to the original development session being measured.

The suffix indicates the retention horizon.

Do not create:

```text
epi_ctl/20260627.md
```

for a T+3 review.

Use:

```text
epi_ctl/20260624_T3.md
```

so all retention reviews remain grouped with the original session.

## Retention Review Behavior

A T+3, T+14, or T+180 review should:

* reference the original audit
* compare scores against prior checkpoints
* explain score changes
* identify what knowledge persisted
* identify what knowledge decayed
* identify whether decay occurred in Strategic Understanding, Operational Control, or Implementation Recall

The goal is not to penalize forgetting.

The goal is to measure which forms of understanding remain durable.

Each retention review should include:

```text
Original Session:
20260624

Current Review:
T+14
```

and a retention trend table:

| Review | SU | OC | IR | E  |
| ------ | -- | -- | -- | -- |
| T      | 17 | 8  | 5  | 28 |
| T+3    | 17 | 8  | 4  | 27 |
| T+14   | 16 | 8  | 2  | 25 |

Interpret changes rather than merely reporting them.

---

# Required Question Categories

The audit MUST contain questions from each category.

## 1. Architecture Comprehension

Examples:

* What is the responsibility of the controller?
* Why does the worker exist as a separate process?
* What invariants does TARGET_STATE.md imply?

Goal:
Determine whether the human maintains a coherent system-level mental model.

Score category:
Strategic Understanding (SU)

---

## 2. Data Flow Tracing

Examples:

* Trace a job from ingestion to completion.
* What state transitions occur during retry?
* Where is idempotency enforced?

Goal:
Determine whether the human can causally trace information through the system.

Score category:
Strategic Understanding (SU)

---

## 3. Failure Prediction

Examples:

* What likely breaks if retry logic moves layers?
* What happens if worker acknowledgements fail?
* What are the consequences of stale state?

Goal:
Measure predictive debugging capability.

Score category:
Strategic Understanding (SU)

---

## 4. Invariant Recognition

Examples:

* What assumptions must remain true for retries to be safe?
* What properties must remain true for queue consistency?
* What conditions prevent duplicate processing?

Goal:
Measure understanding of system correctness conditions.

Score category:
Strategic Understanding (SU)

---

## 5. Reconstruction Ability

Examples:

* Reconstruct the system at the abstraction level needed to modify/debug it.
* Describe the queue lifecycle from memory.
* Approximate the worker execution loop.

Goal:
Measure retained internal compression of the system.

Score category:
Implementation Recall (IR)

---

## 6. Defect Localization / Navigation

Example questions:

* If Slurm job submission behaves strangely, where would you look first, second, and third?
* Which files/components are probably irrelevant?
* What evidence would distinguish scheduler failure from transport failure?

This captures your actual working understanding: knowing the radius of the problem.

Score category:
Operational Control (OC)

---

## 7. Modification Planning

Example questions:

* How would you add KubernetesScheduler?
* What interfaces would need to change?
* Which invariants are at risk?
* What tests would you write first?

This is different from debugging.

Score category:
Operational Control (OC)

---

# Scoring Rubric

Each category receives a score from 0-5.

## 0

No meaningful understanding.

## 1

Extremely shallow understanding.
Answers mostly vague, reactive, or incorrect.

## 2

Partial understanding.
Can identify components but reasoning is weak.

## 3

Operational understanding.
Can generally reason about the system with gaps.

## 4

Strong understanding.
Can predict changes and explain interactions accurately.

## 5

Deep ownership-level understanding.
Can reconstruct, debug, and reason about edge cases confidently.

---

# Rubric Categories

| Strategic Understanding    | Score |
| -------------------------- | ----- |
| Architecture Comprehension | /5    |
| Data Flow Tracing          | /5    |
| Failure Prediction         | /5    |
| Invariant Recognition      | /5    |

| Operational Control              | Score |
| -------------------------------- | ----- |
| Defect Localization / Navigation | /5    |
| Modification Planning            | /5    |

| Implementation Recall    | Score |
| ------------------------ | ----- |
| Reconstruction Ability   | /5    |

Strategic Understanding: /20

Operational Control: /10

Implementation Recall: /5

Surprise Penalty: -/5

Total EC: /35

---

# Surprise Penalty

Estimate how often the human was surprised during:

* testing,
* integration,
* runtime behavior,
* or code review.

Scale:

* 0 = no major surprises
* 5 = system behavior frequently unexpected

This is a penalty term because surprise indicates epistemic gaps.

---

# Interpretation

The primary outputs of this audit are:

```text
SU = Strategic Understanding
OC = Operational Control
IR = Implementation Recall
```

The overall Epistemic Control Score (E) is a useful summary, but should not be interpreted in isolation.

A high overall score can mask important imbalances between strategic understanding, operational control, and implementation recall.

---

# Strategic Understanding (SU)

Measures:

* Architecture Comprehension
* Data Flow Tracing
* Failure Prediction
* Invariant Recognition

Maximum E:

```text
20
```

Interpretation:

## 16-20

Strong strategic understanding.

The human can explain why the system is structured as it is, predict consequences of changes, identify correctness conditions, and reason about failures.

## 11-15

Moderate strategic understanding.

Major architectural concepts are understood, though some causal reasoning gaps remain.

## 6-10

Weak strategic understanding.

Reasoning is increasingly reactive and localized.

The human may understand individual components without understanding the system as a whole.

## 0-5

Architectural understanding is minimal.

The human cannot reliably explain system behavior or predict consequences of changes.

---

# Operational Control (OC)

Measures:

* Defect Localization / Navigation
* Modification Planning

Maximum:

```text
10
```

Interpretation:

Operational Control measures the ability to navigate, debug, and safely change the system using ordinary reference to the codebase.

## 8-10

Strong operational control.

The human can navigate the codebase, localize defects, plan modifications, and operate independently using normal code references.

## 5-7

Moderate operational control.

The human can generally work within the system but may require additional review to safely modify unfamiliar areas.

## 2-4

Weak operational control.

The human knows portions of the system but struggles to navigate, localize, or extend it without significant assistance.

## 0-1

Operational control is minimal.

The human cannot reliably maintain or evolve the system independently.

---

# Implementation Recall (IR)

Measures:

* Reconstruction Ability

Maximum:

```text
5
```

Implementation Recall measures short-term or medium-term recall of implementation details. It is measured because recall affects speed and confidence, but it should not automatically be treated as required for long-term ownership.

Principle:

```text
Low implementation recall may be acceptable when strategic understanding and operational control remain strong.
```

Interpretation:

## 4-5

Strong implementation recall.

The human can reconstruct important implementation details without opening the code and can predict where details live.

## 2-3

Partial implementation recall.

The human remembers the broad shape but must inspect code to recover exact fields, functions, or call paths.

## 0-1

Weak implementation recall.

The human cannot reconstruct implementation details from memory, even when the strategic model may be sound.

---

# Combined Interpretation

The relationship between SU, OC, and IR is often more informative than E.

## High SU, High OC, Low IR

Healthy practical ownership with weak implementation recall.

The human understands the architecture and can operate in the codebase using ordinary references, but does not retain many implementation details from memory. This may be acceptable, especially for boilerplate or generated code.

## High SU, Low OC

Architectural understanding exists, but the human may struggle to operate in the codebase.

The human can reason about design consequences but may need additional navigation practice, debugging exercises, or hands-on implementation experience.

## Low SU, High OC

The human can navigate and patch the system, but may not understand larger design consequences.

This creates risk that local changes violate higher-level invariants.

## Low SU, Low OC

Epistemic drift.

AI-assisted development is likely proceeding faster than understanding can be integrated.

---

# Overall Epistemic Control Score

```text
SU = A + D + F + I
OC = DL + M
IR = R
E = SU + OC + IR - S
```

Where:

```text
A  = Architecture Comprehension
D  = Data Flow Tracing
F  = Failure Prediction
I  = Invariant Recognition
DL = Defect Localization / Navigation
M  = Modification Planning
R  = Reconstruction Ability
S  = Surprise Penalty
```

Maximum:

```text
35
```

Minimum E:

```text
-5
```

Use E for trend analysis over time.

Use SU, OC, and IR for diagnosis and intervention planning.

---

# Longitudinal Metrics

For each review checkpoint, record:

```text
SU = Strategic Understanding
OC = Operational Control
IR = Implementation Recall
E  = Epistemic Control Score
```

Use this table for retention chains:

| Review | SU | OC | IR | E |
| ------ | -- | -- | -- | - |
| T      |    |    |    |   |
| T+3    |    |    |    |   |
| T+14   |    |    |    |   |
| T+180  |    |    |    |   |

Different decay patterns are meaningful.

## High SU + High OC + Falling IR

Interpretation:

Implementation details faded but ownership remains intact.

## High SU + Falling OC

Interpretation:

Architecture remains understood but practical maintainability may be degrading.

## Falling SU + Falling OC

Interpretation:

Potential epistemic drift.

The human may no longer be capable of safely evolving the system.

## Stable SU + Stable OC across long delays

Interpretation:

Evidence of durable ownership.

---

# Additional Required Observations

The audit MUST also record:

## Session Metrics

Collect concrete metrics for the audited development slice. Prefer numbers directly derived from the repository, and clearly label estimates.

Required metrics:

* Approximate coding/review hours
* Files changed
* Lines added
* Lines deleted
* Net line change
* New files created
* Test files added or modified
* Components touched
* Commands/tests run

Recommended additional metrics:

* Number of implementation files touched
* Number of documentation files touched
* Number of test assertions or test cases added, if easy to count
* Number of user-visible design decisions made
* Number of known TODOs or deferred behaviors introduced
* Number of times tests failed before passing, if known
* Number of untracked files included in the audit

Use metrics as context, not as a substitute for the rubric. More lines or more tests do not imply stronger epistemic control. A small change in a critical invariant may deserve more scrutiny than a large mechanical edit.

When metrics cannot be measured from git or shell commands, record them as estimates and explain the basis briefly.

---

## AI Dependency Indicators

* Estimated AI-generated LOC
* Estimated human-authored LOC
* Human-authored tests added
* Number of accepted suggestions without modification

---

## Codex Context And Usage Indicators

Record Codex usage estimates for the audited feature. Use exact values when the environment exposes them; otherwise label them as estimates and explain the basis. When local Codex session files are available, check the session JSONL `token_count` records before falling back to rough estimates.

Required indicators:

* Estimated active conversation/context size at feature end
* Estimated transcript volume, including user, assistant, and tool output
* Estimated cumulative model input tokens processed
* Estimated cumulative model output tokens generated
* Approximate number of EC slices or continuation turns
* Approximate number of shell/tool calls
* Approximate number of patch/edit operations
* Approximate number of focused test runs
* Approximate number of full test-suite runs

Recommended interpretation:

* Note whether usage was low, moderate, or high for the feature size.
* Note whether context volume likely increased epistemic drift risk.
* Note whether future audits should collect exact values during the session instead of reconstructing them afterward.

Backfill source priority:

1. Exact token-count records from local Codex session JSONL files.
2. Exact values exposed by the client UI or logs.
3. Derived counts from session JSONL size, event count, and visible transcript.
4. Git-derived proxies such as commits, files changed, and lines changed.
5. Clearly labeled estimates based on conversation memory.

---

## ActivityWatch Distraction And Context-Switch Metrics

When ActivityWatch is available, record objective attention/activity proxies for the audited local date. Do not over-interpret these as moralized "distraction" scores. Use them as context for epistemic drift risk, especially when architectural vocabulary is changing.

Required metrics:

* ActivityWatch version and hostname.
* Local date and UTC range used for the query.
* Available bucket names used.
* Window-tracked time.
* Not-AFK time.
* AFK time.
* Window event count.
* Distinct app count.
* Window events per tracked active hour.
* Top apps by tracked time, including hours and event counts.
* Top window titles by tracked time, including minutes and event counts.

Recommended interpretation:

* Estimate whether context switching was low, moderate, or high.
* Note whether non-project apps consumed meaningful time.
* Note whether distraction risk came from absence from the task, frequent switching, or external interruption.
* Keep the interpretation separate from the raw metrics.

### ActivityWatch Extraction Notes

ActivityWatch's local API usually runs at:

```text
http://localhost:5600/api/0
```

Useful endpoints:

```text
/info
/buckets/
/buckets/<bucket-id>/events?limit=10000
```

Important gotchas observed on Windows/PowerShell:

* `/api/0/buckets` may return HTTP 308. Use the trailing slash:

  ```text
  /api/0/buckets/
  ```

* `Invoke-RestMethod` may wrap ActivityWatch event payloads in a `value` property for some responses.
* PowerShell timestamp parsing may fail or return null-looking timestamp fields on ActivityWatch event responses. If this happens, use Python's standard `urllib.request` and `json` modules to query and summarize the data.
* Use local-day boundaries converted to UTC. For Eastern time on 2026-06-24, the query range was:

  ```text
  2026-06-24T04:00:00Z to 2026-06-25T04:00:00Z
  ```

Minimal Python approach:

```python
import json
import urllib.request
from datetime import datetime, timedelta, timezone
from zoneinfo import ZoneInfo

base = "http://localhost:5600/api/0"
host = "J-BRAIN"
local_tz = ZoneInfo("America/New_York")
local_start = datetime(2026, 6, 24, 0, 0, 0, tzinfo=local_tz)
local_end = local_start + timedelta(days=1)
start = local_start.astimezone(timezone.utc)
end = local_end.astimezone(timezone.utc)

def get_events(bucket):
    url = f"{base}/buckets/{bucket}/events?limit=10000"
    with urllib.request.urlopen(url, timeout=10) as resp:
        return json.load(resp)
```

Compute event overlap with the UTC range rather than trusting that all events lie wholly inside the target day.

---

## Drift Indicators

* Mismatch between TARGET_STATE.md and implementation
* Areas the human could not explain
* Unpredicted behavior during testing
* Architectural uncertainty

---

## Recommendations

Provide concrete recommendations such as:

* slow development velocity
* increase manual implementation
* improve tests
* rewrite subsystem manually
* update TARGET_STATE.md
* create WHY_STATE.md
* refactor overly opaque logic

Recommendations should prioritize restoring epistemic control over maximizing coding speed.

---

# Retention Experiments

Longitudinal audits are intended to evaluate which interventions improve retention.

Suggested interventions:

* README creation
* WHY_STATE.md documents
* architecture diagrams
* smaller implementation slices
* active recall exercises
* delayed self-quizzing
* manual modification tasks

Use T+3, T+14, and T+180 reviews to determine whether an intervention preserved Strategic Understanding, Operational Control, or Implementation Recall.

Do not treat better immediate recall as sufficient evidence that the intervention improved durable ownership.

---

# Important Constraints

* Be skeptical of overconfidence.
* Reward predictive accuracy, not confidence.
* Prefer specific causal reasoning over broad descriptions.
* Penalize hand-wavy answers.
* If the human cannot explain a subsystem without reading code, score accordingly.
* If the human cannot predict consequences of changes, score accordingly.
* If tests passed but understanding is weak, scores should still remain low.
* Prefer short-answer oral-exam style questions.
* The objective is high-information causal reasoning, not long written responses.
* Evaluate technical reasoning independently from grammar, spelling, and prose quality.
* Prioritize causal correctness, specificity, and predictive reasoning.
The objective is truth, not encouragement.
