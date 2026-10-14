package main

import (
 "context"
 "crypto/subtle"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "log/slog"
 "net"
 "net/http"
 "os"
 "os/exec"
 "strconv"
 "strings"
 "time"
)

const managedLabel = "infra-sandbox-agent.managed"
type Profile struct {
 OS string `json:"os"`
 Name string `json:"name"`
 CPUs string `json:"cpus"`
 MemoryMiB int `json:"memory_mib"`
}
type API struct { token string }
type DiagnosticRequest struct { Command string `json:"command"` }
var profiles=map[string]string{"ubuntu-24.04":"ubuntu:24.04","debian-12":"debian:12-slim","alpine-3.22":"alpine:3.22"}

func main(){
 token:=strings.TrimSpace(os.Getenv("SANDBOX_API_TOKEN"))
 if len(token)<32 {slog.Error("SANDBOX_API_TOKEN is required and must contain at least 32 characters");os.Exit(2)}
 addr:=os.Getenv("SANDBOX_LISTEN_ADDR");if addr==""{addr="127.0.0.1:8080"}
 if _,_,err:=net.SplitHostPort(addr);err!=nil{slog.Error("invalid SANDBOX_LISTEN_ADDR","error",err);os.Exit(2)}
 a:=&API{token:token};mux:=http.NewServeMux()
 mux.HandleFunc("GET /health/live",func(w http.ResponseWriter,r *http.Request){writeJSON(w,200,map[string]string{"status":"ok"})})
 mux.Handle("GET /health/ready",a.auth(http.HandlerFunc(a.ready)))
 mux.Handle("GET /api/profiles",a.auth(http.HandlerFunc(a.listProfiles)))
 mux.Handle("GET /api/sandboxes",a.auth(http.HandlerFunc(a.list)))
 mux.Handle("POST /api/sandboxes",a.auth(http.HandlerFunc(a.create)))
 mux.Handle("POST /api/sandboxes/{id}/stop",a.auth(http.HandlerFunc(a.stop)))
 mux.Handle("POST /api/sandboxes/{id}/diagnostics",a.auth(http.HandlerFunc(a.diagnostics)))
 mux.Handle("DELETE /api/sandboxes/{id}",a.auth(http.HandlerFunc(a.remove)))
 s:=&http.Server{Addr:addr,Handler:securityHeaders(mux),ReadHeaderTimeout:5*time.Second,ReadTimeout:15*time.Second,WriteTimeout:100*time.Second,IdleTimeout:60*time.Second,MaxHeaderBytes:1<<20}
 slog.Info("sandbox API starting","address",addr)
 if err:=s.ListenAndServe();!errors.Is(err,http.ErrServerClosed){slog.Error("API stopped","error",err);os.Exit(1)}
}
func(a *API)auth(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
 const prefix="Bearer ";h:=r.Header.Get("Authorization")
 if !strings.HasPrefix(h,prefix){w.Header().Set("WWW-Authenticate","Bearer");writeError(w,401,"bearer token required");return}
 if subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(h,prefix)),[]byte(a.token))!=1{writeError(w,401,"invalid bearer token");return}
 next.ServeHTTP(w,r)
})}
func(a *API)ready(w http.ResponseWriter,r *http.Request){ctx,cancel:=context.WithTimeout(r.Context(),3*time.Second);defer cancel();if _,e:=docker(ctx,"info","--format","{{.ServerVersion}}");e!=nil{writeError(w,503,"container runtime unavailable");return};writeJSON(w,200,map[string]string{"status":"ready","provider":"docker-container"})}
func(a *API)listProfiles(w http.ResponseWriter,r *http.Request){out:=make([]map[string]string,0,len(profiles));for id,img:=range profiles{out=append(out,map[string]string{"id":id,"image":img})};writeJSON(w,200,out)}
func docker(ctx context.Context,args ...string)([]byte,error){ctx,cancel:=context.WithTimeout(ctx,90*time.Second);defer cancel();out,e:=exec.CommandContext(ctx,"docker",args...).CombinedOutput();if e!=nil{return out,fmt.Errorf("container runtime operation failed: %w: %s",e,strings.TrimSpace(string(out)))};return out,nil}
func(a *API)create(w http.ResponseWriter,r *http.Request){
 var p Profile;d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16<<10));d.DisallowUnknownFields()
 if e:=d.Decode(&p);e!=nil{writeError(w,400,"invalid JSON request");return}
 var extra any;if e:=d.Decode(&extra);e!=io.EOF{writeError(w,400,"request must contain exactly one JSON object");return}
 image,ok:=profiles[p.OS];if !ok{writeError(w,400,"unsupported OS profile");return}
 if p.Name==""{p.Name="lab-"+strconv.FormatInt(time.Now().UnixNano(),36)}
 if !validName(p.Name){writeError(w,400,"name must be lowercase letters, digits and hyphens (max 48 chars)");return}
 if p.CPUs==""{p.CPUs="1"};cpu,e:=strconv.ParseFloat(p.CPUs,64);if e!=nil||cpu<0.25||cpu>2{writeError(w,400,"cpus must be between 0.25 and 2");return}
 if p.MemoryMiB==0{p.MemoryMiB=512};if p.MemoryMiB<128||p.MemoryMiB>2048{writeError(w,400,"memory_mib must be between 128 and 2048");return}
 args:=[]string{"run","-d","--name",p.Name,"--label",managedLabel+"=true","--label","infra-sandbox-agent.profile="+p.OS,"--cpus",strconv.FormatFloat(cpu,'f',-1,64),"--memory",strconv.Itoa(p.MemoryMiB)+"m","--pids-limit","256","--network","none","--read-only","--cap-drop","ALL","--security-opt","no-new-privileges:true","--user","65534:65534","--tmpfs","/tmp:rw,noexec,nosuid,size=64m","--restart","no",image,"sh","-c","while true; do sleep 3600; done"}
 out,e:=docker(r.Context(),args...);if e!=nil{slog.Warn("sandbox creation failed","profile",p.OS,"error",e);writeError(w,502,"sandbox creation failed; inspect service logs");return}
 id:=strings.TrimSpace(string(out));slog.Info("sandbox created","id",id,"profile",p.OS,"cpus",cpu,"memory_mib",p.MemoryMiB)
 writeJSON(w,201,map[string]any{"id":id,"name":p.Name,"image":image,"status":"running","created_at":time.Now().UTC(),"provider":"docker-container","fidelity":"container-functional"})
}
func(a *API)list(w http.ResponseWriter,r *http.Request){out,e:=docker(r.Context(),"ps","-a","--filter","label="+managedLabel+"=true","--format","{{.ID}}|{{.Names}}|{{.Image}}|{{.Status}}");if e!=nil{writeError(w,502,"container inventory unavailable");return};rows:=make([]map[string]string,0);for _,line:=range strings.Split(strings.TrimSpace(string(out)),"\n"){if line==""{continue};p:=strings.SplitN(line,"|",4);if len(p)==4{rows=append(rows,map[string]string{"id":p[0],"name":p[1],"image":p[2],"status":p[3],"provider":"docker-container","fidelity":"container-functional"})}};writeJSON(w,200,rows)}
func managed(ctx context.Context,id string)bool{out,e:=docker(ctx,"inspect","--format","{{.Config.Labels.infra-sandbox-agent.managed}}",id);return e==nil&&strings.TrimSpace(string(out))=="true"}
func(a *API)stop(w http.ResponseWriter,r *http.Request){id:=r.PathValue("id");if !validID(id)||!managed(r.Context(),id){writeError(w,404,"managed sandbox not found");return};if _,e:=docker(r.Context(),"stop","--time","10",id);e!=nil{writeError(w,502,"sandbox stop failed");return};slog.Info("sandbox stopped","id",id);writeJSON(w,200,map[string]string{"id":id,"status":"stopped"})}
func(a *API)diagnostics(w http.ResponseWriter,r *http.Request){
 id:=r.PathValue("id")
 if !validID(id)||!managed(r.Context(),id){writeError(w,404,"managed sandbox not found");return}
 var req DiagnosticRequest
 d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,2<<10));d.DisallowUnknownFields()
 if err:=d.Decode(&req);err!=nil{writeError(w,400,"invalid JSON request");return}
 commands:=map[string][]string{
  "identity":{"id"},
  "os-release":{"cat","/etc/os-release"},
  "kernel":{"uname","-a"},
  "filesystem":{"df","-h"},
  "working-directory":{"pwd"},
  "root-files":{"ls","-la","/"},
 }
 args,ok:=commands[req.Command];if !ok{writeError(w,400,"diagnostic command is not allowlisted");return}
 out,err:=docker(r.Context(),append([]string{"exec",id},args...)...)
 if err!=nil{slog.Warn("sandbox diagnostic failed","id",id,"command",req.Command,"error",err);writeError(w,502,"diagnostic failed; command may not exist in this image");return}
 result:=strings.TrimSpace(string(out));if len(result)>16<<10{result=result[:16<<10]+"\\n[output truncated]"}
 writeJSON(w,200,map[string]string{"id":id,"command":req.Command,"output":result})
}
func(a *API)remove(w http.ResponseWriter,r *http.Request){id:=r.PathValue("id");if !validID(id)||!managed(r.Context(),id){writeError(w,404,"managed sandbox not found");return};if _,e:=docker(r.Context(),"rm","-f",id);e!=nil{writeError(w,502,"sandbox removal failed");return};slog.Info("sandbox removed","id",id);writeJSON(w,200,map[string]string{"id":id,"status":"removed"})}
func validName(s string)bool{if len(s)<1||len(s)>48||s[0]=='-'||s[len(s)-1]=='-'{return false};for _,r:=range s{if !(r>='a'&&r<='z')&&!(r>='0'&&r<='9')&&r!='-'{return false}};return true}
func validID(s string)bool{if s==""||len(s)>64{return false};for _,r:=range s{if !(r>='a'&&r<='z')&&!(r>='A'&&r<='Z')&&!(r>='0'&&r<='9')&&r!='-'&&r!='_'{return false}};return true}
func writeJSON(w http.ResponseWriter,c int,v any){w.Header().Set("Content-Type","application/json");w.Header().Set("Cache-Control","no-store");w.WriteHeader(c);_ = json.NewEncoder(w).Encode(v)}
func writeError(w http.ResponseWriter,c int,m string){writeJSON(w,c,map[string]string{"error":m})}
func securityHeaders(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("X-Content-Type-Options","nosniff");w.Header().Set("X-Frame-Options","DENY");w.Header().Set("Referrer-Policy","no-referrer");if r.Method!=http.MethodGet&&r.Method!=http.MethodHead&&r.Header.Get("Content-Type")!="application/json"{writeError(w,415,"Content-Type must be application/json");return};next.ServeHTTP(w,r)})}
