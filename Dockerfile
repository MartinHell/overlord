FROM golang:1.22-alpine as builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the overlord binary
RUN go build -o overlord .

# Build the overlord-migrate binary
RUN go build -o overlord-migrate migrate/migrate.go

# Start a new stage from scratch
FROM alpine:3

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy entrypoint.sh script
COPY ./scripts/entrypoint.sh /app/entrypoint.sh

# Set entrypoint.sh executable
RUN chmod +x /app/entrypoint.sh

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/overlord /app/overlord
COPY --from=builder /app/overlord-migrate /app/overlord-migrate

# Command to run the executable
CMD ["/app/entrypoint.sh"]
