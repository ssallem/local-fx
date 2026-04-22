# installer/ — Phase 0 dev install scripts

**Not a production installer.** These scripts are for local development and
testing only. MSI / `.pkg` packaging lands in Phase 4.

They do three things on the current user's machine:

1. Generate a deterministic RSA keypair so Chrome gives the unpacked extension
   a stable extension ID across reloads.
2. Drop a Chrome **Native Messaging** host manifest (`com.local.fx.json`) into
   the location Chrome and Edge read it from, registered only for the current
   user (HKCU on Windows, `~/Library/...` on macOS — no admin needed).
3. Record a SHA-256 of the host binary into `integrity.json` for a later
   self-check phase.

## Layout

```
installer/
├── windows/
│   ├── install.ps1                 # register com.local.fx for current user
│   ├── uninstall.ps1               # remove registration + install dir
│   └── com.local.fx.json.tmpl      # manifest template (placeholders)
├── macos/
│   ├── install.sh                  # register com.local.fx for current user
│   ├── uninstall.sh                # remove registration + install dir
│   └── com.local.fx.json.tmpl      # manifest template (placeholders)
└── shared/
    ├── generate-dev-key.ps1        # RSA 2048 + SPKI DER + ext-id + inject manifest.json
    └── generate-dev-key.sh         # same, bash + openssl
```

Template placeholders (exact strings):

- `{{HOST_BINARY_PATH}}` — absolute path to the host binary
- `{{EXTENSION_ID}}`    — 32-char a-p Chrome extension ID

## Setup order

```bash
# 1. Build Go host
cd native-host
go build -o bin/fx-host.exe ./cmd/fx-host            # Windows
# or:
GOOS=darwin GOARCH=arm64 go build -o bin/fx-host-darwin-arm64 ./cmd/fx-host

# 2. Build extension
cd ../extension
npm install
npm run build

# 3. Install Native Messaging host
# Windows (PowerShell):
cd ..\installer\windows
.\install.ps1

# macOS:
cd ../installer/macos
./install.sh

# 4. Load unpacked
#    chrome://extensions -> Developer mode -> Load unpacked -> extension/dist

# 5. Open a new tab and click "Ping Host" -> expect "pong"
```

## How the deterministic extension ID works

Chrome derives the extension ID from the `key` field in `manifest.json`:

```
ext_id = first_16_bytes( SHA256( SPKI_DER_of_public_key ) )
         -> hex-encode -> map '0..f' -> 'a..p'
```

`generate-dev-key.{ps1,sh}` generates a 2048-bit RSA keypair into
`extension/dev-key/`, writes the SPKI DER, computes the ID, and injects the
base64 SPKI DER into `extension/manifest.json` under `"key"`. All existing
fields are preserved. Subsequent `npm run build` copies that key to `dist/`,
so the extension ID stays the same across reloads.

`install.{ps1,sh}` calls the key generator automatically when `-ExtensionId`
is not provided.

## Uninstall

```powershell
# Windows
.\uninstall.ps1                     # remove registry + %LOCALAPPDATA%\LocalFx (asks)
.\uninstall.ps1 -Yes -RemoveDevKey  # also delete extension/dev-key/
```

```bash
# macOS
./uninstall.sh
./uninstall.sh --yes --remove-dev-key
```

## Troubleshooting

**"Specified native messaging host not found"**
- Check the manifest path was registered:
  - Windows: `reg query "HKCU\Software\Google\Chrome\NativeMessagingHosts\com.local.fx"`
  - macOS: `ls -l "~/Library/Application Support/Google/Chrome/NativeMessagingHosts/"`
- Check the `path` field inside that manifest points at an existing binary.

**"Access to the specified native messaging host is forbidden"**
- `allowed_origins[0]` in the manifest must match the extension ID Chrome
  actually gave your extension. Run `generate-dev-key` again and reload the
  unpacked extension so they agree.
- If you manually loaded the extension before injecting `key`, Chrome assigned
  a random ID. Remove and re-load after the key is injected.

**Edge support**
- Scripts also write Edge's registration by default. Pass `-SkipEdge` / `--skip-edge`
  to opt out.

## What is deliberately out of scope (Phase 4+)

- Signed MSI / `.pkg` installers
- System-wide (HKLM / `/Library/Google/...`) registration
- Chrome Canary / Beta / Dev channel paths
- Firefox Native Messaging (different manifest format & path)
- Host binary self-verification against `integrity.json` at runtime
