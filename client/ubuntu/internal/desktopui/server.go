// Package desktopui provides the loopback-only Ubuntu browser client.
package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	sharediskv1 "github.com/share-disk/share-disk/contracts/proto/sharedisk/v1"
)

type localHandler interface {
	Handle(context.Context, *sharediskv1.LocalRequest) *sharediskv1.LocalResponse
}

type Server struct {
	handler localHandler
	tempDir string
	server  *http.Server
	ln      net.Listener
}

func New(address, tempDir string, handler localHandler) (*Server, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || (host != "127.0.0.1" && host != "::1" && host != "localhost") {
		return nil, fmt.Errorf("desktop UI address must be loopback: %q", address)
	}
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	s := &Server{handler: handler, tempDir: tempDir, ln: ln}
	r := chi.NewRouter()
	r.Get("/", s.index)
	r.Get("/api/status", s.status)
	r.Get("/api/files", s.files)
	r.Post("/api/import", s.sameOrigin(s.importFile))
	r.Post("/api/files/{id}/action", s.sameOrigin(s.fileAction))
	r.Get("/api/files/{id}/download", s.download)
	s.server = &http.Server{Handler: securityHeaders(r), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 90 * time.Second}
	return s, nil
}

func (s *Server) Address() string { return "http://" + s.ln.Addr().String() }
func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdown)
	}()
	err := s.server.Serve(s.ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
func (s *Server) Close() error { return s.server.Close() }

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; object-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" || (origin != "http://"+r.Host && origin != "http://localhost:"+portOf(r.Host)) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func portOf(hostport string) string { _, port, _ := net.SplitHostPort(hostport); return port }

func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, desktopHTML)
}

func (s *Server) call(ctx context.Context, req *sharediskv1.LocalRequest) (*sharediskv1.LocalResponse, bool) {
	resp := s.handler.Handle(ctx, req)
	return resp, resp != nil && resp.GetError() == nil
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	resp, ok := s.call(r.Context(), &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_Status{Status: &sharediskv1.StatusRequest{}}})
	if !ok {
		writeProtoError(w, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp.GetStatus())
}

func (s *Server) files(w http.ResponseWriter, r *http.Request) {
	active, ok := s.call(r.Context(), listRequest(false))
	if !ok {
		writeProtoError(w, active)
		return
	}
	trash, ok := s.call(r.Context(), listRequest(true))
	if !ok {
		writeProtoError(w, trash)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"files": active.GetListLocalFiles().GetFiles(), "trash": trash.GetListLocalFiles().GetFiles()})
}

func listRequest(trash bool) *sharediskv1.LocalRequest {
	return &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_ListLocalFiles{ListLocalFiles: &sharediskv1.ListLocalFilesRequest{Trash: trash}}}
}

func (s *Server) importFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<30)
	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "invalid upload", http.StatusBadRequest)
		return
	}
	for {
		part, partErr := reader.NextPart()
		if partErr == io.EOF {
			break
		}
		if partErr != nil {
			http.Error(w, "invalid upload", http.StatusBadRequest)
			return
		}
		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			continue
		}
		temp, createErr := os.CreateTemp(s.tempDir, ".desktop-import-*")
		if createErr != nil {
			http.Error(w, "cannot create import", http.StatusInternalServerError)
			return
		}
		path := temp.Name()
		_, copyErr := io.Copy(temp, part)
		closeErr := temp.Close()
		_ = part.Close()
		defer os.Remove(path)
		if copyErr != nil || closeErr != nil {
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}
		name := filepath.Base(part.FileName())
		resp, ok := s.call(r.Context(), &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_Import{Import: &sharediskv1.ImportRequest{SourcePath: path, Name: name}}})
		if !ok {
			writeProtoError(w, resp)
			return
		}
		writeJSON(w, http.StatusCreated, resp.GetImport())
		return
	}
	http.Error(w, "file is required", http.StatusBadRequest)
}

func (s *Server) fileAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	resp, ok := s.call(r.Context(), &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_MutateLocalFile{MutateLocalFile: &sharediskv1.MutateLocalFileRequest{FileId: chi.URLParam(r, "id"), Action: body.Action, Name: body.Name}}})
	if !ok {
		writeProtoError(w, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp.GetMutateLocalFile())
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	dir, err := os.MkdirTemp(s.tempDir, ".desktop-export-*")
	if err != nil {
		http.Error(w, "download failed", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(dir)
	name := filepath.Base(r.URL.Query().Get("name"))
	if name == "." || name == "" {
		name = "share-disk-file"
	}
	name = strings.NewReplacer("\r", "", "\n", "", `"`, "").Replace(name)
	destination := filepath.Join(dir, name)
	resp, ok := s.call(r.Context(), &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_Export{Export: &sharediskv1.ExportRequest{FileId: chi.URLParam(r, "id"), DestinationPath: destination}}})
	if !ok {
		writeProtoError(w, resp)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, destination)
}

func writeProtoError(w http.ResponseWriter, resp *sharediskv1.LocalResponse) {
	message := "operation failed"
	code := "INTERNAL"
	if resp != nil && resp.GetError() != nil {
		message = resp.GetError().GetMessage()
		code = resp.GetError().GetCode()
	}
	writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": map[string]string{"code": code, "message": message}})
}
func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

const desktopHTML = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Share Disk Ubuntu</title><style>
:root{--ink:#18231d;--muted:#69776f;--green:#126b4e;--mint:#e0f4e9;--bg:#f4f7f5;--line:#e0e7e2;--white:#fff;--red:#ad3d34}*{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at 90% 0,#d8f3e5,transparent 30%),var(--bg);color:var(--ink);font:14px/1.5 Inter,system-ui,-apple-system,"Segoe UI",sans-serif}.app{max-width:1180px;margin:auto;padding:28px}.top{display:flex;align-items:center;justify-content:space-between;margin-bottom:28px}.brand{display:flex;align-items:center;gap:12px}.logo{display:grid;place-items:center;width:44px;height:44px;border-radius:14px;background:var(--green);color:#fff;font-size:20px;font-weight:900}.brand h1{font-size:21px;margin:0}.brand small{color:var(--muted)}button,.button{border:0;border-radius:12px;padding:11px 15px;background:var(--mint);color:var(--green);font:inherit;font-weight:700;cursor:pointer}.primary{background:var(--green);color:#fff}.danger{background:#f9e9e7;color:var(--red)}.hero{display:grid;grid-template-columns:1.6fr 1fr;gap:16px;margin-bottom:18px}.welcome,.status,.panel{background:rgba(255,255,255,.92);border:1px solid var(--line);border-radius:20px;box-shadow:0 15px 45px rgba(25,63,45,.08)}.welcome{padding:24px}.welcome h2{font-size:28px;margin:0 0 5px}.welcome p{color:var(--muted);margin:0}.status{padding:22px;display:flex;align-items:center;gap:13px}.dot{width:11px;height:11px;border-radius:50%;background:#2bb779;box-shadow:0 0 0 7px #dcf5e9}.panel{overflow:hidden}.head{display:flex;align-items:center;justify-content:space-between;padding:18px 21px;border-bottom:1px solid var(--line)}.head h3{margin:0;font-size:18px}.tabs{display:flex;gap:7px}.tabs button.active{background:var(--green);color:#fff}.upload input{display:none}.table{overflow:auto}table{width:100%;border-collapse:collapse;white-space:nowrap}th,td{padding:14px 20px;text-align:left;border-bottom:1px solid var(--line)}th{font-size:12px;color:var(--muted)}.name{font-weight:750}.sub{display:block;color:var(--muted);font-size:12px}.actions{display:flex;gap:6px}.empty{text-align:center;padding:55px;color:var(--muted)}.toast{position:fixed;right:22px;bottom:22px;padding:13px 16px;border-radius:12px;background:#14271e;color:#fff;opacity:0;transform:translateY(10px);transition:.2s}.toast.show{opacity:1;transform:none}@media(max-width:700px){.app{padding:17px}.hero{grid-template-columns:1fr}.welcome h2{font-size:23px}.head{align-items:flex-start;gap:12px;flex-direction:column}.top{align-items:flex-start}.brand small{display:none}}
</style></head><body><main class="app"><header class="top"><div class="brand"><div class="logo">S</div><div><h1>Share Disk</h1><small>Ubuntu 本地文件客户端</small></div></div><label class="button primary upload">导入文件<input id="picker" type="file"></label></header><section class="hero"><article class="welcome"><h2>你的本地文件空间</h2><p>文件正文保存在这台 Ubuntu，操作由本机 Agent 安全执行。</p></article><article class="status"><span class="dot"></span><div><b id="state">正在连接 Agent…</b><span class="sub" id="detail"></span></div></article></section><section class="panel"><div class="head"><h3>文件</h3><div class="tabs"><button class="active" data-view="files">全部文件</button><button data-view="trash">回收站</button><button id="refresh">刷新</button></div></div><div class="table"><table><thead><tr><th>名称</th><th>大小</th><th>状态</th><th>SHA-256</th><th>操作</th></tr></thead><tbody id="rows"></tbody></table><div class="empty" id="empty">暂无文件</div></div></section></main><div class="toast" id="toast"></div><script>
const $=s=>document.querySelector(s),$$=s=>document.querySelectorAll(s);let data={files:[],trash:[]},view='files';const esc=v=>String(v||'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));const size=n=>n<1024?n+' B':n<1048576?(n/1024).toFixed(1)+' KB':n<1073741824?(n/1048576).toFixed(1)+' MB':(n/1073741824).toFixed(1)+' GB';function toast(v){$('#toast').textContent=v;$('#toast').classList.add('show');setTimeout(()=>$('#toast').classList.remove('show'),2500)}async function api(url,opt){const r=await fetch(url,opt);let d={};try{d=await r.json()}catch(e){}if(!r.ok)throw Error((d.error&&d.error.message)||'操作失败');return d}function render(){const list=data[view]||[];$('#rows').innerHTML=list.map(f=>'<tr><td><span class="name">'+esc(f.name)+'</span><span class="sub">'+esc(f.mime||'application/octet-stream')+'</span></td><td>'+size(f.size)+'</td><td>'+esc(f.status)+'</td><td>'+esc(f.sha256.slice(0,14))+'…</td><td><div class="actions">'+(view==='files'?'<a class="button" href="/api/files/'+encodeURIComponent(f.id)+'/download?name='+encodeURIComponent(f.name)+'">下载</a><button data-action="rename" data-id="'+esc(f.id)+'">重命名</button><button class="danger" data-action="trash" data-id="'+esc(f.id)+'">删除</button>':'<button data-action="restore" data-id="'+esc(f.id)+'">恢复</button><button class="danger" data-action="purge" data-id="'+esc(f.id)+'">永久删除</button>')+'</div></td></tr>').join('');$('#empty').style.display=list.length?'none':'block';$$('[data-action]').forEach(b=>b.onclick=()=>act(b.dataset.id,b.dataset.action))}async function load(){try{data=await api('/api/files');const s=await api('/api/status');$('#state').textContent='Agent 正常运行';$('#detail').textContent=s.active_transfers+' 个活跃传输 · '+s.version;render()}catch(e){$('#state').textContent='需要完成 Agent 配置';$('#detail').textContent=e.message;toast(e.message)}}async function act(id,action){let name='';if(action==='rename'){name=prompt('输入新文件名');if(!name)return}if((action==='trash'||action==='purge')&&!confirm(action==='purge'?'永久删除后不可恢复，继续？':'移到回收站？'))return;try{await api('/api/files/'+encodeURIComponent(id)+'/action',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action,name})});toast('操作成功');load()}catch(e){toast(e.message)}}$$('[data-view]').forEach(b=>b.onclick=()=>{view=b.dataset.view;$$('[data-view]').forEach(x=>x.classList.remove('active'));b.classList.add('active');render()});$('#refresh').onclick=load;$('#picker').onchange=async e=>{if(!e.target.files.length)return;const form=new FormData();form.append('file',e.target.files[0]);try{await api('/api/import',{method:'POST',body:form});toast('导入完成并已校验');load()}catch(err){toast(err.message)}e.target.value=''};load()
</script></body></html>`
