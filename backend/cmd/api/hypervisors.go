package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/masterzen/winrm"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

type hypervisorAPI struct{}

type vmCreateRequest struct {
	Name       string `json:"name"`
	CPUs       int    `json:"cpus"`
	MemoryMiB  int    `json:"memory_mib"`
}

type vmPowerRequest struct {
	State string `json:"state"`
}

type vmRecord struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	State    string `json:"state"`
	GuestOS  string `json:"guest_os,omitempty"`
	Provider string `json:"provider"`
}

var safeVMName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)
var safeVMID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func (h *hypervisorAPI) register(mux *http.ServeMux, a *API) {
	mux.Handle("GET /api/providers/{provider}/vms", a.auth(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/providers/{provider}/vms", a.auth(http.HandlerFunc(h.create)))
	mux.Handle("POST /api/providers/{provider}/vms/{id}/power", a.auth(http.HandlerFunc(h.power)))
	mux.Handle("POST /api/providers/{provider}/vms/{id}/snapshots", a.auth(http.HandlerFunc(h.snapshot)))
	mux.Handle("DELETE /api/providers/{provider}/vms/{id}", a.auth(http.HandlerFunc(h.remove)))
}

func requireChangeApproval(r *http.Request) error {
	expected := strings.TrimSpace(os.Getenv("SANDBOX_CHANGE_APPROVAL_TOKEN"))
	provided := strings.TrimSpace(r.Header.Get("X-Approval-Token"))
	if len(expected) < 32 || len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		return errors.New("a valid X-Approval-Token is required; configure SANDBOX_CHANGE_APPROVAL_TOKEN")
	}
	return nil
}

func (h *hypervisorAPI) list(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	var records []vmRecord
	var err error
	switch r.PathValue("provider") {
	case "vsphere":
		records, err = listVSphere(ctx)
	case "hyperv":
		records, err = listHyperV(ctx)
	default:
		writeError(w, 404, "provider must be vsphere or hyperv")
		return
	}
	if err != nil {
		writeError(w, 502, "provider inventory failed; inspect backend logs")
		return
	}
	writeJSON(w, 200, records)
}

func (h *hypervisorAPI) create(w http.ResponseWriter, r *http.Request) {
	if err := requireChangeApproval(r); err != nil {
		writeError(w, 403, err.Error())
		return
	}
	var req vmCreateRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(&req); err != nil {
		writeError(w, 400, "invalid JSON request")
		return
	}
	if !safeVMName.MatchString(req.Name) || !strings.HasPrefix(req.Name, "isa-") {
		writeError(w, 400, "name must start with isa- and use 1-63 letters, digits, dot, underscore or hyphen")
		return
	}
	if req.CPUs == 0 {
		req.CPUs = 2
	}
	if req.MemoryMiB == 0 {
		req.MemoryMiB = 2048
	}
	if req.CPUs < 1 || req.CPUs > 4 || req.MemoryMiB < 512 || req.MemoryMiB > 8192 {
		writeError(w, 400, "cpus must be 1-4 and memory_mib must be 512-8192")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	var record vmRecord
	var err error
	switch r.PathValue("provider") {
	case "vsphere":
		record, err = cloneVSphereTemplate(ctx, req)
	case "hyperv":
		record, err = createHyperVFromVHDX(ctx, req)
	default:
		writeError(w, 404, "provider must be vsphere or hyperv")
		return
	}
	if err != nil {
		writeError(w, 502, "VM provisioning failed; inspect backend logs")
		return
	}
	writeJSON(w, 201, record)
}

func (h *hypervisorAPI) power(w http.ResponseWriter, r *http.Request) {
	if err := requireChangeApproval(r); err != nil {
		writeError(w, 403, err.Error())
		return
	}
	provider, id := r.PathValue("provider"), r.PathValue("id")
	if !safeVMID.MatchString(id) {
		writeError(w, 400, "invalid VM id")
		return
	}
	var req vmPowerRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(&req); err != nil || (req.State != "on" && req.State != "off") {
		writeError(w, 400, "state must be on or off")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	var err error
	switch provider {
	case "vsphere":
		err = powerVSphere(ctx, id, req.State)
	case "hyperv":
		err = powerHyperV(ctx, id, req.State)
	default:
		writeError(w, 404, "provider must be vsphere or hyperv")
		return
	}
	if err != nil {
		writeError(w, 502, "VM power operation failed; inspect backend logs")
		return
	}
	writeJSON(w, 200, map[string]string{"provider": provider, "id": id, "state": req.State})
}

func (h *hypervisorAPI) snapshot(w http.ResponseWriter, r *http.Request) {
	if err := requireChangeApproval(r); err != nil {
		writeError(w, 403, err.Error())
		return
	}
	provider, id := r.PathValue("provider"), r.PathValue("id")
	if !safeVMID.MatchString(id) {
		writeError(w, 400, "invalid VM id")
		return
	}
	name := "infra-lab-" + strconv.FormatInt(time.Now().UTC().Unix(), 10)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	var err error
	switch provider {
	case "vsphere":
		err = snapshotVSphere(ctx, id, name)
	case "hyperv":
		err = snapshotHyperV(ctx, id, name)
	default:
		writeError(w, 404, "provider must be vsphere or hyperv")
		return
	}
	if err != nil {
		writeError(w, 502, "VM snapshot/checkpoint failed; inspect backend logs")
		return
	}
	writeJSON(w, 201, map[string]string{"provider": provider, "id": id, "snapshot": name})
}

func (h *hypervisorAPI) remove(w http.ResponseWriter, r *http.Request) {
	if err := requireChangeApproval(r); err != nil {
		writeError(w, 403, err.Error())
		return
	}
	provider, id := r.PathValue("provider"), r.PathValue("id")
	if !safeVMID.MatchString(id) {
		writeError(w, 400, "invalid VM id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	var err error
	switch provider {
	case "vsphere":
		err = removeVSphere(ctx, id)
	case "hyperv":
		err = removeHyperV(ctx, id)
	default:
		writeError(w, 404, "provider must be vsphere or hyperv")
		return
	}
	if err != nil {
		writeError(w, 502, "VM removal failed; inspect backend logs")
		return
	}
	writeJSON(w, 200, map[string]string{"provider": provider, "id": id, "status": "removed"})
}

func vsphereClient(ctx context.Context) (*govmomi.Client, error) {
	rawURL := strings.TrimSpace(os.Getenv("VSPHERE_URL"))
	user := os.Getenv("VSPHERE_USERNAME")
	password := os.Getenv("VSPHERE_PASSWORD")
	if rawURL == "" || user == "" || password == "" {
		return nil, errors.New("vSphere provider is not configured")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("VSPHERE_URL must be an http(s) vCenter/ESXi URL")
	}
	u.User = url.UserPassword(user, password)
	// Certificate verification stays enabled unless an explicit insecure lab override is set.
	insecure := strings.EqualFold(os.Getenv("VSPHERE_INSECURE_SKIP_VERIFY"), "true")
	c, err := govmomi.NewClient(ctx, u, insecure)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func guestOS(g *types.GuestInfo) string { if g == nil { return "" }; return g.GuestFullName }

func listVSphere(ctx context.Context) ([]vmRecord, error) {
	c, err := vsphereClient(ctx)
	if err != nil { return nil, err }
	defer c.Logout(ctx)
	f := find.NewFinder(c.Client, true)
	vms, err := f.VirtualMachineList(ctx, "*")
	if err != nil { return nil, err }
	out := make([]vmRecord, 0, len(vms))
	for _, vm := range vms {
		var p mo.VirtualMachine
		if err := vm.Properties(ctx, vm.Reference(), []string{"name", "runtime.powerState", "config.instanceUuid", "guest.guestFullName"}, &p); err != nil {
			continue
		}
		out = append(out, vmRecord{ID: vm.Reference().Value, Name: p.Name, State: string(p.Runtime.PowerState), GuestOS: guestOS(p.Guest), Provider: "vsphere"})
	}
	return out, nil
}

func findVSphereVM(ctx context.Context, id string) (*govmomi.Client, *object.VirtualMachine, error) {
	c, err := vsphereClient(ctx)
	if err != nil { return nil, nil, err }
	vm := object.NewVirtualMachine(c.Client, types.ManagedObjectReference{Type: "VirtualMachine", Value: id})
	var p moVirtualMachine
	if err := vm.Properties(ctx, vm.Reference(), []string{"name", "runtime.powerState"}, &p); err != nil {
		_ = c.Logout(ctx)
		return nil, nil, err
	}
	return c, vm, nil
}

func powerVSphere(ctx context.Context, id, state string) error {
	c, vm, err := findVSphereVM(ctx, id)
	if err != nil { return err }
	defer c.Logout(ctx)
	var task *object.Task
	if state == "on" { task, err = vm.PowerOn(ctx) } else { task, err = vm.PowerOff(ctx) }
	if err != nil { return err }
	return task.Wait(ctx)
}

func snapshotVSphere(ctx context.Context, id, name string) error {
	c, vm, err := findVSphereVM(ctx, id)
	if err != nil { return err }
	defer c.Logout(ctx)
	task, err := vm.CreateSnapshot(ctx, name, "Created by Infrastructure Sandbox Agent", false, false)
	if err != nil { return err }
	return task.Wait(ctx)
}

func removeVSphere(ctx context.Context, id string) error {
	c, vm, err := findVSphereVM(ctx, id)
	if err != nil { return err }
	defer c.Logout(ctx)
	task, err := vm.Destroy(ctx)
	if err != nil { return err }
	return task.Wait(ctx)
}

func cloneVSphereTemplate(ctx context.Context, req vmCreateRequest) (vmRecord, error) {
	templatePath := strings.TrimSpace(os.Getenv("VSPHERE_TEMPLATE_PATH"))
	if templatePath == "" { return vmRecord{}, errors.New("VSPHERE_TEMPLATE_PATH is required for cloning") }
	c, err := vsphereClient(ctx)
	if err != nil { return vmRecord{}, err }
	defer c.Logout(ctx)
	f := find.NewFinder(c.Client, true)
	template, err := f.VirtualMachine(ctx, templatePath)
	if err != nil { return vmRecord{}, err }
	folder, err := f.DefaultFolder(ctx)
	if err != nil { return vmRecord{}, err }
	pool, err := f.DefaultResourcePool(ctx)
	if err != nil { return vmRecord{}, err }
	spec := types.VirtualMachineCloneSpec{
		Location: types.VirtualMachineRelocateSpec{Pool: pool.Reference()},
		PowerOn: false,
		Template: false,
		Config: &types.VirtualMachineConfigSpec{NumCPUs: int32(req.CPUs), MemoryMB: int64(req.MemoryMiB)},
	}
	task, err := template.Clone(ctx, folder, req.Name, spec)
	if err != nil { return vmRecord{}, err }
	if err := task.Wait(ctx); err != nil { return vmRecord{}, err }
	vm, err := f.VirtualMachine(ctx, req.Name)
	if err != nil { return vmRecord{}, err }
	return vmRecord{ID: vm.Reference().Value, Name: req.Name, State: "poweredOff", Provider: "vsphere"}, nil
}

func hyperVClient() (*winrm.Client, error) {
	host := strings.TrimSpace(os.Getenv("HYPERV_WINRM_HOST"))
	user := os.Getenv("HYPERV_WINRM_USERNAME")
	password := os.Getenv("HYPERV_WINRM_PASSWORD")
	if host == "" || user == "" || password == "" { return nil, errors.New("Hyper-V WinRM provider is not configured") }
	if strings.ContainsAny(host, "/\\@") { return nil, errors.New("HYPERV_WINRM_HOST must be a hostname or IP, not a URL") }
	port := 5986
	if p := os.Getenv("HYPERV_WINRM_PORT"); p != "" {
		n, err := strconv.Atoi(p); if err != nil || n < 1 || n > 65535 { return nil, errors.New("invalid HYPERV_WINRM_PORT") }; port = n
	}
	endpoint := winrm.NewEndpoint(host, port, true, false, nil, nil, nil, 30*time.Second)
	encryption, err := winrm.NewEncryption("ntlm")
	if err != nil { return nil, err }
	params := *winrm.DefaultParameters
	// NTLM is encrypted at the WinRM message layer; TLS certificate verification remains enabled.
	params.TransportDecorator = func() winrm.Transporter { return encryption }
	return winrm.NewClientWithParameters(endpoint, user, password, &params)
}

func runHyperVPS(ctx context.Context, script string) (string, error) {
	client, err := hyperVClient()
	if err != nil { return "", err }
	encoded := base64.StdEncoding.EncodeToString(utf16LE(script))
	command := "powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy RemoteSigned -EncodedCommand " + encoded
	var stdout, stderr bytes.Buffer
	if err := client.RunWithContext(ctx, command, &stdout, &stderr); err != nil {
		return "", fmt.Errorf("WinRM command failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	result := strings.TrimSpace(stdout.String())
	if result == "" { result = strings.TrimSpace(stderr.String()) }
	return result, nil
}

func utf16LE(s string) []byte {
	units := utf16.Encode([]rune(s))
	out := make([]byte, len(units)*2)
	for i, v := range units { out[2*i] = byte(v); out[2*i+1] = byte(v >> 8) }
	return out
}

func listHyperV(ctx context.Context) ([]vmRecord, error) {
	script := "$ErrorActionPreference='Stop'; Get-VM | Select-Object @{n='id';e={$_.Id.Guid}},Name,State | ConvertTo-Json -Compress"
	out, err := runHyperVPS(ctx, script)
	if err != nil { return nil, err }
	if out == "" || out == "null" { return []vmRecord{}, nil }
	var rows []struct { ID string `json:"id"`; Name string `json:"Name"`; State string `json:"State"` }
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		var one struct { ID string `json:"id"`; Name string `json:"Name"`; State string `json:"State"` }
		if err := json.Unmarshal([]byte(out), &one); err != nil { return nil, fmt.Errorf("parse Hyper-V inventory: %w", err) }
		rows = append(rows, one)
	}
	result := make([]vmRecord, 0, len(rows))
	for _, row := range rows { result = append(result, vmRecord{ID: row.ID, Name: row.Name, State: row.State, Provider: "hyperv"}) }
	return result, nil
}

func createHyperVFromVHDX(ctx context.Context, req vmCreateRequest) (vmRecord, error) {
	template := strings.TrimSpace(os.Getenv("HYPERV_TEMPLATE_VHDX"))
	root := strings.TrimSpace(os.Getenv("HYPERV_VM_ROOT"))
	if template == "" || root == "" { return vmRecord{}, errors.New("HYPERV_TEMPLATE_VHDX and HYPERV_VM_ROOT are required") }
	if !strings.HasSuffix(strings.ToLower(template), ".vhdx") { return vmRecord{}, errors.New("Hyper-V template must be a VHDX") }
	if strings.ContainsAny(template+root, "'\r\n") { return vmRecord{}, errors.New("invalid Hyper-V path configuration") }
	switchName := strings.TrimSpace(os.Getenv("HYPERV_SWITCH_NAME"))
	switchLine := ""
	if switchName != "" {
		if strings.ContainsAny(switchName, "'\r\n") { return vmRecord{}, errors.New("invalid HYPERV_SWITCH_NAME") }
		switchLine = " -SwitchName '" + switchName + "'"
	}
	script := "$ErrorActionPreference='Stop'; " +
		"$name='" + req.Name + "'; $root='" + root + "'; $source='" + template + "'; " +
		"if (Get-VM -Name $name -ErrorAction SilentlyContinue) { throw 'A VM with this name already exists' }; " +
		"$dir=Join-Path $root $name; New-Item -ItemType Directory -Path $dir -Force | Out-Null; " +
		"$disk=Join-Path $dir ($name + '.vhdx'); Copy-Item -LiteralPath $source -Destination $disk -ErrorAction Stop; " +
		"$vm=New-VM -Name $name -Path $dir -MemoryStartupBytes (" + strconv.Itoa(req.MemoryMiB) + "MB) -Generation 2 -VHDPath $disk" + switchLine + "; " +
		"Set-VMProcessor -VMName $name -Count " + strconv.Itoa(req.CPUs) + "; " +
		"[pscustomobject]@{id=$vm.Id.Guid;Name=$vm.Name;State=$vm.State.ToString()} | ConvertTo-Json -Compress"
	out, err := runHyperVPS(ctx, script)
	if err != nil { return vmRecord{}, err }
	var row struct { ID string `json:"id"`; Name string `json:"Name"`; State string `json:"State"` }
	if err := json.Unmarshal([]byte(out), &row); err != nil { return vmRecord{}, fmt.Errorf("parse Hyper-V create response: %w", err) }
	return vmRecord{ID: row.ID, Name: row.Name, State: row.State, Provider: "hyperv"}, nil
}

func powerHyperV(ctx context.Context, id, state string) error {
	action := "Start-VM"
	if state == "off" { action = "Stop-VM -TurnOff" }
	_, err := runHyperVPS(ctx, "$ErrorActionPreference='Stop'; "+action+" -VM (Get-VM -Id '"+id+"') -Confirm:$false | Out-Null")
	return err
}

func snapshotHyperV(ctx context.Context, id, name string) error {
	_, err := runHyperVPS(ctx, "$ErrorActionPreference='Stop'; Checkpoint-VM -VM (Get-VM -Id '"+id+"') -SnapshotName '"+name+"'")
	return err
}

func removeHyperV(ctx context.Context, id string) error {
	_, err := runHyperVPS(ctx, "$ErrorActionPreference='Stop'; $vm=Get-VM -Id '"+id+"'; if ($vm.State -ne 'Off') { Stop-VM -VM $vm -TurnOff -Confirm:$false }; Remove-VM -VM $vm -Force -Confirm:$false")
	return err
}

// Keep an explicit output bound for remote provider commands; provider APIs never accept arbitrary shell text.
func boundedProviderOutput(s string) string {
	if len(s) > 64<<10 { return s[:64<<10] + "\n[truncated]" }
	return strings.TrimSpace(s)
}

