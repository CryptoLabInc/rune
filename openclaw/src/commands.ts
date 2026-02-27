import { execFileSync } from "node:child_process";
import fs from "node:fs";
import net from "node:net";
import path from "node:path";
import type { OpenClawPluginApi } from "openclaw/plugin-sdk";
import {
  loadRuneConfig,
  writeRuneConfig,
  deleteRuneConfig,
  isActive,
  hasConfig,
  hasRequiredFields,
  maskSecret,
  getConfigPath,
  getRuneDir,
  getLogsDir,
  ensureConfigDir,
  resolveRuneCorePath,
  type RuneConfig,
} from "./config.js";
import { getMcpClient, startMcpServer, stopMcpServer } from "./mcp-service.js";

// ============================================================================
// Paths
// ============================================================================

const RUNE_CORE = resolveRuneCorePath();
const INSTALL_SCRIPT = path.join(RUNE_CORE, "scripts", "install.sh");

// ============================================================================
// Registration
// ============================================================================

export function registerRuneCommands(api: OpenClawPluginApi): void {
  // ── rune-status ──────────────────────────────────────────────────────
  api.registerCommand({
    name: "rune-status",
    description: "Check Rune plugin activation status and infrastructure health",
    acceptsArgs: false,
    handler: () => {
      if (!hasConfig()) {
        return {
          text: [
            "Rune Plugin Status",
            "==================",
            "State: Not configured",
            "",
            "Run /rune-configure to set up Rune.",
          ].join("\n"),
        };
      }

      const config = loadRuneConfig()!;
      const state = config.state === "active" ? "Active" : "Dormant";
      const vaultEndpoint = config.vault?.endpoint || "not set";
      const envectorEndpoint = config.envector?.endpoint || "not set";

      // Infrastructure checks
      const venvPath = path.join(RUNE_CORE, ".venv");
      const venvExists = fs.existsSync(venvPath);

      const logFile = path.join(getLogsDir(), "envector-mcp.log");
      let logStatus = "missing";
      if (fs.existsSync(logFile)) {
        const stat = fs.statSync(logFile);
        const ageMs = Date.now() - stat.mtimeMs;
        const ageHours = ageMs / (1000 * 60 * 60);
        logStatus = ageHours < 24 ? "recent" : `stale (${Math.round(ageHours)}h ago)`;
      }

      const mcpConnected = getMcpClient()?.isConnected() ?? false;

      const lines = [
        "Rune Plugin Status",
        "==================",
        `State: ${state}`,
        "",
        "Configuration:",
        `  ${hasConfig() ? "[ok]" : "[!!]"} Config file: ${getConfigPath()}`,
        `  ${config.vault?.endpoint ? "[ok]" : "[!!]"} Vault Endpoint: ${vaultEndpoint}`,
        `  ${config.envector?.endpoint ? "[ok]" : "[!!]"} enVector: ${envectorEndpoint}`,
        "",
        "Infrastructure:",
        `  ${venvExists ? "[ok]" : "[!!]"} Python venv: ${venvExists ? venvPath : "not found"}`,
        `  ${logStatus !== "missing" ? "[ok]" : "[!!]"} MCP server logs: ${logStatus}`,
        `  ${mcpConnected ? "[ok]" : "[!!]"} MCP client: ${mcpConnected ? "connected" : "disconnected"}`,
      ];

      // Recommendations
      const recommendations: string[] = [];
      if (config.state !== "active") {
        recommendations.push("Run /rune-activate to enable organizational memory");
      }
      if (!venvExists) {
        recommendations.push("Python venv missing — /rune-activate will auto-setup");
      }
      if (!config.vault?.endpoint || !config.vault?.token) {
        recommendations.push("Vault not configured — run /rune-configure to set credentials");
      }
      if (!mcpConnected && config.state === "active") {
        recommendations.push("MCP client not connected — try /rune-activate to restart");
      }

      if (recommendations.length > 0) {
        lines.push("", "Recommendations:");
        for (const r of recommendations) {
          lines.push(`  - ${r}`);
        }
      }

      return { text: lines.join("\n") };
    },
  });

  // ── rune-configure ───────────────────────────────────────────────────
  api.registerCommand({
    name: "rune-configure",
    description: "Configure Rune credentials and set up Python environment",
    acceptsArgs: true,
    handler: (ctx) => {
      // This command provides guidance text; the actual interactive setup
      // happens through the agent (LLM) following these instructions.
      // Plugin commands bypass the LLM, so we provide structured guidance.

      const existing = loadRuneConfig();
      if (existing) {
        return {
          text: [
            "Rune is already configured:",
            `  Vault: ${existing.vault?.endpoint || "not set"}`,
            `  Vault Token: ${existing.vault?.token ? maskSecret(existing.vault.token) : "not set"}`,
            `  enVector: ${existing.envector?.endpoint || "not set"}`,
            `  enVector Key: ${existing.envector?.api_key ? maskSecret(existing.envector.api_key) : "not set"}`,
            `  State: ${existing.state}`,
            "",
            "To reconfigure, run /rune-reset first, then /rune-configure.",
            "To activate with current config, run /rune-activate.",
          ].join("\n"),
        };
      }

      // Parse args for quick inline configuration
      // Format: /rune-configure envector=<endpoint> key=<api_key> vault=<endpoint> token=<token>
      const args = ctx.args?.trim() ?? "";
      if (args) {
        const parsed = parseConfigArgs(args);
        if (parsed) {
          ensureConfigDir();
          const config: RuneConfig = {
            vault: {
              endpoint: parsed.vault_endpoint ?? "",
              token: parsed.vault_token ?? "",
            },
            envector: {
              endpoint: parsed.envector_endpoint ?? "",
              api_key: parsed.envector_api_key ?? "",
            },
            state: "dormant",
            metadata: {
              configVersion: "1.0",
              lastUpdated: new Date().toISOString(),
              installedFrom: RUNE_CORE,
            },
          };
          writeRuneConfig(config);

          const { ok, missing } = hasRequiredFields(config);
          return {
            text: [
              "Rune configured!",
              `  Config: ${getConfigPath()}`,
              `  State: dormant`,
              "",
              ok
                ? "All fields set. Run /rune-activate to validate and enable."
                : `Missing: ${missing.join(", ")}. Rune will start in dormant state.`,
            ].join("\n"),
          };
        }
      }

      return {
        text: [
          "Rune Configuration",
          "==================",
          "",
          "Quick setup (all-in-one):",
          "  /rune-configure envector=<endpoint> key=<api_key> vault=<endpoint> token=<token>",
          "",
          "Required:",
          "  envector  - enVector cluster endpoint (e.g. cluster-xxx.envector.io)",
          "  key       - enVector API key (e.g. envector_xxx)",
          "",
          "Optional (for secure search):",
          "  vault     - Vault gRPC endpoint (e.g. tcp://vault-TEAM.oci.envector.io:50051)",
          "  token     - Vault authentication token (e.g. evt_xxx)",
          "",
          "Example:",
          "  /rune-configure envector=runestone-demo.clusters.envector.io key=envector_abc123 vault=tcp://vault.example.io:50051 token=evt_xyz789",
          "",
          "After configuration, run /rune-activate to validate and enable.",
        ].join("\n"),
      };
    },
  });

  // ── rune-activate ────────────────────────────────────────────────────
  api.registerCommand({
    name: "rune-activate",
    description: "Validate infrastructure and activate Rune organizational memory",
    acceptsArgs: false,
    handler: async () => {
      if (!hasConfig()) {
        return { text: "Not configured. Run /rune-configure first." };
      }

      const config = loadRuneConfig()!;
      const { ok, missing } = hasRequiredFields(config);
      if (!ok) {
        return {
          text: `Missing required fields: ${missing.join(", ")}.\nRun /rune-configure to set them.`,
        };
      }

      // Check Python venv
      const venvPython = path.join(RUNE_CORE, ".venv", "bin", "python3");
      if (!fs.existsSync(venvPython)) {
        // Try to auto-setup
        if (fs.existsSync(INSTALL_SCRIPT)) {
          try {
            execFileSync("bash", [INSTALL_SCRIPT], {
              cwd: RUNE_CORE,
              stdio: "pipe",
              timeout: 120_000,
            });
          } catch (err) {
            return {
              text: `Python environment setup failed:\n${String(err)}\nPlease install Python 3.12+ and retry.`,
            };
          }
        } else {
          return {
            text: "Python venv not found and install script missing.\nEnsure rune-core submodule is initialized.",
          };
        }
      }

      // Check MCP import
      try {
        execFileSync(venvPython, ["-c", "import mcp"], { stdio: "pipe", timeout: 10_000 });
      } catch {
        return {
          text: "MCP library not importable in Python venv.\nTry deleting rune-core/.venv and running /rune-activate again.",
        };
      }

      // Validate Vault connectivity
      const vaultEndpoint = config.vault.endpoint;
      let vaultOk = false;
      let vaultError = "";

      try {
        if (vaultEndpoint.startsWith("http://") || vaultEndpoint.startsWith("https://")) {
          const healthUrl = new URL("/health", vaultEndpoint).href;
          const res = await fetch(healthUrl, { signal: AbortSignal.timeout(10_000) });
          if (res.ok) vaultOk = true;
          else vaultError = `Vault health returned ${res.status}`;
        } else {
          // tcp:// or plain host:port
          const cleaned = vaultEndpoint.replace(/^tcp:\/\//, "");
          const [host, portStr] = cleaned.split(":");
          const port = Number(portStr);
          if (host && port > 0) {
            await new Promise<void>((resolve, reject) => {
              const sock = net.connect({ host, port, timeout: 5_000 }, () => {
                sock.destroy();
                resolve();
              });
              sock.on("error", reject);
              sock.on("timeout", () => { sock.destroy(); reject(new Error("connection timed out")); });
            });
            vaultOk = true;
          } else {
            vaultError = "Invalid Vault endpoint format";
          }
        }
      } catch (err) {
        vaultError = `Vault connectivity check failed: ${String(err).slice(0, 200)}`;
      }

      if (!vaultOk) {
        // Stay dormant
        return {
          text: [
            "Vault validation failed:",
            `  ${vaultError}`,
            "",
            `  Endpoint: ${vaultEndpoint}`,
            "  State remains dormant.",
            "",
            "Check the endpoint and retry with /rune-activate.",
            "Run /rune-status for more details.",
          ].join("\n"),
        };
      }

      // All checks passed — activate
      config.state = "active";
      config.metadata = {
        ...config.metadata,
        configVersion: config.metadata?.configVersion ?? "1.0",
        lastUpdated: new Date().toISOString(),
      };
      writeRuneConfig(config);

      // Start MCP server
      await startMcpServer(api);

      // Try reload_pipelines via MCP
      const client = getMcpClient();
      let reloadMsg = "";
      if (client?.isConnected()) {
        try {
          await client.callTool("reload_pipelines", {});
          reloadMsg = "MCP pipelines reloaded.";
        } catch (err) {
          reloadMsg = `MCP pipeline reload failed: ${String(err)}. MCP server may need restart.`;
        }
      } else {
        reloadMsg = "MCP client not connected. Pipelines will initialize on next server start.";
      }

      return {
        text: [
          "Rune activated. Organizational memory is now online.",
          "",
          `  Vault: ${config.vault.endpoint}`,
          `  enVector: ${config.envector.endpoint}`,
          `  ${reloadMsg}`,
        ].join("\n"),
      };
    },
  });

  // ── rune-memorize ────────────────────────────────────────────────────
  api.registerCommand({
    name: "rune-memorize",
    description: "Store organizational context to encrypted memory",
    acceptsArgs: true,
    handler: async (ctx) => {
      if (!isActive()) {
        return { text: "Rune is dormant. Run /rune-configure and /rune-activate first." };
      }

      const text = ctx.args?.trim() ?? "";
      if (!text) {
        return { text: "Usage: /rune-memorize <context to store>\nExample: /rune-memorize We chose PostgreSQL for ACID guarantees" };
      }

      const client = getMcpClient();
      if (!client?.isConnected()) {
        return { text: "MCP server not connected. Try /rune-activate to restart." };
      }

      try {
        const result = await client.callTool("capture", {
          text,
          source: "openclaw_command",
          channel: ctx.channel ?? "cli",
        });

        const content = result.content
          ?.filter((c) => c.type === "text")
          .map((c) => c.text)
          .join("\n");

        return { text: content || "Context stored successfully." };
      } catch (err) {
        return { text: `Failed to store context: ${String(err)}` };
      }
    },
  });

  // ── rune-recall ──────────────────────────────────────────────────────
  api.registerCommand({
    name: "rune-recall",
    description: "Search organizational memory for past decisions and context",
    acceptsArgs: true,
    handler: async (ctx) => {
      if (!isActive()) {
        return { text: "Rune is dormant. Run /rune-configure and /rune-activate first." };
      }

      const query = ctx.args?.trim() ?? "";
      if (!query) {
        return { text: "Usage: /rune-recall <search query>\nExample: /rune-recall Why did we choose PostgreSQL?" };
      }

      const client = getMcpClient();
      if (!client?.isConnected()) {
        return { text: "MCP server not connected. Try /rune-activate to restart." };
      }

      try {
        const result = await client.callTool("recall", { query, topk: 5 });

        const content = result.content
          ?.filter((c) => c.type === "text")
          .map((c) => c.text)
          .join("\n");

        return { text: content || "No relevant results found." };
      } catch (err) {
        return { text: `Search failed: ${String(err)}` };
      }
    },
  });

  // ── rune-reset ───────────────────────────────────────────────────────
  api.registerCommand({
    name: "rune-reset",
    description: "Clear Rune configuration and return to dormant state",
    acceptsArgs: false,
    handler: async () => {
      if (!hasConfig()) {
        return { text: "Nothing to reset. No configuration exists." };
      }

      // Stop MCP if running
      await stopMcpServer(api);

      // Delete config
      deleteRuneConfig();

      return {
        text: "Configuration cleared. Run /rune-configure to set up again.",
      };
    },
  });

  // ── rune-deactivate ──────────────────────────────────────────────────
  api.registerCommand({
    name: "rune-deactivate",
    description: "Pause Rune organizational memory without clearing configuration",
    acceptsArgs: false,
    handler: async () => {
      if (!hasConfig()) {
        return { text: "Nothing to deactivate. No configuration exists." };
      }

      const config = loadRuneConfig()!;
      if (config.state === "dormant") {
        return { text: "Rune is already dormant." };
      }

      // Update state
      config.state = "dormant";
      config.metadata = {
        ...config.metadata,
        configVersion: config.metadata?.configVersion ?? "1.0",
        lastUpdated: new Date().toISOString(),
      };
      writeRuneConfig(config);

      // Try reload_pipelines to disable immediately
      const client = getMcpClient();
      let reloadMsg = "";
      if (client?.isConnected()) {
        try {
          await client.callTool("reload_pipelines", {});
          reloadMsg = "MCP pipelines disabled.";
        } catch {
          reloadMsg = "MCP pipelines will remain live until session restart.";
        }
      }

      // Stop MCP server
      await stopMcpServer(api);

      return {
        text: [
          "Rune deactivated. Organizational memory is paused.",
          `Config preserved — /rune-activate to resume.`,
          reloadMsg ? `  ${reloadMsg}` : "",
        ]
          .filter(Boolean)
          .join("\n"),
      };
    },
  });
}

// ============================================================================
// Helpers
// ============================================================================

function parseConfigArgs(
  args: string,
): {
  envector_endpoint?: string;
  envector_api_key?: string;
  vault_endpoint?: string;
  vault_token?: string;
} | null {
  const result: Record<string, string> = {};
  // Parse key=value pairs
  const regex = /(\w+)=(\S+)/g;
  let match: RegExpExecArray | null;
  while ((match = regex.exec(args)) !== null) {
    const [, key, value] = match;
    result[key] = value;
  }

  if (Object.keys(result).length === 0) return null;

  // Normalize vault endpoint — auto-prepend tcp:// if no scheme
  let vaultEndpoint = result.vault;
  if (vaultEndpoint && !/^(tcp|http|https):\/\//.test(vaultEndpoint)) {
    vaultEndpoint = `tcp://${vaultEndpoint}`;
  }

  return {
    envector_endpoint: result.envector,
    envector_api_key: result.key,
    vault_endpoint: vaultEndpoint,
    vault_token: result.token,
  };
}
