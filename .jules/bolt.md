## 2025-06-18 - Avoid committing temporary benchmark and coverage files
**Learning:** Adding test scripts like `benchmark_test.go` and running go test with coverage can create untracked files (`coverage/`) and staged files that pollute the repository. The PR diff ends up including unrelated content, leading to review rejection.
**Action:** Always run `git status` and `git diff --staged` before submitting. Ensure temporary test scripts are removed and untracked files (especially `coverage/`) are not staged.
