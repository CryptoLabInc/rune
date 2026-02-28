/**
 * Rune Plugin for OpenClaw
 *
 * FHE-encrypted organizational memory for teams.
 * Provides automatic context capture and retrieval through lifecycle hooks,
 * slash commands for manual control, and agent tools for LLM integration.
 *
 * Architecture:
 *   OpenClaw Plugin → MCP Client (stdio) → Python MCP Server → enVector Cloud + Rune-Vault
 */

import type { OpenClawPluginApi } from "openclaw/plugin-sdk";
import { registerRuneCommands } from "./src/commands.js";
import { createMcpService } from "./src/mcp-service.js";
import { registerRuneTools } from "./src/tools.js";
import { registerRuneHooks } from "./src/hooks.js";

const plugin = {
  id: "rune",
  name: "Rune",
  description: "FHE-encrypted organizational memory for teams",
  version: "0.1.1",

  register(api: OpenClawPluginApi) {
    // 1. Slash commands (/rune-status, /rune-configure, /rune-activate, etc.)
    registerRuneCommands(api);

    // 2. MCP server lifecycle (Python process management)
    api.registerService(createMcpService(api));

    // 3. Agent tools (rune_capture, rune_recall)
    registerRuneTools(api);

    // 4. Lifecycle hooks (auto-capture, auto-retrieve, context injection)
    registerRuneHooks(api);

    api.logger.info("rune: plugin registered");
  },
};

export default plugin;
