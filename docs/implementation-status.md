# Implementation status

## Present in this branch
- Streamlit UI with plan review and explicit approval before container creation.
- Python Gemini API planning with an offline deterministic fallback.
- Go REST API with allowlisted Ubuntu 24.04, Debian 12 and Alpine container profiles.
- CPU, memory and PID limits; containers have no network by default.
- List, stop and delete managed containers.
- A Google ADK Go planner entry point in adk-go-agent. It currently provides interactive planning, not execution tools.

## Not implemented yet
- ADK Go workflow integration into the REST API.
- VMware vSphere provider and inventory discovery.
- Hyper-V provider and PowerShell remoting.
- Bootable VM image creation, disk conversion, replication and migration.
- Compatibility catalog populated from authoritative product support matrices.
- Durable database, authentication/RBAC, tenancy, audit event storage and production-grade approval workflow.
- Automated build/test pipeline and live end-to-end validation.

This branch is a starter MVP, not production-ready migration software. Do not use it for production infrastructure changes until the missing controls and provider integrations are implemented and tested.
