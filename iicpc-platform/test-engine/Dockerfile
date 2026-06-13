FROM golang:1.23-alpine

WORKDIR /app

COPY . .

RUN go build -o test-engine .

EXPOSE 9000

CMD ["./test-engine"]