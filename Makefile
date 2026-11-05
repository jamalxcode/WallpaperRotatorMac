NAME    := WallpaperRotator
APP     := $(NAME).app
BIN     := $(APP)/Contents/MacOS/$(NAME)
ICNS    := $(APP)/Contents/Resources/AppIcon.icns
DEST    := /Applications
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')
ifeq ($(VERSION),)
VERSION := 1.1.0
endif
export CGO_ENABLED := 1
# Build every C/Objective-C object (ours and the systray library's) for macOS 13+
export MACOSX_DEPLOYMENT_TARGET := 13.0
export CGO_CFLAGS := -O2 -mmacosx-version-min=13.0
export CGO_LDFLAGS := -mmacosx-version-min=13.0

.PHONY: app universal release install run once test clean plist

# App for this Mac's architecture
app: $(ICNS) plist
	mkdir -p $(APP)/Contents/MacOS
	rm -f $(BIN)
	go build -trimpath -o $(BIN) .
	codesign --force --sign - $(APP)

# Universal (Apple silicon + Intel) app
universal: $(ICNS) plist
	mkdir -p $(APP)/Contents/MacOS build
	GOARCH=arm64 CC="clang -arch arm64" go build -trimpath -o build/$(NAME)-arm64 .
	GOARCH=amd64 CC="clang -arch x86_64" go build -trimpath -o build/$(NAME)-amd64 .
	rm -f $(BIN)
	lipo -create -output $(BIN) build/$(NAME)-arm64 build/$(NAME)-amd64
	codesign --force --sign - $(APP)

# Zip for GitHub Releases
release: universal
	ditto -c -k --keepParent $(APP) build/$(NAME)-$(VERSION).zip
	@echo "built build/$(NAME)-$(VERSION).zip"

plist:
	mkdir -p $(APP)/Contents
	sed 's/VERSION/$(VERSION)/g' Info.plist > $(APP)/Contents/Info.plist

# Build the macOS icon set from the 1024px master in assets/icon.png
$(ICNS): assets/icon.png
	rm -rf build/AppIcon.iconset
	mkdir -p build/AppIcon.iconset $(dir $(ICNS))
	for s in 16 32 128 256 512; do \
		sips -z $$s $$s assets/icon.png --out build/AppIcon.iconset/icon_$${s}x$${s}.png >/dev/null; \
		sips -z $$((s*2)) $$((s*2)) assets/icon.png --out build/AppIcon.iconset/icon_$${s}x$${s}@2x.png >/dev/null; \
	done
	iconutil -c icns build/AppIcon.iconset -o $(ICNS)

install: app
	-pkill -x $(NAME)
	-pkill -x WallpaperChanger
	sleep 1
	rm -rf /Applications/WallpaperRotator.app
	rm -rf /Applications/WallpaperChanger.app
	cp -R $(APP) $(DEST)/
	touch $(DEST)/$(APP)
	open $(DEST)/$(APP)

run: app
	open $(APP)

once:
	go run . --once

test:
	go vet ./...
	go test ./...

clean:
	rm -rf WallpaperRotator.app WallpaperChanger.app build
