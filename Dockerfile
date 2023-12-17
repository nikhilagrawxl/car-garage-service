# Use an official Golang runtime as a parent image
FROM golang:1.17

# Set the working directory to the app directory
WORKDIR /app

# Copy the current directory contents into the container at /app
COPY . /app

# Download and install any required dependencies
RUN go mod download

# Build the car garage service binary
RUN go build -o car-garage-service cmd/main.go

# Expose the port on which the app will run
EXPOSE 8080

# Run the car garage service binary when the container launches
CMD ["./car-garage-service"]
