package main

import (
	"fmt"
	"strings"
)

func shortcutSessionPageHTML(sessionID string) string {
	safe := strings.ReplaceAll(sessionID, "\\", "\\\\")
	safe = strings.ReplaceAll(safe, "\"", "\\\"")
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0">
<title>File Bridge — Backup session</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#0f172a;color:#e2e8f0;padding:20px;min-height:100vh}
h1{font-size:1.25rem;color:#38bdf8;margin-bottom:8px;text-align:center}
.sub{text-align:center;color:#94a3b8;font-size:.85rem;margin-bottom:20px}
.bar{height:10px;background:#334155;border-radius:5px;overflow:hidden;margin:12px 0}
.fill{height:100%%;background:#38bdf8;width:0%%;transition:width .3s}
.stats{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin:16px 0}
.stat{background:#1e293b;padding:12px;border-radius:8px;text-align:center}
.stat b{display:block;font-size:1.4rem;color:#38bdf8}
.ready{color:#4ade80;text-align:center;font-weight:600;margin-top:16px;display:none}
.err{color:#f87171;font-size:.85rem;margin-top:12px}
</style>
</head>
<body>
<h1>File Bridge backup</h1>
<p class="sub" id="label">Session</p>
<div class="bar"><div class="fill" id="fill"></div></div>
<div class="stats">
  <div class="stat"><b id="verified">0</b>Verified</div>
  <div class="stat"><b id="pending">0</b>Pending</div>
  <div class="stat"><b id="failed">0</b>Failed</div>
  <div class="stat"><b id="bytes">0</b>Received</div>
</div>
<p class="ready" id="ready">All files verified on PC — safe to delete on iPhone.</p>
<p class="err" id="failures"></p>
<script>
var sid = "%s";
function fmt(n){if(n<1024)return n+' B';if(n<1048576)return(n/1024).toFixed(1)+' KB';if(n<1073741824)return(n/1048576).toFixed(1)+' MB';return(n/1073741824).toFixed(2)+' GB'}
function poll(){
  if(!sid)return;
  fetch('/api/shortcut/session/progress?sessionId='+encodeURIComponent(sid)).then(function(r){return r.json()}).then(function(d){
    if(!d.ok||!d.progress)return;
    var p=d.progress;
    document.getElementById('label').textContent=p.label||('Session '+p.sessionId);
    var denom=p.expectedCount||p.registered||1;
    var pct=Math.min(100,Math.round((p.verified/denom)*100));
    document.getElementById('fill').style.width=pct+'%%';
    document.getElementById('verified').textContent=p.verified;
    document.getElementById('pending').textContent=p.pending;
    document.getElementById('failed').textContent=p.failed;
    document.getElementById('bytes').textContent=fmt(p.bytesReceived||0);
    document.getElementById('ready').style.display=p.readyToDelete?'block':'none';
    var f=p.failures||[];
    document.getElementById('failures').textContent=f.length?('Failed: '+f.map(function(x){return x.name}).join(', ')):'';
  }).catch(function(){});
}
setInterval(poll,2000);poll();
</script>
</body>
</html>`, safe)
}
