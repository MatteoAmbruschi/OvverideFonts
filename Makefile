.PHONY: pack-ext dev build install fonts

## Download bundled fonts (OpenDyslexic etc.) into assets/fonts/
fonts:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/download-fonts.ps1

## Step 1 (once): sign the Chrome extension → assets/extension.crx
pack-ext:
	go run ./scripts/pack-extension

## Step 2: start dev server (runs pack-ext first, downloads fonts if absent)
dev: fonts pack-ext
	wails dev

## Step 3: build release .exe  ->  build/bin/FontOverride.exe
build: fonts pack-ext
	wails build -platform windows/amd64

## Step 4: install to %LOCALAPPDATA% + create shortcuts (requires admin)
install: build
	powershell -NoProfile -ExecutionPolicy Bypass -File install.ps1
