import { Type } from "@sinclair/typebox";
import type { OpenClawPluginApi } from "openclaw/plugin-sdk";
import { isActive } from "./config.js";
import { getMcpClient } from "./mcp-service.js";

// ============================================================================
// Tool Registration
// ============================================================================

export function registerRuneTools(api: OpenClawPluginApi): void {
  // Tool factory: only expose tools when Rune is active
  api.registerTool(
    () => {
      if (!isActive()) return null;

      return [
        // ── rune_capture ─────────────────────────────────────────────
        {
          name: "rune_capture",
          label: "Rune Capture",
          description:
            "Store an organizational decision, trade-off analysis, or insight to encrypted team memory. " +
            "Use when a decision is made with rationale, trade-offs are analyzed, or lessons are learned.",
          parameters: Type.Object({
            text: Type.String({ description: "Decision or insight text to capture" }),
            source: Type.Optional(Type.String({ description: "Source context (e.g. channel name)" })),
          }),
          async execute(
            _toolCallId: string,
            params: { text: string; source?: string },
          ) {
            const client = getMcpClient();
            if (!client?.isConnected()) {
              return {
                content: [{ type: "text", text: "Rune MCP server not connected." }],
                isError: true,
              };
            }

            try {
              const result = await client.callTool("capture", {
                text: params.text,
                source: params.source ?? "openclaw_agent",
              });
              return {
                content: result.content ?? [{ type: "text", text: "Captured." }],
                details: result,
              };
            } catch (err) {
              return {
                content: [{ type: "text", text: `Rune capture failed: ${String(err)}` }],
                isError: true,
              };
            }
          },
        },

        // ── rune_recall ──────────────────────────────────────────────
        {
          name: "rune_recall",
          label: "Rune Recall",
          description:
            "Natural language query against encrypted organizational memory with LLM-synthesized answers. " +
            "Returns an answer with source citations and confidence levels. " +
            "Use for complex questions about past decisions and organizational knowledge.",
          parameters: Type.Object({
            query: Type.String({ description: "Natural language question about past decisions or knowledge" }),
            topk: Type.Optional(
              Type.Number({ description: "Number of source results (default: 5, max: 10)" }),
            ),
          }),
          async execute(_toolCallId: string, params: { query: string; topk?: number }) {
            const client = getMcpClient();
            if (!client?.isConnected()) {
              return {
                content: [{ type: "text", text: "Rune MCP server not connected." }],
                isError: true,
              };
            }

            try {
              const result = await client.callTool("recall", {
                query: params.query,
                topk: Math.min(params.topk ?? 5, 10),
              });
              return {
                content: result.content ?? [{ type: "text", text: "No results found." }],
                details: result,
              };
            } catch (err) {
              return {
                content: [{ type: "text", text: `Rune recall failed: ${String(err)}` }],
                isError: true,
              };
            }
          },
        },
      ];
    },
    { names: ["rune_capture", "rune_recall"], optional: true },
  );
}
