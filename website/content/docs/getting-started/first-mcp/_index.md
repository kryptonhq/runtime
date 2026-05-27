---
title: Deploying your first MCP server
linkTitle: First MCP server
weight: 4
description: Host any Model Context Protocol server as a Krypton Agent.
---

Krypton hosts [Model Context Protocol](https://modelcontextprotocol.io)
servers as first-class Agents. Whatever your server's transport, you end
up with one CRD, one HTTP endpoint, and tool introspection in the
operator UI.

Two transports are supported:

- **[HTTP](http/)** — your server speaks MCP's streamable-HTTP transport
  natively. Recommended for new code.
- **[STDIO](stdio/)** — wrap an off-the-shelf stdio MCP binary (most
  `npm` packages) with the bundled bridge.

## Once it's running

For any Agent with `spec.protocol: mcp`, the operator UI surfaces an
**MCP Tools** card with the server's `tools/list` output. Each tool
shows its name, description, and `inputSchema`, with a JSON editor +
**Call** button to invoke it.

The control plane exposes two typed endpoints used by that UI and
available to any HTTP client:

```
GET  /v1/agents/{namespace}/{name}/mcp/tools
POST /v1/agents/{namespace}/{name}/mcp/tools/{toolName}    body = arguments JSON
```

Both handle the MCP `initialize` + `notifications/initialized`
handshake internally and unwrap the `content` envelope on the way out.

## What's implemented

| MCP method                    | Supported |
| ----------------------------- | --------- |
| `initialize`                  | ✓         |
| `notifications/initialized`   | ✓         |
| `tools/list`                  | ✓         |
| `tools/call`                  | ✓         |
| `resources/list` + read       | future    |
| `prompts/list` + get          | future    |
| Sampling, completions         | future    |

The Go client lives at
[`internal/mcp/`](https://github.com/kryptonhq/runtime/tree/main/internal/mcp).
Resources, prompts, sampling, completions land when a real consumer
asks for them.
