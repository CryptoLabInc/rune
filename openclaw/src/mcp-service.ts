import { spawn, type ChildProcess } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { OpenClawPluginApi, OpenClawPluginService } from "openclaw/plugin-sdk";
import { isActive, getConfigPath, getLogsDir, ensureConfigDir, resolveRuneCorePath } from "./config.js";
import { RuneMcpClient } from "./mcp-client.js";

// ============================================================================
// Constants
// ============================================================================

const RUNE_CORE = resolveRuneCorePath();
const MCP_SERVER_SCRIPT = path.join(RUNE_CORE, "mcp", "server", "server.py");
const MAX_RESTART_ATTEMPTS = 3;
const RESTART_DELAY_MS = 2_000;

// ============================================================================
// Shared State
// ============================================================================

let mcpClient: RuneMcpClient | null = null;
let mcpProcess: ChildProcess | null = null;
let mcpLogStream: fs.WriteStream | null = null;
let restartCount = 0;
let stopping = false;
let restartTimer: ReturnType<typeof setTimeout> | null = null;

export function getMcpClient(): RuneMcpClient | null {
  return mcpClient;
}

// ============================================================================
// MCP Service
// ============================================================================

export function createMcpService(api: OpenClawPluginApi): OpenClawPluginService {
  return {
    id: "rune-mcp",

    start: async () => {
      if (!isActive()) {
        api.logger.info("rune-mcp: skipping start — Rune is dormant");
        return;
      }
      await startMcpServer(api);
    },

    stop: async () => {
      await stopMcpServer(api);
    },
  };
}

export async function startMcpServer(api: OpenClawPluginApi): Promise<void> {
  if (mcpClient?.isConnected()) {
    api.logger.info("rune-mcp: server already running");
    return;
  }

  ensureConfigDir();

  // Ensure rune-core exists
  if (!fs.existsSync(MCP_SERVER_SCRIPT)) {
    api.logger.warn(
      `rune-mcp: MCP server script not found at ${MCP_SERVER_SCRIPT}. ` +
        "Ensure rune-core submodule is initialized.",
    );
    return;
  }

  // Ensure Python venv exists
  const venvPython = path.join(RUNE_CORE, ".venv", "bin", "python3");
  if (!fs.existsSync(venvPython)) {
    api.logger.info("rune-mcp: creating Python venv...");
    try {
      const { execFileSync } = await import("node:child_process");
      const requirementsTxt = path.join(RUNE_CORE, "requirements.txt");
      execFileSync("python3", ["-m", "venv", path.join(RUNE_CORE, ".venv")], {
        cwd: RUNE_CORE,
        stdio: "pipe",
        timeout: 30_000,
      });
      execFileSync(venvPython, ["-m", "pip", "install", "--quiet", "--upgrade", "pip"], {
        cwd: RUNE_CORE,
        stdio: "pipe",
        timeout: 60_000,
      });
      if (fs.existsSync(requirementsTxt)) {
        execFileSync(venvPython, ["-m", "pip", "install", "--quiet", "-r", requirementsTxt], {
          cwd: RUNE_CORE,
          stdio: "pipe",
          timeout: 120_000,
        });
      }
    } catch (err) {
      api.logger.warn(`rune-mcp: venv setup failed — ${String(err)}`);
      // Continue anyway; might still work with system Python
    }
  }

  const pythonBin = fs.existsSync(venvPython) ? venvPython : "python3";

  // Start MCP server process
  const logFile = path.join(getLogsDir(), "envector-mcp.log");
  mcpLogStream = fs.createWriteStream(logFile, { flags: "a" });

  const child = spawn(pythonBin, [MCP_SERVER_SCRIPT, "--mode", "stdio"], {
    cwd: RUNE_CORE,
    env: {
      ...process.env,
      ENVECTOR_CONFIG: getConfigPath(),
      ENVECTOR_AUTO_KEY_SETUP: "false",
      PYTHONPATH: path.join(RUNE_CORE, "mcp"),
    },
    stdio: ["pipe", "pipe", "pipe"],
  });

  mcpProcess = child;

  // Pipe stderr to log file
  child.stderr?.pipe(mcpLogStream);

  // Setup MCP client
  const client = new RuneMcpClient(child, api.logger);
  mcpClient = client;

  // Handle crashes with auto-restart (skip if intentionally stopping)
  client.on("exit", (code: number | null) => {
    api.logger.warn(`rune-mcp: process exited with code ${code}`);
    mcpClient = null;
    mcpProcess = null;
    if (mcpLogStream) {
      mcpLogStream.end();
      mcpLogStream = null;
    }

    if (stopping) return;

    if (isActive() && restartCount < MAX_RESTART_ATTEMPTS) {
      restartCount++;
      api.logger.info(`rune-mcp: restarting (attempt ${restartCount}/${MAX_RESTART_ATTEMPTS})...`);
      restartTimer = setTimeout(() => {
        restartTimer = null;
        startMcpServer(api).catch((err) => {
          api.logger.error(`rune-mcp: restart failed — ${String(err)}`);
        });
      }, RESTART_DELAY_MS);
    }
  });

  // Initialize MCP protocol
  try {
    await client.initialize();
    restartCount = 0; // Reset on successful init
    api.logger.info("rune-mcp: server started and initialized");
  } catch (err) {
    api.logger.error(`rune-mcp: initialization failed — ${String(err)}`);
    await stopMcpServer(api);
  }
}

export async function stopMcpServer(api: OpenClawPluginApi): Promise<void> {
  stopping = true;

  if (restartTimer) {
    clearTimeout(restartTimer);
    restartTimer = null;
  }

  if (mcpClient) {
    try {
      await mcpClient.close();
    } catch (err) {
      api.logger.debug?.(`rune-mcp: close error — ${String(err)}`);
    }
    mcpClient = null;
  }

  if (mcpProcess) {
    try {
      mcpProcess.kill("SIGTERM");
    } catch {
      // already dead
    }
    mcpProcess = null;
  }

  if (mcpLogStream) {
    mcpLogStream.end();
    mcpLogStream = null;
  }

  restartCount = 0;
  stopping = false;
  api.logger.info("rune-mcp: server stopped");
}
