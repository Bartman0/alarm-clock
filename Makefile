BINARY := alarmclock

# Gio's Vulkan backend crashes on the Pi's V3DV driver; force the OpenGL ES
# backend by dropping Vulkan at build time. (No effect on macOS, which uses
# Metal.)
TAGS := novulkan

.PHONY: build run test vet install

build:
	go build -tags $(TAGS) -o $(BINARY) ./cmd/alarmclock

# Run on a desktop for development (a normal window; the kiosk relies on sway
# to fill the screen). Set ALARMCLOCK_FULLSCREEN=1 for Gio fullscreen.
run:
	go run -tags $(TAGS) ./cmd/alarmclock

test:
	go test ./...

vet:
	go vet ./...

# Build + install as a systemd kiosk service (run on the Raspberry Pi).
install:
	./deploy/install.sh
