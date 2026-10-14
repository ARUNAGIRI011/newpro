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
st.set_page_config(page_title="Infrastructure Sandbox Agent", page_icon="🧪", layout="wide")
st.title("Infrastructure Sandbox Agent")
st.caption("Describe a lab, review the plan, then explicitly approve execution. The diagnostic console runs only fixed, allowlisted commands—not an arbitrary shell.")
with st.sidebar:
    st.subheader("Execution mode")
    st.radio("Mode", ["Manual approval", "Assisted (plan then approve)"])
    st.warning("Pilot scope: Docker containers only. VMware/Hyper-V VM providers and production identity/RBAC are not implemented.")
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
