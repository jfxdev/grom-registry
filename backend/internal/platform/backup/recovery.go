package backup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

type RecoveryOptions struct {
	ListenAddress            string
	BackupRoot               string
	GromDataTarget           string
	SigningCertsTarget       string
	RegistryDataTarget       string
	DistributionConfigTarget string
	TargetGromVersion        string
	PostgresDatabaseURL      string
	MaxUploadBytes           int64
}

func ServeRecovery(ctx context.Context, options RecoveryOptions) error {
	if options.MaxUploadBytes <= 0 {
		return fmt.Errorf("recovery upload limit must be positive")
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("create recovery request token: %w", err)
	}
	requestToken := hex.EncodeToString(tokenBytes)
	handler := newRecoveryHandler(options, requestToken)
	server := &http.Server{
		Addr: options.ListenAddress, Handler: handler,
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second,
	}
	errorsCh := make(chan error, 1)
	go func() { errorsCh <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errorsCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func newRecoveryHandler(options RecoveryOptions, requestToken string) http.Handler {
	var restoring atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("X-Frame-Options", "DENY")
		_, _ = io.WriteString(w, recoveryHTML(requestToken, options.TargetGromVersion))
	})
	mux.HandleFunc("GET /api/backups", func(w http.ResponseWriter, _ *http.Request) {
		summaries, err := List(options.BackupRoot)
		if err != nil {
			writeAgentError(w, http.StatusInternalServerError)
			return
		}
		writeAgentJSON(w, http.StatusOK, summaries)
	})
	mux.HandleFunc("POST /api/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Grom-Recovery-Token") != requestToken {
			writeAgentError(w, http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, options.MaxUploadBytes+(16<<20))
		summary, err := Unbundle(options.BackupRoot, r.Body, options.MaxUploadBytes)
		if err != nil {
			writeAgentJSON(w, http.StatusBadRequest, map[string]string{"error": "The uploaded bundle is incomplete, unsafe, or corrupt"})
			return
		}
		writeAgentJSON(w, http.StatusCreated, summary)
	})
	mux.HandleFunc("POST /api/restore", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Grom-Recovery-Token") != requestToken {
			writeAgentError(w, http.StatusForbidden)
			return
		}
		if !restoring.CompareAndSwap(false, true) {
			writeAgentJSON(w, http.StatusConflict, map[string]string{"error": "A restore is already running"})
			return
		}
		defer restoring.Store(false)
		var input struct {
			BackupID     string `json:"backupId"`
			Confirmation string `json:"confirmation"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || input.Confirmation != "RESTORE" {
			writeAgentJSON(w, http.StatusBadRequest, map[string]string{"error": "Type RESTORE to confirm"})
			return
		}
		summary, err := findSummary(options.BackupRoot, input.BackupID)
		if err != nil {
			writeAgentError(w, http.StatusNotFound)
			return
		}
		inspection, err := Restore(RestoreOptions{
			BackupPath:               summary.Path,
			GromDataTarget:           options.GromDataTarget,
			SigningCertsTarget:       options.SigningCertsTarget,
			RegistryDataTarget:       options.RegistryDataTarget,
			DistributionConfigTarget: options.DistributionConfigTarget,
			TargetGromVersion:        options.TargetGromVersion,
			PostgresDatabaseURL:      options.PostgresDatabaseURL,
		})
		if err != nil {
			writeAgentJSON(w, http.StatusConflict, map[string]string{
				"error": "Restore was refused. Verify the bundle version and confirm every target volume is empty.",
			})
			return
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{
			"backupId": inspection.Manifest.BackupID,
			"status":   "restored",
		})
	})
	return mux
}

func recoveryHTML(requestToken, version string) string {
	tokenJSON, _ := json.Marshal(requestToken)
	versionJSON, _ := json.Marshal(version)
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Grom recovery</title>
  <style>
    :root{color-scheme:dark;--bg:#090d0b;--surface:#111512;--raised:#171b16;--text:#f2ede3;--muted:#aaa39a;--green:#91ad24;--green2:#abc936;--bronze:#a97832;--danger:#db6a65}
    *{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at 50% -20%,#1c2617 0,transparent 38%),var(--bg);color:var(--text);font:15px/1.5 Inter,ui-sans-serif,system-ui,sans-serif}
    main{width:min(900px,calc(100% - 32px));margin:48px auto}.brand{display:flex;align-items:center;gap:12px;margin-bottom:28px}.crest{display:grid;width:42px;height:42px;place-items:center;border:1px solid var(--bronze);border-radius:12px;background:#151b14;color:var(--green2);font-weight:800}.brand small{display:block;color:var(--muted)}
    h1{margin:0;font-size:28px}.lead{max-width:680px;color:var(--muted)}.warning{margin:24px 0;border:1px solid color-mix(in srgb,var(--bronze) 45%,transparent);border-radius:12px;background:color-mix(in srgb,var(--bronze) 8%,transparent);padding:14px 16px}
    .grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}.card{border:1px solid #2a302a;border-radius:14px;background:var(--surface);padding:20px;box-shadow:0 16px 40px rgba(0,0,0,.18)}h2{margin:0 0 8px;font-size:17px}.muted{color:var(--muted);font-size:13px}
    input[type=file],input[type=text],select{width:100%;min-height:46px;margin-top:12px;border:1px solid #343a34;border-radius:9px;background:var(--raised);color:var(--text);padding:10px}button{min-height:44px;margin-top:14px;border:1px solid #bfd84b;border-radius:10px;background:linear-gradient(#b5d43e,var(--green));color:#111;padding:0 16px;font-weight:750;box-shadow:0 3px 0 #40540e;cursor:pointer}button:active{transform:translateY(3px);box-shadow:none}button:disabled{opacity:.5;cursor:not-allowed}.secondary{border-color:#414741;background:var(--raised);color:var(--text);box-shadow:0 2px 0 #070a08}
    .list{display:grid;gap:8px;margin-top:12px}.item{display:flex;gap:10px;align-items:flex-start;border:1px solid #2b312b;border-radius:9px;padding:10px}.item code{display:block;color:var(--muted);font-size:11px;word-break:break-all}.status{min-height:24px;margin-top:14px;color:var(--muted)}.status.error{color:var(--danger)}.status.ok{color:#c9df6c}
    @media(max-width:700px){main{margin:24px auto}.grid{grid-template-columns:1fr}}
  </style>
</head>
<body>
<main>
  <div class="brand"><div class="crest">G</div><div><strong>Grom Registry</strong><small>Isolated recovery mode</small></div></div>
  <h1>Restore an installation</h1>
  <p class="lead">This interface runs from the same Grom image and never needs the source repository. Keep the normal Grom and Distribution containers stopped, and mount only empty replacement volumes.</p>
  <div class="warning"><strong>Destructive boundary:</strong> recovery refuses non-empty targets. After success, stop this container before starting the normal stack.</div>
  <div class="grid">
    <section class="card">
      <h2>1. Supply a recovery bundle</h2>
      <p class="muted">Select a bundle downloaded from Backup &amp; recovery. It is validated before becoming selectable.</p>
      <input id="bundle" type="file" accept=".tar,application/x-tar">
      <button id="upload" class="secondary">Upload and verify</button>
      <p id="upload-status" class="status" role="status"></p>
    </section>
    <section class="card">
      <h2>2. Choose a recovery point</h2>
      <p class="muted">Only complete, checksummed bundles compatible with Grom ` + html.EscapeString(version) + ` are accepted.</p>
      <div id="backups" class="list"></div>
    </section>
    <section class="card" style="grid-column:1/-1">
      <h2>3. Confirm restore</h2>
      <p class="muted">Type RESTORE. The operation restores SQLite, signing material, registry content, and Distribution configuration.</p>
      <input id="confirmation" type="text" autocomplete="off" placeholder="RESTORE">
      <button id="restore" disabled>Restore selected backup</button>
      <p id="restore-status" class="status" role="status"></p>
    </section>
  </div>
</main>
<script>
const token=` + string(tokenJSON) + `,version=` + string(versionJSON) + `;
let selected='';
const backups=document.getElementById('backups'),restore=document.getElementById('restore'),confirmation=document.getElementById('confirmation');
const escapeHTML=(value)=>String(value).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
async function load(){const response=await fetch('/api/backups');const items=await response.json();backups.innerHTML=items.length?items.map(item=>'<label class="item"><input type="radio" name="backup" value="'+escapeHTML(item.backupId)+'"><span><strong>'+new Date(item.createdAt).toLocaleString()+'</strong><code>'+escapeHTML(item.backupId)+'</code><span class="muted">'+escapeHTML(item.gromVersion)+' · '+item.totalBytes.toLocaleString()+' bytes</span></span></label>').join(''):'<p class="muted">No verified bundle is available yet.</p>';backups.querySelectorAll('input').forEach(input=>input.onchange=()=>{selected=input.value;sync()})}
function sync(){restore.disabled=!selected||confirmation.value!=='RESTORE'}
confirmation.oninput=sync;
document.getElementById('upload').onclick=async()=>{const file=document.getElementById('bundle').files[0],status=document.getElementById('upload-status');if(!file){status.textContent='Choose a .tar bundle first.';status.className='status error';return}status.textContent='Uploading and verifying…';status.className='status';try{const response=await fetch('/api/upload',{method:'POST',headers:{'Content-Type':'application/x-tar','X-Grom-Recovery-Token':token},body:file});const body=await response.json();if(!response.ok)throw new Error(body.error||'Upload failed');status.textContent='Bundle verified.';status.className='status ok';await load()}catch(error){status.textContent=error.message;status.className='status error'}};
restore.onclick=async()=>{const status=document.getElementById('restore-status');restore.disabled=true;status.textContent='Restoring and verifying target volumes…';status.className='status';try{const response=await fetch('/api/restore',{method:'POST',headers:{'Content-Type':'application/json','X-Grom-Recovery-Token':token},body:JSON.stringify({backupId:selected,confirmation:confirmation.value})});const body=await response.json();if(!response.ok)throw new Error(body.error||'Restore failed');status.textContent='Restore complete. Stop this recovery container, then start the normal Grom stack.';status.className='status ok'}catch(error){status.textContent=error.message;status.className='status error';sync()}};
load();
</script>
</body>
</html>`
}
