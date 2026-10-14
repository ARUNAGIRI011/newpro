# Production readiness gate

## Implemented in this hardening branch
- Bearer-token authentication on API routes except liveness; API refuses to start if token is absent or shorter than 32 characters.
- Strict JSON decoding, request size limits, input validation, resource limits and explicit OS profile allowlist.
- Non-root API image; managed containers use read-only root filesystem, no network, dropped capabilities, no-new-privileges, a limited tmpfs, and CPU/memory/PID limits.
- Stop/delete ownership checks; structured logs, security headers, bounded HTTP server timeouts and separate liveness/readiness endpoints.
- Docker API is routed through a restricted socket-proxy service instead of mounting the host socket into the API container.
- Restricted diagnostic console runs fixed commands only. It is not an arbitrary interactive shell.
- Go unit tests cover basic validation and authentication.

## Not yet production-ready
1. Docker socket proxy still grants powerful container operations. Use a dedicated isolated worker host/VM, keep the proxy private, and review the proxy allowlist. Do not place this worker on a production workload host.
2. Add TLS ingress, identity-provider login, per-user RBAC, tenant isolation, short-lived credentials and server-side approval records. The current shared API token is a bootstrap control, not multi-user identity.
3. Add durable state, immutable audit events, idempotency, per-tenant quotas, concurrency controls, retention/cleanup policies, metrics, tracing and alerting.
4. Pin images by verified digest, scan image/dependency artifacts, generate SBOMs, and keep a tested OS profile catalogue.
5. vSphere and Hyper-V lifecycle adapters now exist in the orchestrator: vSphere clone/power/snapshot/delete via govmomi; Hyper-V create-from-VHDX/power/checkpoint/delete via WinRM HTTPS. Still add credential vaulting, capability discovery, managed-object inventory scoping, and live integration tests. QEMU/KVM adapter is not implemented.
6. Hypervisor snapshots/checkpoints are implemented, but restore workflows, serial/VNC console or SSH access with per-lab credentials, evidence capture and cleanup remain incomplete. The diagnostic console is deliberately allowlisted.
7. End-to-end cross-hypervisor migration is not implemented: VMware disk export, VMDK-to-VHDX conversion, guest boot repair, network remapping, rollback and migration evidence remain blockers. Run CI and live integration tests on the Windows host and real hypervisors. CI passing is necessary but not sufficient for production acceptance.

## Fidelity boundary
Docker containers support userspace/package/API testing only. They are not bootable VMs and cannot validate kernels, initramfs, GRUB, firmware, disk controllers, guest tools, migration-agent installation, boot recovery or cross-hypervisor migration. Debian 8 is end-of-life: test only in a disconnected disposable VM with validated archived media; do not expose it to a network.

## Local deployment
1. Copy .env.example to .env; keep .env untracked.
2. Generate both SANDBOX_API_TOKEN and SANDBOX_CHANGE_APPROVAL_TOKEN with: python -c "import secrets; print(secrets.token_urlsafe(48))"
3. Set GOOGLE_API_KEY only if Gemini planning is wanted. Configure the remote provider variables only for the hypervisors you will use.
4. Run docker compose up --build.
5. UI is bound to 127.0.0.1:8501; API to 127.0.0.1:8080. Do not publish these ports to the internet directly.

For remote access, deploy behind authenticated TLS ingress with network restrictions and proper user identity. Do not simply change published ports to 0.0.0.0.
