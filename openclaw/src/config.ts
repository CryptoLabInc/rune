import fs from "node:fs";
import path from "node:path";
import os from "node:os";

// ============================================================================
// Types
// ============================================================================

export type RuneConfig = {
  vault: {
    endpoint: string;
    token: string;
  };
  envector: {
    endpoint: string;
    api_key: string;
  };
  state: "active" | "dormant";
  metadata?: {
    configVersion: string;
    lastUpdated: string | null;
    teamId?: string | null;
    installedFrom?: string;
  };
};

// ============================================================================
// Paths
// ============================================================================

const RUNE_DIR = path.join(os.homedir(), ".rune");
const CONFIG_PATH = path.join(RUNE_DIR, "config.json");
const LOGS_DIR = path.join(RUNE_DIR, "logs");

export function getRuneDir(): string {
  return RUNE_DIR;
}

export function getConfigPath(): string {
  return CONFIG_PATH;
}

export function getLogsDir(): string {
  return LOGS_DIR;
}

// ============================================================================
// Config Operations
// ============================================================================

export function ensureConfigDir(): void {
  if (!fs.existsSync(RUNE_DIR)) {
    fs.mkdirSync(RUNE_DIR, { recursive: true, mode: 0o700 });
  }
  if (!fs.existsSync(LOGS_DIR)) {
    fs.mkdirSync(LOGS_DIR, { recursive: true, mode: 0o700 });
  }
}

export function loadRuneConfig(): RuneConfig | null {
  try {
    if (!fs.existsSync(CONFIG_PATH)) {
      return null;
    }
    const raw = fs.readFileSync(CONFIG_PATH, "utf-8");
    return JSON.parse(raw) as RuneConfig;
  } catch {
    return null;
  }
}

export function writeRuneConfig(config: RuneConfig): void {
  ensureConfigDir();
  const json = JSON.stringify(config, null, 2);
  fs.writeFileSync(CONFIG_PATH, json, { encoding: "utf-8", mode: 0o600 });
}

export function deleteRuneConfig(): void {
  try {
    if (fs.existsSync(CONFIG_PATH)) {
      fs.unlinkSync(CONFIG_PATH);
    }
  } catch {
    // ignore
  }
}

export function isActive(): boolean {
  const config = loadRuneConfig();
  return config?.state === "active";
}

export function isDormant(): boolean {
  return !isActive();
}

export function hasConfig(): boolean {
  return fs.existsSync(CONFIG_PATH);
}

export function hasRequiredFields(config: RuneConfig): { ok: boolean; missing: string[] } {
  const missing: string[] = [];
  if (!config.vault?.endpoint) missing.push("vault.endpoint");
  if (!config.vault?.token) missing.push("vault.token");
  if (!config.envector?.endpoint) missing.push("envector.endpoint");
  if (!config.envector?.api_key) missing.push("envector.api_key");
  return { ok: missing.length === 0, missing };
}

export function maskSecret(value: string): string {
  if (!value || value.length < 8) return "***";
  return value.slice(0, 4) + "..." + value.slice(-4);
}

// ============================================================================
// Rune Core Path Resolution
// ============================================================================

/**
 * Resolve the rune-core directory (Python sources: mcp/, agents/, etc.).
 * Checks two locations:
 *   1. npm-installed layout: <plugin>/rune-core/ bundled inside the package
 *   2. Repository layout: parent directory (rune repo root contains mcp/, agents/, etc.)
 */
export function resolveRuneCorePath(): string {
  const pluginRoot = path.resolve(import.meta.dirname ?? __dirname, "..");
  // npm-installed: <plugin>/rune-core/
  const bundled = path.join(pluginRoot, "rune-core");
  if (fs.existsSync(bundled)) return bundled;
  // Repository: parent is the rune repo root (contains mcp/, agents/, scripts/, etc.)
  const repoRoot = path.resolve(pluginRoot, "..");
  return repoRoot;
}
