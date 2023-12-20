#!/bin/zsh

cd /Users/tonevSr/Documents/Programming/_proProjects/ivmostr

go clean
# Set the environment variables
go env -w GOOS='linux'
go env -w GOARCH='amd64'

# Build the binary
go build -v -o build/linux/ivmostr cmd/ivmostr/ivmostr.go

# Reset the environment variables
go env -u GOOS
go env -u GOARCH
go clean

# Deploy to `ivmhome` server

# Get the IP address of the Ubuntu server
server_ip="192.168.178.10"

# Get the name of the executable file to copy
executable_file="ivmostr"

# Copy the executable file to the Ubuntu server
scp "build/linux/ivmostr" "tonev@$server_ip:/home/tonev"

# Handle the app versioning
export VERSION_FILE=version.go
export VERSION = $(shell git describe --tags --abbrev=0)

if [ ! -f "$VERSION_FILE" ]; then
  echo "Version file not found: $VERSION_FILE"
  exit 1
fi

NEW_VERSION=$((${VERSION:1} + 1))

echo "Updating version to $NEW_VERSION..."

sed -i "s/const Version.*/const Version = \"$NEW_VERSION\"/" $VERSION_FILE

git add $VERSION_FILE
