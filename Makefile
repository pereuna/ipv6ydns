BINARY=ip6
SRC=ipv6.go

all:
	GOOS=openbsd GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINARY) $(SRC)
	cp $(BINARY) /mnt/c/temp
