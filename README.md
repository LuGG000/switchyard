# switchyard

Open-source account switcher for Claude Code subscription logins (Pro/Max).

**Status: planning.** No functional code yet. See [docs/PLAN.md](docs/PLAN.md).

## Scope

Subscription logins only (`authMethod: claude.ai`). API-key, Console, Bedrock,
Vertex and Foundry accounts are explicitly out of scope.

## Terms of service and risk

This project is not affiliated with or endorsed by Anthropic. It only launches
the official `claude` binary and never reads, copies, logs or sends credentials.
Anthropic's Consumer Terms do not explicitly address multiple accounts, and it
is unclear whether rotating accounts to work around usage limits is tolerated.
Use only your own accounts, one at a time. You accept the residual risk,
including possible account restrictions.
