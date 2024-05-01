FROM golang:1.22-alpine as builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN go build -o overlord .

# Start a new stage from scratch
FROM alpine:3

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy startup.sh script
COPY ./scripts/startup.sh /app/startup.sh

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/overlord /app/overlord

# Command to run the executable
CMD ["/app/startup.sh"]