package main

import (
 "net/http"
 "net/http/httptest"
 "testing"
)

func TestValidName(t *testing.T) {
 cases:=[]struct{name string; want bool}{
  {"ubuntu-lab-1",true},{"a",true},{"",false},{"-bad",false},{"bad-",false},{"Uppercase",false},{"has space",false},
 }
 for _,tc:=range cases {if got:=validName(tc.name);got!=tc.want{t.Errorf("validName(%q)=%v, want %v",tc.name,got,tc.want)}}
}
func TestValidID(t *testing.T) {
 cases:=[]struct{id string;want bool}{{"abc123",true},{"abcdef-123",true},{"",false},{"id/../../etc",false},{"with space",false}}
 for _,tc:=range cases {if got:=validID(tc.id);got!=tc.want{t.Errorf("validID(%q)=%v, want %v",tc.id,got,tc.want)}}
}
func TestAuthentication(t *testing.T) {
 api:=&API{token:"0123456789abcdef0123456789abcdef"}
 handler:=api.auth(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(http.StatusNoContent)}))
 cases:=[]struct{name,header string;want int}{
  {"missing","",http.StatusUnauthorized},
  {"wrong","Bearer wrong-token",http.StatusUnauthorized},
  {"correct","Bearer 0123456789abcdef0123456789abcdef",http.StatusNoContent},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  req:=httptest.NewRequest(http.MethodGet,"/",nil);if tc.header!=""{req.Header.Set("Authorization",tc.header)}
  rec:=httptest.NewRecorder();handler.ServeHTTP(rec,req);if rec.Code!=tc.want{t.Fatalf("status=%d want=%d",rec.Code,tc.want)}
 })}
}
