# Git Branch Integration

Design document for automatic branch-per-epic functionality in Ticker.

## Overview

Each epic automatically gets its own git branch. This provides clean isolation of work, natural PR workflows, and clear audit trails of changes per epic.

## How It Works

### Sequential Runs (No Worktree)

1. Epic starts → create and checkout `epic/<id>` branch from current HEAD
2. Agent works → commits land on epic branch
3. Epic completes → attempt merge to main:
   - Clean/trivial conflicts → merge automatically, delete branch
   - Real conflicts → leave branch intact, notify user

### Parallel Runs (Worktrees)

1. Epic starts → worktree created with new `epic/<id>` branch
2. Agent works in worktree → commits land on epic branch
3. Epic completes → attempt merge to main:
   - Clean/trivial conflicts → merge automatically, cleanup worktree and branch
   - Real conflicts → leave worktree and branch, notify user

## Branch Naming

Format: `epic/<epic-id>`

Examples:
- `epic/abc`
- `epic/xyz`

## Merge Behavior

### Automatic Merge Conditions

Merge proceeds automatically when:
- No conflicts
- Trivial conflicts (auto-resolvable by git)

### Manual Merge Required

User notified and branch left intact when:
- Real conflicts requiring manual resolution
- Merge would require decision-making

### Merge Strategy

**Options to decide:**
- Regular merge commit (preserves task commit history)
- Squash merge (clean history, one commit per epic)
- Fast-forward when possible

### Branch Cleanup

After successful merge:
- Delete local branch
- Delete remote branch (if pushed)

## Open Questions

1. **Push behavior** - Should epic branches be pushed to remote during work? After completion? Never?

2. **Merge strategy** - Regular merge vs squash? Squash gives cleaner history but loses granular task commits.

3. **Base branch** - Always branch from `main`? Or from current HEAD? What if user is on a feature branch?

4. **Config options** - Should any of this be configurable?
   ```json
   {
     "git": {
       "branch_per_epic": true,
       "branch_prefix": "epic/",
       "auto_merge": true,
       "merge_strategy": "merge|squash|ff",
       "push_branches": false
     }
   }
   ```

5. **Existing branch** - What if `epic/<id>` already exists? (resumed epic, name collision)
   - Reuse existing branch
   - Error and require manual resolution
   - Create `epic/<id>-2`

6. **Dirty working directory** - What if there are uncommitted changes when epic starts?
   - Error and require clean state
   - Stash changes
   - Commit them to the new branch

## Future Considerations

- PR creation integration (gh CLI)
- Branch protection rule awareness
- Support for custom branch naming patterns
- Integration with CI/CD status checks before merge
