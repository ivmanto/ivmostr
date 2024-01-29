#!/bin/zsh

cd /Users/tonevSr/Documents/Programming/_proProjects/ivmostr-tdd

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

# Handle the app versioning
VERSION_FILE="version"
VERSION=$(git describe --tags --abbrev=0)
pwd
echo $VERSION

if [[ ! -f "$VERSION_FILE" ]]; then
    echo "Version file not found: $VERSION_FILE"
    exit 1
fi

echo "Updating version to $VERSION..."
# sed -i "s/const Version.*/const Version = \"$VERSION\"\"/" $VERSION_FILE

echo "version: $VERSION" > $VERSION_FILE
git add $VERSION_FILE
git commit -a -m "v$VERSION $MSG"
git push
