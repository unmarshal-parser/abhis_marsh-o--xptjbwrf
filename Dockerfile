FROM golang:1.18.0-alpine
RUN apk add --no-cache build-base=0.5-r3 \
    openssh=8.8_p1-r1
WORKDIR /src
COPY . ./
COPY run.yml .
ENV CONFIG_FILE_PATH="."
ENV GOARCH=arm64
RUN go build -o indexer-binary ./*.go
CMD ["./indexer-binary"]
