## 1. Components

### CLI Client
lightweight application for submiting workflow and project definitions to a controller and requesting status updates.

#### Submit

#### Status

### Controller

Purpose: Owns orchestration.

Owner: One process.

Lifetime: From process startup until shutdown.

Responsibilities

- admit workflows
- compile work
- schedule work
- record results
- recover after restart

Does NOT Do

- execute Python
- execute R
- copy files directly
- perform customer computations

Collaborates With

Workers
Execution Environments
Persistence
Workflow Compiler

Questions

How should controller failover work?
Should scheduling be event driven?



### Worker

### Worker

Purpose: Executes work assigned by the controller.

Owner: One worker process.

Lifetime: From worker startup until graceful stop, failure, or termination.

Responsibilities

- register with the controller
- maintain a heartbeat
- claim assigned work
- execute one or more work items
- report completion or failure
- stop accepting work when draining

Does NOT Do

- decide workflow dependencies
- decide which work item should run globally
- directly modify controller persistence
- compile workflow definitions

Collaborates With

- Controller
- Work Item Handlers
- Checkpoint Adapters
- Execution Environment

Questions

- Does one worker currently execute only one work item at a time? Yes.
- Who decides when an idle worker exits? The worker requests work, if there is none it exits.  (though we should give a grace period just in case the completed work does create new work atomically.)
- What state survives if the worker process dies? current only attempts and completed failed work. (soon I will have checkpointing images).

#### Claim

#### Execute

#### Report 

#### Repeat (not sure)

#### Heartbeat
### Execution Environment
An execution environment is a combination of Transport, Shell Dialect, Scheduler, and Runtime. 

#### SSH Transport

#### Docker Transport

#### Local Transport

#### Linux Bash Dialect

#### Windows Powershell Dialect

#### Direct Scheduler

#### Slurm Scheduler

#### Local Runtime

#### Singularity Runtime

#### Example: Local Development Environment

#### Example: Dockerize Slurm Test Environmen

#### Example: HPCC

## 2. Controller Objects

## 3. Worker Objects

## 4. Lifecycle of a Work item

### Flow

workflow submitted
→ controller compiles work item
→ work item enters queued state
→ worker claims it
→ attempt begins
→ worker executes
→ worker reports success or failure
→ controller records terminal state

### Queued to Running

Trigger:

A registered worker requests another work item.

Controller behavior:

- finds one eligible queued work item
- records a new attempt
- associates the attempt with the worker session
- moves the work item into running state
- returns the resolved work item and assignment identity

Invariant:

A work item must not be actively owned by two workers at once.

Questions:
### Work-Claim Correctness

1. **Is a claim performed in one database transaction?**

   Yes. The controller performs the complete queued-to-running transition in one database transaction. It validates the worker session, selects a claimable queued work item, creates an attempt, removes the queued record, creates the running assignment, and then commits.

2. **What prevents two workers from claiming the same item?**

   The claim transaction and SQLite's database-locking and isolation behavior prevent duplicate claims.

   Two workers may request work concurrently, but only one transaction can successfully move a particular work item from the queued state to the running state. The correctness guarantee does not primarily depend on an in-process mutex around the `/work/next` handler.

3. **Is the attempt created before or during the claim?**

   The attempt is created during the claim transaction.

   The transaction performs the following sequence:

   ```text
   validate worker and session
       ↓
   verify the session owns no running assignment
       ↓
   select one claimable queued item
       ↓
   create a new attempt
       ↓
   remove the queued record
       ↓
   create the running assignment
       ↓
   commit
   ```

   If the transaction fails, the attempt and running assignment are not committed.

4. **What identity must be included when reporting completion or failure?**

   The report includes:

   ```text
   attempt_id
   work_item_id
   worker_id
   worker_session_id
   ```

   The controller checks that these values match the current running assignment and that the reporting worker session is still active and unexpired. An attempt ID by itself is not sufficient to complete or fail the work.


1. **Where is a work item created?**

   The controller creates work items while compiling a newly submitted workflow or when advancement of a workflow activates a later stage.

2. **What makes it eligible to run?**

   A work item becomes eligible when its workflow stage is active, its dependencies are satisfied, and its resource requirements can be met.

   Work items from an initially active stage may become eligible during workflow admission. Work items for later stages become eligible when the controller determines that their predecessor stages have completed.

   Worker availability does not make a work item eligible. It determines whether an eligible work item can presently be claimed.

3. **Where is it stored while waiting?**

   It is stored persistently in the SQLite `pending_work` table.

4. **How does a worker claim it?**

   A registered worker sends an authenticated HTTP or HTTPS request to:

   `GET /work/next`

   The request includes the worker's live session identity. The controller selects an eligible work item through the persistence layer.

5. **What changes when it becomes running?**

   During the claim operation, the controller atomically removes the work item from `pending_work`, creates its running assignment in `running_work`, associates it with an `attempt_id` and worker session, and then returns the assigned work to the worker.

   The assignment is therefore recorded before the worker begins executing it.

6. **How is success recorded?**

   The worker reports success to:

   `POST /work/complete`

   The controller verifies that the reporting worker still owns the assignment, removes the running assignment, and records the terminal result in `completed_work`, together with its attempt and result evidence.

7. **How is failure recorded?**

   The worker reports failure to:

   `POST /work/fail`

   The controller verifies assignment ownership, removes the running assignment, and records the terminal result in `failed_work`, together with its attempt and failure evidence.

8. **What happens after worker death?**

   When the worker stops sending heartbeats and its session lease expires, the CareTaker marks the worker session dead.

   Work owned by that session is recorded in `abandoned_work` and atomically returned to the pending queue so another worker can claim it. This recovery is subject to a retry ceiling. After the allowed retries are exhausted, another expiration produces a terminal `failed_work` record instead of another requeue.

   Currently, the computation itself restarts because its in-memory execution state is lost. Planned checkpoint images would allow some abandoned work to resume from saved execution state instead.


## 5. Lifecycle of a Worker

claimed work item
→ worker reads work-item type
→ chooses a handler
→ handler performs the operation
→ handler returns success, failure, and evidence

### Question

After the worker receives a work item, how does it decide which code executes it?

### Initial model

The work item contains a type or operation name. The worker dispatches it to
the corresponding implementation.

Examples might include:

- `python_script`
- `asset.materialize`
- `commit_data`
- geospatial operations

## Worker Execution and Dispatch

### 1. What field identifies the work-item type?

The work-item type is identified by the `WorkItem.Type` field.

This field determines which operation the worker will execute.

---

### 2. What function first receives the claimed work item?

The worker lifecycle (`runWorkerLoop`) receives the claimed work item from the controller and calls:

```go
Worker.Run(item)
```

`Worker.Run` is the generic entry point for executing all work items.

---

### 3. How is dispatch implemented?

Dispatch is currently implemented using **switch statements**.

The execution flow is:

```text
runWorkerLoop
    ↓
Worker.Run
    ↓
Worker.runWorkItem
    ↓
switch (item.Type)
    ↓
specific operation handler
```

For trusted Go operations, the worker first builds an `OperationContext` and then performs a second switch to call the appropriate operation.

---

### 4. What function executes a Python script?

Python work items are executed by:

```text
Worker.runPythonScript()
```

implemented in:

```text
cmd/worker/work_python.go
```

---

### 5. What common result type do handlers return?

All handlers return:

```go
(WorkEvidence, error)
```

- `WorkEvidence` describes the successful outputs produced by the operation.
- `error` indicates whether execution failed.

---

### 6. Which layer converts the handler result into `/work/complete` or `/work/fail`?

This responsibility belongs to `runWorkerLoop()`.

The control flow is:

```text
Worker.Run(item)
        │
        ├── success
        │       ↓
        │ ReportWorkComplete(...)
        │
        └── failure
                ↓
          ReportWorkFailed(...)
                ↓
          StopWorker(...)
```

The operation handlers never communicate directly with the controller.

Instead:

- **Operation handlers** perform the work.
- **Worker.Run** dispatches to the correct handler.
- **runWorkerLoop** manages the worker lifecycle and reports outcomes back to the controller.

---

## Architectural Boundary

```text
runWorkerLoop
    owns:
        • worker lifecycle
        • registration
        • heartbeat
        • fetching work
        • reporting completion/failure
        • graceful shutdown

Worker.Run / runWorkItem
    owns:
        • generic execution
        • dispatching work items

Individual handlers
    (runPythonScript, AssetMaterialize, commitData, ...)
    own:
        • operation-specific behavior
```
 
## 6. Execution Environments

## 7. Persistance

## 8. Plugins

## 9. Failure and Recovery

## 10. Checkpointing

## 11. Log

2026-08-04

Today I started to document my own understanding of the GORC system on top the docs/concept which is very verbose.










