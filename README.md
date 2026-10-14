# Infrastructure Sandbox Agent

MVP: Streamlit chat UI, Python Gemini planner, Go REST control plane, and Docker container sandboxes.

## Scope boundary
This MVP provisions isolated Linux containers for functional testing. It does NOT yet provision VMware or Hyper-V virtual machines and does not perform cross-hypervisor migration. Docker containers cannot validate VM firmware, bootloader/initramfs, LVM boot disks, hypervisor drivers or migration fidelity.

## Requirements
- Go 1.25+
- Python 3.11+
- Docker Desktop with Linux containers enabled
- Optional Gemini API key from Google AI Studio

## Run on Windows PowerShell
1. Copy .env.example to .env and set GOOGLE_API_KEY if using Gemini.
2. Terminal 1: cd backend; go run ./cmd/api
3. Terminal 2: cd ui; py -m venv .venv; .venv\Scripts\Activate.ps1; pip install -r requirements.txt; streamlit run app.py
4. Open http://localhost:8501.

The Go API uses fixed Docker arguments, allowlisted OS profiles, CPU/RAM limits and a network-disabled container. Do not expose it to the internet. Mounting the Docker socket grants substantial host control.

## API
- GET /health
- GET /api/profiles
- GET /api/sandboxes
- POST /api/sandboxes
- POST /api/sandboxes/{id}/stop
- DELETE /api/sandboxes/{id}

## Production roadmap
Add Google ADK Go workflow orchestration, authenticated vSphere and Hyper-V providers, compatibility catalogs, durable state, RBAC, quotas, TLS, audit evidence, approval gates and real end-to-end migration tests. Do not treat a successful container test as proof of VM migration compatibility. Gemini quotas/model access vary by project; verify current AI Studio limits.
