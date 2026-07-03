# Contributing with AI

If you're using an AI coding assistant to extend or modify Stackyard, this
project already has the context that tool needs — load these files, in
this order, before making changes:

1. **[`CLAUDE.md`](https://github.com/KamerrEzz/stackyard/blob/main/CLAUDE.md)**
   — project conventions: comment style, what belongs in code vs. in
   documentation, and pointers to the other files below. Start here; it's
   short and tells you where everything else lives.
2. **[`spec.md`](https://github.com/KamerrEzz/stackyard/blob/main/spec.md)**
   — the functional specification: problem statement, goals, and
   acceptance criteria for every feature, module by module.
3. **[`plan.md`](https://github.com/KamerrEzz/stackyard/blob/main/plan.md)**
   — the architecture and technical design backing that spec: how the Go
   backend and React frontend are structured, storage schema, and the
   key technical decisions behind them.
4. **[`tasks.md`](https://github.com/KamerrEzz/stackyard/blob/main/tasks.md)**
   — the full phase-by-phase build history as a checklist. Useful for
   understanding what already exists and in what order it was built.
5. **[`docs/STATE.md`](https://github.com/KamerrEzz/stackyard/blob/main/docs/STATE.md)**
   — a long, detailed running log of every work session: decisions made,
   rationale behind them, and gotchas discovered along the way. This is
   the richest source of **why** things are the way they are anywhere in
   the repo — when `tasks.md` or the code itself doesn't explain a
   choice, `STATE.md` almost certainly does.

For a quick pass, `CLAUDE.md` + `docs/STATE.md` + `tasks.md` alone will
get an AI assistant (or a human) most of the way to full context; add
`spec.md`/`plan.md` when the change touches product scope or
architecture directly.

`docs/STATE.md` is this project's internal development log, distinct from
this public documentation site — it is not meant to be polished reading,
but it is the most honest record of the project's history.
