import type { ChildProcess } from "node:child_process";
import { EventEmitter } from "node:events";
import readline from "node:readline";
import type { PluginLogger } from "openclaw/plugin-sdk";

// ============================================================================
// Types
// ============================================================================

type JsonRpcRequest = {
  jsonrpc: "2.0";
  id: number;
  method: string;
  params?: Record<string, unknown>;
};

type JsonRpcResponse = {
  jsonrpc: "2.0";
  id: number;
  result?: unknown;
  error?: { code: number; message: string; data?: unknown };
};

export type McpToolResult = {
  content: Array<{ type: string; text: string }>;
  isError?: boolean;
  [key: string]: unknown;
};

// ============================================================================
// MCP Client (JSON-RPC 2.0 over stdio)
// ============================================================================

export class RuneMcpClient extends EventEmitter {
  private process: ChildProcess;
  private logger: PluginLogger;
  private nextId = 1;
  private pending = new Map<number, { resolve: (v: unknown) => void; reject: (e: Error) => void }>();
  private connected = false;
  private rl: readline.Interface | null = null;

  constructor(process: ChildProcess, logger: PluginLogger) {
    super();
    this.process = process;
    this.logger = logger;
    this.setupIo();
  }

  private setupIo(): void {
    if (!this.process.stdout) {
      this.logger.error("rune-mcp: process stdout not available");
      return;
    }

    this.rl = readline.createInterface({ input: this.process.stdout });
    this.rl.on("line", (line) => {
      this.handleLine(line);
    });

    this.process.on("exit", (code) => {
      this.connected = false;
      this.rejectAllPending(new Error(`MCP process exited with code ${code}`));
      this.emit("exit", code);
    });

    this.process.on("error", (err) => {
      this.connected = false;
      this.rejectAllPending(err);
      this.emit("error", err);
    });

    this.connected = true;
  }

  private handleLine(line: string): void {
    const trimmed = line.trim();
    if (!trimmed) return;

    try {
      const msg = JSON.parse(trimmed) as JsonRpcResponse;
      if (msg.id != null && this.pending.has(msg.id)) {
        const entry = this.pending.get(msg.id)!;
        this.pending.delete(msg.id);
        if (msg.error) {
          entry.reject(new Error(`MCP error ${msg.error.code}: ${msg.error.message}`));
        } else {
          entry.resolve(msg.result);
        }
      }
    } catch {
      this.logger.debug?.(`rune-mcp: non-JSON line from server: ${trimmed.slice(0, 100)}`);
    }
  }

  private rejectAllPending(err: Error): void {
    for (const [, entry] of this.pending) {
      entry.reject(err);
    }
    this.pending.clear();
  }

  private send(request: JsonRpcRequest): void {
    if (!this.process.stdin || !this.connected) {
      throw new Error("MCP process not connected");
    }
    const json = JSON.stringify(request);
    this.process.stdin.write(json + "\n");
  }

  async initialize(): Promise<void> {
    const result = await this.request("initialize", {
      protocolVersion: "2024-11-05",
      capabilities: {},
      clientInfo: { name: "openclaw-rune", version: "0.1.0" },
    });
    this.logger.info(`rune-mcp: initialized — ${JSON.stringify(result)}`);

    // Send initialized notification (no id = notification)
    if (this.process.stdin && this.connected) {
      const notification = JSON.stringify({
        jsonrpc: "2.0",
        method: "notifications/initialized",
      });
      this.process.stdin.write(notification + "\n");
    }
  }

  private async request(method: string, params?: Record<string, unknown>): Promise<unknown> {
    const id = this.nextId++;
    const request: JsonRpcRequest = { jsonrpc: "2.0", id, method, params };

    return new Promise<unknown>((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`MCP request ${method} timed out after 30s`));
      }, 30_000);

      this.pending.set(id, {
        resolve: (v) => {
          clearTimeout(timeout);
          resolve(v);
        },
        reject: (e) => {
          clearTimeout(timeout);
          reject(e);
        },
      });

      try {
        this.send(request);
      } catch (err) {
        clearTimeout(timeout);
        this.pending.delete(id);
        reject(err);
      }
    });
  }

  async callTool(name: string, args: Record<string, unknown>): Promise<McpToolResult> {
    const result = (await this.request("tools/call", { name, arguments: args })) as McpToolResult;
    return result;
  }

  async listTools(): Promise<unknown> {
    return this.request("tools/list", {});
  }

  isConnected(): boolean {
    return this.connected;
  }

  async close(): Promise<void> {
    this.connected = false;
    this.rejectAllPending(new Error("Client closing"));
    this.rl?.close();

    if (this.process.stdin) {
      this.process.stdin.end();
    }

    return new Promise<void>((resolve) => {
      const timeout = setTimeout(() => {
        this.process.kill("SIGKILL");
        resolve();
      }, 5_000);

      this.process.once("exit", () => {
        clearTimeout(timeout);
        resolve();
      });

      this.process.kill("SIGTERM");
    });
  }
}
