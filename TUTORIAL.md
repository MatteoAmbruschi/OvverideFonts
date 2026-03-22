# Come è stata costruita FontOverride

> **Destinatario:** chi vuole comprendere questa applicazione in Go/React e Wails.  
> Ogni sezione spiega *cosa* fa il codice, *perché* è stato scritto così e *come* si collegano i pezzi.

---

## Indice

1. [Panoramica dell'architettura](#1-panoramica-dellarchitettura)
2. [Struttura del progetto](#2-struttura-del-progetto)
3. [Go: concetti base prima di iniziare](#3-go-concetti-base-prima-di-iniziare)
4. [Wails: Go + React in un unico .exe](#4-wails-go--react-in-un-unico-exe)
5. [main.go — punto di ingresso e file embedding](#5-maingo--punto-di-ingresso-e-file-embedding)
6. [internal/fonts — leggere i font dal Registry](#6-internalfonts--leggere-i-font-dal-registry)
7. [internal/server — il "canale" tra Go e Chrome](#7-internalserver--il-canale-tra-go-e-chrome)
8. [internal/registry — scrivere nel Registro di Windows](#8-internalregistry--scrivere-nel-registro-di-windows)
9. [internal/installer — installare l'estensione Chrome](#9-internalinstaller--installare-lestensione-chrome)
10. [app.go — il collante tra frontend e backend](#10-appgo--il-collante-tra-frontend-e-backend)
11. [L'estensione Chrome (MV3)](#11-lestensione-chrome-mv3)
12. [La firma CRX — perché le chiavi RSA](#12-la-firma-crx--perché-le-chiavi-rsa)
13. [scripts/pack-extension — lo strumento di firma](#13-scriptspack-extension--lo-strumento-di-firma)
14. [Font bundled: OpenDyslexic e altri](#14-font-bundled-opendyslexic-e-altri)
15. [Il frontend React](#15-il-frontend-react)
16. [UAC: perché serve l'amministratore](#16-uac-perché-serve-lamministratore)
17. [Makefile — automatizzare i passi di build](#17-makefile--automatizzare-i-passi-di-build)
18. [install.ps1 — installare l'app nel sistema](#18-installps1--installare-lapp-nel-sistema)
19. [Flusso completo end-to-end](#19-flusso-completo-end-to-end)
20. [Come aggiungere nuovi font](#20-come-aggiungere-nuovi-font)

---

## 1. Panoramica dell'architettura

FontOverride deve fare tre cose distinte:

| Obiettivo | Tecnica usata |
|---|---|
| Override font su **Chrome** | Estensione Chrome che inietta CSS |
| Override font nelle **app native** (Notepad, Word…) | Windows Registry `FontSubstitutes` |
| **Distribuire** l'estensione senza il Chrome Web Store | Chrome Enterprise Policy (file locale) |

Queste tre cose richiedono componenti diversi che devono comunicare tra loro:

```
┌─────────────────────────────────────────────────────────────────┐
│  FontOverride.exe  (processo Windows, richiede Admin)           │
│                                                                 │
│  ┌──────────────┐    eventi JS    ┌───────────────────────────┐ │
│  │  React UI    │◄───────────────►│  Go Backend (Wails)      │ │
│  │  (Vite/TS)   │                 │                           │ │
│  └──────────────┘                 │  app.go  ←→  server.go   │ │
│                                   │  fonts.go    registry.go  │ │
│                                   │  extension.go             │ │
│                                   └──────────┬────────────────┘ │
└──────────────────────────────────────────────┼─────────────────┘
                                               │ HTTP localhost:59842
                                               ▼
                              ┌────────────────────────────────┐
                              │  Chrome Extension (MV3)        │
                              │  background.js  → poll /font   │
                              │  content.js     → inject CSS   │
                              └────────────────────────────────┘
```

Il problema fondamentale: **Chrome e il processo .exe sono due processi separati**, non possono condividere la memoria. L'unico modo per farli parlare è attraverso una rete, anche se locale. Per questo esiste il piccolo server HTTP.

---

## 2. Struttura del progetto

```
OvverideFonts/
├── main.go                    # Punto di ingresso Go
├── app.go                     # Metodi esposti al frontend React
├── go.mod                     # Equivalente di package.json per Go
├── wails.json                 # Config finestra (titolo, dimensioni…)
├── Makefile                   # Comandi di build automatizzati
├── install.ps1                # Installer PowerShell
│
├── internal/                  # Codice Go interno (non esposto)
│   ├── fonts/fonts.go         # Legge font installati + bundled
│   ├── server/server.go       # Server HTTP locale
│   ├── registry/registry.go   # R/W Registro di Windows
│   └── installer/installer.go # Installa l'estensione Chrome
│
├── extension/                 # Sorgente dell'estensione Chrome (JS)
│   ├── manifest.json
│   ├── background.js
│   └── content.js
│
├── scripts/                   # Tutti gli script di build/setup
│   ├── download-fonts.ps1     # Scarica OpenDyslexic e altri
│   └── pack-extension/        # Firma l'estensione → assets/extension.crx
│       └── main.go
│
├── assets/                    # File embedded nel binario
│   ├── extension.crx          # Estensione Chrome firmata
│   ├── update_manifest.xml    # Manifest Omaha per Chrome Policy
│   └── fonts/                 # Font bundled (es. OpenDyslexic)
│
├── frontend/                  # React app (TypeScript + Vite)
│   └── src/
│       ├── App.tsx
│       └── App.css
│
└── build/
    ├── extension.pem          # Chiave privata RSA (NON committare!)
    ├── windows/wails.exe.manifest  # Richiesta privilegi Admin
    └── bin/FontOverride.exe   # Output finale
```

---

## 3. Go: concetti base prima di iniziare

Se vieni da JavaScript/Python, Go ha alcune particolarità importanti.

### Package e import
Ogni file Go inizia con `package nomepackage`. I file nella stessa cartella appartengono allo stesso package e possono usare le loro funzioni liberamente, senza import. Per usare un package esterno si usa `import`:

```go
import (
    "os"                           // stdlib: file, env vars
    winreg "golang.org/x/sys/windows/registry"  // import con alias
)
```

### Struct e metodi
Go non ha classi. Si usano `struct` (contenitori di dati) e `metodi` (funzioni associate a una struct):

```go
// Definizione della struct
type Server struct {
    font   string   // campo privato (minuscola = privato al package)
    Active bool     // campo pubblico (maiuscola = esportato)
}

// Metodo sulla struct — (s *Server) è il "receiver", come "self" in Python
func (s *Server) SetFont(font string) {
    s.font = font
}
```

### Puntatori (`*` e `&`)
```go
s := &Server{}      // crea un Server e restituisce un *puntatore* ad esso
s.SetFont("Arial")  // Go derefenzia automaticamente
```
Se passi una struct senza `*`, Go fa una copia. Con `*` passi il riferimento all'originale.

### Gestione degli errori
Go non ha eccezioni. Le funzioni restituiscono `(valore, error)`:

```go
key, err := registry.OpenKey(...)
if err != nil {
    return err  // oppure gestisci il caso
}
// qui sei sicuro che key è valida
defer key.Close()  // verrà eseguito quando la funzione termina
```

### Goroutine e `go func()`
Una goroutine è un thread leggero. Si avvia con `go`:

```go
go func() {
    _ = srv.ListenAndServe()  // questo gira in background
}()
// il codice qui sotto esegue SUBITO, senza aspettare
```

---

## 4. Wails: Go + React in un unico .exe

[Wails](https://wails.io) è il framework che permette di creare app desktop con Go come backend e qualsiasi framework web come frontend.

**Come funziona:**
1. Compila il frontend React con Vite → produce file HTML/JS/CSS in `frontend/dist/`
2. Quei file vengono embedded nel binario Go con `//go:embed`
3. Wails usa **WebView2** (il motore del browser Edge, già installato su Windows 10/11) per mostrare l'UI
4. Il backend Go espone funzioni chiamabili dal frontend via generazione automatica di binding TypeScript

```
React chiama → ApplyFont("Arial")
      ↓ (Wails genera automaticamente questo bridge)
Go riceve   → func (a *App) ApplyFont(name string) error { ... }
```

Per esporre un metodo Go al frontend basta che:
- Sia un **metodo** della struct App (non una funzione standalone)
- Il nome inizi con **lettera maiuscola** (go: maiuscola = pubblico/esportato)

---

## 5. main.go — punto di ingresso e file embedding

```go
//go:embed all:frontend/dist
var frontendAssets embed.FS

//go:embed assets/extension.crx assets/update_manifest.xml
var extensionAssets embed.FS

//go:embed assets/fonts
var fontAssets embed.FS
```

### Cos'è `//go:embed`?
È una direttiva del compilatore (non un commento normale). Dice a Go: *"al momento della compilazione, leggi questi file dal disco e mettili dentro la variabile"*.  
Il risultato è che il `.exe` finale **contiene al suo interno** tutti questi file. Niente DLL esterne, niente cartelle da distribuire. Un singolo eseguibile.

`embed.FS` è essenzialmente un filesystem in memoria — puoi aprire file, leggere cartelle, esattamente come faresti con `os.Open`, ma i dati vengono dalla memoria del processo.

### La finestra Wails
```go
wails.Run(&options.App{
    Title:     "Font Override",
    Width:     420,
    Height:    560,
    MinWidth:  420,
    MinHeight: 560,
    MaxWidth:  420,
    MaxHeight: 560,
    // ...
    Bind: []interface{}{app},  // esponi i metodi di App al frontend
})
```

`Bind` è la lista di oggetti i cui metodi pubblici vengono resi chiamabili da JavaScript. Wails genera automaticamente `frontend/wailsjs/go/main/App.ts` con le firme TypeScript corrette.

---

## 6. internal/fonts — leggere i font dal Registry

### Dove vive la lista dei font in Windows?
Windows tiene traccia di tutti i font installati nel Registro di sistema:

```
HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts
```

Qui ci sono voci come:
```
"Arial (TrueType)"  →  "arial.ttf"
"Segoe UI (TrueType)"  →  "segoeui.ttf"
```

### Il codice

```go
func List() []string {
    k, err := winreg.OpenKey(winreg.LOCAL_MACHINE,
        `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`,
        winreg.READ)
    if err != nil {
        return nil
    }
    defer k.Close()

    names, err := k.ReadValueNames(-1)  // legge tutti i nomi delle chiavi
    // ...
    for _, name := range names {
        if clean := stripRegistrySuffix(name); clean != "" {
            result = append(result, clean)
        }
    }
}
```

`stripRegistrySuffix` rimuove il suffisso ` (TrueType)` o ` (OpenType)` dai nomi:  
`"Arial (TrueType)"` → `"Arial"`

---

## 7. internal/server — il "canale" tra Go e Chrome

### Il problema
L'estensione Chrome vive nel processo di Chrome. Il backend Go vive nel processo FontOverride.exe. Non possono condividere variabili o memoria. L'unico modo che Chrome ha per comunicare con codice locale è una chiamata HTTP.

### La soluzione: un mini server HTTP
Go ha una libreria standard `net/http` potentissima. Creare un server web richiede pochissimo codice:

```go
const Port = 59842  // porta fissa, scelta arbitrariamente

func (s *Server) Start() {
    mux := http.NewServeMux()
    mux.HandleFunc("/font", s.handleFont)       // GET /font → stato corrente
    mux.HandleFunc("/fonts/", s.handleFontFile) // GET /fonts/xxx.otf → file font

    s.srv = &http.Server{
        Addr:    fmt.Sprintf("127.0.0.1:%d", Port),
        Handler: mux,
    }
    go func() { _ = s.srv.ListenAndServe() }()  // avvia in background
}
```

### L'endpoint `/font`
Risponde con JSON:
```json
{
  "font": "OpenDyslexic",
  "active": true,
  "fontUrl": "http://127.0.0.1:59842/fonts/OpenDyslexic-Regular.otf"
}
```

- Se nessun font è attivo: `{"font":"","active":false,"fontUrl":""}`
- `fontUrl` è presente solo per i font bundled (non installati su Windows)

### Thread safety con `sync.RWMutex`
Il server serve richieste HTTP in goroutine concorrenti. Nel frattempo, il codice Go principale può scrivere `font` e `active`. Due goroutine che leggono/scrivono la stessa variabile contemporaneamente = **race condition** = bug difficile da riprodurre.

`sync.RWMutex` risolve questo:
- Tanti lettori possono leggere contemporaneamente (`RLock`)
- Un solo scrittore alla volta, che blocca i lettori (`Lock`)

```go
func (s *Server) SetFont(font string, active bool) {
    s.mu.Lock()           // blocca tutto
    s.font, s.active = font, active
    s.mu.Unlock()         // sblocca
}

func (s *Server) GetFont() (string, bool) {
    s.mu.RLock()          // permette lettura concorrente
    defer s.mu.RUnlock()  // viene eseguito alla fine della funzione
    return s.font, s.active
}
```

---

## 8. internal/registry — scrivere nel Registro di Windows

Il Registro di Windows è un database gerarchico che memorizza configurazioni di sistema e di applicazioni. È organizzato in **hive** (radici):

| Hive | Abbreviazione | Uso |
|---|---|---|
| `HKEY_LOCAL_MACHINE` | `HKLM` | Impostazioni globali (tutti gli utenti) — richiede Admin |
| `HKEY_CURRENT_USER` | `HKCU` | Impostazioni dell'utente corrente — non richiede Admin |

### FontSubstitutes — override font nativi

```
HKCU\Software\Microsoft\Windows NT\CurrentVersion\FontSubstitutes
```

Qui puoi dire a Windows: *"ogni volta che un'applicazione chiede il font `Arial`, dagli invece `OpenDyslexic`"*.

```go
func ApplyFontSubstitutes(fontName string) error {
    k, _, err := winreg.CreateKey(winreg.CURRENT_USER, fontSubsKey, winreg.SET_VALUE)
    // ...
    for _, name := range commonFonts {  // Arial, Verdana, Segoe UI, ecc.
        k.SetStringValue(name, fontName)
    }
}
```

`winreg.CreateKey` crea la chiave se non esiste, altrimenti la apre. Il terzo valore di ritorno (`_`) è un flag che indica se è stata creata o aperta — qui non ci serve.

⚠️ **Nota:** non tutte le app rispettano FontSubstitutes. Le app moderni (come Chrome) usano il loro motore di rendering e ignorano questa impostazione. Ecco perché serve anche l'estensione Chrome.

### Chrome Enterprise Policy

```
HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist
```

Chrome legge le *Enterprise Policy* dal registro all'avvio. Se trova questa chiave, installa automaticamente l'estensione specificata. Il valore è:

```
"1" = "maglimbllfdgmbjklbfjiakndbgdpeem;file:///C:/Users/.../FontOverride/update_manifest.xml"
```

`[ID estensione];[URL del manifest di aggiornamento]`

Questa chiave richiede `HKLM` (admin).

---

## 9. internal/installer — installare l'estensione Chrome

```go
func (m *Manager) EnsureInstalled() error {
    dir := filepath.Join(os.Getenv("APPDATA"), "FontOverride")
    os.MkdirAll(dir, 0o755)

    // 1. Estrai extension.crx dal binario nella cartella AppData
    crxData, _ := m.assets.ReadFile("assets/extension.crx")
    os.WriteFile(crxDst, crxData, 0o644)

    // 2. Estrai update_manifest.xml e sostituisci il placeholder con il path reale
    xmlData, _ := m.assets.ReadFile("assets/update_manifest.xml")
    xml := strings.ReplaceAll(string(xmlData), "CRX_PATH_PLACEHOLDER", crxURL)
    os.WriteFile(manifestDst, []byte(xml), 0o644)

    // 3. Scrivi la policy nel registro
    return registry.InstallChromeExtension(ExtensionID, filepath.ToSlash(manifestDst))
}
```

**Perché `filepath.ToSlash`?**  
Windows usa `\` come separatore di path (`C:\Users\...`), ma le URL usano `/`. Il manifest XML contiene un URL `file:///...`, quindi dobbiamo convertire i backslash in forward slash.

**Cleanup alla chiusura:**
```go
func (m *Manager) Cleanup() {
    registry.UninstallChromeExtension()     // rimuove la policy HKLM
    os.RemoveAll(filepath.Join(os.Getenv("APPDATA"), "FontOverride"))  // elimina la cartella
    os.Remove(filepath.Join(os.Getenv("APPDATA"), "FontOverride.exe")) // elimina eventuale exe rimasto
}
```

L'app non lascia residui dopo la chiusura: tutto quello che ha scritto (file, registro) viene rimosso.

---

## 10. app.go — il collante tra frontend e backend

`app.go` è il file che Wails "conosce". I suoi metodi pubblici diventano funzioni chiamabili da React.

### Ciclo di vita
```go
func (a *App) startup(ctx context.Context) {
    a.ctx = ctx               // context Wails (per eventi, log, ecc.)
    a.srv.Start()             // avvia il server HTTP
    a.ext.EnsureInstalled()   // installa l'estensione Chrome
}

func (a *App) shutdown(_ context.Context) {
    a.srv.Stop()                   // ferma il server HTTP
    registry.RevertFontSubstitutes() // rimuove FontSubstitutes
    a.cleanupUserFonts()           // rimuove font bundled installati
    a.ext.Cleanup()               // rimuove policy Chrome e file
}
```

`startup` e `shutdown` sono **hook speciali di Wails**: vengono chiamati automaticamente all'avvio e alla chiusura della finestra.

### ApplyFont
```go
func (a *App) ApplyFont(name string) error {
    a.srv.SetFont(name, true)        // aggiorna lo stato nel server HTTP

    if a.srv.BundledFontFilename(name) != "" {
        a.installBundledFont(name)   // estrai il file .otf in Windows\Fonts
    }

    registry.ApplyFontSubstitutes(name)  // scrivi nel Registro
    runtime.EventsEmit(a.ctx, "fontChanged", name, true)  // notifica React
    return nil
}
```

`runtime.EventsEmit` è come `EventEmitter.emit()` in Node.js — invia un evento al frontend React, che lo riceve con `EventsOn("fontChanged", callback)`.

---

## 11. L'estensione Chrome (MV3)

### Manifest V3 (MV3)
Chrome nel 2023 ha deprecato Manifest V2 a favore di V3. Le differenze principali per noi:
- Il background script non è più una "pagina" persistente ma un **Service Worker** che può essere sospeso da Chrome
- Le Content Security Policy sono più restrittive
- Le chiamate di rete nel content script sono limitate

### manifest.json spiegato

```json
{
  "manifest_version": 3,
  "permissions": ["tabs", "scripting"],
  "host_permissions": ["http://localhost:59842/*"],
  "background": {
    "service_worker": "background.js"
  },
  "content_scripts": [{
    "matches": ["<all_urls>"],
    "js": ["content.js"],
    "run_at": "document_start",
    "all_frames": true
  }]
}
```

- `"tabs"`: permesso di accedere alla lista delle tab aperte
- `"scripting"`: permesso di eseguire codice nelle pagine
- `"host_permissions": ["http://localhost:59842/*"]`: il content script può fare `fetch()` verso localhost:59842 (il nostro server Go)
- `"run_at": "document_start"`: il content script si inietta **prima** che la pagina finisca di caricare — così il font è attivo dall'inizio
- `"all_frames": true`: si inietta anche negli iframe (es. widget di terze parti)

### background.js — il polling

```javascript
async function sync() {
  const r = await fetch(API);                      // chiede lo stato al server Go
  const { font, active, fontUrl = "" } = await r.json();

  if (font !== lastFont || active !== lastActive || fontUrl !== lastFontUrl) {
    // qualcosa è cambiato → notifica tutte le tab
    const tabs = await chrome.tabs.query({});
    for (const tab of tabs) {
      chrome.tabs.sendMessage(tab.id, { font, active, fontUrl }).catch(() => {});
    }
  }
}
setInterval(sync, 1000);  // ogni secondo
```

Il `.catch(() => {})` è necessario perché alcune tab (es. `chrome://settings`) non hanno content scripts e il `sendMessage` fallirebbe con un errore non gestito.

### content.js — l'iniezione CSS

```javascript
function apply(font, fontUrl) {
  const safe = font.replace(/'/g, "\\'");  // sanitizza apici nel nome font

  // Per font bundled: inietta @font-face che dice al browser dove trovare il file
  if (fontUrl) {
    let face = document.getElementById(FACE_ID) || document.createElement("style");
    face.id = FACE_ID;
    document.head.prepend(face);
    face.textContent = `@font-face{font-family:'${safe}';src:url('${fontUrl}') format('opentype');}`;
  }

  // Override tutti gli elementi con !important
  let s = document.getElementById(STYLE_ID) || document.createElement("style");
  s.id = STYLE_ID;
  document.head.prepend(s);
  s.textContent = `*,*::before,*::after{font-family:'${safe}' !important}`;
}
```

**Perché `@font-face`?**  
Se il font è un file bundled (es. `OpenDyslexic-Regular.otf`), Chrome non sa cosa sia `OpenDyslexic` — non è installato su Windows. Dobbiamo insegnarglielo con `@font-face` prima di usarlo nel CSS. Il `src: url(...)` punta al nostro server Go che serve il file direttamente dalla memoria del processo.

**Perché `!important`?**  
Senza `!important`, le regole CSS specifiche del sito web vincerebbero sulla nostra regola generica `*`. Con `!important` la nostra regola ha la precedenza su tutto, a prescindere dalla specificità del selettore.

---

## 12. La firma CRX — perché le chiavi RSA

Questa è probabilmente la parte più interessante e misteriosa. Perché un'estensione Chrome deve essere "firmata"?

### Il problema: identità e autenticità

Chrome riconosce le estensioni tramite un **ID univoco** come `maglimbllfdgmbjklbfjiakndbgdpeem`. Questo ID deve essere **stabile nel tempo** — se cambia, Chrome pensa che sia un'estensione diversa.

L'ID non può essere scelto arbitrariamente: deve derivare matematicamente da una chiave crittografica. In questo modo:
1. Solo chi possiede la chiave privata può creare/aggiornare quell'estensione
2. Chiunque può verificare l'autenticità dell'estensione usando la chiave pubblica
3. L'ID è deterministico: stessa chiave = stesso ID sempre

### Come funziona la crittografia a chiave pubblica (RSA)

È come un lucchetto con due chiavi:
- **Chiave privata**: solo tu ce l'hai, la tieni segreta. Serve per **firmare**.
- **Chiave pubblica**: la dai a tutti. Serve per **verificare** la firma.

```
Tu firmi con chiave privata → produce una "firma digitale"
Chiunque verifica con chiave pubblica → conferma che solo tu potevi produrla
```

### Come Chrome usa questo

1. Generi una coppia di chiavi RSA-2048
2. Prendi i file dell'estensione, li metti in uno ZIP
3. Calcoli SHA256(ZIP)
4. Firmi quel hash con la chiave privata → ottieni la firma
5. Costruisci il CRX3: `[header con firma e chiave pubblica][ZIP]`

Quando Chrome carica il `.crx`:
1. Estrae la chiave pubblica dall'header
2. Ricalcola SHA256(ZIP)
3. Usa la chiave pubblica per verificare che la firma corrisponda all'hash
4. Calcola l'ID: SHA256(chiave pubblica in formato DER) → prendi i primi 16 byte → ogni nibble diventa una lettera a-p

### Perché lettere a-p?
Chrome usa Base16 (hex), ma invece di `0-9a-f` usa `a-p`:
- `0` → `a`
- `1` → `b`
- ...
- `15` (`f` in hex) → `p`

Questo perché gli ID Chrome devono contenere solo lettere minuscole (per compatibilità storica con i nomi dei file su certi filesystem).

### Il file `build/extension.pem`
```
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA...
-----END RSA PRIVATE KEY-----
```

Questo è il file dove viene salvata la chiave privata RSA. **Non va mai committato su Git** (è nel `.gitignore`). Se lo perdi, l'ID dell'estensione cambierebbe e Chrome tratterrebbe la nuova versione come un'estensione completamente diversa.

---

## 13. scripts/pack-extension — lo strumento di firma

Tutto il processo di firma è automatizzato in `scripts/pack-extension/main.go`. Si esegue con:

```bash
go run ./scripts/pack-extension
```

Fa 4 cose:

### Passo 1: Carica o genera la chiave
```go
func loadOrGenKey(path string) *rsa.PrivateKey {
    if raw, err := os.ReadFile(path); err == nil {
        // il file esiste → caricalo
        block, _ := pem.Decode(raw)
        key, _ := x509.ParsePKCS1PrivateKey(block.Bytes)
        return key
    }
    // non esiste → genera una nuova chiave RSA-2048
    k, _ := rsa.GenerateKey(rand.Reader, 2048)
    // salvala in PEM (formato testo standard per chiavi crittografiche)
    os.WriteFile(path, pem.EncodeToMemory(...), 0o600)
    return k
}
```

`rand.Reader` è il generatore di numeri casuali crittograficamente sicuro del sistema operativo — fondamentale per chiavi RSA sicure.

### Passo 2: Zippare i file dell'estensione
```go
func mustZip(dir string) []byte {
    var buf bytes.Buffer
    w := zip.NewWriter(&buf)
    filepath.WalkDir(dir, func(path string, d DirEntry, err error) error {
        f, _ := w.Create(relativePath)
        data, _ := os.ReadFile(path)
        f.Write(data)
        return nil
    })
    w.Close()
    return buf.Bytes()
}
```

`bytes.Buffer` funziona come un file in memoria — scrivi dentro e poi leggi tutto. Non crea file temporanei su disco.

### Passo 3: Costruire il CRX3

Il formato CRX3 è un [formato binario documentato da Google](https://cs.chromium.org/chromium/src/components/crx_file/crx3.proto). Usa [Protocol Buffers](https://protobuf.dev/) per la serializzazione, ma la parte del proto è così semplice che è stata reimplementata manualmente senza dipendenze esterne:

```go
// Format: "Cr24" | version(uint32) | headerLen(uint32) | header(proto) | zip
func packCRX3(key *rsa.PrivateKey, zipData []byte) []byte {
    // 1. Calcola l'ID (primi 16 byte dell'hash della chiave pubblica)
    pubDER, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
    h := sha256.Sum256(pubDER)

    // 2. Costruisci i "signed data" (quello che viene firmato)
    signedData := pbLenDelim(1, h[:16])  // crx_id field in proto

    // 3. Firma: SHA256(prefisso + signedData + zip)
    sigInput := buildSigInput(signedData, zipData)
    digest := sha256.Sum256(sigInput)
    sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])

    // 4. Assembla l'header proto e tutto il CRX
    proof  := append(pbLenDelim(1, pubDER), pbLenDelim(2, sig)...)
    header := append(pbLenDelim(2, proof), pbLenDelim(10000, signedData)...)

    var out bytes.Buffer
    out.WriteString("Cr24")                              // magic number
    binary.Write(&out, binary.LittleEndian, uint32(3))  // versione CRX
    binary.Write(&out, binary.LittleEndian, uint32(len(header)))
    out.Write(header)
    out.Write(zipData)
    return out.Bytes()
}
```

I `uint32` vengono scritti in **little-endian** (byte meno significativo prima) — è il formato nativo dei processori x86/x64 e quello che usa il formato CRX3.

### Passo 4: Aggiorna ExtensionID nel codice Go
```go
func patchExtID(path, extID string) error {
    src, _ := os.ReadFile(path)
    // Trova e sostituisce: const ExtensionID = "vecchio_id"
    // con:                 const ExtensionID = "nuovo_id"
    // usa la stringa stessa come delimitatore per trovare l'inizio/fine
}
```

Questo evita di dover aggiornare manualmente la costante ogni volta.

---

## 14. Font bundled: OpenDyslexic e altri

### Il problema
Se un utente vuole usare OpenDyslexic ma non lo ha installato su Windows, il sistema non può applicarlo come FontSubstitute, e Chrome non sa dove trovarlo.

### La soluzione in tre parti

**1. I file font live nel binario**
```go
//go:embed assets/fonts
var fontAssets embed.FS
```

`scripts/download-fonts.ps1` scarica i font (es. da GitHub) in `assets/fonts/`. Questi poi vengono embedded nel `.exe` al momento della build.

**2. Il server Go li serve via HTTP**
```go
// GET /fonts/OpenDyslexic-Regular.otf
func (s *Server) handleFontFile(w http.ResponseWriter, r *http.Request) {
    filename := strings.TrimPrefix(r.URL.Path, "/fonts/")
    // protezione path traversal: blocca "../" e "/"
    if strings.ContainsAny(filename, "/\\") || strings.Contains(filename, "..") {
        http.NotFound(w, r); return
    }
    data, _ := s.fontAssets.ReadFile("assets/fonts/" + filename)
    w.Header().Set("Content-Type", "font/otf")
    w.Write(data)
}
```

Il controllo `..` è fondamentale per sicurezza: senza di esso un utente malintenzionato potrebbe richiedere `/fonts/../../../windows/system32/kernel32.dll` (path traversal attack).

**3. Chrome usa `@font-face` per caricarli**
Il content script inietta:
```css
@font-face {
  font-family: 'OpenDyslexic';
  src: url('http://127.0.0.1:59842/fonts/OpenDyslexic-Regular.otf') format('opentype');
}
* { font-family: 'OpenDyslexic' !important }
```

**4. Windows user fonts (per le app native)**
```go
func (a *App) installBundledFont(displayName string) error {
    // estrai il .otf dal binario
    data, _ := a.fontAssets.ReadFile("assets/fonts/" + filename)
    
    // copialo in %LOCALAPPDATA%\Microsoft\Windows\Fonts\
    fontDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts")
    os.WriteFile(dst, data, 0o644)
    
    // registralo in HKCU (senza admin!)
    return registry.InstallUserFont(displayName, dst)
}
```

`%LOCALAPPDATA%\Microsoft\Windows\Fonts\` è la cartella dei font per-utente introdotta in Windows 10 — non richiede admin ed è riconosciuta dal sistema.

---

## 15. Il frontend React

Il frontend è una Single Page Application React con TypeScript, costruita con Vite.

### Come React "parla" con Go

Wails genera automaticamente dei file TypeScript in `frontend/wailsjs/go/main/App.ts`:

```typescript
// auto-generato da Wails
export function ApplyFont(arg1: string): Promise<string>;
export function GetFonts(): Promise<string[]>;
export function GetStatus(): Promise<main.Status>;
```

Nel componente React si importano e si chiamano come normali funzioni async:

```typescript
import { GetFonts, ApplyFont, ResetFont, GetStatus } from "../wailsjs/go/main/App";

// nel componente:
const [fonts, setFonts] = useState<string[]>([]);

useEffect(() => {
    GetFonts().then(list => setFonts(list));
}, []);
```

### Gli eventi real-time (push da Go a React)

Quando l'utente clicca "Apply", `app.go` emette un evento:
```go
runtime.EventsEmit(a.ctx, "fontChanged", name, true)
```

React si iscrive a questo evento:
```typescript
import { EventsOn } from "../wailsjs/runtime/runtime";

const unsub = EventsOn("fontChanged", (font: string, active: boolean) => {
    setActiveFont(font);
    setIsActive(active);
});
return unsub;  // cleanup alla distruzione del componente
```

Questo è il pattern pub/sub (publish/subscribe): Go pubblica eventi, React si iscrive.

### La ricerca font con navigazione da tastiera

```typescript
const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") setKbIndex(i => Math.min(i + 1, filtered.length - 1));
    if (e.key === "ArrowUp")   setKbIndex(i => Math.max(i - 1, 0));
    if (e.key === "Enter")     setSelected(filtered[kbIndex >= 0 ? kbIndex : 0]);
    if (e.key === "Escape")    setQuery("");
}, [filtered, kbIndex]);
```

`useCallback` memoizza la funzione: viene ricreata solo quando cambiano `filtered` o `kbIndex`, non ad ogni render. Ottimizzazione di performance.

### Layout fluido senza overflow

```css
.app { display: flex; flex-direction: column; height: 100vh; }
.font-section { flex: 1; min-height: 0; display: flex; flex-direction: column; }
.picker-box { flex: 1; min-height: 0; /* ... */ }
.font-list { flex: 1; overflow-y: auto; }
.actions { flex-shrink: 0; margin-top: auto; }
```

`flex: 1` + `min-height: 0` è il pattern magico per far sì che un elemento flex cresca per riempire lo spazio disponibile **e** permetta al suo contenuto di scorrere. Senza `min-height: 0`, il browser non farebbe shrink del flex item al di sotto del suo contenuto naturale.

`.actions { margin-top: auto }` spinge sempre i bottoni in fondo, anche se la lista è corta.

---

## 16. UAC: perché serve l'amministratore

### Il problema
Scrivere in `HKLM` (HKEY_LOCAL_MACHINE) richiede privilegi di amministratore. Senza quelli, la chiamata `registry.InstallChromeExtension` fallirebbe silenziosamente o con un errore di accesso negato.

### La soluzione: manifest UAC

Il file `build/windows/wails.exe.manifest` contiene:
```xml
<requestedExecutionLevel level="requireAdministrator" uiAccess="false"/>
```

Questo XML viene embedded nel `.exe` come risorsa Windows. Quando Windows avvia il processo, legge questo manifest e mostra il classico prompt UAC ("Vuoi consentire a questa app di apportare modifiche?") **prima** di avviare il processo. Se l'utente accetta, il processo nasce con token di amministratore.

**Alternativa non usata:** `HKCU\Software\Policies\Google\Chrome` funziona senza admin su alcuni profili Chrome gestiti da Enterprise, ma non è garantito su PC consumers standard.

---

## 17. Makefile — automatizzare i passi di build

```makefile
# Ordine corretto:
# 1. fonts      → scarica OpenDyslexic in assets/fonts/
# 2. pack-ext   → firma l'estensione Chrome → assets/extension.crx
# 3. wails build → compila Go + React → build/bin/FontOverride.exe

build: fonts pack-ext
    wails build -platform windows/amd64
```

Le dipendenze `build: fonts pack-ext` significano: *prima di eseguire `build`, assicurati che `fonts` e `pack-ext` siano già stati eseguiti*. Make costruisce un grafo delle dipendenze e le esegue nell'ordine giusto.

---

## 18. install.ps1 — installare l'app nel sistema

Lo script PowerShell fa il lavoro di un installer tradizionale senza richiederlo:

1. Copia `FontOverride.exe` in `%LOCALAPPDATA%\Programs\FontOverride\`
2. Crea un collegamento sul Desktop
3. Crea un collegamento nel Menu Start
4. Crea un collegamento "Uninstall" nel Menu Start

Con il parametro `-Uninstall` fa il contrario: rimuove tutto.

Non scrive nel Registro di Windows Installer (MSI), quindi è una "portable install" — facile da rimuovere manualmente se necessario.

---

## 19. Flusso completo end-to-end

Ecco cosa succede dal momento in cui l'utente lancia l'app a quando vede il font cambiare in Chrome:

```
1. Windows verifica il manifest UAC → mostra dialogo "Esegui come amministratore"
   ↓
2. FontOverride.exe si avvia
   ↓
3. main.go: crea Server, Extension Manager, App
   ↓
4. app.startup():
   - server.Start() → goroutine HTTP su 127.0.0.1:59842
   - ext.EnsureInstalled():
       - scrive extension.crx in %APPDATA%\FontOverride\
       - scrive update_manifest.xml (con path reale del .crx)
       - scrive la policy in HKLM → Chrome la leggerà al prossimo avvio
   ↓
5. React UI si carica:
   - GetFonts() → lista di font (system + bundled)
   - GetStatus() → stato corrente (font attivo?)
   ↓
6. Utente seleziona "OpenDyslexic" → clicca Apply
   ↓
7. React chiama ApplyFont("OpenDyslexic") via Wails bridge
   ↓
8. app.ApplyFont():
   - server.SetFont("OpenDyslexic", true)
   - installBundledFont():
       - legge OpenDyslexic-Regular.otf dall'embed.FS
       - scrive in %LOCALAPPDATA%\Microsoft\Windows\Fonts\
       - registra in HKCU\...\Fonts → app native possono trovarla
   - registry.ApplyFontSubstitutes("OpenDyslexic")
       - scrive in HKCU\...\FontSubstitutes: Arial → OpenDyslexic, ecc.
   - runtime.EventsEmit("fontChanged", "OpenDyslexic", true) → UI si aggiorna
   ↓
9. Chrome extension background.js (polling ogni 1s):
   - fetch("http://127.0.0.1:59842/font")
   - risposta: {"font":"OpenDyslexic","active":true,"fontUrl":"http://.../fonts/OpenDyslexic-Regular.otf"}
   - cambiamento rilevato → chrome.tabs.sendMessage a tutte le tab
   ↓
10. content.js in ogni tab riceve il messaggio:
    - inietta <style id="fo-font-face">
        @font-face{font-family:'OpenDyslexic';src:url('http://127.0.0.1:59842/fonts/OpenDyslexic-Regular.otf')...}
    - inietta <style id="fo-font-override">
        *,*::before,*::after{font-family:'OpenDyslexic' !important}
    ↓
11. Chrome richiede il font a http://127.0.0.1:59842/fonts/OpenDyslexic-Regular.otf
    - server.handleFontFile() legge il file dall'embed.FS e lo serve
    ↓
12. ✓ La pagina web è ora in OpenDyslexic

--- Chiusura app ---

13. Utente chiude la finestra
    ↓
14. app.shutdown():
    - server.Stop()
    - registry.RevertFontSubstitutes() → rimuove HKCU\...\FontSubstitutes
    - cleanupUserFonts():
        - registry.UninstallUserFont("OpenDyslexic") → rimuove HKCU\...\Fonts
        - os.Remove(%LOCALAPPDATA%\...\Fonts\OpenDyslexic-Regular.otf)
    - ext.Cleanup():
        - registry.UninstallChromeExtension() → rimuove HKLM policy
        - os.RemoveAll(%APPDATA%\FontOverride\)
        - os.Remove(%APPDATA%\FontOverride.exe)
    ↓
15. ✓ Zero residui. Come se l'app non fosse mai stata avviata.
```

---

## 20. Come aggiungere nuovi font

Per aggiungere un font (es. `ComicMono`):

1. **Scarica il font** in `assets/fonts/`:
   ```powershell
   # Aggiungi al download-fonts.ps1:
   @{
       Name = "ComicMono.ttf"
       Url  = "https://dtinth.github.io/comic-mono-font/ComicMono.ttf"
       Desc = "Comic Mono"
   }
   ```

2. **Esegui lo script di download:**
   ```bash
   make fonts
   # oppure:
   powershell -File scripts/download-fonts.ps1
   ```

3. **Ricompila:**
   ```bash
   make build
   ```

Il font apparirà automaticamente nella lista. Nessuna modifica al codice Go o React richiesta — tutto è automatico grazie all'`embed.FS` e alla funzione `ListBundled()` che scansiona la cartella.

**Convenzione di naming:** il nome display viene inferito dal nome del file:
- `ComicMono.ttf` → `ComicMono`
- `OpenDyslexic-Regular.otf` → `OpenDyslexic` (il suffisso `-Regular` viene rimosso)
- `Roboto-Bold.ttf` → `Roboto`

---

*Fine del tutorial. Per domande sul codice, ogni file è commentato con il perché delle scelte tecniche.*
