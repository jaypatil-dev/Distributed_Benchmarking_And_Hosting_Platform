# Use Go 1.26 alpine as base image (lightweight)
FROM golang:1.26-alpine

# Install docker CLI so we can spin up contestant containers
RUN apk add --no-cache docker-cli

# Set working directory inside container
WORKDIR /app

# Copy dependency files first (for better caching)
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy all source code
COPY . .

# Compile Go code into a binary called 'main'
RUN go build -o main .

# No port exposed - submission engine is internal only
EXPOSE 8081

# Run the binary when container starts
CMD ["./main"]