FROM golang:1.27-alpine AS build
ARG APP
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/${APP}

FROM alpine:3.20
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
