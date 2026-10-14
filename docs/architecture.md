# Architecture and provider boundaries

- Streamlit: chat, plan review, explicit approval, inventory and manual actions.
- Python Gemini planner: constrained JSON proposal only; no infrastructure credentials or shell execution.
- Go API: allowlisted Docker lifecycle operations and resource limits; authenticated remote VMware vSphere and Hyper-V adapters for real VM inventory and lifecycle.
- Planned Google ADK Go workflow layer: compatibility assessment, step sequencing, approvals and evidence generation. Keep execution deterministic and separate from model output.

## Providers
1. Docker containers for low-cost functional tests.
2. VMware vSphere provider using govmomi over HTTPS; clone from a configured template, inventory, power, snapshot and delete agent-owned VMs.
3. Hyper-V provider using WinRM over HTTPS with NTLM message encryption; create from a configured remote VHDX, inventory, power, checkpoint and delete agent-owned VMs.
4. QEMU/KVM provider for local bootable VM tests where host virtualization is supported (not implemented yet).

## Compatibility
Maintain a versioned catalog covering OS/kernel, hypervisor version, firmware, disk controller, target drivers, migration-tool version and validated workflow. Unknown combinations must be UNVERIFIED, not compatible.

## Fidelity tiers
- container-functional: scripts, APIs, package/service tests; no independent kernel or bootloader.
- qemu-vm: real guest boot and disk tests on QEMU hardware; not proof of VMware/Hyper-V compatibility.
- hypervisor-vm: real source and target hypervisors; required for cross-hypervisor migration proof.
- cluster: real nodes, supported storage, networks, quorum and failure injection; claim failover coverage only after successful tests.


## Remote provider boundary
Docker hosts only the orchestration process. The vSphere provider calls the remote vCenter/ESXi management API; the Hyper-V provider executes fixed, application-authored PowerShell scripts on the remote Windows host over WinRM HTTPS. The API does not accept arbitrary PowerShell or shell commands. Real VM changes require the API bearer token plus a separate approval token, and are restricted to agent-created VMs carrying the expected ownership marker. This MVP provisions VMs from templates; it does not yet export VMware disks, convert VMDK to VHDX, repair guest boot, remap networks, or perform end-to-end cross-hypervisor migration.
