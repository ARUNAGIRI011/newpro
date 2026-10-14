# Infrastructure Sandbox Agent

A controlled lab MVP using Streamlit, Python Gemini planning, a Go REST API and Docker containers.

## Current implementation
- Natural-language plan generation with Gemini or an offline fallback.
- Explicit user approval before creating a sandbox.
- Go API with bearer-token authentication and an allowlisted Linux profile catalogue.
- Managed sandbox inventory, stop/delete operations and a restricted diagnostic console.
- Docker API routed through a private socket proxy in Compose.

## Important scope boundary
This branch provisions isolated Linux containers for functional testing. It does not yet provision VMware/Hyper-V/QEMU virtual machines or perform cross-hypervisor migration. Container tests cannot validate guest kernels, bootloaders, initramfs, firmware, disk controllers or migration fidelity.

## Requirements
- Docker Desktop / Docker Engine with Compose
- Go 1.25+ and Python 3.11+ for native development
- Optional Gemini API key from Google AI Studio

## Run locally
1. Copy .env.example to .env (do not commit .env).
2. Generate a token using: python -c "import secrets; print(secrets.token_urlsafe(48))"; place it in SANDBOX_API_TOKEN.
3. Optionally set GOOGLE_API_KEY.
4. Run docker compose up --build.
5. Open http://localhost:8501. Both published services bind to loopback.

The API fails closed when the token is missing or shorter than 32 characters. Never expose the API directly to the internet. The Docker socket proxy is still powerful; run this stack on a dedicated disposable worker host, not a production workload host.

## API
- GET /health/live (liveness)
- GET /health/ready (authenticated readiness)
- GET /api/profiles
- GET /api/sandboxes
- POST /api/sandboxes
- POST /api/sandboxes/{id}/stop
- POST /api/sandboxes/{id}/diagnostics
- DELETE /api/sandboxes/{id}

All API endpoints except liveness require Authorization: Bearer <SANDBOX_API_TOKEN>.

## Production blockers
Identity-provider login, RBAC, per-tenant isolation/quotas, TLS ingress, durable audit events, idempotency, image digest pinning, SBOM/scanning, VM console/terminal sessions, and real VMware/Hyper-V/QEMU provider adapters remain unimplemented. See docs/production-readiness.md.
