package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ManifestFileStatus is the server-side state for one expected file in a backup session.
type ManifestFileStatus string

const (
	StatusPending  ManifestFileStatus = "pending"
	StatusVerified ManifestFileStatus = "verified"
	StatusFailed   ManifestFileStatus = "failed"
)

// ManifestEntry tracks one photo/video the iPhone intends to back up.
type ManifestEntry struct {
	Key      string             `json:"key"`
	Name     string             `json:"name"`
	Size     int64              `json:"size"`
	SHA256   string             `json:"sha256,omitempty"`
	Status   ManifestFileStatus `json:"status"`
	Reason   string             `json:"reason,omitempty"`
	FileName string             `json:"fileName,omitempty"`
	SavePath string             `json:"savePath,omitempty"`
	Updated  string             `json:"updated"`
}

// BackupSession groups a full "back up then delete" run from the iOS Shortcut.
type BackupSession struct {
	ID            string                    `json:"id"`
	Label         string                    `json:"label"`
	CreatedAt     string                    `json:"createdAt"`
	ExpectedCount int                       `json:"expectedCount"`
	Files         map[string]*ManifestEntry `json:"files"`
	BytesReceived int64                     `json:"bytesReceived"`
}

// SessionProgress is returned to the phone page and the desktop dashboard.
type SessionProgress struct {
	SessionID     string           `json:"sessionId"`
	Label         string           `json:"label"`
	ExpectedCount int              `json:"expectedCount"`
	Registered    int              `json:"registered"`
	Verified      int              `json:"verified"`
	Failed        int              `json:"failed"`
	Pending       int              `json:"pending"`
	BytesReceived int64            `json:"bytesReceived"`
	ReadyToDelete bool             `json:"readyToDelete"`
	Phase         string           `json:"phase"`
	Failures      []ManifestEntry  `json:"failures,omitempty"`
}

// CheckResult tells the Shortcut whether it can skip uploading a file.
type CheckResult struct {
	Key        string             `json:"key"`
	Status     ManifestFileStatus `json:"status"`
	SkipUpload bool               `json:"skipUpload"`
	SHA256     string             `json:"sha256,omitempty"`
	Reason     string             `json:"reason,omitempty"`
}

// SessionManager stores active backup sessions in memory and on disk under the save folder.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*BackupSession
	saveDir  string
	activeID string
}

func NewSessionManager() *SessionManager {
	return &SessionManager{sessions: make(map[string]*BackupSession)}
}

func (sm *SessionManager) SetSaveDir(dir string) {
	sm.mu.Lock()
	sm.saveDir = dir
	sm.mu.Unlock()
}

func sessionStoreDir(saveDir string) string {
	return filepath.Join(saveDir, ".filebridge", "sessions")
}

func (sm *SessionManager) persist(sess *BackupSession) {
	if sm.saveDir == "" || sess == nil {
		return
	}
	dir := sessionStoreDir(sm.saveDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, sess.ID+".json"), data, 0644)
}

func (sm *SessionManager) loadFromDisk(id string) *BackupSession {
	if sm.saveDir == "" {
		return nil
	}
	path := filepath.Join(sessionStoreDir(sm.saveDir), id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var sess BackupSession
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil
	}
	if sess.Files == nil {
		sess.Files = make(map[string]*ManifestEntry)
	}
	return &sess
}

func (sm *SessionManager) getSession(id string) *BackupSession {
	sm.mu.RLock()
	sess := sm.sessions[id]
	sm.mu.RUnlock()
	if sess != nil {
		return sess
	}
	sess = sm.loadFromDisk(id)
	if sess == nil {
		return nil
	}
	sm.mu.Lock()
	sm.sessions[id] = sess
	sm.mu.Unlock()
	return sess
}

func (sm *SessionManager) Create(label string, expectedCount int) *BackupSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	now := time.Now().Format(time.RFC3339)
	sess := &BackupSession{
		ID:            id,
		Label:         label,
		CreatedAt:     now,
		ExpectedCount: expectedCount,
		Files:         make(map[string]*ManifestEntry),
	}
	sm.sessions[id] = sess
	sm.activeID = id
	sm.persist(sess)
	return sess
}

func (sm *SessionManager) ActiveSessionID() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.activeID
}

func (sm *SessionManager) SetActive(id string) {
	sm.mu.Lock()
	sm.activeID = id
	sm.mu.Unlock()
}

// ClientFileKey is the stable id the Shortcut sends (name + size + optional phone hash).
func ClientFileKey(name string, size int64, sha256hex string) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte{0})
	h.Write([]byte(fmt.Sprintf("%d", size)))
	h.Write([]byte{0})
	h.Write([]byte(strings.ToLower(strings.TrimSpace(sha256hex))))
	return hex.EncodeToString(h.Sum(nil))
}

func (sm *SessionManager) RegisterFiles(sessionID string, entries []ManifestEntry) {
	sess := sm.getSession(sessionID)
	if sess == nil {
		return
	}
	sm.mu.Lock()
	now := time.Now().Format(time.RFC3339)
	for _, e := range entries {
		key := e.Key
		if key == "" {
			key = ClientFileKey(e.Name, e.Size, e.SHA256)
		}
		if sess.Files[key] == nil {
			sess.Files[key] = &ManifestEntry{
				Key:    key,
				Name:   e.Name,
				Size:   e.Size,
				SHA256: strings.ToLower(strings.TrimSpace(e.SHA256)),
				Status: StatusPending,
				Updated: now,
			}
		} else {
			ex := sess.Files[key]
			if e.Name != "" {
				ex.Name = e.Name
			}
			if e.Size != 0 {
				ex.Size = e.Size
			}
			if e.SHA256 != "" {
				ex.SHA256 = strings.ToLower(strings.TrimSpace(e.SHA256))
			}
			ex.Updated = now
		}
	}
	sm.persist(sess)
	sm.mu.Unlock()
}

func fileOnDiskVerified(path string, wantSize int64, wantSHA string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	if err != nil || st.Size() != wantSize {
		return false
	}
	if wantSHA == "" {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), wantSHA)
}

func (sm *SessionManager) CheckFiles(sessionID string, queries []ManifestEntry) []CheckResult {
	sess := sm.getSession(sessionID)
	out := make([]CheckResult, 0, len(queries))
	if sess == nil {
		for _, q := range queries {
			key := q.Key
			if key == "" {
				key = ClientFileKey(q.Name, q.Size, q.SHA256)
			}
			out = append(out, CheckResult{Key: key, Status: StatusFailed, SkipUpload: false, Reason: "unknown session"})
		}
		return out
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for _, q := range queries {
		key := q.Key
		if key == "" {
			key = ClientFileKey(q.Name, q.Size, q.SHA256)
		}
		entry := sess.Files[key]
		if entry == nil {
			entry = &ManifestEntry{
				Key: key, Name: q.Name, Size: q.Size,
				SHA256: strings.ToLower(strings.TrimSpace(q.SHA256)),
				Status: StatusPending, Updated: time.Now().Format(time.RFC3339),
			}
			sess.Files[key] = entry
		}
		res := CheckResult{Key: key, Status: entry.Status}
		switch entry.Status {
		case StatusVerified:
			if fileOnDiskVerified(entry.SavePath, entry.Size, entry.SHA256) {
				res.SkipUpload = true
				res.SHA256 = entry.SHA256
				res.Status = StatusVerified
			} else {
				entry.Status = StatusPending
				entry.Reason = "file missing on PC"
				res.Status = StatusPending
				res.SkipUpload = false
			}
		case StatusFailed:
			res.Reason = entry.Reason
		default:
			res.Status = StatusPending
		}
		out = append(out, res)
	}
	sm.persist(sess)
	return out
}

func (sm *SessionManager) getSessionEntry(sessionID, key string) *ManifestEntry {
	sess := sm.getSession(sessionID)
	if sess == nil {
		return nil
	}
	sm.mu.RLock()
	e := sess.Files[key]
	sm.mu.RUnlock()
	return e
}

func (sm *SessionManager) MarkVerified(sessionID, key string, record UploadRecord) {
	sess := sm.getSession(sessionID)
	if sess == nil || key == "" {
		return
	}
	sm.mu.Lock()
	now := time.Now().Format(time.RFC3339)
	entry := sess.Files[key]
	wasVerified := entry != nil && entry.Status == StatusVerified
	if entry == nil {
		entry = &ManifestEntry{Key: key, Status: StatusPending}
		sess.Files[key] = entry
	}
	entry.Status = StatusVerified
	entry.Reason = ""
	entry.FileName = record.FileName
	entry.SavePath = record.SavePath
	entry.Size = record.ReceivedSize
	entry.SHA256 = record.SHA256
	entry.Updated = now
	if !wasVerified {
		sess.BytesReceived += record.ReceivedSize
	}
	sm.persist(sess)
	sm.mu.Unlock()
}

func (sm *SessionManager) MarkFailed(sessionID, key, reason string) {
	sess := sm.getSession(sessionID)
	if sess == nil || key == "" {
		return
	}
	sm.mu.Lock()
	entry := sess.Files[key]
	if entry == nil {
		entry = &ManifestEntry{Key: key}
		sess.Files[key] = entry
	}
	entry.Status = StatusFailed
	entry.Reason = reason
	entry.Updated = time.Now().Format(time.RFC3339)
	sm.persist(sess)
	sm.mu.Unlock()
}

// FindVerifiedByContent returns an existing verified entry with matching name, size, and sha256.
func (sm *SessionManager) FindVerifiedByContent(sessionID, name string, size int64, sha string) *ManifestEntry {
	sess := sm.getSession(sessionID)
	if sess == nil {
		return nil
	}
	sha = strings.ToLower(strings.TrimSpace(sha))
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for _, e := range sess.Files {
		if e.Status == StatusVerified && e.Size == size && e.SHA256 == sha &&
			strings.EqualFold(filepath.Base(e.Name), filepath.Base(name)) {
			if fileOnDiskVerified(e.SavePath, e.Size, e.SHA256) {
				return e
			}
		}
	}
	return nil
}

func (sm *SessionManager) Progress(sessionID string) SessionProgress {
	sess := sm.getSession(sessionID)
	if sess == nil {
		return SessionProgress{Phase: "unknown"}
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var verified, failed, pending int
	var failures []ManifestEntry
	for _, e := range sess.Files {
		switch e.Status {
		case StatusVerified:
			if fileOnDiskVerified(e.SavePath, e.Size, e.SHA256) {
				verified++
			} else {
				pending++
			}
		case StatusFailed:
			failed++
			failures = append(failures, *e)
		default:
			pending++
		}
	}
	registered := len(sess.Files)
	denom := sess.ExpectedCount
	if denom == 0 {
		denom = registered
	}
	ready := denom > 0 && verified == denom && failed == 0 && pending == 0
	phase := "uploading"
	if ready {
		phase = "ready_to_delete"
	} else if registered > 0 && pending == 0 && (verified+failed) == registered {
		if failed > 0 {
			phase = "complete_with_failures"
		} else {
			phase = "ready_to_delete"
			ready = true
		}
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].Name < failures[j].Name })
	if len(failures) > 20 {
		failures = failures[:20]
	}
	return SessionProgress{
		SessionID:     sess.ID,
		Label:         sess.Label,
		ExpectedCount: sess.ExpectedCount,
		Registered:    registered,
		Verified:      verified,
		Failed:        failed,
		Pending:       pending,
		BytesReceived: sess.BytesReceived,
		ReadyToDelete: ready,
		Phase:         phase,
		Failures:      failures,
	}
}

func (sm *SessionManager) ListRecent(limit int) []SessionProgress {
	if limit <= 0 {
		limit = 5
	}
	dir := sessionStoreDir(sm.saveDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		sm.mu.RLock()
		id := sm.activeID
		sm.mu.RUnlock()
		if id != "" {
			return []SessionProgress{sm.Progress(id)}
		}
		return nil
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			ids = append(ids, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[len(ids)-limit:]
	}
	out := make([]SessionProgress, 0, len(ids))
	for _, id := range ids {
		out = append(out, sm.Progress(id))
	}
	return out
}
