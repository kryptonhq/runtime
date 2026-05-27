"""Tiny FastMCP server used to smoke-test Krypton's MCP support.

Three toy tools: echo, add, time. No external deps beyond fastmcp.
Speaks MCP over streamable HTTP — JSON-RPC 2.0 at POST /.
"""
import datetime
import os

from fastmcp import FastMCP

mcp = FastMCP("mcp-hello-python")


@mcp.tool
def echo(message: str) -> str:
    """Echo the message back."""
    return message


@mcp.tool
def add(a: float, b: float) -> str:
    """Add two numbers and return the sum as a string."""
    return str(a + b)


@mcp.tool
def now() -> str:
    """Return the current UTC time in RFC 3339."""
    return datetime.datetime.now(datetime.UTC).isoformat()


if __name__ == "__main__":
    mcp.run(
        transport="http",
        host="0.0.0.0",
        port=int(os.getenv("PORT", "8080")),
        path="/",
    )
