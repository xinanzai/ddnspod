go build -ldflags="-s -w" -o ddnspod.exe main.go
upx.exe ddnspod.exe