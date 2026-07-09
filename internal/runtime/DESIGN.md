# Runtime Design

## Scope

`internal/runtime` is the workspace-scoped agent state machine.

It owns one turn loop:

1. accept a turn request
2. plan one next step
3. execute that step
4. evaluate the observation
5. decide whether to finish, continue, or replan

This package does not own ingress protocols such as stdin, Telegram, or HTTP.

## Main Objects

### Runtime

`Runtime` is the top-level workspace process object.

It owns:

- workspace SQLite handle
- tool registry
- dispatcher
- single-flight turn lock

It exposes a narrow surface:

- `RunTurn(ctx, text)`
- `WorkspaceDB()`
- `Close()`

### Dispatcher

`Dispatcher` is the event loop coordinator.

It owns the runtime flow modules:

- `Planner`
- `PlanExecutor`
- `Evaluator`

It does not decide business policy itself. It only routes the current event to the correct module and feeds the returned next event back into the loop.

## Event Model

The runtime loop is modeled as explicit state-transition requests.

### Events

- `EvTurnRequested`
  - a new user turn should be planned
- `EvStepScheduled`
  - a step has been added to the current plan and should now be executed
- `EvStepObserved`
  - one tool invocation completed and its observation should now be evaluated
- `EvTrajectoryEvaluationRequested`
  - the current trajectory should be evaluated for complete / continue / abort / replan

### Normal Flow

```text
EvTurnRequested
  -> Planner
  -> EvStepScheduled

EvStepScheduled
  -> PlanExecutor
  -> EvStepObserved

EvStepObserved
  -> StepEvaluator
  -> EvTrajectoryEvaluationRequested

EvTrajectoryEvaluationRequested
  -> TrajectoryEvaluator
  -> nil | EvTurnRequested
```

`nil` means the turn is finished.

## Runtime Modules

### Planner

`planning` is responsible only for choosing the next step.

It has three internal stages:

1. route decision
   - graph route or LLM route
2. step selection
   - choose one next tool and goal
3. argument generation
   - generate schema-valid arguments for the selected tool

Planner does not execute tools and does not judge outcomes.

### PlanExecutor

`execution` is responsible only for running the next runnable step.

It:

- finds the next runnable pending step
- marks it running
- renders step arguments
- invokes the selected tool
- emits `EvStepObserved`

It does not decide whether the trajectory is good or bad.

### Evaluator

`judge` was renamed conceptually to `evaluator`.

It has two layers:

- `StepEvaluator`
  - evaluates one observed step
  - records route transition success/failure
  - either requests replan or requests trajectory evaluation
- `TrajectoryEvaluator`
  - evaluates the whole trajectory
  - internally split into:
    - `TrajectoryEvaluatorJudge`: gets the LLM verdict
    - `TrajectoryDecisionApplier`: applies the verdict to task state

Evaluator does not choose tools.

## Task And Plan Boundaries

### Task

`Task` is the aggregate root for one user turn.

It intentionally keeps the most important turn-local state in one place:

- `UserInput`
- `Plan`
- `Reply`
- `TrajectorySummary`
- `ReplanCount`

This is a pragmatic choice. The code favors easy orchestration over aggressively splitting state into many structs.

`Task` should own high-level turn mutations such as:

- set user input
- append step
- mark step done
- increment replan count
- mark completed / aborted / continuing / replanning
- truncate plan for replanning

### Plan

`Plan` owns the ordered step sequence.

It should own step-sequence operations such as:

- create empty plan
- get last step / last node
- get last done step output
- count steps
- check whether plan has steps
- truncate from a step
- mark step status
- find runnable step

Rule of thumb:

- turn-level mutations belong on `Task`
- step-sequence mutations belong on `Plan`

## State Mutation Rules

To keep orchestration simple without losing control, runtime code should prefer methods over direct field mutation.

Good:

- `task.SetUserInput(...)`
- `task.MarkCompleted(...)`
- `task.MarkReplanning(...)`
- `task.MarkStepDone(...)`
- `plan.LastStep()`
- `plan.TruncateFromStep(...)`

Avoid when possible:

- direct writes to `task.Reply`
- direct writes to `task.TrajectorySummary`
- direct writes to `task.ReplanCount`
- direct slicing of `plan.Steps`
- direct indexing into `plan.Steps[len-1]`

## Design Intent

This runtime is not trying to be a general workflow engine.

The current design optimizes for:

- readable control flow
- explicit state transitions
- low indirection during agent orchestration
- incremental refactoring safety

The design does not optimize for:

- maximal abstraction purity
- highly generic actor frameworks
- concurrency-heavy execution graphs

That tradeoff is intentional.
