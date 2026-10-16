import { createContext, useContext } from 'react';

export type Lang = 'ja' | 'en' | 'ms';

const translations = {
  ja: {
    appTitle: 'File Bridge',
    serverRunning: 'サーバー起動中',
    serverStopped: 'サーバー停止中',
    saveLocation: '保存先',
    notSet: '未設定',
    change: '変更',
    recentUploads: '受信履歴',
    noFiles: 'まだファイルを受信していません',
    firewallWarning:
      'iPhoneから接続できない場合は、Windowsファイアウォールの設定を確認してください。このアプリの通信を許可するか、一時的にファイアウォールを無効にしてください。両方のデバイスが同じWi-Fiに接続されている必要があります。',
    imageCompression: '画像圧縮',
    compressImages: '保存前に画像を圧縮',
    imageQuality: '画質',
    keepOriginal: '元画像も保存',
    compressed: '圧縮済み',
    compressionFailed: '圧縮失敗（元ファイルを保存）',
    backupSession: 'バックアップセッション（iOSショートカット）',
    backupIdle: 'ショートカットの実行を待っています',
    backupVerified: '検証済み',
    backupPending: '未完了',
    backupFailed: '失敗',
    backupReady: 'すべて検証済み — iPhoneから削除できます',
    backupBytes: '受信済み',
    shortcutErrors: 'ショートカットエラー',
    noShortcutErrors: 'エラーなし',
    serverPort: 'サーバーポート',
    portHint: 'iOSショートカットはこのポートを使います（1024–65535）',
    applyPort: '適用して再起動',
    autoPickPort: '空きポートを自動選択',
    portApplyFailed: 'ポートを変更できませんでした',
  },
  en: {
    appTitle: 'File Bridge',
    serverRunning: 'Server Running',
    serverStopped: 'Server Stopped',
    saveLocation: 'Save Location',
    notSet: 'Not set',
    change: 'Change',
    recentUploads: 'Recent Uploads',
    noFiles: 'No files received yet',
    firewallWarning:
      'If your iPhone cannot connect, check Windows Firewall settings. Allow this app through the firewall or temporarily disable it. Both devices must be on the same Wi-Fi network.',
    imageCompression: 'Image Compression',
    compressImages: 'Compress images before saving',
    imageQuality: 'Quality',
    keepOriginal: 'Keep original copy',
    compressed: 'Compressed',
    compressionFailed: 'Compression failed (original saved)',
    backupSession: 'Backup session (iOS Shortcut)',
    backupIdle: 'Waiting for a Shortcut run',
    backupVerified: 'Verified',
    backupPending: 'Pending',
    backupFailed: 'Failed',
    backupReady: 'All verified — safe to delete on iPhone',
    backupBytes: 'Received',
    shortcutErrors: 'Shortcut errors',
    noShortcutErrors: 'No errors logged',
    serverPort: 'Server port',
    portHint: 'Your iOS Shortcut must use this port (1024–65535)',
    applyPort: 'Apply & restart server',
    autoPickPort: 'Auto-pick free port',
    portApplyFailed: 'Could not change port',
  },
  ms: {
    appTitle: 'File Bridge',
    serverRunning: 'Pelayan berjalan',
    serverStopped: 'Pelayan dihentikan',
    saveLocation: 'Lokasi simpanan',
    notSet: 'Belum ditetapkan',
    change: 'Tukar',
    recentUploads: 'Muat naik terkini',
    noFiles: 'Belum ada fail diterima',
    firewallWarning:
      'Jika iPhone tidak boleh menyambung, semak Windows Firewall. Benarkan aplikasi ini atau lumpuhkan sementara. Kedua-dua peranti mesti pada Wi-Fi yang sama.',
    imageCompression: 'Mampatan imej',
    compressImages: 'Mampatkan imej sebelum simpan',
    imageQuality: 'Kualiti',
    keepOriginal: 'Simpan salinan asal',
    compressed: 'Dimampatkan',
    compressionFailed: 'Mampatan gagal (asal disimpan)',
    backupSession: 'Sesi sandaran (iOS Shortcut)',
    backupIdle: 'Menunggu Shortcut dijalankan',
    backupVerified: 'Disahkan',
    backupPending: 'Belum selesai',
    backupFailed: 'Gagal',
    backupReady: 'Semua disahkan — selamat dipadam dari iPhone',
    backupBytes: 'Diterima',
    shortcutErrors: 'Ralat Shortcut',
    noShortcutErrors: 'Tiada ralat',
    serverPort: 'Port pelayan',
    portHint: 'Shortcut iOS mesti guna port ini (1024–65535)',
    applyPort: 'Guna & mula semula pelayan',
    autoPickPort: 'Pilih port kosong auto',
    portApplyFailed: 'Tidak boleh menukar port',
  },
} as const;

export type TranslationKey = keyof (typeof translations)['ja'];

export interface LanguageContextType {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: (key: TranslationKey) => string;
}

export const LanguageContext = createContext<LanguageContextType>({
  lang: 'ja',
  setLang: () => {},
  t: (key) => translations.ja[key],
});

export function useLanguage() {
  return useContext(LanguageContext);
}

export function getTranslation(lang: Lang, key: TranslationKey): string {
  return translations[lang]?.[key] ?? translations.en[key] ?? translations.ja[key];
}
