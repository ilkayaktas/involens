# Commit Conventions

## Git Safety

- **Never run `git commit` without explicit user approval.** Propose the commit
  message and the list of staged files, then wait for confirmation.
- **Never run `git push` without explicit user approval.** After committing, show
  the commit hash and ask before pushing.
- Run git add for files that are part of recent implementation, but **never run `git add .` or `git add -A` without
  user approval** of the file list.
- Never amend a commit that has already been pushed to the remote.
- Never use `--force` or `--no-verify` unless the user explicitly requests it.
- If `gofmt -l .` returns any output, warn the user and suggest formatting before
  committing.

## Commit Workflow

1. Run `git status` and `git diff` to summarise all staged and unstaged changes.
2. Propose a commit message following the Conventional Commits format below.
3. Wait for explicit user approval of the message and file list before committing.
4. After commit succeeds, show `git log --oneline -1`.
5. Wait for explicit user approval before pushing.
6. Push to the current tracking remote/branch and confirm success.

## Message Format

Every commit message must match:

```
^(feat|build|ci|fix|perf|refactor|test|chore|doc|docs)(\(.*\))?(!)?: (.*)$
```

### Type Reference

| Type       | When to use                                   |
|------------|-----------------------------------------------|
| `feat`     | New feature visible to users                  |
| `fix`      | Bug fix                                       |
| `refactor` | Code change that is neither a feat nor a fix  |
| `perf`     | Performance improvement                       |
| `test`     | Adding or updating tests                      |
| `build`    | Build system or dependency changes            |
| `ci`       | CI/CD configuration changes                   |
| `chore`    | Maintenance tasks (no production code change) |
| `docs`     | Documentation only                            |

### Subject Line Rules

- Use imperative, present tense: "Add feature" not "Added feature"
- Capitalize the first letter after the colon
- No period at the end
- Maximum 70 characters
- Be specific: "Fix nil pointer in Health handler" not "Fix bug"

### Scope

Optional, in parentheses after the type. Use the package or layer name:

- `feat(handler): Add user registration endpoint`
- `fix(mongodb): Handle connection timeout on startup`
- `refactor(config): Extract timeout defaults into constants`
- `test(handler): Add table-driven tests for Health endpoint`
- `build(docker): Add healthcheck to redis service`

### Body Guidelines

- Explain **what** and **why**, not how
- Use imperative mood and present tense
- Include motivation for the change
- Contrast with previous behaviour when relevant
- Separate from subject line with a blank line

### Breaking Changes

Append `!` after the scope and add a `BREAKING CHANGE:` footer:

```
feat(config)!: Rename MONGO_URI to MONGODB_URI

BREAKING CHANGE: env var renamed for consistency with upstream driver docs
```

### Examples

Simple:
```
fix(mongodb): Handle nil client on failed connection
```

With body:
```
feat(handler): Add Health endpoint with mongo and redis checks

Return 200 when all dependencies are reachable, 503 when any check
fails. Each check runs with a 2-second context deadline to avoid
blocking the response on a hung connection.
```

Build change:
```
build: Add Dockerfile and docker-compose with mongo and redis
```

Chore:
```
chore: Tidy go.mod after adding redis and mongodb dependencies
```

Docs:
```
docs: Add AGENTS.md with project layout and conventions
```
