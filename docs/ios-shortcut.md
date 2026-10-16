# iPhone: upload photos to File Bridge and delete them after verification

A web page in Safari cannot delete photos from your library. An iOS **Shortcut**
can: it uploads each photo, checks that the PC received exactly the same bytes
(SHA-256), and then deletes **only the verified photos**. iOS shows one
confirmation dialog before deleting; that dialog cannot be skipped.

Deleted photos go to **Recently Deleted** in Photos for 30 days, so you can still undo.

## What you need

1. File Bridge running on the PC, with the iPhone on the same Wi-Fi.
2. Under the QR code, the app shows:
   - **iOS Shortcut URL**, e.g. `http://192.168.1.20:8765/api/shortcut/upload`
   - **X-FileBridge-Token**, a 32-character secret
   The port is fixed (8765 by default, `"port"` in `config.json`) so the URL stays the same.
   If the PC's IP changes, update the URL in the Shortcut. A DHCP reservation in your router stops it changing.

## The endpoint

`POST /api/shortcut/upload?name=<file name>`

- Header `X-FileBridge-Token: <token>` (required, otherwise 401)
- Body: the raw file (or a multipart form with one file field)
- Optional header `X-Expected-Size: <bytes>`: the response is `ok:false` if the size differs
- Response: `{"ok": true, "fileName": "IMG_0001.HEIC", "size": 2345678, "sha256": "ab12..."}`

## Build the Shortcut (Shortcuts app → + → New Shortcut)

1. **Select Photos**: turn on **Select Multiple**. In "Include", choose Images and Videos.
2. **Set Variable** `Verified` with an empty **List** (add a **List** action with no items, then Set Variable).
   Simpler alternative: skip this step. **Add to Variable** in step 9 creates the variable.
3. **Repeat with Each** item in *Photos*.
4. Inside the repeat: **Get Details of Images** → *Name* of *Repeat Item* (gives e.g. `IMG_0001`).
5. **Get Contents of URL**:
   - URL: `http://192.168.1.20:8765/api/shortcut/upload?name=` + the *Name* variable
   - Method: **POST**
   - Headers: `X-FileBridge-Token` = your token
   - Request Body: **File**, File = *Repeat Item*
6. **Get Dictionary Value**: key `ok` in *Contents of URL*. Also add a second
   **Get Dictionary Value** with key `sha256` in *Contents of URL* and save it with **Set Variable** `ServerHash`.
7. **Generate Hash**: type **SHA256** of *Repeat Item* (this hashes the file on the phone).
8. **If** *ServerHash* **is** *Hash* (the result of step 7)
   (if you skip the hash check, use: If *Dictionary Value* (ok) **is** `1` / true).
9. Inside the If: **Add to Variable** → add *Repeat Item* to `Verified`.
   (Otherwise: leave empty, or add **Show Notification** "Failed: Name".)
10. **End If**, **End Repeat**.
11. **Count** items in `Verified`, then **Show Notification** "Verified: Count".
12. **Delete Photos** → *Verified*. iOS asks once, "Allow to delete N photos?" Tap Delete.

Only photos whose server checksum matched the phone's checksum are in
`Verified`. A failed or interrupted upload is never deleted.

### Notes and limits

- Live Photos and HEIC: the Shortcut sends the photo file iOS hands it (usually the HEIC
  still). The paired Live Photo video is not sent separately, so deleting removes the motion part too.
  If that matters, turn the Live Photo off first, or keep the originals.
- Very large videos are fine. The server streams to disk and the cap is 64 GB per request.
  The Shortcut must stay in the foreground while it runs.
- In Settings → Shortcuts → Advanced, enable **Allow Deleting Without Confirmation**
  if your iOS version offers it. Otherwise one confirmation per run is normal.
- Keep the token private. Anyone on your Wi-Fi with the token can upload files to the PC
  (they cannot read or delete anything).
- Image compression in File Bridge (if enabled) happens **after** hashing. The
  checksum always refers to the exact bytes sent by the phone.
