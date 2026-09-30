FROM golang:1.24-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/hudang ./cmd/hudang

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hudang /hudang
ENTRYPOINT ["/hudang", "--config", "/etc/hudang/config.yaml"]
