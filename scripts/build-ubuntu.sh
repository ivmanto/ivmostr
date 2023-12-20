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

# Change the permissions of the executable file on the Ubuntu server
#ssh "tonev@$server_ip" "chmod +x /usr/local/bin/$executable_file"
