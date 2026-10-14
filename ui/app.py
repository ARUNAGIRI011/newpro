import json
import os
import re
import requests
import streamlit as st

try:
    from google import genai
except ImportError:
    genai = None

API = os.getenv("SANDBOX_API_URL", "http://localhost:8080")
MODEL = os.getenv("GEMINI_MODEL", "gemini-2.5-flash")
API_TOKEN = os.getenv("SANDBOX_API_TOKEN", "").strip()
API_HEADERS = {"Authorization": f"Bearer {API_TOKEN}"} if API_TOKEN else {}
CHANGE_TOKEN = os.getenv("SANDBOX_CHANGE_APPROVAL_TOKEN", "").strip()
CHANGE_HEADERS = {**API_HEADERS, "X-Approval-Token": CHANGE_TOKEN} if API_HEADERS and CHANGE_TOKEN else {}
st.set_page_config(page_title="Infrastructure Sandbox Agent", page_icon="🧪", layout="wide")
st.title("Infrastructure Sandbox Agent")
st.caption("Describe a lab, review the plan, then explicitly approve execution. The diagnostic console runs only fixed, allowlisted commands—not an arbitrary shell.")
with st.sidebar:
    st.subheader("Execution mode")
    st.radio("Mode", ["Manual approval", "Assisted (plan then approve)"])
    st.warning("Docker containers are for functional tests. VMware and Hyper-V actions target real remote hypervisors; all changes require a separate approval token and managed isa-* VM names.")
    try:
        h=requests.get(f"{API}/health/ready",headers=API_HEADERS,timeout=3).json()
        st.success(f"API: {h.get('status')} · {h.get('provider')}")
    except Exception as e:
        st.error(f"API unavailable: {e}")

def make_plan(text):
    key=os.getenv("GOOGLE_API_KEY","").strip()
    if not key or genai is None:
        distro="ubuntu-24.04" if re.search(r"ubuntu\s*24",text,re.I) else ("debian-12" if re.search(r"debian\s*12",text,re.I) else "alpine-3.22")
        return {"summary":f"Deterministic fallback: create one {distro} container.","profile":{"os":distro,"name":"lab-"+distro.replace(".","-"),"cpus":"1","memory_mib":512},"steps":["Validate allowlisted profile","Create network-disabled container","Record container ID/status"],"needs_real_vm":bool(re.search(r"vmware|hyper-v|migrat|lvm boot|firmware|bootloader|kernel driver",text,re.I))}
    client=genai.Client(api_key=key)
    prompt=f'''Return ONLY valid JSON with keys summary, profile, steps, needs_real_vm.
Allowed profile.os: ubuntu-24.04, debian-12, alpine-3.22.
profile fields: os, name (lowercase letters/digits/hyphens, max 48 chars), cpus (0.25-2), memory_mib (128-2048).
Never output shell commands, secrets, or claim a container is a VM.
If request mentions VMware, Hyper-V, VM migration, firmware, bootloader, LVM boot disks or hypervisor drivers, set needs_real_vm=true.
User request: {text}'''
    response=client.models.generate_content(model=MODEL,contents=prompt)
    raw=(response.text or "").strip()
    raw=re.sub(r"^\x60{3}(?:json)?\s*|\s*\x60{3}$","",raw,flags=re.I)
    result=json.loads(raw)
    if result.get("profile",{}).get("os") not in {"ubuntu-24.04","debian-12","alpine-3.22"}:
        raise ValueError("Unsupported OS profile from planner.")
    return result

if "plan" not in st.session_state: st.session_state.plan=None
request=st.text_area("Describe your infrastructure lab",placeholder="Create an Ubuntu 24.04 functional test container with 1 CPU and 512 MiB RAM.")
if st.button("Generate plan",type="primary"):
    if not request.strip(): st.warning("Enter a request.")
    else:
        try: st.session_state.plan=make_plan(request)
        except Exception as e: st.error(f"Planning failed: {e}")
plan=st.session_state.plan
if plan:
    st.subheader("Proposed plan")
    st.write(plan.get("summary",""))
    st.json(plan.get("profile",{}))
    for i,step in enumerate(plan.get("steps",[]),1): st.write(f"{i}. {step}")
    if plan.get("needs_real_vm"): st.warning("This request needs real hypervisor adapters. Execution is disabled to avoid falsely simulating VM migration.")
    approve=st.checkbox("I reviewed this plan and approve creating the Docker container.")
    if st.button("Execute approved plan",disabled=not approve or bool(plan.get("needs_real_vm"))):
        try:
            r=requests.post(f"{API}/api/sandboxes",json=plan["profile"],headers=API_HEADERS,timeout=100)
            if r.ok: st.success("Sandbox created"); st.json(r.json())
            else: st.error(f"Provisioning failed ({r.status_code}): {r.text}")
        except Exception as e: st.error(str(e))
st.divider()
st.subheader("Real hypervisor labs")
st.caption("The Go API runs inside Docker and connects to vCenter over HTTPS and Hyper-V over WinRM HTTPS. Provisioning clones a configured vSphere template or copies a configured Hyper-V VHDX. VM creation, power, snapshots/checkpoints and deletion require the separate change-approval token.")
provider_label = st.selectbox("Hypervisor target", ["VMware vSphere", "Microsoft Hyper-V"])
provider = "vsphere" if provider_label == "VMware vSphere" else "hyperv"
with st.form("create-real-vm"):
    vm_name = st.text_input("Managed VM name (must start with isa-)", value="isa-ubuntu-lab")
    vm_cpus = st.selectbox("vCPU", [1, 2, 4], index=1)
    vm_memory = st.selectbox("Memory (MiB)", [1024, 2048, 4096, 8192], index=1)
    create_vm_approved = st.checkbox("I approve creating this real VM from the configured template")
    create_vm = st.form_submit_button("Create real VM", disabled=not bool(CHANGE_HEADERS))
    if create_vm:
        if not create_vm_approved:
            st.warning("Check the approval box before creating a VM.")
        elif not re.fullmatch(r"isa-[A-Za-z0-9._-]{1,59}", vm_name):
            st.error("Use a name beginning with isa- and only letters, digits, dot, underscore or hyphen.")
        else:
            try:
                rr = requests.post(f"{API}/api/providers/{provider}/vms",
                    json={"name": vm_name, "cpus": vm_cpus, "memory_mib": vm_memory},
                    headers=CHANGE_HEADERS, timeout=220)
                if rr.ok:
                    st.success("VM creation request completed")
                    st.json(rr.json())
                else:
                    st.error(f"VM creation failed ({rr.status_code}): {rr.text}")
            except Exception as e:
                st.error(f"VM creation failed: {e}")
if not CHANGE_HEADERS:
    st.warning("Real hypervisor changes are disabled in this UI until SANDBOX_API_TOKEN and SANDBOX_CHANGE_APPROVAL_TOKEN are configured.")
try:
    inventory_response = requests.get(f"{API}/api/providers/{provider}/vms", headers=API_HEADERS, timeout=45)
    if inventory_response.ok:
        vms = inventory_response.json()
        if not vms:
            st.info("No VMs returned by this provider.")
        for vm in vms:
            with st.container(border=True):
                a1, a2, a3 = st.columns([3, 2, 2])
                a1.write(f"**{vm.get('name')}**")
                a1.caption(f"{vm.get('provider')} · {vm.get('id')}")
                a2.write(vm.get("state", "unknown"))
                a2.caption(vm.get("guest_os", ""))
                approve_vm_change = a3.checkbox("Approve change", key=f"approve-vm-{provider}-{vm.get('id')}")
                action_cols = st.columns(4)
                vm_id = vm.get("id", "")
                if action_cols[0].button("Power on", key=f"on-{provider}-{vm_id}", disabled=not bool(CHANGE_HEADERS) or not approve_vm_change):
                    rr = requests.post(f"{API}/api/providers/{provider}/vms/{vm_id}/power", json={"state":"on"}, headers=CHANGE_HEADERS, timeout=150)
                    st.write(rr.json())
                if action_cols[1].button("Power off", key=f"off-{provider}-{vm_id}", disabled=not bool(CHANGE_HEADERS) or not approve_vm_change):
                    rr = requests.post(f"{API}/api/providers/{provider}/vms/{vm_id}/power", json={"state":"off"}, headers=CHANGE_HEADERS, timeout=150)
                    st.write(rr.json())
                if action_cols[2].button("Snapshot", key=f"snap-{provider}-{vm_id}", disabled=not bool(CHANGE_HEADERS) or not approve_vm_change):
                    rr = requests.post(f"{API}/api/providers/{provider}/vms/{vm_id}/snapshots", json={}, headers=CHANGE_HEADERS, timeout=200)
                    st.write(rr.json())
                if action_cols[3].button("Delete VM", key=f"delete-vm-{provider}-{vm_id}", disabled=not bool(CHANGE_HEADERS) or not approve_vm_change):
                    rr = requests.delete(f"{API}/api/providers/{provider}/vms/{vm_id}", headers=CHANGE_HEADERS, timeout=220)
                    st.write(rr.json())
    else:
        st.warning(f"Provider inventory unavailable ({inventory_response.status_code}): {inventory_response.text}")
except Exception as e:
    st.caption(f"Hypervisor inventory unavailable: {e}")

st.divider()
st.subheader("Managed sandboxes")
try:
    items=requests.get(f"{API}/api/sandboxes",headers=API_HEADERS,timeout=10).json()
    if not items: st.info("No managed sandboxes yet.")
    for item in items:
        with st.container(border=True):
            c1,c2,c3=st.columns([3,2,2])
            c1.write(f"**{item.get('name')}**"); c1.caption(item.get("id",""))
            c2.write(item.get("image","")); c2.caption(item.get("status",""))
            sid=item.get("id","")
            if c3.button("Stop",key="stop-"+sid):
                st.write(requests.post(f"{API}/api/sandboxes/{sid}/stop",headers=API_HEADERS,timeout=30).json())
            if c3.button("Delete",key="delete-"+sid):
                rr=requests.delete(f"{API}/api/sandboxes/{sid}",headers=API_HEADERS,timeout=30)
                st.write(rr.json())
            diag=st.selectbox("Safe diagnostic command",["identity","os-release","kernel","filesystem","working-directory","root-files"],key="diag-"+sid)
            if st.button("Run diagnostic",key="run-diag-"+sid):
                rr=requests.post(f"{API}/api/sandboxes/{sid}/diagnostics",json={"command":diag},headers=API_HEADERS,timeout=20)
                if rr.ok:
                    st.code(rr.json().get("output",""),language="text")
                else:
                    st.error(f"Diagnostic failed ({rr.status_code}): {rr.text}")
except Exception as e: st.caption(f"Inventory unavailable: {e}")
