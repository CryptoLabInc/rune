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
- Text editor (VS Code recommended)
- Claude Code or Claude Desktop for testing

### Local Development

1. **Clone your fork**
   ```bash
   git clone https://github.com/YOUR-USERNAME/rune.git
   cd rune
   ```

2. **Link to Claude**
   ```bash
   # For Claude Code
   ln -s $(pwd) ~/.claude/skills/rune-dev

   # For Claude Desktop
   ln -s $(pwd) ~/Library/Application\ Support/Claude/skills/rune-dev
   ```

3. **Test changes**
   - Restart Claude
   - Test plugin functionality
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
   rm -rf ~/.rune ~/.claude/skills/rune
   /plugin install github.com/YOUR-USERNAME/rune
   # Verify installation process
   ```

2. **Configuration**
   ```
   /rune configure
   # Test with valid credentials
   # Test with invalid credentials
   # Test with partial credentials
   ```

3. **State Transitions**
   - Install (dormant) → Configure (active)
   - Active → Reset (dormant)
   - Active → Reconfigure (active)

4. **Commands**
   - Test `/rune status` in both states
   - Test `/rune remember` with various inputs
   - Test `/rune recall` with various queries
   - Test `/rune reset` with confirmation

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

- **Testing**: Test plugin on different platforms (macOS, Linux, Windows)
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
├── README.md              # Project overview
├── SKILL.md               # Claude skill definition
├── LICENSE                # MIT License
├── CONTRIBUTING.md        # This file
├── .github/
│   └── claude-plugin.json # Plugin metadata
├── setup/
│   └── check-prerequisites.md
├── config/
│   ├── config.template.json
│   └── README.md
└── examples/
    ├── team-setup-example.md
    └── usage-patterns.md
```

### Key Files

- **SKILL.md**: Core plugin logic that Claude reads
- **README.md**: User-facing documentation
- **.github/claude-plugin.json**: Plugin metadata for `/plugin` command
- **config/**: Configuration templates and docs

## Release Process

Maintainers only:

1. Update version in `.github/claude-plugin.json`
2. Update CHANGELOG.md
3. Create git tag: `git tag v0.1.0`
4. Push tag: `git push origin v0.1.0`
5. Create GitHub release with notes

## Questions?

- **Issues**: https://github.com/CryptoLabInc/rune/issues
- **Email**: zotanika@cryptolab.co.kr
- **Full Project**: https://github.com/CryptoLabInc/rune

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

## Recognition

Contributors will be recognized in:
- README.md contributors section (coming soon)
- Release notes
- GitHub contributors page

Thank you for contributing to Rune Plugin!
