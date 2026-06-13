# Use Go 1.26 alpine as base image (lightweight)
FROM golang:1.26-alpine

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

# Copy static frontend files into container
COPY static/ ./static/

# Tell Docker this service listens on port 8080
EXPOSE 8080

# Run the binary when container starts
CMD ["./main"]