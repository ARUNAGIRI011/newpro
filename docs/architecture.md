# Architecture and provider boundaries

- Streamlit: chat, plan review, explicit approval, inventory and manual actions.
- Python Gemini planner: constrained JSON proposal only; no infrastructure credentials or shell execution.
- Go API: allowlisted Docker lifecycle operations and resource limits.
- Planned Google ADK Go workflow layer: compatibility assessment, step sequencing, approvals and evidence generation. Keep execution deterministic and separate from model output.

## Providers
1. Docker containers for low-cost functional tests.
2. VMware vSphere API provider for real source VMs.
3. Hyper-V provider via authenticated PowerShell remoting and supported Hyper-V cmdlets.
4. QEMU/KVM provider for local bootable VM tests where host virtualization is supported.

## Compatibility
Maintain a versioned catalog covering OS/kernel, hypervisor version, firmware, disk controller, target drivers, migration-tool version and validated workflow. Unknown combinations must be UNVERIFIED, not compatible.

## Fidelity tiers
- container-functional: scripts, APIs, package/service tests; no independent kernel or bootloader.
- qemu-vm: real guest boot and disk tests on QEMU hardware; not proof of VMware/Hyper-V compatibility.
- hypervisor-vm: real source and target hypervisors; required for cross-hypervisor migration proof.
- cluster: real nodes, supported storage, networks, quorum and failure injection; claim failover coverage only after successful tests.
