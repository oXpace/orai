"""Optional tool integrations. Core session/message features never depend on them."""


def mcp_servers(project, provider):
    """Per-process MCP entries injected at role launch; global CLI config is left alone."""
    servers = {}
    if project.config.wiki:
        from orai.integrations import qmd

        settings = qmd.Settings(project)
        if provider == "codex":
            servers[settings.server_name] = {
                "url": settings.endpoint,
                "startup_timeout_sec": 20,
                "tool_timeout_sec": 180,
            }
        else:
            servers[settings.server_name] = {"type": "http", "url": settings.endpoint}
    # CodeGraph registration stays the user's choice (`codegraph install --location local`);
    # injecting a second codegraph server would duplicate a global one.
    return servers
