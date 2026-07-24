BINARY := alarmclock

.PHONY: build run test vet install

build:
	go build -o $(BINARY) ./cmd/alarmclock

# Run windowed on a desktop for development.
run:
	ALARMCLOCK_WINDOWED=1 go run ./cmd/alarmclock

test:
	go test ./...

vet:
	go vet ./...

# Build + install as a systemd kiosk service (run on the Raspberry Pi).
install:
	./deploy/install.sh
