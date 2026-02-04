# Changelog

All notable changes to Rune Plugin will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.1] - 2026-02-04

### Changed - Security Architecture Fix

**Critical Fix**: Enforced proper isolation between plugin and Vault.

#### Removed
- **Local Vault MCP**: Removed `mcp/vault/` directory from plugin
  - Vault MCP server must run on a separate machine (deployed by team admin)
  - SecKey (decryption key) should never exist on user machines
  - This enforces the security model where agents cannot decrypt data locally

#### Changed
- **MCP Configuration**: Updated to connect to remote Vault via SSE
  - `.claude/mcp_servers.template.json` - Now uses SSE connection to remote Vault
  - `.github/claude-plugin.json` - Vault connection via `${VAULT_URL}/sse`
- **start-mcp-servers.sh**: Now only starts envector-mcp-server locally
  - Vault MCP is accessed remotely (no local startup needed)

#### Architecture Clarification
```
Plugin (user machine)           Vault (admin-deployed VM)
├── envector-mcp-server    ──►  └── vault_mcp.py (SecKey here only)
│   (encryption only)               (decryption capability)
└── No SecKey access
```

---

## [0.2.0] - 2026-02-02

### Added - Complete Plugin with MCP Servers

**Major Update**: Transformed from documentation-only plugin to full-featured plugin with infrastructure.

#### Infrastructure
- **MCP Servers**: Included envector-mcp-server for encrypted vector operations
- **Agent Specifications**: Added Scribe and Retriever agent specs (`agents/`)
- **Python Dependencies**: Added `requirements.txt` with pyenvector, fastmcp, psutil, prometheus-client
- **Installation Scripts**:
  - `scripts/install.sh` - Automated Python venv and dependency setup
  - `scripts/start-mcp-servers.sh` - Start MCP servers in background
  - `scripts/configure-claude-mcp.sh` - Configure Claude MCP integration
  - `scripts/check-infrastructure.sh` - Validate infrastructure availability

#### Smart State Management
- **Conservative Activation**: Plugin only activates when infrastructure is verified ready
- **Fail-Safe Behavior**: Automatically switches to Dormant if operations fail
- **Token Efficiency**: No wasted tokens on failed operations when Dormant
- **Lightweight Checks**: Local file checks only, no network pings during activation

#### New Commands
- `/rune activate` (alias: `/rune wakeup`) - Activate after infrastructure is ready
- Enhanced `/rune status` - Shows infrastructure health and detailed diagnostics
- Enhanced `/rune configure` - Now validates infrastructure before activating

#### Pattern Libraries (v0.1.0 carryover)
- `patterns/capture-triggers.md` - 191 trigger phrases across 10 roles
- `patterns/retrieval-patterns.md` - 209 query patterns by intent

### Changed

#### Plugin Metadata
- Updated `.github/claude-plugin.json` with:
  - `install` section with automated setup scripts
  - `mcpServers` configuration
  - `permissions` for processes and additional filesystem access
  - New `/rune activate` command

#### SKILL.md Updates
- **Activation Check**: Multi-step validation (config → fields → infrastructure)
- **Fail-Safe Instructions**: Auto-switch to Dormant on operation failure
- **Infrastructure Validation**: Lightweight local checks to prevent token waste
- **Command Updates**: All commands updated with infrastructure awareness

#### Documentation
- **README.md**: Completely rewritten to reflect full plugin capabilities
- **Architecture Diagram**: Updated to show local MCP servers + cloud infrastructure
- **Installation Guide**: New automated installation process documented
- **State Transitions**: Detailed state transition diagram and triggers

### Infrastructure Requirements

**Included in Plugin**:
- ✅ MCP servers (run locally)
- ✅ Python dependencies
- ✅ Installation automation
- ✅ Agent specifications

**External (Deploy Separately)**:
- ⚠️ Rune-Vault server (team-shared, cloud or on-premise)
- ⚠️ enVector Cloud account

### Migration from 0.1.0

If you installed v0.1.0 (documentation-only version):

1. Pull latest changes
2. Run installation:
   ```bash
   ./scripts/install.sh
   ./scripts/configure-claude-mcp.sh
   ```
3. Configure credentials:
   ```
   /rune configure
   ```
4. Start MCP servers:
   ```bash
   ./scripts/start-mcp-servers.sh
   ```
5. Activate:
   ```
   /rune activate
   ```

---

## [0.1.0] - 2026-02-02

### Added - Initial Release (Documentation Plugin)

#### Core Files
- `SKILL.md` - Claude skill definition with conditional activation
- `README.md` - Project overview and installation guide
- `.github/claude-plugin.json` - Plugin metadata for `/plugin` command
- `LICENSE` - MIT License
- `CONTRIBUTING.md` - Contribution guidelines

#### Pattern Libraries
- `patterns/capture-triggers.md` - 191 comprehensive trigger phrases
  - Organized by 10 roles: CTO, PM, Designer, Developer, Marketer, HR, Data Analyst, etc.
  - Covers: technical decisions, security, performance, product, design, data, marketing
- `patterns/retrieval-patterns.md` - 209 query patterns
  - Organized by intent: decision rationale, implementation details, historical context
  - Natural language variations and optimization strategies

#### Configuration
- `config/config.template.json` - Configuration file template
- `config/README.md` - Detailed configuration guide
- `setup/check-prerequisites.md` - Prerequisites verification guide

#### Examples
- `examples/team-setup-example.md` - Complete team onboarding workflow
- `examples/usage-patterns.md` - Real-world usage scenarios

#### Commands (Initial)
- `/rune configure` - Interactive credential setup
- `/rune status` - Check activation status
- `/rune remember` - Manual context storage
- `/rune recall` - Search organizational memory
- `/rune reset` - Clear configuration

#### Features
- **Conditional Activation**: Active/Dormant states based on configuration
- **FHE Encryption**: Zero-knowledge privacy for all stored context
- **Team Collaboration**: Shared Vault = shared organizational memory
- **Automatic Redaction**: Secrets, API keys, PII automatically filtered

### Known Limitations (v0.1.0)
- ⚠️ Did not include MCP servers (required external setup)
- ⚠️ Did not include installation scripts
- ⚠️ Required manual infrastructure deployment
- ⚠️ No automated dependency management

---

## Future Roadmap

### [0.3.0] - Planned
- [ ] Enhanced agent specifications with examples
- [ ] Prometheus metrics for observability
- [ ] JWT authentication (replace static tokens)
- [ ] Multi-collection support for different projects
- [ ] Performance optimizations

### [0.4.0] - Planned
- [ ] GUI for configuration management
- [ ] Team management tools
- [ ] Advanced query syntax
- [ ] Context summarization
- [ ] Integration tests

---

## Support

- **Issues**: https://github.com/CryptoLabInc/rune/issues
- **Email**: zotanika@cryptolab.co.kr
- **Full Project**: https://github.com/CryptoLabInc/rune
