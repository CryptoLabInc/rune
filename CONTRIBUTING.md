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
   - Follow existing code style
   - Update documentation
   - Test your changes

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
- Python 3.12
- Node.js 18+ (for TypeScript wrapper)
- Text editor (VS Code recommended)
- An MCP-compatible agent for testing (Claude Code, Codex CLI, etc.)

### Local Development

1. **Clone your fork**
   ```bash
   git clone https://github.com/YOUR-USERNAME/rune.git
   cd rune
   ```

2. **Install locally**
   ```bash
   ./scripts/install.sh
   ```
   This sets up the venv, dependencies, and registers MCP servers in Claude Code/Desktop.

3. **Run tests**
   ```bash
   source .venv/bin/activate
   python -m pytest agents/tests/ -v
   ```

4. **Test changes in an agent**
   - Restart your agent (Claude Code, Codex CLI, etc.)
   - Test plugin functionality with `/rune:status`
   - Verify documentation accuracy

## Documentation

### README.md
- Keep updated with latest features
- Include clear examples
- Update installation instructions if changed

### SKILL.md
- This is what Claude reads
- Be precise with instructions
- Test all commands before documenting
- Include edge cases

### Examples
- Provide realistic scenarios
- Test all example commands
- Update when features change

## Testing

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
   - Test `/rune:memorize` with various inputs
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
├── README.md                # Project overview
├── SKILL.md                 # Claude skill definition
├── CLAUDE.md                # Project guidelines for Claude
├── AGENT_INTEGRATION.md     # Multi-agent setup guide
├── CONTRIBUTING.md          # This file
├── LICENSE                  # Apache License 2.0
├── requirements.txt         # Python dependencies
├── package.json             # TypeScript wrapper metadata (version source)
├── openclaw.plugin.json     # Plugin metadata
├── index.ts                 # TypeScript entry point
├── src/                     # TypeScript wrapper (hooks, commands, tools)
│   ├── config.ts            # Config reader with mtime caching
│   ├── hooks.ts             # Hook definitions
│   ├── commands.ts          # Slash command definitions
│   ├── tools.ts             # Tool definitions
│   ├── mcp-client.ts        # MCP client management
│   └── mcp-service.ts       # MCP service lifecycle
├── .claude-plugin/
│   ├── plugin.json          # Plugin metadata (Claude Code)
│   └── marketplace.json     # Marketplace listing
├── mcp/
│   ├── server/
│   │   └── server.py        # MCP server (stdio transport)
│   └── adapter/             # enVector SDK + Vault adapters
├── agents/
│   ├── common/              # Shared modules
│   │   ├── config.py        # LLMConfig, load/save config
│   │   ├── llm_client.py    # Multi-provider LLM client
│   │   ├── llm_utils.py     # JSON parsing utilities
│   │   ├── embedding_service.py
│   │   └── schemas.py
│   ├── scribe/              # Scribe agent (context capture + webhook server)
│   │   └── handlers/        # Slack, Notion webhook handlers
│   ├── retriever/           # Retriever agent (context recall)
│   └── tests/               # Agent tests (pytest)
├── patterns/                # Capture trigger patterns (en/ko/ja)
├── commands/                # Claude skill command definitions
├── scripts/                 # Install, configure, start scripts
├── config/
│   ├── config.template.json # Config file template
│   └── README.md            # Configuration guide
├── setup/                   # Prerequisites check guide
└── examples/                # Usage examples
```

### Key Files

- **SKILL.md**: Core plugin logic that Claude reads
- **README.md**: User-facing documentation
- **package.json**: Version and TypeScript metadata
- **openclaw.plugin.json**: Plugin metadata
- **mcp/server/server.py**: The MCP server (stdio transport)
- **agents/common/config.py**: `LLMConfig` dataclass and config schema
- **agents/common/llm_client.py**: Multi-provider LLM abstraction
- **config/**: Configuration templates and docs

## Release Process

Maintainers only:

1. Update version in `package.json` and `openclaw.plugin.json`
2. Update CHANGELOG.md
3. Run tests: `python -m pytest agents/tests/ -v`
4. Create git tag: `git tag v0.2.0`
5. Push tag: `git push origin v0.2.0`
6. Create GitHub release with notes

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
