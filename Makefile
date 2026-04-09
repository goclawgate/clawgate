.PHONY: build run clean release

build:
	go build -ldflags="-s -w" -o clawgate .

run:
	go run .

clean:
	rm -f clawgate clawgate.exe
	rm -rf builds/

release: clean
	mkdir -p builds
	GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o builds/clawgate-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -ldflags="-s -w" -o builds/clawgate-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 go build -ldflags="-s -w" -o builds/clawgate-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o builds/clawgate-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o builds/clawgate-windows-amd64.exe .
	cd builds && sha256sum * > checksums.txt
