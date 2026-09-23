FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/kilncurve ./cmd/server

FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/kilncurve /kilncurve
EXPOSE 8080
USER app
ENTRYPOINT ["/kilncurve"]
