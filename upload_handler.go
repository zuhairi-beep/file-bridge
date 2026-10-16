package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// UploadRecord represents a single file upload record
type UploadRecord struct {
	FileName     string `json:"fileName"`
	Size         int64  `json:"size"`
	Timestamp    string `json:"timestamp"`
	SavePath     string `json:"savePath"`
	Compressed   bool   `json:"compressed,omitempty"`
	OriginalSize int64  `json:"originalSize,omitempty"`
	// ReceivedSize and SHA256 describe the exact bytes received from the
	// phone (before any compression) and are used for client-side verification.
	ReceivedSize int64  `json:"receivedSize"`
	SHA256       string `json:"sha256"`
}

// maxUploadSize is the max size of a single upload request (64GB). Files are
// streamed to disk, so this is only a safety cap, not a memory limit.
const maxUploadSize = 64 << 30

// uploadTexts holds translations for the mobile upload page
type uploadTexts struct {
	PageTitle     string
	Heading       string
	SelectFiles   string
	FileHint      string
	UploadBtn     string
	Uploading     string
	UploadingPct  string
	SuccessSuffix string
	UploadFailed  string
	NetworkError  string
	Cancelled     string
	// Verification texts
	Verifying       string
	Verified        string
	Retrying        string
	VerifiedPrefix  string
	VerifiedSuffix  string
	SafeToDelete    string
	PartialVerified string
	FailedRetry     string
}

var uploadTranslations = map[string]uploadTexts{
	"ja": {
		PageTitle:       "File Bridge - アップロード",
		Heading:         "File Bridge",
		SelectFiles:     "ファイルを選択",
		FileHint:        "画像・動画・PDF など",
		UploadBtn:       "アップロード",
		Uploading:       "アップロード中...",
		UploadingPct:    "アップロード中... ",
		SuccessSuffix:   " 件のファイルをアップロードしました",
		UploadFailed:    "アップロードに失敗しました",
		NetworkError:    "ネットワークエラーです。接続を確認してください。",
		Cancelled:       "アップロードがキャンセルされました。",
		Verifying:       "検証中...",
		Verified:        "検証済み",
		Retrying:        "再試行",
		VerifiedPrefix:  "全 ",
		VerifiedSuffix:  " 件のファイルを検証しました",
		SafeToDelete:    "PC側でサイズとSHA-256が一致しました。スマホから削除しても安全です。",
		PartialVerified: " 件検証済み。",
		FailedRetry:     " 件失敗しました。もう一度アップロードしてください。",
	},
	"en": {
		PageTitle:       "File Bridge - Upload",
		Heading:         "File Bridge",
		SelectFiles:     "Select Files",
		FileHint:        "Images, videos, PDFs, etc.",
		UploadBtn:       "Upload",
		Uploading:       "Uploading...",
		UploadingPct:    "Uploading... ",
		SuccessSuffix:   " file(s) uploaded successfully!",
		UploadFailed:    "Upload failed",
		NetworkError:    "Network error. Please check your connection.",
		Cancelled:       "Upload cancelled.",
		Verifying:       "verifying...",
		Verified:        "verified",
		Retrying:        "retry",
		VerifiedPrefix:  "All ",
		VerifiedSuffix:  " files verified",
		SafeToDelete:    "Size and SHA-256 checksum match on the PC. It is safe to delete them from your phone.",
		PartialVerified: " verified.",
		FailedRetry:     " failed. Tap Upload to retry them.",
	},
}

var uploadPageTemplate = template.Must(template.New("upload").Parse(uploadPageTmpl))

// handleUploadPage serves the mobile upload HTML page
func (fs *FileServer) handleUploadPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lang := r.URL.Query().Get("lang")
	texts, ok := uploadTranslations[lang]
	if !ok {
		texts = uploadTranslations["ja"]
	}

	var buf bytes.Buffer
	if err := uploadPageTemplate.Execute(&buf, texts); err != nil {
		log.Printf("Failed to render upload page: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

// handleFileUpload handles multipart file upload
func (fs *FileServer) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	saveDir := fs.app.GetSaveDir()
	if saveDir == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Save directory not configured",
		})
		return
	}

	// Ensure save directory exists
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		log.Printf("Failed to create save directory: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to create save directory",
		})
		return
	}

	// Stream the multipart body part by part straight to disk. Nothing is
	// buffered in memory, so request size is limited only by maxUploadSize
	// and free disk space. The phone page sends one file per request, but
	// several files in one request are still accepted.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	mr, err := r.MultipartReader()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Expected multipart/form-data upload"})
		return
	}

	var results []UploadRecord
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Failed to read multipart stream: %v", err)
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error": "Upload interrupted or too large", "files": results,
			})
			return
		}
		if part.FormName() != "files" || part.FileName() == "" {
			part.Close()
			continue
		}
		record, err := fs.saveUploadedPart(saveDir, part)
		part.Close()
		if err != nil {
			log.Printf("Failed to save %s: %v", part.FileName(), err)
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": "Failed to save " + part.FileName(), "files": results,
			})
			return
		}
		results = append(results, record)
		fs.app.addUploadRecord(record)
	}

	if len(results) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No files uploaded"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"count":   len(results),
		"files":   results,
	})
}

// saveUploadedPart streams one file part to a temporary ".part" file while
// computing its SHA-256, then moves it into place (optionally compressing
// images). ReceivedSize/SHA256 describe the exact bytes received from the
// phone, so the client can verify the transfer before deleting anything.
func (fs *FileServer) saveUploadedPart(saveDir string, part *multipart.Part) (UploadRecord, error) {
	return fs.saveUploadedStream(saveDir, part.FileName(), part)
}

// saveUploadedStream is the shared implementation for web and Shortcut uploads.
func (fs *FileServer) saveUploadedStream(saveDir, name string, part io.Reader) (UploadRecord, error) {
	safeName := sanitizeFilename(name)

	tmp, err := os.CreateTemp(saveDir, ".filebridge-*.part")
	if err != nil {
		return UploadRecord{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op after a successful rename

	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, h), part)
	if cerr := tmp.Chmod(0644); err == nil {
		err = cerr // CreateTemp makes 0600 files; saved files should be normal
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return UploadRecord{}, err
	}
	sum := hex.EncodeToString(h.Sum(nil))

	record := UploadRecord{
		Timestamp:    time.Now().Format("2006-01-02 15:04:05"),
		ReceivedSize: written,
		SHA256:       sum,
	}

	cfg := fs.app.config
	if cfg.CompressImages && IsCompressibleImage(safeName) {
		data, rerr := os.ReadFile(tmpPath)
		if rerr == nil {
			comp, cerr := CompressImage(data, safeName, cfg.ImageQuality)
			if cerr == nil && comp.DidCompress {
				if cfg.KeepOriginal {
					ext := filepath.Ext(safeName)
					origPath := resolveUniquePath(saveDir, strings.TrimSuffix(safeName, ext)+"_original"+ext)
					if err := os.Rename(tmpPath, origPath); err != nil {
						log.Printf("Failed to keep original %s: %v", origPath, err)
					}
				}
				outName := safeName
				if ext := filepath.Ext(safeName); strings.ToLower(ext) != comp.Extension {
					outName = strings.TrimSuffix(safeName, ext) + comp.Extension
				}
				destPath := resolveUniquePath(saveDir, outName)
				if err := os.WriteFile(destPath, comp.Data, 0644); err != nil {
					return UploadRecord{}, err
				}
				record.FileName = filepath.Base(destPath)
				record.SavePath = destPath
				record.Size = int64(len(comp.Data))
				record.Compressed = true
				record.OriginalSize = comp.OriginalSize
				log.Printf("File saved (compressed): %s (%d -> %d bytes)", destPath, written, record.Size)
				return record, nil
			} else if cerr != nil {
				log.Printf("Compression failed for %s: %v, saving original", safeName, cerr)
			}
		}
	}

	destPath := resolveUniquePath(saveDir, safeName)
	if err := os.Rename(tmpPath, destPath); err != nil {
		return UploadRecord{}, err
	}
	record.FileName = filepath.Base(destPath)
	record.SavePath = destPath
	record.Size = written
	log.Printf("File saved: %s (%d bytes, sha256 %s)", destPath, written, sum)
	return record, nil
}

// handleShortcutUpload is the iOS Shortcuts endpoint. It requires the
// pairing token in the X-FileBridge-Token header and accepts either a
// multipart form (any file field) or a raw request body (file name in the
// ?name= query or X-File-Name header). It returns
// {"ok":true,"fileName":...,"size":N,"sha256":"..."} for one file.
func (fs *FileServer) handleShortcutUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !fs.shortcutAuth(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "invalid token"})
		return
	}
	saveDir := fs.app.GetSaveDir()
	if saveDir == "" || os.MkdirAll(saveDir, 0755) != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "save directory unavailable"})
		return
	}
	if fs.app.sessions != nil {
		fs.app.sessions.SetSaveDir(saveDir)
	}
	sessionID := r.Header.Get("X-FileBridge-Session")
	clientKey := r.Header.Get("X-File-Client-Key")
	expectedSHA := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Expected-SHA256")))

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	// Idempotent resume: already verified in this session — return without rewriting disk.
	if fs.app.sessions != nil && sessionID != "" && clientKey != "" {
		if existing := fs.app.sessions.getSessionEntry(sessionID, clientKey); existing != nil &&
			existing.Status == StatusVerified && fileOnDiskVerified(existing.SavePath, existing.Size, existing.SHA256) {
			io.Copy(io.Discard, r.Body)
			fs.app.onSessionUpdated(sessionID)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok": true, "fileName": existing.FileName, "size": existing.Size,
				"sha256": existing.SHA256, "skipped": true, "sessionId": sessionID,
			})
			return
		}
	}

	var (
		name string
		src  io.Reader
	)
	if mr, err := r.MultipartReader(); err == nil {
		for {
			part, err := mr.NextPart()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "no file in form"})
				return
			}
			if part.FileName() != "" {
				defer part.Close()
				name, src = part.FileName(), part
				break
			}
			part.Close()
		}
	} else {
		name = r.URL.Query().Get("name")
		if name == "" {
			name = r.Header.Get("X-File-Name")
		}
		if name == "" {
			name = "shortcut_" + time.Now().Format("20060102_150405")
		}
		src = r.Body
	}

	record, err := fs.saveUploadedStream(saveDir, name, src)
	if err != nil {
		log.Printf("Shortcut upload failed for %s: %v", name, err)
		if fs.app.sessions != nil && sessionID != "" && clientKey != "" {
			fs.app.sessions.MarkFailed(sessionID, clientKey, "upload interrupted")
			fs.app.onSessionUpdated(sessionID)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "save failed"})
		return
	}

	// Optional end-to-end check: if the Shortcut sends X-Expected-Size, the
	// response is ok only when the received size matches.
	if exp := r.Header.Get("X-Expected-Size"); exp != "" && exp != strconv.FormatInt(record.ReceivedSize, 10) {
		// Do not keep a file the phone considers incomplete.
		os.Remove(record.SavePath)
		if fs.app.sessions != nil && sessionID != "" && clientKey != "" {
			fs.app.sessions.MarkFailed(sessionID, clientKey, "size mismatch")
			fs.app.onSessionUpdated(sessionID)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": "size mismatch", "size": record.ReceivedSize, "sha256": record.SHA256})
		return
	}
	if expectedSHA != "" && !strings.EqualFold(record.SHA256, expectedSHA) {
		os.Remove(record.SavePath)
		if fs.app.sessions != nil && sessionID != "" && clientKey != "" {
			fs.app.sessions.MarkFailed(sessionID, clientKey, "sha256 mismatch")
			fs.app.onSessionUpdated(sessionID)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok": false, "error": "sha256 mismatch", "size": record.ReceivedSize, "sha256": record.SHA256,
		})
		return
	}
	fs.app.addUploadRecord(record)
	if fs.app.sessions != nil && sessionID != "" && clientKey != "" {
		fs.app.sessions.MarkVerified(sessionID, clientKey, record)
		fs.app.onSessionUpdated(sessionID)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"fileName":  record.FileName,
		"size":      record.ReceivedSize,
		"sha256":    record.SHA256,
		"sessionId": sessionID,
	})
}

// sanitizeFilename removes dangerous characters and path traversal attempts
func sanitizeFilename(name string) string {
	// Get only the base name (prevent path traversal)
	name = filepath.Base(name)

	// Remove null bytes
	name = strings.ReplaceAll(name, "\x00", "")

	// Replace dangerous characters
	re := regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	name = re.ReplaceAllString(name, "_")

	// Remove leading/trailing dots and spaces
	name = strings.Trim(name, ". ")

	// Ensure valid UTF-8
	if !utf8.ValidString(name) {
		name = strings.ToValidUTF8(name, "_")
	}

	// If empty after sanitization, use a default name
	if name == "" {
		name = fmt.Sprintf("upload_%s", time.Now().Format("20060102_150405"))
	}

	// Limit length (255 chars for Windows)
	if len(name) > 200 {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		if len(base) > 200-len(ext) {
			base = base[:200-len(ext)]
		}
		name = base + ext
	}

	return name
}

// resolveUniquePath returns a unique file path, renaming if collision
func resolveUniquePath(dir, name string) string {
	destPath := filepath.Join(dir, name)

	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		return destPath
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)

	for i := 1; i < 10000; i++ {
		newName := fmt.Sprintf("%s (%d)%s", base, i, ext)
		destPath = filepath.Join(dir, newName)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			return destPath
		}
	}

	// Fallback: use timestamp
	newName := fmt.Sprintf("%s_%s%s", base, time.Now().Format("20060102_150405"), ext)
	return filepath.Join(dir, newName)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// uploadPageTmpl is the Go template for the mobile upload page
const uploadPageTmpl = `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
<title>{{.PageTitle}}</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  background: #0f172a;
  color: #e2e8f0;
  min-height: 100vh;
  padding: 20px;
}
.container {
  max-width: 480px;
  margin: 0 auto;
}
h1 {
  font-size: 1.5rem;
  text-align: center;
  margin-bottom: 24px;
  color: #38bdf8;
}
.upload-area {
  border: 2px dashed #475569;
  border-radius: 12px;
  padding: 32px 16px;
  text-align: center;
  margin-bottom: 16px;
  transition: border-color 0.2s;
}
.upload-area.active {
  border-color: #38bdf8;
  background: rgba(56, 189, 248, 0.05);
}
.file-input-label {
  display: inline-block;
  background: #2563eb;
  color: white;
  padding: 14px 28px;
  border-radius: 8px;
  font-size: 1.1rem;
  cursor: pointer;
  margin-bottom: 12px;
  -webkit-tap-highlight-color: transparent;
}
.file-input-label:active {
  background: #1d4ed8;
}
input[type="file"] { display: none; }
.file-list {
  margin: 16px 0;
  text-align: left;
}
.file-item {
  background: #1e293b;
  padding: 10px 14px;
  border-radius: 8px;
  margin-bottom: 8px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.9rem;
}
.file-item .name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-right: 8px;
}
.file-item .size { color: #94a3b8; white-space: nowrap; margin-right: 8px; }
.file-item .remove-btn {
  background: none;
  border: none;
  color: #64748b;
  font-size: 1.2rem;
  cursor: pointer;
  padding: 0 4px;
  line-height: 1;
  -webkit-tap-highlight-color: transparent;
}
.file-item .remove-btn:active { color: #f87171; }
.send-btn {
  display: block;
  width: 100%;
  padding: 16px;
  background: #16a34a;
  color: white;
  border: none;
  border-radius: 8px;
  font-size: 1.1rem;
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
.send-btn:disabled {
  background: #334155;
  color: #64748b;
}
.send-btn:active:not(:disabled) { background: #15803d; }
.progress-bar {
  width: 100%;
  height: 6px;
  background: #334155;
  border-radius: 3px;
  margin: 16px 0;
  overflow: hidden;
  display: none;
}
.progress-bar .fill {
  height: 100%;
  background: #38bdf8;
  width: 0%;
  transition: width 0.2s;
  border-radius: 3px;
}
.status {
  text-align: center;
  margin: 16px 0;
  font-size: 0.95rem;
  min-height: 1.5em;
}
.status.success { color: #4ade80; }
.summary { color: #94a3b8; font-size: 0.85rem; margin-bottom: 8px; }
.file-item .state { white-space: nowrap; font-size: 0.8rem; color: #38bdf8; margin-right: 6px; }
.file-item .state.ok { color: #4ade80; }
.file-item .state.err { color: #f87171; }
.status.error { color: #f87171; }
.status.uploading { color: #38bdf8; }
</style>
</head>
<body>
<div class="container">
  <h1>{{.Heading}}</h1>
  <div class="upload-area" id="uploadArea">
    <label class="file-input-label" for="fileInput">{{.SelectFiles}}</label>
    <input type="file" id="fileInput" multiple>
    <p style="color:#94a3b8; margin-top:8px; font-size:0.85rem;">{{.FileHint}}</p>
  </div>
  <div class="file-list" id="fileList"></div>
  <div class="progress-bar" id="progressBar"><div class="fill" id="progressFill"></div></div>
  <div class="status" id="status"></div>
  <button class="send-btn" id="sendBtn" disabled>{{.UploadBtn}}</button>
</div>

<script>
var T = {
  uploading: '{{.Uploading}}',
  uploadingPct: '{{.UploadingPct}}',
  successSuffix: '{{.SuccessSuffix}}',
  uploadFailed: '{{.UploadFailed}}',
  networkError: '{{.NetworkError}}',
  cancelled: '{{.Cancelled}}',
  verifying: '{{.Verifying}}',
  verified: '{{.Verified}}',
  retrying: '{{.Retrying}}',
  verifiedPrefix: '{{.VerifiedPrefix}}',
  verifiedSuffix: '{{.VerifiedSuffix}}',
  safeToDelete: '{{.SafeToDelete}}',
  partialVerified: '{{.PartialVerified}}',
  failedRetry: '{{.FailedRetry}}'
};

var fileInput = document.getElementById('fileInput');
var fileList = document.getElementById('fileList');
var sendBtn = document.getElementById('sendBtn');
var statusEl = document.getElementById('status');
var progressBar = document.getElementById('progressBar');
var progressFill = document.getElementById('progressFill');

var selectedFiles = [];

fileInput.addEventListener('change', function() {
  var newFiles = Array.from(this.files);
  var existingNames = {};
  selectedFiles.forEach(function(f) { existingNames[f.name + '_' + f.size] = true; });
  newFiles.forEach(function(f) {
    if (!existingNames[f.name + '_' + f.size]) {
      selectedFiles.push(f);
    }
  });
  this.value = '';
  renderFileList();
  sendBtn.disabled = selectedFiles.length === 0;
  statusEl.textContent = '';
  statusEl.className = 'status';
});

function renderFileList(locked) {
  fileList.innerHTML = '';
  if (selectedFiles.length > 0) {
    var total = selectedFiles.reduce(function(s, f) { return s + f.size; }, 0);
    var sum = document.createElement('div');
    sum.className = 'summary';
    sum.textContent = selectedFiles.length + ' \u00b7 ' + formatSize(total);
    fileList.appendChild(sum);
  }
  selectedFiles.forEach(function(f, i) {
    var div = document.createElement('div');
    div.className = 'file-item';
    var nameSpan = document.createElement('span');
    nameSpan.className = 'name';
    nameSpan.textContent = f.name;
    var sizeSpan = document.createElement('span');
    sizeSpan.className = 'size';
    sizeSpan.textContent = formatSize(f.size);
    var removeBtn = document.createElement('button');
    removeBtn.className = 'remove-btn';
    removeBtn.textContent = '\u00d7';
    removeBtn.setAttribute('data-index', i);
    removeBtn.addEventListener('click', function() {
      selectedFiles.splice(parseInt(this.getAttribute('data-index')), 1);
      renderFileList();
      sendBtn.disabled = selectedFiles.length === 0;
    });
    var stateSpan = document.createElement('span');
    stateSpan.className = 'state';
    f._stateEl = stateSpan;
    div.appendChild(nameSpan);
    div.appendChild(sizeSpan);
    div.appendChild(stateSpan);
    if (!locked) div.appendChild(removeBtn);
    fileList.appendChild(div);
  });
}

// ---- Upload: one file per request, CONCURRENCY at a time, with retry ----
// Each file is verified after upload: the server returns the size and SHA-256
// of the bytes it received; we hash the local file and compare.
var CONCURRENCY = 2, MAX_RETRIES = 3;
var verifiedCount = 0, failedFiles = [];

sendBtn.addEventListener('click', function() {
  if (selectedFiles.length === 0) return;
  var queue = selectedFiles.slice();
  var total = queue.length, done = 0;
  verifiedCount = 0; failedFiles = [];
  sendBtn.disabled = true;
  fileInput.disabled = true;
  renderFileList(true);
  progressBar.style.display = 'block';
  progressFill.style.width = '0%';
  statusEl.textContent = T.uploading + ' 0 / ' + total;
  statusEl.className = 'status uploading';

  function next() {
    var f = queue.shift();
    if (!f) return Promise.resolve();
    return uploadWithRetry(f, 1).then(function(ok) {
      done++;
      if (ok) verifiedCount++; else failedFiles.push(f);
      progressFill.style.width = Math.round(done / total * 100) + '%';
      statusEl.textContent = T.uploading + ' ' + done + ' / ' + total;
      return next();
    });
  }
  var workers = [];
  for (var i = 0; i < CONCURRENCY; i++) workers.push(next());
  Promise.all(workers).then(function() {
    progressBar.style.display = 'none';
    fileInput.disabled = false;
    if (failedFiles.length === 0) {
      statusEl.innerHTML = '<b>' + T.verifiedPrefix + verifiedCount + T.verifiedSuffix + '</b><br>' + T.safeToDelete;
      statusEl.className = 'status success';
      selectedFiles = [];
      sendBtn.disabled = true;
    } else {
      statusEl.textContent = verifiedCount + ' / ' + total + T.partialVerified + ' ' + failedFiles.length + T.failedRetry;
      statusEl.className = 'status error';
      selectedFiles = failedFiles.slice();
      renderFileList(false);
      sendBtn.disabled = false;
    }
  });
});

function uploadWithRetry(f, attempt) {
  setFileState(f, (attempt > 1 ? T.retrying + ' ' : '') + '0%', '');
  return uploadOne(f).then(function(rec) {
    if (rec.receivedSize !== f.size) throw new Error('size mismatch');
    setFileState(f, T.verifying, '');
    return sha256File(f).then(function(hex) {
      if (hex !== rec.sha256) throw new Error('checksum mismatch');
      setFileState(f, '\u2713 ' + T.verified, 'ok');
      return true;
    });
  }).catch(function(err) {
    if (attempt < MAX_RETRIES) {
      return new Promise(function(r) { setTimeout(r, 1000 * attempt); })
        .then(function() { return uploadWithRetry(f, attempt + 1); });
    }
    setFileState(f, '\u2717 ' + (err && err.message ? err.message : T.uploadFailed), 'err');
    return false;
  });
}

function uploadOne(f) {
  return new Promise(function(resolve, reject) {
    var fd = new FormData();
    fd.append('files', f);
    var xhr = new XMLHttpRequest();
    xhr.open('POST', '/api/upload');
    xhr.upload.addEventListener('progress', function(e) {
      if (e.lengthComputable) setFileState(f, Math.round(e.loaded / e.total * 100) + '%', '');
    });
    xhr.addEventListener('load', function() {
      if (xhr.status === 200) {
        try { resolve(JSON.parse(xhr.responseText).files[0]); } catch (e) { reject(e); }
      } else {
        var msg = T.uploadFailed;
        try { msg = JSON.parse(xhr.responseText).error || msg; } catch (e) {}
        reject(new Error(msg));
      }
    });
    xhr.addEventListener('error', function() { reject(new Error(T.networkError)); });
    xhr.addEventListener('abort', function() { reject(new Error(T.cancelled)); });
    xhr.send(fd);
  });
}

function setFileState(f, text, cls) {
  var el = f._stateEl;
  if (!el) return;
  el.textContent = text;
  el.className = 'state ' + cls;
}

// Incremental SHA-256 in plain JS (crypto.subtle is unavailable on plain
// http:// LAN pages and cannot hash in chunks). Reads the file in 4MB slices
// so multi-GB videos never sit in memory.
var K = new Uint32Array([0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2]);
function Sha256() {
  this.h = new Uint32Array([0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19]);
  this.buf = new Uint8Array(64); this.bufLen = 0; this.len = 0; this.w = new Uint32Array(64);
}
Sha256.prototype.block = function(d, o) {
  var w = this.w, h = this.h, i, t1, t2;
  for (i = 0; i < 16; i++) w[i] = (d[o+4*i]<<24)|(d[o+4*i+1]<<16)|(d[o+4*i+2]<<8)|d[o+4*i+3];
  for (i = 16; i < 64; i++) {
    var x = w[i-15], y = w[i-2];
    w[i] = (((x>>>7)|(x<<25))^((x>>>18)|(x<<14))^(x>>>3)) + w[i-16] + (((y>>>17)|(y<<15))^((y>>>19)|(y<<13))^(y>>>10)) + w[i-7];
  }
  var a=h[0],b=h[1],c=h[2],e=h[4],f=h[5],g=h[6],hh=h[7],dd=h[3];
  for (i = 0; i < 64; i++) {
    t1 = (hh + (((e>>>6)|(e<<26))^((e>>>11)|(e<<21))^((e>>>25)|(e<<7))) + ((e&f)^(~e&g)) + K[i] + w[i])|0;
    t2 = ((((a>>>2)|(a<<30))^((a>>>13)|(a<<19))^((a>>>22)|(a<<10))) + ((a&b)^(a&c)^(b&c)))|0;
    hh=g; g=f; f=e; e=(dd+t1)|0; dd=c; c=b; b=a; a=(t1+t2)|0;
  }
  h[0]+=a; h[1]+=b; h[2]+=c; h[3]+=dd; h[4]+=e; h[5]+=f; h[6]+=g; h[7]+=hh;
};
Sha256.prototype.update = function(d) {
  var i = 0, n = d.length;
  this.len += n;
  if (this.bufLen) {
    while (i < n && this.bufLen < 64) this.buf[this.bufLen++] = d[i++];
    if (this.bufLen < 64) return;
    this.block(this.buf, 0); this.bufLen = 0;
  }
  for (; i + 64 <= n; i += 64) this.block(d, i);
  while (i < n) this.buf[this.bufLen++] = d[i++];
};
Sha256.prototype.hex = function() {
  var bits = this.len * 8, pad = new Uint8Array(((this.bufLen < 56) ? 56 : 120) - this.bufLen + 8);
  pad[0] = 0x80;
  var hi = Math.floor(bits / 4294967296), lo = bits >>> 0, p = pad.length;
  pad[p-8]=hi>>>24; pad[p-7]=hi>>>16; pad[p-6]=hi>>>8; pad[p-5]=hi;
  pad[p-4]=lo>>>24; pad[p-3]=lo>>>16; pad[p-2]=lo>>>8; pad[p-1]=lo;
  var len = this.len; this.update(pad); this.len = len;
  var s = '';
  for (var i = 0; i < 8; i++) s += ('00000000' + this.h[i].toString(16)).slice(-8);
  return s;
};
function sha256File(f) {
  var CHUNK = 4 << 20, h = new Sha256(), off = 0;
  function step() {
    if (off >= f.size) return Promise.resolve(h.hex());
    var slice = f.slice(off, off + CHUNK);
    return new Promise(function(res, rej) {
      var fr = new FileReader();
      fr.onload = function() { h.update(new Uint8Array(fr.result)); off += CHUNK; res(); };
      fr.onerror = function() { rej(fr.error); };
      fr.readAsArrayBuffer(slice);
    }).then(step);
  }
  return step();
}

function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
}

function escapeHtml(s) {
  var div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}
</script>
</body>
</html>`
