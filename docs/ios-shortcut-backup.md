# iOS Shortcut: back up all photos, then delete (v1.2+)

> Fork of [shogo-tsunoda/file-bridge](https://github.com/shogo-tsunoda/file-bridge) (MIT). Original author: **Shogo Tsunoda**.

This guide is for **large libraries** (thousands of photos, many GB). It uses a **backup session** on the PC so you can resume after Wi‑Fi drops, PC sleep, or re-running the Shortcut.

**Languages:** English (this file) | [Bahasa Melayu](ios-shortcut-ms.md)

## What is new in v1.2

| Feature | Description |
|--------|-------------|
| Backup session | One run = one `sessionId`; manifest on the PC tracks every file |
| Resume | `POST /api/shortcut/session/check` — skip files already verified on disk |
| Idempotent upload | Re-uploading a verified file returns `skipped: true` (no duplicate) |
| PC dashboard | File Bridge window shows verified / pending / failed / bytes live |
| Phone progress page | Open `/shortcut/session?sessionId=…` in Safari during the run |
| Safe delete | Delete only when **all** registered items are verified (or only the verified list after you confirm failures) |

## Before you start

1. Install **File Bridge v1.2+** on Windows and note **iOS Shortcut URL** + **X-FileBridge-Token** under the QR code.
2. Turn **off image compression** in File Bridge for this workflow (compression changes bytes after hash; keep verification simple).
3. Use a **DHCP reservation** for the PC on your router so the IP in the Shortcut changes rarely.
4. Keep the Shortcut **in the foreground**; plug the iPhone into power for huge libraries.

### Importable `.shortcut` file?

Apple does not let third parties ship signed Shortcut files from the web. You build the Shortcut on your iPhone (steps below). You can export and re-import on your own devices via the Shortcuts app share sheet.

## API overview (token required except progress GET)

All `POST` requests need header: `X-FileBridge-Token: <token from app>`.

| Endpoint | Purpose |
|----------|---------|
| `POST /api/shortcut/session/create` | Start session → `{ sessionId, progressURL, checkURL, phoneURL }` |
| `POST /api/shortcut/session/register` | Register expected files (batch) `{ sessionId, files: [{ name, size, sha256?, key? }] }` |
| `POST /api/shortcut/session/check` | Before upload: which files can be skipped `{ sessionId, files: [...] }` |
| `POST /api/shortcut/upload?name=…` | Upload one file (same as v1.1) + session headers |
| `GET /api/shortcut/session/progress?sessionId=…` | Live counts |
| `POST /api/shortcut/session/fail` | Mark a file failed (hash mismatch, etc.) |

### Upload headers (v1.2)

- `X-FileBridge-Session`: session id from create
- `X-File-Client-Key`: stable id (see below)
- `X-Expected-Size`: bytes on the phone (recommended)
- `X-Expected-SHA256`: SHA-256 hex of the **same** file object you POST (recommended)

**Client key** (if you do not compute it in Shortcuts): server accepts `name` + `size` + `sha256` in register/check and derives the key. In Shortcuts, you can pass only `name`, `size`, and `sha256` in JSON; omit `key`.

## Recommended Shortcut structure

### A. Select media (avoid “Select All” in the picker)

Use **Find Photos** (or **Find Videos**) instead of **Select Photos**:

- **Find Photos** where **Date Taken** is after a very old date (or leave filter empty for *all*), sort **Oldest First** if you like.
- Optional: add **Filter** → Album is … for one album at a time (run the Shortcut once per album).

This returns every matching item without tapping thousands of thumbnails.

Combine photos and videos:

1. **Find Photos** → variable `Photos`
2. **Find Videos** → variable `Videos`
3. **Combine** `Photos` and `Videos` → `All Media`

### B. Create session

1. **Get Contents of URL**  
   - URL: `http://<PC>:48765/api/shortcut/session/create`  
   - Method: POST  
   - Headers: `X-FileBridge-Token`  
   - Request body: JSON `{"label":"iPhone backup","expectedCount":<Count of All Media>}`  
2. **Get Dictionary Value** `sessionId` → variable `SessionId`  
3. Optional: **Open URL** `phoneURL` from the response so Safari shows live progress.

### C. Register batch (optional but good for progress bar)

Inside **Repeat with Each** in chunks of e.g. 50 (use **Get Item from List** with range — or register all at once if &lt; 500):

- For each item: **Get Details** Name, **Get Details** Size, **Generate Hash** SHA256 of **Repeat Item**  
- Build JSON list → **POST** `registerURL` with `{ sessionId, files: [...] }`

You can skip register and rely on check/upload only; `expectedCount` from create still sets the denominator.

### D. For each file: check → upload → verify

Inside **Repeat with Each** `All Media`:

1. **Get Details** → Name → `Name`  
2. **Get Details** → Size (or Photo Size) → `Size`  
3. **Generate Hash** SHA256 of **Repeat Item** → `PhoneHash`  
4. **POST** `/api/shortcut/session/check` with one file `{ name, size, sha256 }` and `sessionId`  
5. **If** `skipUpload` is true → **Next** (continue repeat)  
6. **Get Contents of URL** upload URL `?name=` + `Name`  
   - Method POST  
   - Headers: token, `X-FileBridge-Session`, `X-File-Client-Key` (from check result `key` if present), `X-Expected-Size`, `X-Expected-SHA256` = `PhoneHash`  
   - Body: File = Repeat Item  
   - **Retry**: wrap in a sub-shortcut or use Repeat 3× with **Wait** 2s / 4s / 8s on failure  
7. Parse response `sha256` → `ServerHash`  
8. **If** `ServerHash` **is** `PhoneHash` **and** `ok` is true → add to variable `Verified`  
9. **Otherwise** → **POST** `/api/shortcut/session/fail` with reason `hash mismatch` (do **not** add to `Verified`)

### E. Finish and delete

1. **GET** progress URL — read `readyToDelete`  
2. **If** `readyToDelete` is true → **Delete Photos** → `Verified` only  
3. **Else** → **Show Alert** listing `failed` count; offer to delete **only** `Verified` or cancel  

iOS always shows a confirmation before delete (unless you enabled **Allow Deleting Without Confirmation** in Shortcuts settings).

## HEIC, Live Photos, and hash mismatches

- The Shortcut must hash **the same object** it uploads: use **Generate Hash** on **Repeat Item**, then POST **Repeat Item** as the file body.  
- **Live Photos**: iOS usually gives the still HEIC; the paired movie is not uploaded separately. Deleting removes both.  
- If **Generate Hash** and the server disagree but **sizes match**, treat as **failure** (do not delete). Re-run the Shortcut; resume will retry.  
- Do not enable **Compress images** on the PC for this workflow.

## Network / PC sleep

- If upload fails, retry with backoff; **check** skips completed files.  
- If the PC IP changes, update the Shortcut URLs (or use a DHCP reservation).  
- Session state is stored under your save folder in `.filebridge/sessions/` so a PC restart does not lose the manifest.

## Headless server (advanced)

```text
FileBridge.exe -headless -dir "D:\Backups\iPhone" -port 48765 -token <token>
```

Same HTTP API; no desktop dashboard, but progress JSON still works.
