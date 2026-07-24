# Issue Tracker: GitHub

Threadline tasks and PRDs live in GitHub Issues for `monkeylabx/threadline`. Run the `gh` CLI from the repository so it can infer the repository from `git remote`.

## Conventions

- Create: `gh issue create --title "..." --body "..."`
- Read: `gh issue view <number> --comments`
- List: `gh issue list --state open --json number,title,body,labels,comments`
- Comment: `gh issue comment <number> --body "..."`
- Apply or remove labels: `gh issue edit <number> --add-label "..."` or `--remove-label "..."`
- Close: `gh issue close <number> --comment "..."`
- When a skill says to publish to the issue tracker, create a GitHub Issue.
- When a skill says to fetch the relevant ticket, read the corresponding Issue, comments, and labels.

## Pull Requests As A Triage Surface

PRs as a request surface: no.

## Wayfinding

- Map: use a summary Issue with the `wayfinder:map` label.
- Child: prefer GitHub Sub-issues and apply a `wayfinder:<type>` label.
- Blocking: prefer native GitHub Issue Dependencies.
- Claim: assign the Issue to the current driver before starting; this is the task's first write.
- Resolve: record the conclusion and verification evidence, close the Issue, and update the Map's decision record.
