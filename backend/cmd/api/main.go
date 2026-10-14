package main

import (
 "context"
 "encoding/json"
 "fmt"
 "log"
 "net/http"
 "os"
 "os/exec"
 "strconv"
 "strings"
 "time"
)

type Profile struct { OS string; Name string; CPUs string; Memory_mib int }
type Sandbox struct { ID string; Name string; Image string; Status string; CreatedAt time.Time; Provider string }
type API struct{}
var profiles=map[string]string{"ubuntu-24.04":"ubuntu:24.04","debian-12":"debian:12-slim","alpine-3.22":"alpine:3.22"}

func main(){
 addr:=os.Getenv("SANDBOX_LISTEN_ADDR"); if addr=="" {addr="127.0.0.1:8080"}
 a:=&API{}; mux:=http.NewServeMux()
 mux.HandleFunc("GET /health",func(w http.ResponseWriter,r *http.Request){writeJSON(w,200,map[string]string{"status":"ok","provider":"docker-container"})})
 mux.HandleFunc("GET /api/profiles",func(w http.ResponseWriter,r *http.Request){out:=[]map[string]string{};for id,img:=range profiles{out=append(out,map[string]string{"id":id,"image":img})};writeJSON(w,200,out)})
 mux.HandleFunc("GET /api/sandboxes",a.list)
 mux.HandleFunc("POST /api/sandboxes",a.create)
 mux.HandleFunc("POST /api/sandboxes/{id}/stop",a.stop)
 mux.HandleFunc("DELETE /api/sandboxes/{id}",a.remove)
 s:=&http.Server{Addr:addr,Handler:headers(mux),ReadHeaderTimeout:5*time.Second}
 log.Printf("sandbox API listening on %s",addr);log.Fatal(s.ListenAndServe())
}
func docker(ctx context.Context,args ...string)([]byte,error){
 ctx,cancel:=context.WithTimeout(ctx,90*time.Second);defer cancel()
 out,err:=exec.CommandContext(ctx,"docker",args...).CombinedOutput()
 if err!=nil{return out,fmt.Errorf("docker command failed: %w: %s",err,strings.TrimSpace(string(out)))}
 return out,nil
}
func (a *API) create(w http.ResponseWriter,r *http.Request){
 var p Profile
 if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16<<10)).Decode(&p);err!=nil{writeError(w,400,"invalid JSON");return}
 image,ok:=profiles[p.OS];if !ok{writeError(w,400,"unsupported OS profile");return}
 if p.Name==""{p.Name="lab-"+strconv.FormatInt(time.Now().Unix(),10)}
 if !validName(p.Name){writeError(w,400,"name must be lowercase letters, digits and hyphens (max 48 chars)");return}
 if p.CPUs==""{p.CPUs="1"};cpu,err:=strconv.ParseFloat(p.CPUs,64);if err!=nil||cpu<0.25||cpu>2{writeError(w,400,"cpus must be between 0.25 and 2");return}
 if p.Memory_mib==0{p.Memory_mib=512};if p.Memory_mib<128||p.Memory_mib>2048{writeError(w,400,"memory_mib must be between 128 and 2048");return}
 args:=[]string{"run","-d","--name",p.Name,"--hostname",p.Name,"--label","infra-sandbox-agent.managed=true","--label","infra-sandbox-agent.profile="+p.OS,"--cpus",strconv.FormatFloat(cpu,'f',-1,64),"--memory",strconv.Itoa(p.Memory_mib)+"m","--pids-limit","256","--network","none","--restart","no",image,"sh","-c","while true; do sleep 3600; done"}
 out,err:=docker(r.Context(),args...);if err!=nil{writeError(w,502,err.Error());return}
 writeJSON(w,201,map[string]any{"id":strings.TrimSpace(string(out)),"name":p.Name,"image":image,"status":"running","created_at":time.Now().UTC(),"provider":"docker-container"})
}
func (a *API) list(w http.ResponseWriter,r *http.Request){
 out,err:=docker(r.Context(),"ps","-a","--filter","label=infra-sandbox-agent.managed=true","--format","{{.ID}}|{{.Names}}|{{.Image}}|{{.Status}}")
 if err!=nil{writeError(w,502,err.Error());return};rows:=[]map[string]string{}
 for _,line:=range strings.Split(strings.TrimSpace(string(out)),"\n"){if line==""{continue};p:=strings.SplitN(line,"|",4);if len(p)==4{rows=append(rows,map[string]string{"id":p[0],"name":p[1],"image":p[2],"status":p[3],"provider":"docker-container"})}}
 writeJSON(w,200,rows)
}
func (a *API) stop(w http.ResponseWriter,r *http.Request){id:=r.PathValue("id");if !validID(id){writeError(w,400,"invalid ID");return};out,e:=docker(r.Context(),"inspect","--format","{{ index .Config.Labels \"infra-sandbox-agent.managed\" }}",id);if e!=nil||strings.TrimSpace(string(out))!="true"{writeError(w,404,"managed sandbox not found");return};if _,e=docker(r.Context(),"stop","--time","10",id);e!=nil{writeError(w,502,e.Error());return};writeJSON(w,200,map[string]string{"id":id,"status":"stopped"})}
func (a *API) remove(w http.ResponseWriter,r *http.Request){
 id:=r.PathValue("id");if !validID(id){writeError(w,400,"invalid ID");return}
 out,e:=docker(r.Context(),"inspect","--format","{{ index .Config.Labels \"infra-sandbox-agent.managed\" }}",id)
 if e!=nil||strings.TrimSpace(string(out))!="true"{writeError(w,404,"managed sandbox not found");return}
 if _,e=docker(r.Context(),"rm","-f",id);e!=nil{writeError(w,502,e.Error());return};writeJSON(w,200,map[string]string{"id":id,"status":"removed"})
}
func validName(s string)bool{if len(s)<1||len(s)>48{return false};for _,r:=range s{if !(r>='a'&&r<='z')&&!(r>='0'&&r<='9')&&r!='-'{return false}};return s[0]!='-'&&s[len(s)-1]!='-'}
func validID(s string)bool{if s==""||len(s)>64{return false};for _,r:=range s{if !(r>='a'&&r<='z')&&!(r>='A'&&r<='Z')&&!(r>='0'&&r<='9')&&r!='-'&&r!='_'{return false}};return true}
func writeJSON(w http.ResponseWriter,c int,v any){w.Header().Set("Content-Type","application/json");w.Header().Set("Cache-Control","no-store");w.WriteHeader(c);_ = json.NewEncoder(w).Encode(v)}
func writeError(w http.ResponseWriter,c int,msg string){writeJSON(w,c,map[string]string{"error":msg})}
func headers(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("X-Content-Type-Options","nosniff");if r.Method!="GET"&&r.Method!="HEAD"&&r.Header.Get("Content-Type")!="application/json"{writeError(w,415,"Content-Type must be application/json");return};next.ServeHTTP(w,r)})}
