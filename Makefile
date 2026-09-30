PLUGIN := opencode-session
VERSION := 0.1.7
OUT := dist/$(PLUGIN)-v$(VERSION).so

.PHONY: build test check clean

build:
	mkdir -p dist
	go build -trimpath -buildmode=c-shared -o $(OUT) .
	rm -f dist/$(PLUGIN)-v$(VERSION).h

test:
	go test ./...

check: test build

clean:
	rm -rf dist
