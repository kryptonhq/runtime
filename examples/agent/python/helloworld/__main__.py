"""Krypton-friendly server harness for the helloworld A2A agent.

The upstream `__main__.py` (see UPSTREAM_README.md) hard-codes
`uvicorn.run(app, host='127.0.0.1', port=9999)` — fine for local dev,
useless inside a container where the sidecar needs to reach the
process on 0.0.0.0. This file rebuilds the same Starlette app
verbatim but binds to 0.0.0.0:$PORT (default 8080).

The interesting bit — `HelloWorldAgentExecutor` — is the unmodified
upstream `agent_executor.py`.
"""
import os

import uvicorn
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.routes import create_agent_card_routes, create_jsonrpc_routes
from a2a.server.tasks import InMemoryTaskStore
from a2a.types import AgentCapabilities, AgentCard, AgentInterface, AgentSkill
from starlette.applications import Starlette

from agent_executor import HelloWorldAgentExecutor


PORT = int(os.getenv("PORT", "8080"))
PUBLIC_URL = os.getenv("PUBLIC_URL", f"http://0.0.0.0:{PORT}")


skill = AgentSkill(
    id="echo_bot",
    name="Echo Bot",
    description='Acknowledges a request and replies with "Hello, World!"',
    input_modes=["text/plain"],
    output_modes=["text/plain"],
    tags=["a2a", "echo-example"],
    examples=["hi", "how are you"],
)

public_agent_card = AgentCard(
    name="Hello World Agent",
    description="No-LLM A2A smoke-test agent.",
    version="0.0.1",
    default_input_modes=["text/plain"],
    default_output_modes=["text/plain"],
    capabilities=AgentCapabilities(streaming=True),
    supported_interfaces=[
        AgentInterface(protocol_binding="JSONRPC", url=PUBLIC_URL),
    ],
    skills=[skill],
)

request_handler = DefaultRequestHandler(
    agent_executor=HelloWorldAgentExecutor(),
    task_store=InMemoryTaskStore(),
    agent_card=public_agent_card,
)

routes = []
routes.extend(create_agent_card_routes(public_agent_card))
routes.extend(create_jsonrpc_routes(request_handler, "/", enable_v0_3_compat=True))

app = Starlette(routes=routes)


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=PORT)
