# iPhone: sandarkan semua foto, kemudian padam (v1.2+)

> Fork [shogo-tsunoda/file-bridge](https://github.com/shogo-tsunoda/file-bridge) (MIT). Penulis asal: **Shogo Tsunoda**.

Panduan ini untuk **banyak ribu foto/video**. PC menyimpan **sesi sandaran** supaya anda boleh sambung semula selepas Wi‑Fi terputus atau PC tidur.

**Bahasa:** [English](ios-shortcut-backup.md) | Bahasa Melayu (fail ini)

## Apa yang baru dalam v1.2

- **Sesi sandaran** — satu `sessionId` setiap kali anda jalankan Shortcut  
- **Sambung semula** — semak fail yang sudah disahkan di PC sebelum muat naik semula  
- **Papan pemuka PC** — tetingkap File Bridge tunjuk verified / pending / failed  
- **Halaman kemajuan di telefon** — buka `/shortcut/session?sessionId=…` dalam Safari  
- **Padam selamat** — padam hanya selepas semua fail disahkan (atau subset yang disahkan dengan pengesahan anda)

## Apa yang anda perlukan

1. File Bridge **v1.2+** di Windows, iPhone dan PC pada **Wi‑Fi yang sama**.  
2. Salin **URL Shortcut iOS** dan **X-FileBridge-Token** dari aplikasi (jangan taip IP tanpa port `:48765`).  
3. Matikan **pemampatan imej** dalam File Bridge untuk aliran ini.  
4. Cadangan: tetapkan **DHCP reservation** untuk PC supaya IP tidak berubah.

### Fail `.shortcut` boleh import?

Apple tidak membenarkan fail Shortcut yang ditandatangani dari internet. Anda bina Shortcut sendiri di iPhone (langkah di bawah). Selepas siap, anda boleh eksport/import antara peranti anda melalui app Shortcuts.

## API ringkas

Header untuk semua `POST`: `X-FileBridge-Token: <token>`.

| Endpoint | Fungsi |
|----------|--------|
| `POST /api/shortcut/session/create` | Mula sesi → dapat `sessionId` |
| `POST /api/shortcut/session/check` | Semak fail yang boleh dilangkau (sudah verified) |
| `POST /api/shortcut/upload?name=…` | Muat naik satu fail + header sesi |
| `GET /api/shortcut/session/progress?sessionId=…` | Kiraan kemajuan |

Header muat naik tambahan:

- `X-FileBridge-Session` — id sesi  
- `X-File-Client-Key` — id stabil (atau hantar `name`, `size`, `sha256` dalam check/register)  
- `X-Expected-Size` — saiz fail pada iPhone  
- `X-Expected-SHA256` — hash SHA-256 objek yang sama yang dihantar  

## Bina Shortcut (ringkas)

### 1. Pilih semua media

Guna **Find Photos** / **Find Videos** (bukan pemilih **Select Photos** untuk ribuan fail):

- **Find Photos** (semua, atau tapis album/tarikh)  
- **Find Videos**  
- **Combine** → `All Media`  
- **Count** → gunakan dalam `expectedCount` semasa create session  

### 2. Cipta sesi

**Get Contents of URL** → `POST http://<PC>:48765/api/shortcut/session/create`  
Body JSON: `{"label":"Sandaran iPhone","expectedCount":<Count>}`  
Simpan `sessionId`.

### 3. Ulang setiap fail

Untuk setiap item dalam `All Media`:

1. **Get Details** → Nama, Saiz  
2. **Generate Hash** SHA256 pada **Repeat Item** → `PhoneHash`  
3. **POST** `/api/shortcut/session/check` dengan `{ sessionId, files: [{ name, size, sha256 }] }`  
4. Jika `skipUpload` benar → teruskan ke fail seterusnya  
5. **POST** `/api/shortcut/upload?name=` + nama  
   - Header: token, `X-FileBridge-Session`, `X-Expected-Size`, `X-Expected-SHA256` = `PhoneHash`  
   - Body: Fail = Repeat Item  
   - Cuba semula 3 kali dengan **Wait** 2s, 4s, 8s jika gagal  
6. Bandingkan `sha256` respons dengan `PhoneHash`  
7. Jika sama dan `ok` → **Add to Variable** `Verified`  
8. Jika tidak → **POST** `/api/shortcut/session/fail` (jangan padam fail ini)  

### 4. Padam

1. **GET** progress — jika `readyToDelete` benar  
2. **Delete Photos** → hanya `Verified`  
3. Jika ada gagal: tunjuk amaran; padam hanya yang disahkan jika anda setuju  

Foto yang dipadam masuk **Recently Deleted** (30 hari).

## HEIC & Live Photo

- Hash mesti pada **objek yang sama** yang dihantar (Repeat Item).  
- Live Photo biasanya hanya menghantar still HEIC; video pasangan tidak dihantar berasingan.  
- Jika hash tidak sepadan walaupun saiz sama → anggap gagal, jangan padam; jalankan semula Shortcut (resume akan langkau yang sudah OK).  

## Wi‑Fi / PC tidur

- Gagal muat naik: cuba semula dengan jeda; **check** melangkau fail yang sudah verified.  
- IP berubah: kemas kini URL dalam Shortcut atau guna DHCP reservation.  
- Sesi disimpan dalam folder simpanan: `.filebridge/sessions/`.

Panduan Inggeris penuh: [ios-shortcut-backup.md](ios-shortcut-backup.md).
