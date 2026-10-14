# Infrastructure Sandbox Agent

A controlled lab orchestrator using Streamlit, Python Gemini planning, a Go REST API and Docker containers. Docker hosts the control plane; real VM operations are sent to remote VMware vSphere and Hyper-V hosts over their management protocols.

## Current implementation
- Natural-language plan generation with Gemini or an offline fallback.
- Explicit user approval before creating a sandbox.
- Go API with bearer-token authentication and an allowlisted Linux profile catalogue.
- Managed sandbox inventory, stop/delete operations and a restricted diagnostic console.
- Docker API routed through a private socket proxy in Compose.

## Real hypervisor adapters (new)
The Go API runs inside Docker and talks to remote hypervisor management endpoints; neither VMware ESXi/vCenter nor Hyper-V runs inside the container.

- **VMware vSphere:** authenticated govmomi SDK over HTTPS; inventory, clone a configured template, power on/off, snapshot, and delete agent-managed VMs.
- **Hyper-V:** authenticated WinRM over HTTPS with NTLM message encryption; inventory, create a Generation 2 VM from a configured remote VHDX, power on/off, checkpoint, and delete agent-managed VMs.
- Provisioned VM names must start with `isa-`. All mutation routes require both API bearer authentication and a separate `X-Approval-Token`; the UI has per-action approval controls.
- The vSphere clone uses the template's inherited storage/network settings and the default resource pool/folder. Hyper-V uses the configured switch, if supplied.
- These are real hypervisor lifecycle adapters, **not yet an end-to-end cross-hypervisor migration engine**. VMware disk export, VMDK-to-VHDX conversion, guest boot repair, network remapping, migration evidence/rollback and automated compatibility validation are not implemented. Never call this migration success.

Container tests still cannot validate guest kernels, bootloaders, initramfs, firmware, disk controllers or migration fidelity.

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
On a Linux Docker Engine host, copy .env.example to .env and configure `SANDBOX_API_TOKEN` and a distinct random `SANDBOX_CHANGE_APPROVAL_TOKEN`. Add provider connection settings only for the remote hypervisors you intend to manage, then run `docker compose up --build`. The socket-proxy mount and permissions must be validated for the target host; do not assume the Linux socket mount works on Windows Docker Desktop.

### Provider configuration
- vSphere: set `VSPHERE_URL=https://<vcenter>/sdk`, `VSPHERE_USERNAME`, `VSPHERE_PASSWORD`, and `VSPHERE_TEMPLATE_PATH` to the inventory path of a tested template. Use a least-privilege vCenter account and a valid trusted certificate; leave `VSPHERE_INSECURE_SKIP_VERIFY=false`.
- Hyper-V: configure WinRM **HTTPS** on the remote Windows Hyper-V host, then set `HYPERV_WINRM_HOST`, credentials, `HYPERV_TEMPLATE_VHDX` (a path on the Hyper-V host), `HYPERV_VM_ROOT` (a path on that host), and optionally `HYPERV_SWITCH_NAME`. Do not enable Basic authentication or unencrypted WinRM. Restrict port 5986 to the orchestrator host.
- Secrets are environment-injected for this MVP. For shared or production use, replace this with a secret manager and short-lived credentials; do not commit real values to `.env` or logs.
- The Docker network must be able to route to vCenter HTTPS and the Hyper-V WinRM HTTPS endpoint. The API does not need hypervisor binaries or access to the hypervisor host's local Docker socket for these operations.

The API fails closed when the token is missing or shorter than 32 characters. Never expose the API directly to the internet. The Docker socket proxy is still powerful; run this stack on a dedicated disposable worker host, not a production workload host.

## Hypervisor API
- `GET /api/providers/vsphere/vms` and `GET /api/providers/hyperv/vms`
- `POST /api/providers/{vsphere|hyperv}/vms` clones/creates from the configured template
- `POST /api/providers/{provider}/vms/{id}/power` with `{"state":"on"}` or `{"state":"off"}`
- `POST /api/providers/{provider}/vms/{id}/snapshots`
- `DELETE /api/providers/{provider}/vms/{id}`

All provider routes require `Authorization: Bearer <SANDBOX_API_TOKEN>`. Every mutation also requires `X-Approval-Token: <SANDBOX_CHANGE_APPROVAL_TOKEN>`. The API only permits lifecycle mutations on VMs with names starting `isa-`; still use a dedicated lab account and disposable lab capacity.

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
