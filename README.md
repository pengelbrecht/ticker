# Ticker

Autonomous AI agent loop runner for completing epics with Claude Code.

Ticker implements the "Ralph Wiggum technique" - running AI agents in continuous loops until tasks are complete. It orchestrates Claude Code CLI to autonomously work through tasks in an epic, detecting completion signals, managing budgets, and providing checkpointing for long-running sessions.

## Installation

### Quick Install (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/pengelbrecht/ticker/main/scripts/install.sh | sh
```

This script will:
- Detect your OS (Linux, macOS, Windows) and architecture (amd64, arm64)
- Download the latest release from GitHub
- Install to `/usr/local/bin` (or `~/.local/bin` if not writable)
- Skip installation if already up to date

You can also specify a custom install directory:

```bash
TICKER_INSTALL_DIR=/opt/bin curl -fsSL https://raw.githubusercontent.com/pengelbrecht/ticker/main/scripts/install.sh | sh
```

### Manual Installation

Download the appropriate binary from the [releases page](https://github.com/pengelbrecht/ticker/releases) and add it to your PATH.

### Build from Source

```bash
git clone https://github.com/pengelbrecht/ticker.git
cd ticker
go build -o ticker ./cmd/ticker
```

### Upgrading

```bash
ticker upgrade
```

Or re-run the install script.

## Requirements

- [Claude Code CLI](https://github.com/anthropics/claude-code) - The AI agent that performs the actual work
- [Ticks](https://github.com/pengelbrecht/ticks) (`tk`) - Issue tracker CLI for task management

## Usage

### Basic Commands

```bash
# Interactive epic picker (TUI mode)
ticker run

# Run a specific epic
ticker run <epic-id>

# Auto-select next ready epic
ticker run --auto

# Run in headless mode (no TUI)
ticker run <epic-id> --headless

# Resume from a checkpoint
ticker resume <checkpoint-id>

# List checkpoints
ticker checkpoints [epic-id]

# Self-update
ticker upgrade
```

### Budget Controls

```bash
# Set maximum iterations (default: 50)
ticker run <epic-id> -n 100

# Set maximum cost in USD (default: $20)
ticker run <epic-id> --max-cost 50.0

# Set checkpoint interval (default: every 5 iterations)
ticker run <epic-id> --checkpoint-interval 10

# Disable checkpointing
ticker run <epic-id> --checkpoint-interval 0
```

### TUI Controls

When running in TUI mode:

| Key | Action |
|-----|--------|
| `p` | Pause/Resume execution |
| `q` / `Ctrl+C` | Quit |
| `j` / `Down` | Scroll down |
| `k` / `Up` | Scroll up |
| `g` | Scroll to top |
| `G` | Scroll to bottom |

## How It Works

1. **Epic Selection**: Choose an epic to work on (interactively or via `--auto`)
2. **Task Loop**: For each iteration:
   - Get the next task from the epic using `tk next`
   - Build a prompt with task details and acceptance criteria
   - Execute Claude Code CLI with the prompt
   - Parse output for control signals
   - Close completed tasks and continue
3. **Signal Detection**: The agent can emit signals to control flow:
   - `<promise>COMPLETE</promise>` - Task completed successfully
   - `<promise>EJECT: reason</promise>` - Agent needs human intervention
   - `<promise>BLOCKED: reason</promise>` - Task is blocked
4. **Budget Enforcement**: Stops when iteration or cost limits are reached
5. **Checkpointing**: Saves state periodically for resume capability

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success - epic completed |
| 1 | Max iterations reached |
| 2 | Agent ejected (EJECT signal) |
| 3 | Agent blocked (BLOCKED signal) |
| 4 | Error occurred |

## Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `TICKER_INSTALL_DIR` | Custom installation directory for the install script |
| `XDG_CONFIG_HOME` | Config directory for update cache (default: `~/.config`) |

### Checkpoints

Checkpoints are stored in `.ticker/checkpoints/` relative to the working directory. Each checkpoint contains:
- Epic ID and iteration number
- Token usage and cost
- Completed tasks
- Git commit SHA at time of checkpoint

## Development

### Building

```bash
go build -o ticker ./cmd/ticker
```

### Testing

```bash
go test ./...
```

### Release

Releases are automated via GoReleaser on git tag push:

```bash
git tag v1.0.0
git push origin v1.0.0
```

## License

MIT
