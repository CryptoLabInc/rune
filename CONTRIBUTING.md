# Contributing to Rune Plugin

Thank you for your interest in contributing to Rune Plugin! This document provides guidelines for contributing.

## Code of Conduct

Be respectful, collaborative, and constructive in all interactions.

## How to Contribute

### Reporting Issues

Before creating an issue:
1. Check if the issue already exists
2. Collect relevant information:
   - Plugin version
   - Claude version (Code/Desktop)
   - Error messages
   - Steps to reproduce

Create issue at: https://github.com/CryptoLabInc/rune/issues

### Suggesting Features

Feature requests should include:
- **Use case**: What problem does this solve?
- **Proposed solution**: How should it work?
- **Alternatives considered**: What other approaches did you consider?
- **Impact**: Who benefits from this feature?

### Contributing Code

1. **Fork the repository**
   ```bash
   git fork https://github.com/CryptoLabInc/rune.git
   cd rune
   ```

2. **Create a feature branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

3. **Make your changes**
   - Follow existing code style (`gofmt -w` + `go vet ./...` should be clean)
   - Update documentation
   - Test your changes:
     - Tests live alongside the package they cover, named `*_test.go`
     - Run with `go test ./...` (use `-race` before submitting)
   - Pass cross-agent invariant checks (see checklist below)

4. **Commit your changes**
   ```bash
   git commit -m "feat: add your feature description"
   ```

   Commit message format:
   - `feat:` New feature
   - `fix:` Bug fix
   - `docs:` Documentation changes
   - `refactor:` Code refactoring
   - `test:` Test additions/changes
   - `chore:` Maintenance tasks

5. **Push and create PR**
   ```bash
   git push origin feature/your-feature-name
   ```
   Then create a Pull Request on GitHub.

## Development Setup

### Prerequisites
- Git
- Go 1.26.2+ (the toolchain pin is in `go.mod`)
- Text editor (VS Code recommended)
- An MCP-compatible agent for testing (Claude Code, Codex CLI, etc.)

### Local Development

1. **Clone your fork**
   ```bash
   git clone https://github.com/YOUR-USERNAME/rune.git
   cd rune
   ```

2. **Clean up previous installations (if any)**

   If you previously installed a versioned (marketplace) Rune plugin, you must remove the cached version first to avoid conflicts:
   ```bash
   # Remove Rune configuration
   rm -rf ~/.rune

   # Remove cached plugin (Claude Code stores installed plugins here)
   rm -rf ~/.claude/plugins/cache/*/rune
   ```

3. **Install the local plugin in Claude Code**

   Register your local directory as a marketplace source and install from it:
   ```bash
   # Add the local directory as a marketplace
   claude plugin marketplace add ./

   # Install the plugin from the local marketplace
   claude plugin install rune
   ```

   This registers the local `rune/` directory so Claude Code loads your development copy instead of the published version.

4. **Run tests**
   ```bash
   go build ./...
   go vet ./...
   go test -race ./...
   ```

5. **Test changes in an agent**
   - Restart your agent (Claude Code, Codex CLI, etc.)
   - Test plugin functionality with `/rune:status`
   - Verify documentation accuracy

## Documentation

### README.md
- Keep updated with latest features
- Include clear examples
- Update installation instructions if changed

### Examples
- Provide realistic scenarios
- Test all example commands
- Update when features change

## Testing

### Cross-Agent Invariant Checklist

Use this checklist for any integration/setup/runtime change:

- [ ] The Go binary at `cmd/rune-mcp/` remains the single MCP server entry point — no per-agent server process
- [ ] Agent-specific scripts stay thin adapters (registration/wiring only)
- [ ] Codex-only commands (`codex mcp ...`) are clearly separated from cross-agent/common instructions
- [ ] Claude/Gemini/OpenAI instructions do not include Codex-only commands
- [ ] `SKILL.md`, `commands/rune/*.toml`, and `AGENT_INTEGRATION.md` remain consistent on common vs agent-specific boundaries
- [ ] Plugin manifests (`.claude-plugin/plugin.json`, `gemini-extension.json`) point at the same Go binary command path

### Manual Testing Checklist

- [ ] Plugin installs successfully
- [ ] Configuration prompts work correctly
- [ ] Active state enables all features
- [ ] Dormant state shows appropriate messages
- [ ] Commands work as documented
- [ ] Error messages are clear
- [ ] Documentation is accurate

### Test Scenarios

1. **Fresh Install**
   ```bash
   rm -rf ~/.rune ~/.claude/plugins/cache/*/rune
   /plugin install github.com/YOUR-USERNAME/rune
   # Verify installation process
   ```

2. **Configuration**
   ```
   /rune:configure
   # Test with valid credentials
   # Test with invalid credentials
   # Test with partial credentials
   ```

3. **State Transitions**
   - Install (dormant) → Configure (active)
   - Active → Reset (dormant)
   - Active → Reconfigure (active)

4. **Commands**
   - Test `/rune:status` in both states
   - Test `/rune:capture` with various inputs
   - Test `/rune:recall` with various queries
   - Test `/rune:reset` with confirmation

## Pull Request Guidelines

### PR Title Format
```
<type>: <short description>
```

Examples:
- `feat: add support for environment variable configuration`
- `fix: handle missing config file gracefully`
- `docs: improve installation instructions`

### PR Description

Include:
- **What**: What changes were made?
- **Why**: Why were these changes necessary?
- **How**: How do the changes work?
- **Testing**: How were the changes tested?
- **Screenshots**: If UI-related changes

### Review Process

1. Maintainer reviews PR
2. Feedback provided (if needed)
3. Changes requested (if needed)
4. Approval and merge (when ready)

Typical response time: 2-5 business days

## Areas We Need Help

### High Priority

- **Testing**: Test plugin on different platforms (macOS, Linux)
- **Documentation**: Improve clarity, add more examples
- **Error Handling**: Better error messages for common issues
- **Security**: Review credential handling and storage

### Medium Priority

- **Features**: Environment variable support, team management tools
- **Examples**: More real-world usage scenarios
- **Integration**: Support for additional Claude versions

### Low Priority

- **Optimization**: Performance improvements
- **Convenience**: Quality-of-life enhancements
- **Localization**: Multi-language support

## Style Guide

### Markdown
- Use ATX-style headers (`#` not underlines)
- Fence code blocks with language identifiers
- Use `bash` for shell commands
- Use `json` for JSON examples
- Keep line length reasonable (80-100 chars)

### JSON
- 2-space indentation
- No trailing commas
- Use double quotes
- Meaningful key names

### Documentation
- Write in active voice
- Be concise but complete
- Include examples
- Link to related docs

## Project Structure

```
rune/
├── README.md                    # Project overview
├── CLAUDE.md                    # Claude Code project guidelines
├── GEMINI.md                    # Gemini CLI context file
├── CONTRIBUTING.md              # This file
├── LICENSE                      # Apache License 2.0
├── go.mod / go.sum              # Go module pin (toolchain + deps)
├── package.json                 # Package metadata (version source)
├── .claude-plugin/
│   ├── plugin.json              # Claude Code plugin manifest (points at bin/rune-mcp)
│   └── marketplace.json         # Marketplace listing
├── gemini-extension.json        # Gemini CLI extension manifest
├── hooks/
│   └── hooks.json               # Gemini lifecycle hooks
├── cmd/
│   └── rune-mcp/                # MCP server entry point (Go, stdio transport)
├── internal/
│   ├── mcp/                     # tool registration + handler dispatch
│   ├── service/                 # CaptureService / RecallService / LifecycleService
│   ├── lifecycle/               # boot loop + state machine
│   ├── adapters/                # vault / envector / embedder / config / logio gRPC clients
│   ├── domain/                  # schemas + typed errors (Python parity)
│   ├── policy/                  # pure helpers (novelty, rerank, query parse)
│   └── obs/                     # slog handler with sensitive-data redaction
├── agents/
│   ├── claude/                  # Claude agent specs (.md, referenced by plugin.json)
│   │   ├── scribe.md
│   │   └── retriever.md
│   ├── gemini/                  # Gemini agent specs (.md)
│   │   ├── scribe.md
│   │   └── retriever.md
│   └── codex/                   # Codex agent spec
│       └── scribe.md
├── commands/
│   ├── claude/                  # Claude commands (.md format)
│   └── rune/                    # Gemini commands (.toml format)
├── patterns/                    # Capture trigger patterns (en/ko/ja)
├── scripts/
│   ├── dev/v04/                 # dev-only helpers (preflight, etc.)
│   └── ...                      # legacy install scripts (Gemini path — under review)
├── docs/                        # architecture / migration / spec
└── examples/                    # Usage examples
```

### Key Files

- **CLAUDE.md**: Claude Code project guidelines
- **GEMINI.md**: Gemini CLI context file
- **.claude-plugin/plugin.json**: Claude Code plugin manifest (points at `${CLAUDE_PLUGIN_ROOT}/bin/rune-mcp`)
- **gemini-extension.json**: Gemini CLI extension manifest
- **cmd/rune-mcp/main.go**: MCP server entry point — wires services + boot loop + slog
- **internal/lifecycle/boot.go**: Boot loop (Vault dial → EncKey → embedder → envector → Active)
- **internal/service/{capture,recall,lifecycle}.go**: 8 MCP tool handlers

## Release Process

Maintainers only:

1. Update version in `package.json`, `.claude-plugin/plugin.json`, and `gemini-extension.json`
2. Update CHANGELOG.md
3. Run tests: `go test -race ./...`
4. Create git tag: `git tag v0.4.0`
5. Push tag: `git push origin v0.4.0`
6. Create GitHub release with notes (attach platform binaries — see Task #30 for the release pipeline plan)

## Questions?

- **Issues**: https://github.com/CryptoLabInc/rune/issues
- **Email**: zotanika@cryptolab.co.kr
- **Full Project**: https://github.com/CryptoLabInc/rune

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

## Recognition

Contributors will be recognized in:
- README.md contributors section (coming soon)
- Release notes
- GitHub contributors page

Thank you for contributing to Rune Plugin!
