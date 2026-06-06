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

> Could the human meaningfully reason about, modify, debug, and reconstruct this system without AI assistance?

---

# Audit Procedure

1. Review:

   * TARGET_STATE.md
   * relevant git diff
   * tests added or modified
   * architectural discussions if available
   * coding-session metrics where available

2. Generate a focused epistemic audit.

3. Ask questions interactively.

4. Force prediction before explanation.

5. Evaluate answers strictly.

6. Produce:

   * rubric scores
   * session metrics
   * observed weaknesses
   * epistemic drift indicators
   * recommendations

7. Save the completed audit into:

   ```text
   epi_ctl/YYYYMMDD.md
   ```

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

# Required Question Categories

The audit MUST contain questions from each category.

## 1. Architecture Comprehension

Examples:

* What is the responsibility of the controller?
* Why does the worker exist as a separate process?
* What invariants does TARGET_STATE.md imply?

Goal:
Determine whether the human maintains a coherent system-level mental model.

---

## 2. Data Flow Tracing

Examples:

* Trace a job from ingestion to completion.
* What state transitions occur during retry?
* Where is idempotency enforced?

Goal:
Determine whether the human can causally trace information through the system.

---

## 3. Failure Prediction

Examples:

* What likely breaks if retry logic moves layers?
* What happens if worker acknowledgements fail?
* What are the consequences of stale state?

Goal:
Measure predictive debugging capability.

---

## 4. Invariant Recognition

Examples:

* What assumptions must remain true for retries to be safe?
* What properties must remain true for queue consistency?
* What conditions prevent duplicate processing?

Goal:
Measure understanding of system correctness conditions.

---

## 5. Reconstruction Ability

Examples:

* Sketch the controller-worker protocol without opening the code.
* Describe the queue lifecycle from memory.
* Approximate the worker execution loop.

Goal:
Measure retained internal compression of the system.

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

| Category                   | Score |
| -------------------------- | ----- |
| Architecture Comprehension | /5    |
| Data Flow Tracing          | /5    |
| Failure Prediction         | /5    |
| Invariant Recognition      | /5    |
| Reconstruction Ability     | /5    |

Total:

```text
/25
```

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

# Epistemic Control Score

Compute:

```text
E = (A + D + F + I + R) - S
```

Where:

* A = Architecture
* D = Data Flow
* F = Failure Prediction
* I = Invariant Recognition
* R = Reconstruction
* S = Surprise Penalty

Maximum:

```text
25
```

Minimum:

```text
-5
```

---

# Interpretation

## 20-25

Strong epistemic control.

Human retains meaningful architectural ownership despite AI assistance.

## 15-19

Moderate control.

System remains understandable but drift risk exists.

## 10-14

Weakening control.

Understanding becoming localized and reactive.

## Below 10

Epistemic instability.

AI acceleration is likely exceeding human integration capacity.

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
