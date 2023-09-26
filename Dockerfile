# syntax=docker/dockerfile:1-labs

FROM golang:1.19 as relayer

WORKDIR /app

ADD . /app
RUN go build -o ./build/bin/relayer .

FROM golang:1.19

RUN apt-get update && \
    apt-get install -y jq curl && \
    rm -rf /var/lib/apt/lists

WORKDIR /app

COPY --from=relayer /app/build/bin/relayer ./
COPY entrypoint.sh .

ENTRYPOINT ["/app/entrypoint.sh"]