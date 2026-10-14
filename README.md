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
- Go 1.26.5+ and Python 3.11+ for native development
- Optional Gemini API key from Google AI Studio

## Run locally on the Windows VM
For the existing Windows lab host, use native Go + Streamlit with Docker Desktop running. This avoids assuming that the Linux Docker socket mount used by the Compose stack is available through Docker Desktop on Windows.

1. Generate a token in PowerShell: `py -c "import secrets; print(secrets.token_urlsafe(48))"`.
2. Copy the generated token. In Terminal 1, set it and start the API:
   ```powershell
   $env:SANDBOX_API_TOKEN = "PASTE_GENERATED_TOKEN"
   $env:SANDBOX_LISTEN_ADDR = "127.0.0.1:8080"
   cd backend
   go run ./cmd/api
   ```
3. In Terminal 2, set the same token and start Streamlit:
   ```powershell
   $env:SANDBOX_API_TOKEN = "PASTE_GENERATED_TOKEN"
   $env:SANDBOX_API_URL = "http://127.0.0.1:8080"
   # Optional: set this only if Gemini planning is wanted
   $env:GOOGLE_API_KEY = "YOUR_GEMINI_API_KEY"
   cd ui
   py -m venv .venv
   .venv\Scripts\Activate.ps1
   pip install -r requirements.txt
   streamlit run app.py
   ```
4. Open http://localhost:8501. Keep both processes bound to loopback.

## Compose deployment
On a Linux Docker Engine host, copy .env.example to .env, generate and set SANDBOX_API_TOKEN, optionally set GOOGLE_API_KEY, then run docker compose up --build. The socket-proxy mount and permissions must be validated for the target host; do not assume the Linux socket mount works on Windows Docker Desktop.

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
