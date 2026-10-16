import { useState, useEffect, useCallback } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import './App.css';
import { GetServerInfo, SelectSaveDir, GetUploadHistory, GetBackupSessionProgress, GetShortcutFailures, GetPortSettings, ApplyServerPort, PickFreePortAndApply, SetLang, GetCompressSettings, SetCompressSettings, SetLANIP } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';
import { LanguageContext, getTranslation, type Lang, type TranslationKey } from './i18n';

interface ServerInfo {
  running: boolean;
  url: string;
  port: number;
  lanIP: string;
  saveDir: string;
  lang: string;
  shortcutURL?: string;
  shortcutToken?: string;
  portWarning?: string;
  lanIPs?: string[];
}

interface UploadRecord {
  fileName: string;
  size: number;
  timestamp: string;
  savePath: string;
  compressed?: boolean;
  originalSize?: number;
}

interface CompressSettings {
  compressImages: boolean;
  imageQuality: number;
  keepOriginal: boolean;
}

interface ShortcutFailure {
  time: string;
  path: string;
  reason: string;
  detail: string;
  clientIP?: string;
}

interface SessionProgress {
  sessionId?: string;
  label?: string;
  expectedCount?: number;
  registered?: number;
  verified?: number;
  pending?: number;
  failed?: number;
  bytesReceived?: number;
  readyToDelete?: boolean;
  phase?: string;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
}

function App() {
  const [serverInfo, setServerInfo] = useState<ServerInfo | null>(null);
  const [history, setHistory] = useState<UploadRecord[]>([]);
  const [lang, setLangState] = useState<Lang>('ja');
  const [compress, setCompress] = useState<CompressSettings>({
    compressImages: false,
    imageQuality: 80,
    keepOriginal: false,
  });
  const [session, setSession] = useState<SessionProgress | null>(null);
  const [shortcutFailures, setShortcutFailures] = useState<ShortcutFailure[]>([]);
  const [portInput, setPortInput] = useState('48765');
  const [portError, setPortError] = useState('');
  const [portBusy, setPortBusy] = useState(false);

  const t = useCallback((key: TranslationKey) => getTranslation(lang, key), [lang]);

  const refreshInfo = useCallback(async () => {
    try {
      const info = await GetServerInfo();
      const si = info as unknown as ServerInfo;
      setServerInfo(si);
      if (si.lang === 'ja' || si.lang === 'en') {
        setLangState(si.lang as Lang);
      }
    } catch (e) {
      console.error('Failed to get server info:', e);
    }
  }, []);

  const refreshHistory = useCallback(async () => {
    try {
      const h = await GetUploadHistory();
      setHistory((h || []) as unknown as UploadRecord[]);
    } catch (e) {
      console.error('Failed to get history:', e);
    }
  }, []);

  const refreshShortcutFailures = useCallback(async () => {
    try {
      const f = await GetShortcutFailures();
      setShortcutFailures((f || []) as unknown as ShortcutFailure[]);
    } catch (e) {
      console.error('Failed to get shortcut failures:', e);
    }
  }, []);

  const refreshSession = useCallback(async () => {
    try {
      const p = await GetBackupSessionProgress('');
      const sp = p as unknown as SessionProgress;
      if (sp.phase && sp.phase !== 'idle') {
        setSession(sp);
      } else {
        setSession(null);
      }
    } catch (e) {
      console.error('Failed to get session progress:', e);
    }
  }, []);

  const refreshPortSettings = useCallback(async () => {
    try {
      const s = await GetPortSettings();
      const ps = s as { configuredPort?: number; activePort?: number };
      if (ps.configuredPort) setPortInput(String(ps.configuredPort));
      else if (ps.activePort) setPortInput(String(ps.activePort));
    } catch (e) {
      console.error('Failed to get port settings:', e);
    }
  }, []);

  const refreshCompress = useCallback(async () => {
    try {
      const s = await GetCompressSettings();
      const cs = s as unknown as CompressSettings;
      setCompress(cs);
    } catch (e) {
      console.error('Failed to get compress settings:', e);
    }
  }, []);

  const handleCompressChange = async (updates: Partial<CompressSettings>) => {
    const next = { ...compress, ...updates };
    setCompress(next);
    try {
      await SetCompressSettings(next.compressImages, next.imageQuality, next.keepOriginal);
    } catch (e) {
      console.error('Failed to save compress settings:', e);
    }
  };

  useEffect(() => {
    refreshInfo();
    refreshHistory();
    refreshCompress();
    refreshSession();
    refreshShortcutFailures();
    refreshPortSettings();

    const cancelUpload = EventsOn('upload:completed', () => {
      refreshHistory();
    });
    const cancelSession = EventsOn('session:progress', (p: SessionProgress) => {
      if (p?.phase && p.phase !== 'idle') setSession(p);
      else setSession(null);
    });
    const cancelShortcut = EventsOn('shortcut:failure', () => {
      refreshShortcutFailures();
    });
    const cancelServer = EventsOn('server:changed', () => {
      refreshInfo();
      refreshPortSettings();
    });

    const interval = setInterval(() => {
      refreshInfo();
      refreshSession();
    }, 2000);

    return () => {
      cancelUpload();
      cancelSession();
      cancelShortcut();
      cancelServer();
      clearInterval(interval);
    };
  }, [refreshInfo, refreshHistory, refreshSession, refreshShortcutFailures, refreshPortSettings]);

  const handleApplyPort = async () => {
    setPortError('');
    setPortBusy(true);
    const port = parseInt(portInput, 10);
    if (Number.isNaN(port) || port < 1024 || port > 65535) {
      setPortError('Port must be 1024–65535');
      setPortBusy(false);
      return;
    }
    try {
      await ApplyServerPort(port);
      await refreshInfo();
      await refreshPortSettings();
    } catch (e) {
      setPortError(e instanceof Error ? e.message : t('portApplyFailed'));
    } finally {
      setPortBusy(false);
    }
  };

  const handleAutoPickPort = async () => {
    setPortError('');
    setPortBusy(true);
    try {
      const port = await PickFreePortAndApply();
      setPortInput(String(port));
      await refreshInfo();
      await refreshPortSettings();
    } catch (e) {
      setPortError(e instanceof Error ? e.message : t('portApplyFailed'));
    } finally {
      setPortBusy(false);
    }
  };

  const handleSetLang = async (newLang: Lang) => {
    setLangState(newLang);
    try {
      await SetLang(newLang);
      // Refresh to get updated URL with new lang param
      await refreshInfo();
    } catch (e) {
      console.error('Failed to set language:', e);
    }
  };

  const handleSelectDir = async () => {
    try {
      const newDir = await SelectSaveDir();
      if (newDir) {
        setServerInfo(prev => prev ? { ...prev, saveDir: newDir } : null);
      }
    } catch (e) {
      console.error('Failed to select directory:', e);
    }
  };

  const running = serverInfo?.running ?? false;
  const url = serverInfo?.url ?? '';

  return (
    <LanguageContext.Provider value={{ lang, setLang: handleSetLang, t }}>
      <div id="app">
        <div className="header">
          <div className="lang-switcher">
            <button
              className={`lang-btn ${lang === 'ja' ? 'active' : ''}`}
              onClick={() => handleSetLang('ja')}
            >
              JA
            </button>
            <button
              className={`lang-btn ${lang === 'en' ? 'active' : ''}`}
              onClick={() => handleSetLang('en')}
            >
              EN
            </button>
          </div>
          <h1>{t('appTitle')}</h1>
        </div>

        <div style={{ textAlign: 'center' }}>
          <span className={`status-badge ${running ? 'running' : 'stopped'}`}>
            <span className="status-dot" />
            {running ? t('serverRunning') : t('serverStopped')}
          </span>
        </div>

        {running && url && (
          <div className="qr-section">
            <div className="qr-wrapper">
              <QRCodeSVG value={url} size={180} level="M" />
            </div>
            <div className="qr-url" style={{ fontSize: '0.9rem', marginTop: 8, wordBreak: 'break-all', userSelect: 'text', fontFamily: 'monospace' }}>
              {url}
            </div>
            {serverInfo?.lanIPs && serverInfo.lanIPs.length > 1 && (
              <div style={{ fontSize: '0.75rem', marginTop: 6 }}>
                PC address:{' '}
                <select
                  value={serverInfo.lanIP}
                  onChange={async (e) => { await SetLANIP(e.target.value); refreshInfo(); }}
                >
                  {serverInfo.lanIPs.map((ip) => <option key={ip} value={ip}>{ip}</option>)}
                </select>
              </div>
            )}
            {serverInfo?.shortcutURL && (
              <div className="shortcut-info" style={{ fontSize: '0.75rem', marginTop: 8, wordBreak: 'break-all', userSelect: 'text' }}>
                <div>iOS Shortcut URL: {serverInfo.shortcutURL}</div>
                <div>X-FileBridge-Token: {serverInfo.shortcutToken}</div>
                {serverInfo.portWarning && <div style={{ color: '#fbbf24' }}>⚠ {serverInfo.portWarning}</div>}
              </div>
            )}
          </div>
        )}

        <div className="session-section">
          <div className="label">{t('backupSession')}</div>
          {!session ? (
            <div className="empty-history">{t('backupIdle')}</div>
          ) : (
            <>
              <div className="session-label">{session.label || session.sessionId}</div>
              <div className="session-stats">
                <span>{t('backupVerified')}: <b>{session.verified ?? 0}</b></span>
                <span>{t('backupPending')}: <b>{session.pending ?? 0}</b></span>
                <span>{t('backupFailed')}: <b>{session.failed ?? 0}</b></span>
                <span>{t('backupBytes')}: <b>{formatSize(session.bytesReceived ?? 0)}</b></span>
              </div>
              {session.readyToDelete && (
                <div className="session-ready">{t('backupReady')}</div>
              )}
              {serverInfo?.shortcutURL && session.sessionId && (
                <div className="session-phone-url" style={{ fontSize: '0.7rem', marginTop: 6, wordBreak: 'break-all' }}>
                  Phone progress: {serverInfo.shortcutURL.replace('/api/shortcut/upload', '/shortcut/session')}?sessionId={session.sessionId}
                </div>
              )}
            </>
          )}
        </div>

        <div className="port-section">
          <div className="label">{t('serverPort')}</div>
          <p className="port-hint">{t('portHint')}</p>
          <div className="port-row">
            <input
              type="number"
              min={1024}
              max={65535}
              className="port-input"
              value={portInput}
              onChange={(e) => setPortInput(e.target.value)}
              disabled={portBusy}
            />
            <button type="button" className="change-btn" onClick={handleApplyPort} disabled={portBusy}>
              {t('applyPort')}
            </button>
          </div>
          <button type="button" className="auto-port-btn" onClick={handleAutoPickPort} disabled={portBusy}>
            {t('autoPickPort')}
          </button>
          {portError && <div className="port-error">{portError}</div>}
          {serverInfo && serverInfo.running && serverInfo.port !== Number(portInput) && (
            <div className="port-error" style={{ color: '#fbbf24' }}>
              Active: {serverInfo.port} (update Shortcut if you changed the port)
            </div>
          )}
        </div>

        <div className="save-section">
          <div className="label">{t('saveLocation')}</div>
          <div className="dir-row">
            <div className="dir-path" title={serverInfo?.saveDir}>
              {serverInfo?.saveDir ?? t('notSet')}
            </div>
            <button className="change-btn" onClick={handleSelectDir}>
              {t('change')}
            </button>
          </div>
        </div>

        <details className="compress-section">
          <summary className="compress-summary">{t('imageCompression')}</summary>
          <div className="compress-body">
            <label className="compress-toggle">
              <input
                type="checkbox"
                checked={compress.compressImages}
                onChange={(e) => handleCompressChange({ compressImages: e.target.checked })}
              />
              <span>{t('compressImages')}</span>
            </label>
            {compress.compressImages && (
              <>
                <div className="quality-row">
                  <span className="quality-label">{t('imageQuality')}</span>
                  <input
                    type="range"
                    min={30}
                    max={95}
                    step={5}
                    value={compress.imageQuality}
                    onChange={(e) => handleCompressChange({ imageQuality: Number(e.target.value) })}
                    className="quality-slider"
                  />
                  <span className="quality-value">{compress.imageQuality}%</span>
                </div>
                <label className="compress-toggle">
                  <input
                    type="checkbox"
                    checked={compress.keepOriginal}
                    onChange={(e) => handleCompressChange({ keepOriginal: e.target.checked })}
                  />
                  <span>{t('keepOriginal')}</span>
                </label>
              </>
            )}
          </div>
        </details>

        <div className="history-section">
          <div className="label">{t('shortcutErrors')}</div>
          {shortcutFailures.length === 0 ? (
            <div className="empty-history">{t('noShortcutErrors')}</div>
          ) : (
            shortcutFailures.slice(0, 8).map((f, i) => (
              <div key={i} className="history-item shortcut-failure-item">
                <div className="file-info">
                  <div className="file-name">{f.reason}</div>
                  <div className="file-meta">{f.time} · {f.path}</div>
                  <div className="file-meta" style={{ color: '#f87171' }}>{f.detail}</div>
                </div>
              </div>
            ))
          )}
        </div>

        <div className="history-section">
          <div className="label">{t('recentUploads')}</div>
          {history.length === 0 ? (
            <div className="empty-history">{t('noFiles')}</div>
          ) : (
            history.map((record, i) => (
              <div key={i} className="history-item">
                <div className="file-info">
                  <div className="file-name" title={record.fileName}>
                    {record.fileName}
                  </div>
                  <div className="file-meta">
                    {record.timestamp}
                    {record.compressed && record.originalSize ? (
                      <span className="compress-badge">
                        {' '}({formatSize(record.originalSize)} → {formatSize(record.size)})
                      </span>
                    ) : null}
                  </div>
                </div>
                <div className="file-size">{formatSize(record.size)}</div>
              </div>
            ))
          )}
        </div>

        <div className="warning-box">
          {t('firewallWarning')}
        </div>
      </div>
    </LanguageContext.Provider>
  );
}

export default App;
